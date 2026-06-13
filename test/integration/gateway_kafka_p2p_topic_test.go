package integration

import (
	"fmt"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"google.golang.org/protobuf/proto"
)

// TestGateway_Kafka_P2PTopicCanonical verifies that P2P topics are normalized
// so that both conversation sides map to the same Kafka key, preserving
// partition affinity regardless of which user sends the message.
func TestGateway_Kafka_P2PTopicCanonical(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token1, userID1 := registerAndLogin(t, "kafka-p2p-user1", "123456")
	_, userID2 := registerAndLogin(t, "kafka-p2p-user2", "123456")

	wsConn := wsConnect(t)
	defer wsConn.Close()

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    token1,
				DeviceId: "web-p2p-canonical",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	reader := createKafkaReader(testKafkaBroker, ts.kafkaTopic, "test-group-p2p-canonical")
	defer reader.Close()

	// Build a reversed topic (larger ID first) and the expected canonical topic.
	reversedTopic := fmt.Sprintf("p2p_%d_%d", userID2, userID1)
	expectedTopic := fmt.Sprintf("p2p_%d_%d", userID1, userID2)

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_PUBLISH,
		Seq: 2,
		Payload: &v1.Packet_SendReq{
			SendReq: &v1.SendMessageRequest{
				Topic:       reversedTopic,
				MsgType:     v1.MsgType_MSG_TYPE_TEXT,
				Content:     []byte("canonical topic test"),
				ClientMsgId: "p2p-canonical-001",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	msg := consumeKafkaMessage(t, reader, 5*time.Second)

	if string(msg.Key) != expectedTopic {
		t.Fatalf("expected Kafka key='%s' after normalization, got '%s'", expectedTopic, string(msg.Key))
	}

	var upstream v1.UpstreamMessage
	if err := proto.Unmarshal(msg.Value, &upstream); err != nil {
		t.Fatalf("failed to unmarshal upstream: %v", err)
	}
	if upstream.Topic != expectedTopic {
		t.Fatalf("expected upstream topic='%s', got '%s'", expectedTopic, upstream.Topic)
	}

	t.Logf("p2p topic canonical verified: %s -> %s", reversedTopic, upstream.Topic)
}
