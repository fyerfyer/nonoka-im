package integration

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	agentdomain "nonoka-im/internal/agent/domain"
	agentdata "nonoka-im/internal/data/agent"
	imapp "nonoka-im/internal/im/app"
	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/messaging"
)

type flakyPublisher struct {
	fail  atomic.Bool
	calls atomic.Int32
}

func (p *flakyPublisher) Publish(context.Context, messaging.MessageEvent) error {
	p.calls.Add(1)
	if p.fail.Load() {
		return errors.New("kafka unavailable")
	}
	return nil
}

// TestOutbox_KafkaOutageNoLoss verifies the transactional outbox keeps the
// event pending while Kafka is down and publishes it exactly once after
// recovery.
func TestOutbox_KafkaOutageNoLoss(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()

	eventID := fmt.Sprintf("evt-outbox-%d", time.Now().UnixNano())
	ev := messaging.NewMessageEvent(991, "p2p_1_2", 1, 1, time.Now().Unix(), nil, "outbox-client")
	ev.EventID = eventID
	payload, err := ev.Marshal()
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	msg := &domain.Message{
		ID: 991, Topic: "p2p_1_2", SenderID: 1, Type: domain.MessageTypeText,
		Content: []byte("outbox fault"), TopicSeq: 1, ClientMsgID: "outbox-client", Timestamp: time.Now().Unix(),
	}
	outbox := &domain.OutboxEvent{
		EventID: eventID, MsgID: msg.ID, Topic: msg.Topic, TopicSeq: msg.TopicSeq,
		Payload: payload, Status: domain.OutboxPending, NextAttemptAt: time.Now(), CreatedAt: time.Now(),
	}
	if _, _, err := ts.storage.SaveP2P(ctx, msg, outbox); err != nil {
		t.Fatalf("persist message and outbox: %v", err)
	}

	pub := &flakyPublisher{}
	pub.fail.Store(true)
	service := &imapp.OutboxService{Repo: ts.storage, Publisher: pub, Backoff: time.Millisecond}
	if _, err := service.Relay(ctx, 10); err == nil {
		t.Fatal("expected relay failure while Kafka is down")
	}
	if pending, failed, _, err := ts.storage.OutboxStats(ctx); err != nil || pending != 1 || failed != 0 {
		t.Fatalf("outbox after outage: pending=%d failed=%d err=%v", pending, failed, err)
	}

	time.Sleep(5 * time.Millisecond)
	pub.fail.Store(false)
	if _, err := service.Relay(ctx, 10); err != nil {
		t.Fatalf("relay after recovery: %v", err)
	}
	if pending, failed, _, err := ts.storage.OutboxStats(ctx); err != nil || pending != 0 || failed != 0 {
		t.Fatalf("outbox after recovery: pending=%d failed=%d err=%v", pending, failed, err)
	}
	if pub.calls.Load() < 2 {
		t.Fatalf("expected a retry then success, calls=%d", pub.calls.Load())
	}
}

// TestReleaseRunOnShutdown verifies a safe in-flight run can be requeued
// immediately without waiting for lease expiry.
func TestReleaseRunOnShutdown(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()
	agentStore := agentdata.NewStore(ts.data.DB())
	run := &agentdomain.AgentRun{
		EventID: fmt.Sprintf("evt-release-%d", time.Now().UnixNano()),
		AgentID: 1, ActorUserID: 1, Topic: "p2p_1_2", MsgID: 1, TopicSeq: 1, Status: agentdomain.RunQueued,
	}
	if created, err := agentStore.EnqueueRun(ctx, run); err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}
	claimed, ok, err := agentStore.ClaimRun(ctx, run.ID, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	released, err := agentStore.ReleaseRun(ctx, claimed.ID, claimed.AttemptID)
	if err != nil || !released {
		t.Fatalf("release: released=%v err=%v", released, err)
	}
	again, ok, err := agentStore.ClaimRun(ctx, run.ID, time.Minute)
	if err != nil || !ok || again.AttemptID == claimed.AttemptID {
		t.Fatalf("reclaim after release: ok=%v err=%v attempt=%q", ok, err, again.AttemptID)
	}
}
