package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/messaging"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestOutboxAdmin_ReplayBatchAndAudit(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := t.Context()

	adminToken, adminID := registerAndLogin(t, "outbox-admin", "123456")
	if err := ts.openFGA.SetPlatformAdmin(ctx, adminID, true); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	eventID := fmt.Sprintf("evt-dlq-%d", time.Now().UnixNano())
	ev := messaging.NewMessageEvent(992, "p2p_1_2", 1, 1, time.Now().Unix(), nil, "dlq-client")
	ev.EventID = eventID
	payload, _ := ev.Marshal()
	msg := &domain.Message{
		ID: 992, Topic: "p2p_1_2", SenderID: 1, Type: domain.MessageTypeText,
		Content: []byte("dlq"), TopicSeq: 1, ClientMsgID: "dlq-client", Timestamp: time.Now().Unix(),
	}
	if _, _, err := ts.storage.SaveP2P(ctx, msg, &domain.OutboxEvent{
		EventID: eventID, MsgID: msg.ID, Topic: msg.Topic, TopicSeq: msg.TopicSeq,
		Payload: payload, Status: domain.OutboxPending, NextAttemptAt: time.Now(), CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("save outbox: %v", err)
	}
	var row struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := ts.mongoDB.Collection("agent_event_outbox").FindOne(ctx, bson.M{"event_id": eventID}).Decode(&row); err != nil {
		t.Fatalf("find outbox: %v", err)
	}
	if _, err := ts.mongoDB.Collection("agent_event_outbox").UpdateOne(ctx,
		bson.M{"_id": row.ID}, bson.M{"$set": bson.M{"status": domain.OutboxFailed, "last_error": "poison"}}); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	replay := httpPostAuth(t, testBaseURL+"/v1/admin/agent-outbox/replay", adminToken, map[string]any{
		"ids": []string{row.ID.Hex()},
	})
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("replay status: %d", replay.StatusCode)
	}
	var result struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(replay.Body).Decode(&result); err != nil || result.Count != 1 {
		t.Fatalf("replay result: %+v err=%v", result, err)
	}
	replay.Body.Close()

	failed := httpGet(t, testBaseURL+"/v1/admin/agent-outbox/failed", adminToken)
	if failed.StatusCode != http.StatusOK {
		t.Fatalf("list failed status: %d", failed.StatusCode)
	}
	failed.Body.Close()

	var auditCount int64
	if err := ts.data.DB().Raw("SELECT count(*) FROM audit_events WHERE action = 'outbox.replay'").Scan(&auditCount).Error; err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("outbox replay was not audited")
	}
}
