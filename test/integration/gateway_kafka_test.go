package integration

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/gateway"
	"google.golang.org/protobuf/proto"
)

// ============================================
// Kafka 集成测试
// ============================================

// TestGateway_Kafka_Publish_Basic verifies that a message published via WebSocket
// is successfully delivered to Kafka.
func TestGateway_Kafka_Publish_Basic(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token, _ := registerAndLogin(t, "kafka-basic-user", "123456")

	wsConn := wsConnect(t)
	defer wsConn.Close()

	// Authenticate
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    token,
				DeviceId: "web-001",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second) // consume auth response

	// Create a Kafka reader BEFORE publishing to only capture new messages
	reader := createKafkaReader(testKafkaBroker, ts.kafkaTopic, "test-group-basic")
	defer reader.Close()

	// Publish a message
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_PUBLISH,
		Seq: 2,
		Payload: &v1.Packet_SendReq{
			SendReq: &v1.SendMessageRequest{
				Topic:       "p2p_1_2",
				MsgType:     v1.MsgType_MSG_TYPE_TEXT,
				Content:     []byte("hello kafka"),
				ClientMsgId: "kafka-msg-001",
			},
		},
	})

	// Wait for ACK from gateway
	ack := wsReadPacket(t, wsConn, 2*time.Second)
	if ack.Cmd != v1.Command_CMD_PUBLISH {
		t.Fatalf("expected CMD_PUBLISH ACK, got %v", ack.Cmd)
	}

	reply := ack.GetSendReply()
	if reply == nil {
		t.Fatalf("expected SendReply payload, got nil")
	}
	if reply.ClientMsgId != "kafka-msg-001" {
		t.Fatalf("expected client_msg_id=kafka-msg-001, got %s", reply.ClientMsgId)
	}

	// Consume from Kafka to verify message was produced
	msg := consumeKafkaMessage(t, reader, 5*time.Second)
	if msg == nil {
		t.Fatal("expected message in Kafka, got nil")
	}

	var upstream v1.UpstreamMessage
	if err := proto.Unmarshal(msg.Value, &upstream); err != nil {
		t.Fatalf("failed to unmarshal upstream message: %v", err)
	}

	if upstream.ClientMsgId != "kafka-msg-001" {
		t.Fatalf("expected client_msg_id=kafka-msg-001 in Kafka, got %s", upstream.ClientMsgId)
	}
	if upstream.Topic != "p2p_1_2" {
		t.Fatalf("expected topic=p2p_1_2 in Kafka, got %s", upstream.Topic)
	}
	if string(upstream.Content) != "hello kafka" {
		t.Fatalf("expected content='hello kafka' in Kafka, got %s", string(upstream.Content))
	}

	t.Logf("kafka message verified: topic=%s, client_msg_id=%s, sender=%d",
		upstream.Topic, upstream.ClientMsgId, upstream.SenderId)
}

