package sdk

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestRealtimeSubscriptionCloseRace exercises emit/cancel/close concurrently.
// Run with -race; before the fix this could panic with send on a closed channel.
func TestRealtimeSubscriptionCloseRace(t *testing.T) {
	c := &RealtimeClient{
		subs: make(map[uint64]chan RealtimeEvent),
		done: make(chan struct{}),
	}

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		ch, cancel := c.Subscribe()
		wg.Add(1)
		go func(ch <-chan RealtimeEvent, cancel func()) {
			defer wg.Done()
			for range ch {
			}
			cancel()
		}(ch, cancel)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.emit(RealtimeEvent{Type: RealtimeMessage})
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.closeEvents()
	}()
	wg.Wait()

	if !c.closed {
		t.Fatal("closeEvents did not mark the client closed")
	}
}

func TestRealtimeManualReconnectKeepsSubscriptions(t *testing.T) {
	c := &RealtimeClient{
		subs: make(map[uint64]chan RealtimeEvent),
		done: make(chan struct{}),
		cfg: Config{Realtime: RealtimeConfig{
			DisableAutoReconnect: true,
		}},
	}
	ch, cancel := c.Subscribe()
	defer cancel()

	conn := &websocket.Conn{}
	c.mu.Lock()
	c.ws = conn
	c.authed = true
	c.mu.Unlock()

	c.handleDisconnect(conn, errors.New("network down"))

	select {
	case event := <-ch:
		if event.Type != RealtimeDisconnected {
			t.Fatalf("event type = %q, want disconnected", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive disconnected event")
	}

	// Subscriptions must remain open so a caller can reconnect manually.
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("subscription was closed on a manual-reconnect-capable disconnect")
		}
	default:
	}
	if c.closed {
		t.Fatal("disconnect marked the client permanently closed")
	}
}
