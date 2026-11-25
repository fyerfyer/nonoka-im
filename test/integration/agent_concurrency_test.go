package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"nonoka-im/internal/agent/domain"
	agentdata "nonoka-im/internal/data/agent"
)

// TestAgentQueue_100ConcurrentClaims is the release-gate queue/lease check:
// 100 independent runs can be claimed concurrently, each exactly once, and the
// queue gauges drain to zero.
func TestAgentQueue_100ConcurrentClaims(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()
	store := agentdata.NewStore(ts.data.DB())

	const total = 100
	runs := make([]*domain.AgentRun, 0, total)
	for i := 0; i < total; i++ {
		run := &domain.AgentRun{
			EventID: fmt.Sprintf("evt-load-%d-%d", time.Now().UnixNano(), i),
			AgentID: int64(7000 + i), ActorUserID: int64(8000 + i),
			Topic: fmt.Sprintf("p2p_%d_%d", 8000+i, 7000+i),
			MsgID: int64(i + 1), TopicSeq: 1, Status: domain.RunQueued,
		}
		if created, err := store.EnqueueRun(ctx, run); err != nil || !created {
			t.Fatalf("enqueue %d: created=%v err=%v", i, created, err)
		}
		runs = append(runs, run)
	}
	queued, _, err := store.QueueStats(ctx)
	if err != nil || queued < total {
		t.Fatalf("queue stats before claims: queued=%d err=%v", queued, err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	attempts := map[string]int{}
	for _, run := range runs {
		wg.Add(1)
		go func(id uint64) {
			defer wg.Done()
			claimed, ok, err := store.ClaimRun(ctx, id, time.Minute)
			if err != nil {
				t.Errorf("claim %d: %v", id, err)
				return
			}
			if !ok {
				return
			}
			mu.Lock()
			attempts[claimed.AttemptID]++
			mu.Unlock()
		}(run.ID)
	}
	wg.Wait()
	if len(attempts) != total {
		t.Fatalf("claimed %d distinct runs, want %d", len(attempts), total)
	}
	for attempt, count := range attempts {
		if count != 1 {
			t.Fatalf("attempt %s won %d times", attempt, count)
		}
	}
	queued, _, err = store.QueueStats(ctx)
	if err != nil || queued != 0 {
		t.Fatalf("queue stats after claims: queued=%d err=%v", queued, err)
	}
}
