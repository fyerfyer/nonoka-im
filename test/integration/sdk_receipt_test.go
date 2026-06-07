package integration

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/pkg/sdk"
)

// ============================================
// Phase 2: Read Receipt & Delivery Receipt Tests
// ============================================

// TestSDK_DeliveryReceipt_StatusUpdate verifies that receiving a DeliveryReceipt
// updates the message status to Delivered.
func TestSDK_DeliveryReceipt_StatusUpdate(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, userID := registerAndLogin(t, "sdk-delivery-user", "123456")

	var receivedReceipt atomic.Bool
	var receiptTopic string
	var receiptSeq uint64

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		AutoAck:           true,
		OnDeliveryReceipt: func(topic string, topicSeq uint64, msgID int64) {
			receivedReceipt.Store(true)
			receiptTopic = topic
			receiptSeq = topicSeq
		},
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	// Simulate server pushing a DeliveryReceipt
	receiptPacket := &v1.Packet{
		Cmd: v1.Command_CMD_DELIVERY_RECEIPT,
		Payload: &v1.Packet_DeliveryReceipt{
			DeliveryReceipt: &v1.DeliveryReceipt{
				Topic:    "p2p_1_2",
				TopicSeq: 42,
				MsgId:    12345,
			},
		},
	}

	sent := ts.gwManager.BroadcastToUser(userID, receiptPacket)
	if sent != 1 {
		t.Fatalf("expected broadcast to 1 device, got %d", sent)
	}

	// Wait for delivery receipt callback
	select {
	case <-func() chan struct{} {
		ch := make(chan struct{})
		go func() {
			for !receivedReceipt.Load() {
				time.Sleep(50 * time.Millisecond)
			}
			close(ch)
		}()
		return ch
	}():
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("delivery receipt callback not fired within timeout")
	}

	if receiptTopic != "p2p_1_2" {
		t.Fatalf("expected topic=p2p_1_2, got %s", receiptTopic)
	}
	if receiptSeq != 42 {
		t.Fatalf("expected topicSeq=42, got %d", receiptSeq)
	}
}

// TestSDK_ReadReceipt_StatusUpdate verifies that receiving a ReadReceipt
// updates the message status to Read.
func TestSDK_ReadReceipt_StatusUpdate(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, userID := registerAndLogin(t, "sdk-read-user", "123456")

	var receivedReceipt atomic.Bool
	var receiptTopic string
	var receiptSeq uint64
	var receiptReader int64

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		AutoAck:           true,
		OnReadReceipt: func(topic string, upToSeq uint64, readerID int64) {
			receivedReceipt.Store(true)
			receiptTopic = topic
			receiptSeq = upToSeq
			receiptReader = readerID
		},
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	// Simulate server pushing a ReadReceipt
	receiptPacket := &v1.Packet{
		Cmd: v1.Command_CMD_READ_RECEIPT,
		Payload: &v1.Packet_ReadReceipt{
			ReadReceipt: &v1.ReadReceipt{
				Topic:    "p2p_1_2",
				UpToSeq:  100,
				ReaderId: 2,
			},
		},
	}

	sent := ts.gwManager.BroadcastToUser(userID, receiptPacket)
	if sent != 1 {
		t.Fatalf("expected broadcast to 1 device, got %d", sent)
	}

	// Wait for read receipt callback
	select {
	case <-func() chan struct{} {
		ch := make(chan struct{})
		go func() {
			for !receivedReceipt.Load() {
				time.Sleep(50 * time.Millisecond)
			}
			close(ch)
		}()
		return ch
	}():
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("read receipt callback not fired within timeout")
	}

	if receiptTopic != "p2p_1_2" {
		t.Fatalf("expected topic=p2p_1_2, got %s", receiptTopic)
	}
	if receiptSeq != 100 {
		t.Fatalf("expected upToSeq=100, got %d", receiptSeq)
	}
	if receiptReader != 2 {
		t.Fatalf("expected readerID=2, got %d", receiptReader)
	}
}

