package integration

import (
	"context"
	"testing"
	"time"

	"nonoka-im/pkg/sdk"
)

func TestConversationManagerCreatesConversationsOnDemand(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "conv-get-user", "123456")

	client := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	conv := client.Conversations.Get("p2p_1_2")
	if conv == nil {
		t.Fatal("expected non-nil conversation")
	}
	if conv.Topic != "p2p_1_2" {
		t.Fatalf("expected topic=p2p_1_2, got %s", conv.Topic)
	}
	if conv.Type != sdk.ConversationTypeP2P {
		t.Fatalf("expected P2P conversation type")
	}

	conv2 := client.Conversations.Get("p2p_1_2")
	if conv != conv2 {
		t.Fatal("expected same conversation instance for same topic")
	}

	groupConv := client.Conversations.Get("grp_test")
	if groupConv.Type != sdk.ConversationTypeGroup {
		t.Fatalf("expected Group conversation type")
	}
}

func TestConversationCanSendTextMessage(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "conv-send-user", "123456")

	client := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	conv := client.Conversations.Get("p2p_1_2")

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()

	result, err := conv.SendText(sendCtx, "hello from conversation")
	if err != nil {
		t.Fatalf("send text failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil send result")
	}
	if result.ClientMsgID == "" {
		t.Fatal("expected non-empty client_msg_id")
	}
}

func TestConversationRoutesIncomingMessages(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token1, userID1 := registerAndLogin(t, "conv-route-sender", "123456")
	token2, userID2 := registerAndLogin(t, "conv-route-receiver", "123456")
	topic := sdk.P2PTopic(userID1, userID2)

	receiver := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "sdk-receiver",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
		AutoAck:           true,
	})
	defer receiver.Close()

	conv := receiver.Conversations.Get(topic)
	msgReceived := make(chan *sdk.Message, 1)
	conv.OnMessage = func(msg *sdk.Message) {
		select {
		case msgReceived <- msg:
		default:
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := receiver.Connect(ctx); err != nil {
		t.Fatalf("receiver connect failed: %v", err)
	}

	sender := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "sdk-sender",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer sender.Close()

	if err := sender.Connect(ctx); err != nil {
		t.Fatalf("sender connect failed: %v", err)
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()
	if _, err := sender.SendText(sendCtx, topic, "routed message"); err != nil {
		t.Fatalf("sender send failed: %v", err)
	}

	select {
	case msg := <-msgReceived:
		if msg.Topic != topic {
			t.Fatalf("expected topic=%s, got %s", topic, msg.Topic)
		}
		if string(msg.Content) != "routed message" {
			t.Fatalf("expected content='routed message', got %s", string(msg.Content))
		}
		if len(conv.Messages) == 0 {
			t.Fatal("expected message to be appended to conversation.Messages")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("conversation OnMessage not fired within timeout")
	}
}

func TestConversationTracksUnreadCount(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token1, userID1 := registerAndLogin(t, "conv-unread-sender", "123456")
	token2, userID2 := registerAndLogin(t, "conv-unread-receiver", "123456")
	topic := sdk.P2PTopic(userID1, userID2)

	receiver := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "sdk-receiver",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
		AutoAck:           true,
	})
	defer receiver.Close()

	conv := receiver.Conversations.Get(topic)
	msgReceived := make(chan *sdk.Message, 1)
	conv.OnMessage = func(msg *sdk.Message) {
		select {
		case msgReceived <- msg:
		default:
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := receiver.Connect(ctx); err != nil {
		t.Fatalf("receiver connect failed: %v", err)
	}

	if conv.GetUnreadCount() != 0 {
		t.Fatalf("expected unread count 0, got %d", conv.GetUnreadCount())
	}

	sender := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "sdk-sender",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer sender.Close()

	if err := sender.Connect(ctx); err != nil {
		t.Fatalf("sender connect failed: %v", err)
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()
	if _, err := sender.SendText(sendCtx, topic, "unread test"); err != nil {
		t.Fatalf("sender send failed: %v", err)
	}

	select {
	case <-msgReceived:
	case <-time.After(15 * time.Second):
		t.Fatal("message not received within timeout")
	}

	time.Sleep(200 * time.Millisecond)
	if conv.GetUnreadCount() != 1 {
		t.Fatalf("expected unread count 1, got %d", conv.GetUnreadCount())
	}

	markCtx, markCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer markCancel()
	if err := conv.MarkRead(markCtx); err != nil {
		t.Fatalf("mark read failed: %v", err)
	}

	if conv.GetUnreadCount() != 0 {
		t.Fatalf("expected unread count 0 after mark read, got %d", conv.GetUnreadCount())
	}
}

func TestConversationManagerListsAllConversations(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "conv-all-user", "123456")

	client := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	_ = client.Conversations.Get("p2p_1_2")
	_ = client.Conversations.Get("p2p_1_3")
	_ = client.Conversations.Get("grp_test")

	all := client.Conversations.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 conversations, got %d", len(all))
	}
}

func TestConversationReturnsLastMessage(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token1, userID1 := registerAndLogin(t, "conv-last-sender", "123456")
	token2, userID2 := registerAndLogin(t, "conv-last-receiver", "123456")
	topic := sdk.P2PTopic(userID1, userID2)

	receiver := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "sdk-receiver",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
		AutoAck:           true,
	})
	defer receiver.Close()

	conv := receiver.Conversations.Get(topic)
	msgReceived := make(chan *sdk.Message, 1)
	conv.OnMessage = func(msg *sdk.Message) {
		select {
		case msgReceived <- msg:
		default:
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := receiver.Connect(ctx); err != nil {
		t.Fatalf("receiver connect failed: %v", err)
	}

	if conv.LastMessage() != nil {
		t.Fatal("expected nil LastMessage for empty conversation")
	}

	sender := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "sdk-sender",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer sender.Close()

	if err := sender.Connect(ctx); err != nil {
		t.Fatalf("sender connect failed: %v", err)
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()
	if _, err := sender.SendText(sendCtx, topic, "last msg test"); err != nil {
		t.Fatalf("sender send failed: %v", err)
	}

	select {
	case <-msgReceived:
	case <-time.After(15 * time.Second):
		t.Fatal("message not received within timeout")
	}

	time.Sleep(200 * time.Millisecond)
	last := conv.LastMessage()
	if last == nil {
		t.Fatal("expected non-nil LastMessage")
	}
	if last.TopicSeq == 0 {
		t.Fatal("expected non-zero topicSeq for last message")
	}
}

func TestConversationCanSendImageAndFile(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "conv-media-user", "123456")

	client := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	conv := client.Conversations.Get("p2p_1_2")

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()

	result, err := conv.SendImage(sendCtx, "https://example.com/image.png")
	if err != nil {
		t.Fatalf("send image failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for image")
	}

	result, err = conv.SendFile(sendCtx, "https://example.com/file.pdf", "doc.pdf")
	if err != nil {
		t.Fatalf("send file failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for file")
	}
}
