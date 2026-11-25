package realtime

import (
	"context"
	"sync"
	"testing"
)

type recordingTransport struct {
	mu        sync.Mutex
	delivered map[int64][]RunEvent
}

func (t *recordingTransport) PushRunEventToUser(_ context.Context, userID int64, event RunEvent) (int32, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.delivered == nil {
		t.delivered = map[int64][]RunEvent{}
	}
	t.delivered[userID] = append(t.delivered[userID], event)
	return 1, nil
}

func TestRunBroadcasterSequencesAndKeepsDurableSteps(t *testing.T) {
	store := NewMemoryRunEventStore()
	transport := &recordingTransport{}
	var steps []RunEvent
	broadcaster := NewRunBroadcaster(store, transport, func(_ context.Context, event RunEvent) error {
		steps = append(steps, event)
		return nil
	}, nil)

	ctx := context.Background()
	for _, eventType := range []RunEventType{RunStarted, RunDelta, RunToolStarted, RunCompleted} {
		event := RunEvent{RunID: 9, Type: eventType, Topic: "p2p_1_2", Delta: "x"}
		if err := broadcaster.Publish(ctx, event, []int64{1, 2, 2}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.Since(ctx, 9, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("expected four buffered events, got %d", len(events))
	}
	for i, event := range events {
		if event.Seq != uint64(i+1) {
			t.Fatalf("sequence not monotonic: %+v", events)
		}
	}
	if len(steps) != 3 {
		t.Fatalf("delta should not be durable, got steps=%+v", steps)
	}
	if len(transport.delivered[1]) != 4 || len(transport.delivered[2]) != 4 {
		t.Fatalf("recipients not fanned out: %+v", transport.delivered)
	}
}

func TestRunBroadcasterFallsBackWhenStoreFails(t *testing.T) {
	transport := &recordingTransport{}
	broadcaster := NewRunBroadcaster(nil, transport, nil, nil)
	if err := broadcaster.Publish(context.Background(), RunEvent{RunID: 3, Type: RunStarted}, []int64{7}); err != nil {
		t.Fatal(err)
	}
	if got := transport.delivered[7][0].Seq; got == 0 {
		t.Fatalf("expected local fallback sequence, got %d", got)
	}
}