// TestGateway_Kafka_Publish_Content verifies that Kafka message fields
// (key, headers, payload) are correctly populated.
func TestGateway_Kafka_Publish_Content(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token, userID := registerAndLogin(t, "kafka-content-user", "123456")

	wsConn := wsConnect(t)
	defer wsConn.Close()

	// Authenticate
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    token,
				DeviceId: "web-content",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	// Create reader before publish
	reader := createKafkaReader(testKafkaBroker, ts.kafkaTopic, "test-group-content")
	defer reader.Close()

	// Publish with specific content
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_PUBLISH,
		Seq: 2,
		Payload: &v1.Packet_SendReq{
			SendReq: &v1.SendMessageRequest{
				Topic:       "grp_42",
				MsgType:     v1.MsgType_MSG_TYPE_IMAGE,
				Content:     []byte(`{"url":"https://example.com/img.png","width":800}`),
				ClientMsgId: "content-msg-002",
			},
		},
	})

	// Consume ACK
	wsReadPacket(t, wsConn, 2*time.Second)

	// Consume from Kafka
	msg := consumeKafkaMessage(t, reader, 5*time.Second)

	// Verify Kafka message key is the topic (for partition affinity)
	if string(msg.Key) != "grp_42" {
		t.Fatalf("expected Kafka message key='grp_42', got '%s'", string(msg.Key))
	}

	// Verify headers
	headers := make(map[string]string)
	for _, h := range msg.Headers {
		headers[h.Key] = string(h.Value)
	}

	expectedSenderID := string(headers["sender_id"])
	if expectedSenderID == "" {
		t.Fatal("expected sender_id header in Kafka message")
	}

	if headers["client_msg_id"] != "content-msg-002" {
		t.Fatalf("expected client_msg_id header='content-msg-002', got '%s'", headers["client_msg_id"])
	}

	// Verify protobuf payload
	var upstream v1.UpstreamMessage
	if err := proto.Unmarshal(msg.Value, &upstream); err != nil {
		t.Fatalf("failed to unmarshal upstream message: %v", err)
	}

	if upstream.SenderId != userID {
		t.Fatalf("expected sender_id=%d, got %d", userID, upstream.SenderId)
	}
	if upstream.MsgType != int32(v1.MsgType_MSG_TYPE_IMAGE) {
		t.Fatalf("expected msg_type=%d, got %d", v1.MsgType_MSG_TYPE_IMAGE, upstream.MsgType)
	}
	if upstream.Timestamp == 0 {
		t.Fatal("expected non-zero timestamp")
	}

	t.Logf("kafka content verified: key=%s, sender=%s, client_msg_id=%s, msg_type=%d",
		string(msg.Key), headers["sender_id"], headers["client_msg_id"], upstream.MsgType)
}

// TestGateway_Kafka_ConcurrentPublish verifies that concurrent publishes
// from multiple clients all land in Kafka correctly.
func TestGateway_Kafka_ConcurrentPublish(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	const concurrency = 10
	const messagesPerClient = 5

	tokens := make([]string, concurrency)
	for i := 0; i < concurrency; i++ {
		username := "kafka-concurrent-user-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		tokens[i], _ = registerAndLogin(t, username, "123456")
	}

	// Create reader BEFORE any publish to capture only test messages
	reader := createKafkaReader(testKafkaBroker, ts.kafkaTopic, "test-group-concurrent")
	defer reader.Close()

	// Open WebSocket connections, authenticate, and publish concurrently
	var wg sync.WaitGroup
	var successCount int32

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			wsConn := wsConnect(t)
			defer wsConn.Close()

			// Auth
			wsSendPacket(t, wsConn, &v1.Packet{
				Cmd: v1.Command_CMD_AUTH,
				Seq: 1,
				Payload: &v1.Packet_AuthReq{
					AuthReq: &v1.AuthRequest{
						Token:    tokens[idx],
						DeviceId: "device-kafka-" + string(rune('0'+idx)),
					},
				},
			})
			wsReadPacket(t, wsConn, 2*time.Second)

			// Publish messages
			for j := 0; j < messagesPerClient; j++ {
				wsSendPacket(t, wsConn, &v1.Packet{
					Cmd: v1.Command_CMD_PUBLISH,
					Seq: uint64(j + 2),
					Payload: &v1.Packet_SendReq{
						SendReq: &v1.SendMessageRequest{
							Topic:       "p2p_1_2",
							MsgType:     v1.MsgType_MSG_TYPE_TEXT,
							Content:     []byte("concurrent kafka message"),
							ClientMsgId: "kafka-concurrent-" + string(rune('0'+idx)) + "-" + string(rune('0'+j)),
						},
					},
				})

				// Read ACK (skip any push notifications that arrive first)
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					resp := wsReadPacketOrNil(t, wsConn, 300*time.Millisecond)
					if resp == nil {
						continue // short read timeout, keep trying until overall deadline
					}
					if resp.Cmd == v1.Command_CMD_PUBLISH {
						atomic.AddInt32(&successCount, 1)
						break
					}
					// Otherwise it's a push notification (CMD_NOTIFY) — continue reading
				}
			}
		}(i)
	}

	wg.Wait()

	expected := int32(concurrency * messagesPerClient)
	if successCount != expected {
		t.Fatalf("expected %d ACKs, got %d", expected, successCount)
	}

	// Give Kafka a moment to make messages available
	time.Sleep(500 * time.Millisecond)

	// Consume all messages from Kafka
	var kafkaMessages int32
	for i := 0; i < int(expected); i++ {
		msg := consumeKafkaMessageOrNil(reader, 5*time.Second)
		if msg == nil {
			t.Logf("stopped consuming after %d messages", i)
			break
		}

		var upstream v1.UpstreamMessage
		if err := proto.Unmarshal(msg.Value, &upstream); err != nil {
			t.Logf("failed to unmarshal message %d: %v", i, err)
			continue
		}

		if upstream.Topic == "" || upstream.ClientMsgId == "" {
			t.Logf("message %d has empty fields", i)
			continue
		}

		atomic.AddInt32(&kafkaMessages, 1)
	}

	if kafkaMessages != expected {
		t.Fatalf("expected %d messages in Kafka, got %d", expected, kafkaMessages)
	}

	t.Logf("concurrent kafka publish verified: %d messages produced and consumed", kafkaMessages)
}

