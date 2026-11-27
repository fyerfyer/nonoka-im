package gateway

import (
	"context"
	"io"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

func testPeerPacket() *v1.Packet {
	return &v1.Packet{
		Cmd: v1.Command_CMD_READ_RECEIPT,
		Payload: &v1.Packet_ReadReceipt{ReadReceipt: &v1.ReadReceipt{
			Topic: "p2p_1_2", UpToSeq: 3, ReaderId: 1,
		}},
	}
}

// TestPeerBusFansOutToOtherGatewayManagers verifies the cross-node receipt
// path: a packet published by one gateway is delivered to a user connected to
// another gateway. It is skipped when the local Redis test instance is absent.
func TestPeerBusFansOutToOtherGatewayManagers(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6380"})
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skip("redis test instance not available")
	}
	defer client.Close()

	logger := log.NewStdLogger(io.Discard)
	m1 := NewManager(logger)
	m2 := NewManager(logger)
	conn := newTestConnection(42, "peer-bus-conn")
	m2.Add(conn)

	bus1 := NewPeerBus(client, m1, logger)
	bus2 := NewPeerBus(client, m2, logger)
	defer bus1.Stop()
	defer bus2.Stop()
	if err := bus1.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := bus2.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	packet := testPeerPacket()
	if err := bus1.Publish(ctx, []int64{42}, packet); err != nil {
		t.Fatal(err)
	}
	select {
	case <-conn.sendCh:
	case <-time.After(2 * time.Second):
		t.Fatal("peer packet was not delivered to the remote local manager")
	}
}
