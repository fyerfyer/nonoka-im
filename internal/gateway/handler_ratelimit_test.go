package gateway

import (
	"io"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/metrics"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/proto"
)

// newRateLimitTestHandler builds a handler with storage/session dependencies
// omitted; publish only needs the producer, manager and metrics.
func newRateLimitTestHandler(t *testing.T, msgPerSec float64, burst int) (*Handler, *Connection) {
	t.Helper()
	logger := log.NewStdLogger(io.Discard)
	m := NewManager(logger, metrics.NewMetrics())
	h := NewHandler(m, nil, NewNoopProducer(), nil, []byte("secret"),
		HeartbeatConfig{Interval: 30 * time.Second, Timeout: 90 * time.Second},
		logger, metrics.NewMetrics())
	h.SetMessageRateLimit(msgPerSec, burst)

	// A connection without a live websocket is enough: SendWithTimeout only
	// pushes marshaled packets into the buffered send channel.
	c := NewConnection(nil, "rate-limit-conn", 0, 0, nil)
	c.SetAuthed(1, "dev-1")
	return h, c
}

func publishPacket(seq uint64) *v1.Packet {
	return &v1.Packet{
		Cmd: v1.Command_CMD_PUBLISH,
		Seq: seq,
		Payload: &v1.Packet_SendReq{
			SendReq: &v1.SendMessageRequest{
				Topic:       "p2p_1_2",
				MsgType:     v1.MsgType_MSG_TYPE_TEXT,
				Content:     []byte("hello"),
				ClientMsgId: "msg",
			},
		},
	}
}

// readSendChPacket reads one packet from the connection's send buffer.
func readSendChPacket(t *testing.T, c *Connection) *v1.Packet {
	t.Helper()
	select {
	case data := <-c.sendCh:
		var pkt v1.Packet
		if err := proto.Unmarshal(data, &pkt); err != nil {
			t.Fatalf("failed to unmarshal sent packet: %v", err)
		}
		return &pkt
	case <-time.After(2 * time.Second):
		t.Fatalf("no packet written to send channel")
		return nil
	}
}

func TestHandler_PublishRateLimit_BurstThenReject(t *testing.T) {
	// Tiny rate: effectively no refill during the test, burst of 3.
	h, c := newRateLimitTestHandler(t, 0.001, 3)

	// First 3 publishes are accepted and ACKed.
	for i := 1; i <= 3; i++ {
		h.HandlePacket(c, publishPacket(uint64(i)))
		pkt := readSendChPacket(t, c)
		if pkt.GetSendReply() == nil {
			t.Fatalf("burst packet %d: expected ACK, got %v", i, pkt)
		}
	}

	// The 4th exceeds the bucket: error packet, no ACK, connection stays open.
	h.HandlePacket(c, publishPacket(4))
	pkt := readSendChPacket(t, c)
	errResp := pkt.GetError()
	if errResp == nil {
		t.Fatalf("expected error response when rate limited, got %v", pkt)
	}
	if errResp.Code != errCodeRateLimited {
		t.Fatalf("expected error code %d, got %d", errCodeRateLimited, errResp.Code)
	}
	if c.State() != ConnStateAuthed {
		t.Fatalf("rate limiting must not close the connection")
	}
}

func TestHandler_PublishRateLimit_PerConnectionBuckets(t *testing.T) {
	h, c1 := newRateLimitTestHandler(t, 0.001, 2)

	// Exhaust conn1's bucket.
	for i := 1; i <= 2; i++ {
		h.HandlePacket(c1, publishPacket(uint64(i)))
		readSendChPacket(t, c1)
	}
	h.HandlePacket(c1, publishPacket(3))
	if pkt := readSendChPacket(t, c1); pkt.GetError() == nil {
		t.Fatalf("expected conn1 to be rate limited")
	}

	// A different connection has its own bucket and is unaffected.
	c2 := NewConnection(nil, "other-conn", 0, 0, nil)
	c2.SetAuthed(2, "dev-2")
	h.HandlePacket(c2, publishPacket(1))
	if pkt := readSendChPacket(t, c2); pkt.GetSendReply() == nil {
		t.Fatalf("expected conn2 publish to be accepted, got %v", pkt)
	}
}

func TestHandler_PublishRateLimit_CloseReleasesBucket(t *testing.T) {
	h, c := newRateLimitTestHandler(t, 0.001, 1)

	h.HandlePacket(c, publishPacket(1))
	readSendChPacket(t, c)
	h.HandlePacket(c, publishPacket(2))
	if pkt := readSendChPacket(t, c); pkt.GetError() == nil {
		t.Fatalf("expected rate limited packet")
	}

	// After close-cleanup the connID's bucket is gone; a reconnect with the
	// same ID starts fresh.
	h.OnConnectionClose(c)
	if _, ok := h.limiters.Load(c.ConnID()); ok {
		t.Fatalf("limiter must be dropped on connection close")
	}
	c2 := NewConnection(nil, c.ConnID(), 0, 0, nil)
	c2.SetAuthed(1, "dev-1")
	h.HandlePacket(c2, publishPacket(3))
	if pkt := readSendChPacket(t, c2); pkt.GetSendReply() == nil {
		t.Fatalf("expected fresh bucket after close, got %v", pkt)
	}
}

func TestHandler_PublishRateLimit_DisabledAllowsAll(t *testing.T) {
	h, c := newRateLimitTestHandler(t, 0, 0) // disabled
	for i := 1; i <= 50; i++ {
		h.HandlePacket(c, publishPacket(uint64(i)))
		if pkt := readSendChPacket(t, c); pkt.GetSendReply() == nil {
			t.Fatalf("publish %d must be allowed when limiter disabled, got %v", i, pkt)
		}
	}
}