// TestGateway_Kafka_PartitionAffinity verifies that messages with the same
// topic are routed using the same Kafka message key (for partition affinity).
func TestGateway_Kafka_PartitionAffinity(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token, _ := registerAndLogin(t, "kafka-affinity-user", "123456")

	wsConn := wsConnect(t)
	defer wsConn.Close()

	// Authenticate
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    token,
				DeviceId: "web-affinity",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	// Create reader before publish
	reader := createKafkaReader(testKafkaBroker, ts.kafkaTopic, "test-group-affinity")
	defer reader.Close()

	// Send multiple messages to the same topic
	const messageCount = 5
	topic := "p2p_100_200"

	for i := 0; i < messageCount; i++ {
		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd: v1.Command_CMD_PUBLISH,
			Seq: uint64(i + 2),
			Payload: &v1.Packet_SendReq{
				SendReq: &v1.SendMessageRequest{
					Topic:       topic,
					MsgType:     v1.MsgType_MSG_TYPE_TEXT,
					Content:     []byte("affinity test message"),
					ClientMsgId: "affinity-msg-" + string(rune('0'+i)),
				},
			},
		})
		wsReadPacket(t, wsConn, 2*time.Second)
	}

	// Consume messages and verify they all have the same key
	for i := 0; i < messageCount; i++ {
		msg := consumeKafkaMessage(t, reader, 5*time.Second)
		if string(msg.Key) != topic {
			t.Fatalf("message %d: expected key='%s', got '%s'", i, topic, string(msg.Key))
		}
	}

	t.Logf("partition affinity verified: %d messages all keyed with '%s'", messageCount, topic)
}

// TestGateway_Kafka_Publish_NoAuth verifies that publish without auth
// is rejected and no message is sent to Kafka.
func TestGateway_Kafka_Publish_NoAuth(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	wsConn := wsConnect(t)
	defer wsConn.Close()

	// Create reader before attempting unauthorized publish
	reader := createKafkaReader(testKafkaBroker, ts.kafkaTopic, "test-group-noauth")
	defer reader.Close()

	// Try to publish without auth
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_PUBLISH,
		Seq: 1,
		Payload: &v1.Packet_SendReq{
			SendReq: &v1.SendMessageRequest{
				Topic:       "p2p_1_2",
				MsgType:     v1.MsgType_MSG_TYPE_TEXT,
				Content:     []byte("unauthorized"),
				ClientMsgId: "noauth-msg-001",
			},
		},
	})

	// Expect error response
	resp := wsReadPacket(t, wsConn, 2*time.Second)
	if resp.Cmd != v1.Command_CMD_PUBLISH {
		t.Fatalf("expected CMD_PUBLISH response, got %v", resp.Cmd)
	}

	errResp := resp.GetError()
	if errResp == nil {
		t.Fatalf("expected Error payload, got nil")
	}
	if errResp.Message != "authentication required" {
		t.Fatalf("expected 'authentication required' error, got: %+v", errResp)
	}

	// Verify no message was produced to Kafka within short timeout
	msg := consumeKafkaMessageOrNil(reader, 2*time.Second)
	if msg != nil {
		t.Fatal("expected no message in Kafka for unauthorized publish")
	}

	t.Log("unauthorized publish correctly rejected and no kafka message produced")
}