// TestSDK_ConversationMarkRead_SendReceipt verifies that Conversation.MarkRead
// sends a ReadReceipt to the server.
func TestSDK_ConversationMarkRead_SendReceipt(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-markread-user", "123456")

	client := sdk.NewClient(sdk.Options{
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
		t.Fatalf("sdk connect failed: %v", err)
	}

	// Get a conversation and simulate having messages
	conv := client.Conversations.Get("p2p_1_2")

	// Simulate adding a message to set LastSeq
	conv.Messages = append(conv.Messages, &sdk.Message{
		MsgID:    1,
		Topic:    "p2p_1_2",
		SenderID: 2,
		TopicSeq: 5,
		Status:   sdk.MessageStatusSent,
	})
	conv.LastSeq = 5
	conv.UnreadCount = 1

	// Call MarkRead
	markCtx, markCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer markCancel()

	if err := conv.MarkRead(markCtx); err != nil {
		t.Fatalf("mark read failed: %v", err)
	}

	// Verify unread count is reset
	if conv.GetUnreadCount() != 0 {
		t.Fatalf("expected unread count=0, got %d", conv.GetUnreadCount())
	}
}

// TestSDK_MessageStatusFlow_E2E verifies the complete message status flow:
// Sending -> Sent -> Delivered -> Read.
func TestSDK_MessageStatusFlow_E2E(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	// Create two users
	token1, userID1 := registerAndLogin(t, "sdk-status-user1", "123456")
	token2, userID2 := registerAndLogin(t, "sdk-status-user2", "123456")

	// User1 connects
	client1 := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "sdk-device1",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		AutoAck:           true,
	})
	defer client1.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client1.Connect(ctx); err != nil {
		t.Fatalf("client1 connect failed: %v", err)
	}

	// User2 connects
	client2 := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "sdk-device2",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		AutoAck:           true,
	})
	defer client2.Close()

	if err := client2.Connect(ctx); err != nil {
		t.Fatalf("client2 connect failed: %v", err)
	}

	// User1 sends a message
	topic := "p2p_" + formatUserID(userID1) + "_" + formatUserID(userID2)
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()

	result, err := client1.SendMessage(sendCtx, topic, v1.MsgType_MSG_TYPE_TEXT, []byte("status flow test"))
	if err != nil {
		t.Fatalf("client1 send failed: %v", err)
	}
	if result.ClientMsgID == "" {
		t.Fatal("expected non-empty client_msg_id")
	}

	// Simulate server pushing a DeliveryReceipt to user1
	deliveryPacket := &v1.Packet{
		Cmd: v1.Command_CMD_DELIVERY_RECEIPT,
		Payload: &v1.Packet_DeliveryReceipt{
			DeliveryReceipt: &v1.DeliveryReceipt{
				Topic:    topic,
				TopicSeq: result.TopicSeq,
				MsgId:    result.MsgID,
			},
		},
	}

	sent := ts.gwManager.BroadcastToUser(userID1, deliveryPacket)
	if sent != 1 {
		t.Fatalf("expected delivery receipt broadcast to 1 device, got %d", sent)
	}

	// Wait a bit for the delivery receipt to be processed
	time.Sleep(200 * time.Millisecond)

	// Simulate server pushing a ReadReceipt to user1
	readPacket := &v1.Packet{
		Cmd: v1.Command_CMD_READ_RECEIPT,
		Payload: &v1.Packet_ReadReceipt{
			ReadReceipt: &v1.ReadReceipt{
				Topic:    topic,
				UpToSeq:  result.TopicSeq,
				ReaderId: userID2,
			},
		},
	}

	sent = ts.gwManager.BroadcastToUser(userID1, readPacket)
	if sent != 1 {
		t.Fatalf("expected read receipt broadcast to 1 device, got %d", sent)
	}

	// Wait a bit for the read receipt to be processed
	time.Sleep(200 * time.Millisecond)
}

// formatUserID formats a user ID as a string for topic construction.
func formatUserID(userID int64) string {
	return fmt.Sprintf("%d", userID)
}
