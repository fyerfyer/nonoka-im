package integration

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"nonoka-im/pkg/sdk"
)

func TestSDK_DeliveryReceipt_StatusUpdate(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token1, userID1 := registerAndLogin(t, "sdk-delivery-sender", "123456")
	token2, userID2 := registerAndLogin(t, "sdk-delivery-receiver", "123456")
	topic := sdk.P2PTopic(userID1, userID2)

	var received atomic.Bool
	var receiptTopic string
	var receiptSeq uint64
	var receiptMsgID int64

	sender := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "sdk-sender",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second, // must outlive gateway-side kafka produce retries
		OnDeliveryReceipt: func(topic string, topicSeq uint64, msgID int64) {
			receiptTopic = topic
			receiptSeq = topicSeq
			receiptMsgID = msgID
			received.Store(true)
		},
	})
	defer sender.Close()

	receiver := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "sdk-receiver",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second, // must outlive gateway-side kafka produce retries
		AutoAck:           true,
	})
	defer receiver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := receiver.Connect(ctx); err != nil {
		t.Fatalf("receiver connect failed: %v", err)
	}
	if err := sender.Connect(ctx); err != nil {
		t.Fatalf("sender connect failed: %v", err)
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()
	if _, err := sender.SendText(sendCtx, topic, "delivery receipt test"); err != nil {
		t.Fatalf("sender send failed: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if received.Load() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !received.Load() {
		t.Fatal("delivery receipt callback not fired within timeout")
	}

	if receiptTopic != topic {
		t.Fatalf("expected topic=%s, got %s", topic, receiptTopic)
	}
	if receiptSeq == 0 {
		t.Fatal("expected non-zero topicSeq")
	}
	if receiptMsgID == 0 {
		t.Fatal("expected non-zero msgID")
	}
}

func TestSDK_ReadReceipt_StatusUpdate(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token1, userID1 := registerAndLogin(t, "sdk-read-sender", "123456")
	token2, userID2 := registerAndLogin(t, "sdk-read-receiver", "123456")
	topic := sdk.P2PTopic(userID1, userID2)

	var received atomic.Bool
	var receiptTopic string
	var receiptSeq uint64
	var receiptReader int64

	sender := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "sdk-sender",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second, // must outlive gateway-side kafka produce retries
		OnReadReceipt: func(topic string, upToSeq uint64, readerID int64) {
			receiptTopic = topic
			receiptSeq = upToSeq
			receiptReader = readerID
			received.Store(true)
		},
	})
	defer sender.Close()

	receiver := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "sdk-receiver",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second, // must outlive gateway-side kafka produce retries
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
	if err := sender.Connect(ctx); err != nil {
		t.Fatalf("sender connect failed: %v", err)
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()
	if _, err := sender.SendText(sendCtx, topic, "read receipt test"); err != nil {
		t.Fatalf("sender send failed: %v", err)
	}

	select {
	case <-msgReceived:
	case <-time.After(15 * time.Second):
		t.Fatal("message not received within timeout")
	}

	markCtx, markCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer markCancel()
	if err := conv.MarkRead(markCtx); err != nil {
		t.Fatalf("mark read failed: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if received.Load() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !received.Load() {
		t.Fatal("read receipt callback not fired within timeout")
	}

	if receiptTopic != topic {
		t.Fatalf("expected topic=%s, got %s", topic, receiptTopic)
	}
	if receiptSeq == 0 {
		t.Fatal("expected non-zero upToSeq")
	}
	if receiptReader != userID2 {
		t.Fatalf("expected readerID=%d, got %d", userID2, receiptReader)
	}
}

func TestSDK_MessageStatusFlow_E2E(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token1, userID1 := registerAndLogin(t, "sdk-status-user1", "123456")
	token2, userID2 := registerAndLogin(t, "sdk-status-user2", "123456")
	topic := sdk.P2PTopic(userID1, userID2)

	var delivered atomic.Bool
	var read atomic.Bool

	sender := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "sdk-device1",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second, // must outlive gateway-side kafka produce retries
		OnDeliveryReceipt: func(topic string, topicSeq uint64, msgID int64) {
			delivered.Store(true)
		},
		OnReadReceipt: func(topic string, upToSeq uint64, readerID int64) {
			read.Store(true)
		},
	})
	defer sender.Close()

	receiver := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "sdk-device2",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second, // must outlive gateway-side kafka produce retries
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

	if err := sender.Connect(ctx); err != nil {
		t.Fatalf("client1 connect failed: %v", err)
	}
	if err := receiver.Connect(ctx); err != nil {
		t.Fatalf("client2 connect failed: %v", err)
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()
	if _, err := sender.SendText(sendCtx, topic, "status flow test"); err != nil {
		t.Fatalf("client1 send failed: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if delivered.Load() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !delivered.Load() {
		t.Fatal("delivery receipt not received")
	}

	select {
	case <-msgReceived:
	case <-time.After(15 * time.Second):
		t.Fatal("message not received by receiver")
	}

	markCtx, markCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer markCancel()
	if err := conv.MarkRead(markCtx); err != nil {
		t.Fatalf("mark read failed: %v", err)
	}

	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if read.Load() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !read.Load() {
		t.Fatal("read receipt not received")
	}
}