// TestGateway_Kafka_Publish_DifferentTopics verifies that messages to different topics
// have different keys and are independently tracked.
func TestGateway_Kafka_Publish_DifferentTopics(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token, _ := registerAndLogin(t, "kafka-multi-topic-user", "123456")

	wsConn := wsConnect(t)
	defer wsConn.Close()

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    token,
				DeviceId: "web-multi",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	// Create reader before publish
	reader := createKafkaReader(testKafkaBroker, ts.kafkaTopic, "test-group-multi")
	defer reader.Close()

	// Send messages to different topics
	topics := []string{"p2p_1_2", "grp_42", "sys_123"}
	for i, topic := range topics {
		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd: v1.Command_CMD_PUBLISH,
			Seq: uint64(i + 2),
			Payload: &v1.Packet_SendReq{
				SendReq: &v1.SendMessageRequest{
					Topic:       topic,
					MsgType:     v1.MsgType_MSG_TYPE_TEXT,
					Content:     []byte("topic specific message"),
					ClientMsgId: "multi-topic-" + string(rune('0'+i)),
				},
			},
		})
		wsReadPacket(t, wsConn, 2*time.Second)
	}

	// Consume all messages (order is not guaranteed across partitions)
	receivedTopics := make(map[string]bool)
	for i := 0; i < len(topics); i++ {
		msg := consumeKafkaMessage(t, reader, 5*time.Second)

		var upstream v1.UpstreamMessage
		if err := proto.Unmarshal(msg.Value, &upstream); err != nil {
			t.Fatalf("failed to unmarshal message %d: %v", i, err)
		}

		// Verify key matches topic
		if string(msg.Key) != upstream.Topic {
			t.Fatalf("message %d: key '%s' does not match topic '%s'", i, string(msg.Key), upstream.Topic)
		}

		// Verify it's one of the expected topics
		found := false
		for _, expectedTopic := range topics {
			if upstream.Topic == expectedTopic {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("message %d: unexpected topic '%s'", i, upstream.Topic)
		}

		// Check for duplicates
		if receivedTopics[upstream.Topic] {
			t.Fatalf("message %d: duplicate topic '%s'", i, upstream.Topic)
		}
		receivedTopics[upstream.Topic] = true
	}

	// Verify all topics were received
	for _, topic := range topics {
		if !receivedTopics[topic] {
			t.Fatalf("topic '%s' was not received", topic)
		}
	}

	t.Logf("multi-topic publish verified: %d messages with correct keys", len(topics))
}

// TestGateway_Kafka_BatchSend verifies that the producer correctly handles
// batch message sending and that every message lands in Kafka without failures.
// This validates the P1-4 Kafka producer batching path in the default sync mode.
func TestGateway_Kafka_BatchSend(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token, _ := registerAndLogin(t, "kafka-batch-user", "123456")

	wsConn := wsConnect(t)
	defer wsConn.Close()

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    token,
				DeviceId: "web-batch",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	reader := createKafkaReader(testKafkaBroker, ts.kafkaTopic, "test-group-batch")
	defer reader.Close()

	// Send multiple messages rapidly to exercise the producer batch path
	const messageCount = 20
	for i := 0; i < messageCount; i++ {
		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd: v1.Command_CMD_PUBLISH,
			Seq: uint64(i + 2),
			Payload: &v1.Packet_SendReq{
				SendReq: &v1.SendMessageRequest{
					Topic:       "p2p_1_2",
					MsgType:     v1.MsgType_MSG_TYPE_TEXT,
					Content:     []byte("batch message"),
					ClientMsgId: "batch-msg-" + strconv.Itoa(i),
				},
			},
		})
		wsReadPacket(t, wsConn, 2*time.Second)
	}

	// Allow the consumer reader to catch up with the produced messages
	time.Sleep(500 * time.Millisecond)

	// Consume all messages from Kafka
	var kafkaMessages int32
	for i := 0; i < messageCount; i++ {
		msg := consumeKafkaMessageOrNil(reader, 5*time.Second)
		if msg == nil {
			break
		}
		atomic.AddInt32(&kafkaMessages, 1)
	}

	if kafkaMessages != messageCount {
		t.Fatalf("expected %d messages in Kafka, got %d", messageCount, kafkaMessages)
	}

	// Verify producer has no failed deliveries
	if kp, ok := ts.kafkaProducer.(*gateway.KafkaProducer); ok {
		if kp.FailedCount() > 0 {
			t.Fatalf("producer had %d failed deliveries", kp.FailedCount())
		}
	}

	t.Logf("producer batch send verified: %d messages, no failures", messageCount)
}

