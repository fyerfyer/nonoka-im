package realtime

import (
	"errors"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDisconnectKeepsSubscriptionsWhenAutoReconnectDisabled(t *testing.T) {
	client := New(Config{DisableAutoReconnect: true}, nil, nil)
	events, cancel := client.Subscribe()
	defer cancel()

	conn := &websocket.Conn{}
	client.mu.Lock()
	client.ws = conn
	client.authed = true
	client.mu.Unlock()

	client.handleDisconnect(conn, errors.New("network down"))

	select {
	case event := <-events:
		if event.Type != EventDisconnected {
			t.Fatalf("event type = %d, want disconnected", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive disconnected event")
	}

	select {
	case _, ok := <-events:
		if !ok {
			t.Fatal("subscription was closed on a manual-reconnect-capable disconnect")
		}
	default:
	}
	if client.State() == "closed" {
		t.Fatal("disconnect marked the client permanently closed")
	}
}

func TestJitterDelayPreservesLowerBound(t *testing.T) {
	const delay = 100 * time.Millisecond
	for i := 0; i < 100; i++ {
		got := jitterDelay(delay)
		if got < delay/2 || got > delay {
			t.Fatalf("jitter = %s, want [%s,%s]", got, delay/2, delay)
		}
	}
}
