package sdk

import (
	"sync"
	"testing"
)

// TestRealtimeSubscriptionCloseRace exercises emit/cancel/close concurrently.
// Run with -race; before the fix this could panic with send on a closed channel.
func TestRealtimeSubscriptionCloseRace(t *testing.T) {
	c := &RealtimeClient{subs: make(map[uint64]chan RealtimeEvent)}

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
