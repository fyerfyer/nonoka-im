package gateway

import (
	"io"
	"sync"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/gorilla/websocket"
)

// newTestConnection creates a minimal Connection for manager tests.
func newTestConnection(userID int64, connID string) *Connection {
	c := &Connection{
		connID:   connID,
		userID:   userID,
		deviceID: "test",
		wsConn:   &websocket.Conn{}, // not used
		sendCh:   make(chan []byte, 1),
		closeCh:  make(chan struct{}),
	}
	c.state.Store(int32(ConnStateAuthed))
	c.lastActive.Store(time.Now().UnixNano())
	return c
}

// TestManager_Range_ConcurrentModify verifies that Range can safely be called
// while other goroutines are adding/removing connections.
func TestManager_Range_ConcurrentModify(t *testing.T) {
	m := NewManager(log.NewStdLogger(io.Discard))

	const iterations = 1000
	var wg sync.WaitGroup

	// Goroutine A: repeatedly iterate over all connections.
	wg.Go(func() {
		for range iterations {
			m.Range(func(c *Connection) bool {
				_ = c.UserID()
				return true
			})
		}
	})

	// Goroutine B: repeatedly add/remove connections.
	wg.Go(func() {
		for i := 0; i < iterations; i++ {
			c := newTestConnection(int64(i), "conn-"+string(rune('a'+i%26)))
			m.Add(c)
			m.Remove(c)
		}
	})

	wg.Wait()
}

// TestManager_BroadcastToUser_PreMarshal verifies that BroadcastToUser marshals
// the packet once and delivers it to all devices of a user.
func TestManager_BroadcastToUser_PreMarshal(t *testing.T) {
	m := NewManager(log.NewStdLogger(io.Discard))

	userID := int64(42)
	c1 := newTestConnection(userID, "conn-1")
	c2 := newTestConnection(userID, "conn-2")
	m.Add(c1)
	m.Add(c2)

	packet := &v1.Packet{
		Cmd: v1.Command_CMD_NOTIFY,
		Payload: &v1.Packet_Notify{
			Notify: &v1.MessagePush{
				MsgId:   1,
				Content: []byte("hello"),
			},
		},
	}

	sent := m.BroadcastToUser(userID, packet)
	if sent != 2 {
		t.Fatalf("expected 2 deliveries, got %d", sent)
	}
}

func TestManager_Add_ReturnsOnlySameDeviceConnections(t *testing.T) {
	m := NewManager(log.NewStdLogger(io.Discard))

	old := newTestConnection(42, "old")
	old.deviceID = "web-a"
	otherDevice := newTestConnection(42, "other")
	otherDevice.deviceID = "web-b"
	replacement := newTestConnection(42, "new")
	replacement.deviceID = "web-a"

	m.Add(old)
	m.Add(otherDevice)
	replaced := m.Add(replacement)

	if len(replaced) != 1 || replaced[0] != old {
		t.Fatalf("expected only old same-device connection, got %#v", replaced)
	}
}