// TestGateway_Kafka_SendReceipt verifies that after a message is published and
// processed by MsgWorker, the sender receives a CMD_SEND_RECEIPT with the
// assigned msg_id and topic_seq.
func TestGateway_Kafka_SendReceipt(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token, _ := registerAndLogin(t, "kafka-receipt-user", "123456")

	wsConn := wsConnect(t)
	defer wsConn.Close()

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    token,
				DeviceId: "web-receipt",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second) // consume auth response

	clientMsgID := "receipt-test-001"
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_PUBLISH,
		Seq: 2,
		Payload: &v1.Packet_SendReq{
			SendReq: &v1.SendMessageRequest{
				Topic:       "p2p_1_2",
				MsgType:     v1.MsgType_MSG_TYPE_TEXT,
				Content:     []byte("receipt test message"),
				ClientMsgId: clientMsgID,
			},
		},
	})

	// 1. Read ACK for the publish
	ack := wsReadPacket(t, wsConn, 2*time.Second)
	if ack.Cmd != v1.Command_CMD_PUBLISH {
		t.Fatalf("expected CMD_PUBLISH ack, got %v", ack.Cmd)
	}
	ackReply := ack.GetSendReply()
	if ackReply == nil {
		t.Fatalf("expected SendMessageReply in ACK")
	}
	if ackReply.ClientMsgId != clientMsgID {
		t.Fatalf("ACK client_msg_id mismatch: got %s, want %s", ackReply.ClientMsgId, clientMsgID)
	}
	if ackReply.MsgId != 0 {
		t.Fatalf("ACK should not contain msg_id (expected 0, got %d)", ackReply.MsgId)
	}
	if ackReply.TopicSeq != 0 {
		t.Fatalf("ACK should not contain topic_seq (expected 0, got %d)", ackReply.TopicSeq)
	}

	// 2. Wait for SendReceipt from MsgWorker
	// Use a background goroutine to read so that short read deadlines don't
	// interfere with the server's readLoop (which shares the same TCP conn).
	receiptCh := make(chan *v1.SendReceipt, 1)
	go func() {
		defer close(receiptCh)
		for {
			resp := wsReadPacketOrNil(t, wsConn, 10*time.Second)
			if resp == nil {
				return
			}
			if resp.Cmd == v1.Command_CMD_SEND_RECEIPT {
				if r := resp.GetSendReceipt(); r != nil {
					receiptCh <- r
					return
				}
			}
			// Ignore heartbeats and other packets, keep reading
		}
	}()

	var receipt *v1.SendReceipt
	select {
	case r, ok := <-receiptCh:
		if ok {
			receipt = r
		}
	case <-time.After(10 * time.Second):
	}

	if receipt == nil {
		t.Fatalf("did not receive CMD_SEND_RECEIPT within timeout")
	}

	if receipt.ClientMsgId != clientMsgID {
		t.Fatalf("receipt client_msg_id mismatch: got %s, want %s", receipt.ClientMsgId, clientMsgID)
	}
	if receipt.MsgId == 0 {
		t.Fatalf("receipt should contain non-zero msg_id")
	}
	if receipt.TopicSeq == 0 {
		t.Fatalf("receipt should contain non-zero topic_seq")
	}
	if receipt.Topic != "p2p_1_2" {
		t.Fatalf("receipt topic mismatch: got %s, want p2p_1_2", receipt.Topic)
	}

	t.Logf("send receipt verified: msg_id=%d, topic_seq=%d", receipt.MsgId, receipt.TopicSeq)
}
