package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"nonoka-im/pkg/sdk"
)

func TestSDKRealtimeSendAndIdempotency(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	ctx := t.Context()
	token1, user1 := registerAndLogin(t, "sdk-v1-sender", "123456")
	token2, user2 := registerAndLogin(t, "sdk-v1-receiver", "123456")

	sender := newTestSDKClient(t, token1, "sdk-sender")
	defer sender.Close()
	receiver := newTestSDKClient(t, token2, "sdk-receiver")
	defer receiver.Close()

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sender.Connect(connectCtx); err != nil {
		t.Fatalf("sender connect: %v", err)
	}
	if err := receiver.Connect(connectCtx); err != nil {
		t.Fatalf("receiver connect: %v", err)
	}
	events := receiver.Realtime.Events()

	userLow, userHigh := user1, user2
	if userLow > userHigh {
		userLow, userHigh = userHigh, userLow
	}
	topic := fmt.Sprintf("p2p_%d_%d", userLow, userHigh)
	clientID := "sdk-idempotent-" + fmt.Sprint(time.Now().UnixNano())
	request := sdk.SendRequest{
		Topic: topic, Type: sdk.MessageTypeText, Content: []byte("sdk v1 hello"),
		ClientMessageID: clientID,
	}
	first, err := sender.Messages.Send(ctx, request)
	if err != nil {
		t.Fatalf("send first: %v", err)
	}
	second, err := sender.Messages.Send(ctx, request)
	if err != nil {
		t.Fatalf("send second: %v", err)
	}
	if first.MessageID == 0 || second.MessageID != first.MessageID {
		t.Fatalf("idempotent send returned different messages: %+v %+v", first, second)
	}

	// Wait for the recipient to receive the live push.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Type == sdk.RealtimeMessage && event.Message != nil && event.Message.Topic == topic {
				if string(event.Message.Content) != "sdk v1 hello" {
					t.Fatalf("unexpected push content: %q", event.Message.Content)
				}
				goto received
			}
		case <-deadline:
			t.Fatal("timed out waiting for realtime push")
		}
	}

received:
	page, err := receiver.Conversations.Get(topic).LoadHistory(ctx, 20)
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	count := 0
	for _, message := range page {
		if string(message.Content) == "sdk v1 hello" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("idempotent send produced %d persisted copies", count)
	}

	// Agent CRUD through the new resource module.
	agent, err := sender.Agents.Create(ctx, sdk.AgentInput{
		Name: "sdk-created-agent", Instruction: "test sdk agent",
		LLMServiceID: testModelServiceID, LLMModelID: testModelID,
		CapabilityPresets: []string{"im.read"}, ToolPolicies: map[string]string{},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	detail, err := sender.Agents.Get(ctx, agent.ID)
	if err != nil || detail.Name != "sdk-created-agent" || detail.Draft == nil {
		t.Fatalf("get agent: %+v err=%v", detail, err)
	}
	version, err := sender.Agents.Publish(ctx, agent.ID, detail.Draft.Revision, "sdk publish")
	if err != nil || version.Version != 1 || version.PublishNote != "sdk publish" {
		t.Fatalf("publish agent: %+v err=%v", version, err)
	}
	versions, err := sender.Agents.Versions(ctx, agent.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %+v err=%v", versions, err)
	}
}
