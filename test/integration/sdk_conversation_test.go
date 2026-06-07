package integration

import (
	"context"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/pkg/sdk"
)

// TestConversationManagerCreatesConversationsOnDemand verifies Get lazily creates and caches conversations.
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
		RequestTimeout:    5 * time.Second,
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

	// Second Get should return the exact same instance
	conv2 := client.Conversations.Get("p2p_1_2")
	if conv != conv2 {
		t.Fatal("expected same conversation instance for same topic")
	}

	groupConv := client.Conversations.Get("grp_test")
	if groupConv.Type != sdk.ConversationTypeGroup {
		t.Fatalf("expected Group conversation type")
	}
}

// TestConversationCanSendTextMessage verifies a conversation can send a text message.
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
		RequestTimeout:    5 * time.Second,
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

// TestConversationRoutesIncomingMessages verifies server pushes are routed to the correct conversation.
func TestConversationRoutesIncomingMessages(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, userID := registerAndLogin(t, "conv-route-user", "123456")

	client := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		AutoAck:           true,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	conv := client.Conversations.Get("p2p_1_2")
	msgReceived := make(chan *sdk.Message, 1)
	conv.OnMessage = func(msg *sdk.Message) {
		msgReceived <- msg
	}

	broadcastPacket := &v1.Packet{
		Cmd: v1.Command_CMD_NOTIFY,
		Seq: 99,
		Payload: &v1.Packet_Notify{
			Notify: &v1.MessagePush{
				MsgId:     12345,
				Topic:     "p2p_1_2",
				SenderId:  2,
				MsgType:   int32(v1.MsgType_MSG_TYPE_TEXT),
				Content:   []byte("routed message"),
				Timestamp: time.Now().Unix(),
				TopicSeq:  1,
			},
		},
	}

	sent := ts.gwManager.BroadcastToUser(userID, broadcastPacket)
	if sent != 1 {
		t.Fatalf("expected broadcast to 1 device, got %d", sent)
	}

	select {
	case msg := <-msgReceived:
		if msg.Topic != "p2p_1_2" {
			t.Fatalf("expected topic=p2p_1_2, got %s", msg.Topic)
		}
		if string(msg.Content) != "routed message" {
			t.Fatalf("expected content='routed message', got %s", string(msg.Content))
		}
		if len(conv.Messages) == 0 {
			t.Fatal("expected message to be appended to conversation.Messages")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("conversation OnMessage not fired within timeout")
	}
}

// TestConversationTracksUnreadCount verifies unread count increments on received messages and resets on mark read.
func TestConversationTracksUnreadCount(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, userID := registerAndLogin(t, "conv-unread-user", "123456")

	client := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		AutoAck:           true,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	conv := client.Conversations.Get("p2p_1_2")

	if conv.GetUnreadCount() != 0 {
		t.Fatalf("expected unread count 0, got %d", conv.GetUnreadCount())
	}

	broadcastPacket := &v1.Packet{
		Cmd: v1.Command_CMD_NOTIFY,
		Seq: 99,
		Payload: &v1.Packet_Notify{
			Notify: &v1.MessagePush{
				MsgId:     12345,
				Topic:     "p2p_1_2",
				SenderId:  2,
				MsgType:   int32(v1.MsgType_MSG_TYPE_TEXT),
				Content:   []byte("unread test"),
				Timestamp: time.Now().Unix(),
				TopicSeq:  1,
			},
		},
	}

	ts.gwManager.BroadcastToUser(userID, broadcastPacket)
	time.Sleep(500 * time.Millisecond)

	if conv.GetUnreadCount() != 1 {
		t.Fatalf("expected unread count 1, got %d", conv.GetUnreadCount())
	}

	markCtx, markCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer markCancel()
	if err := conv.MarkRead(markCtx); err != nil {
		t.Fatalf("mark read failed: %v", err)
	}

	if conv.GetUnreadCount() != 0 {
		t.Fatalf("expected unread count 0 after mark read, got %d", conv.GetUnreadCount())
	}
}

// TestConversationManagerListsAllConversations verifies All returns every managed conversation.
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
		RequestTimeout:    5 * time.Second,
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

// TestConversationReturnsLastMessage verifies LastMessage returns the most recently received message.
func TestConversationReturnsLastMessage(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, userID := registerAndLogin(t, "conv-last-user", "123456")

	client := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		AutoAck:           true,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	conv := client.Conversations.Get("p2p_1_2")

	if conv.LastMessage() != nil {
		t.Fatal("expected nil LastMessage for empty conversation")
	}

	broadcastPacket := &v1.Packet{
		Cmd: v1.Command_CMD_NOTIFY,
		Seq: 99,
		Payload: &v1.Packet_Notify{
			Notify: &v1.MessagePush{
				MsgId:     12345,
				Topic:     "p2p_1_2",
				SenderId:  2,
				MsgType:   int32(v1.MsgType_MSG_TYPE_TEXT),
				Content:   []byte("last msg test"),
				Timestamp: time.Now().Unix(),
				TopicSeq:  5,
			},
		},
	}

	ts.gwManager.BroadcastToUser(userID, broadcastPacket)
	time.Sleep(300 * time.Millisecond)

	last := conv.LastMessage()
	if last == nil {
		t.Fatal("expected non-nil LastMessage")
	}
	if last.TopicSeq != 5 {
		t.Fatalf("expected topicSeq=5, got %d", last.TopicSeq)
	}
}

// TestConversationCanSendImageAndFile verifies media messages can be sent through a conversation.
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
		RequestTimeout:    5 * time.Second,
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
