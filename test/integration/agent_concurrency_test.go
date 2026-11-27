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
	testAgentQueueClaims(t, 100, 100)
}

// TestAgentQueue_500ConcurrentClaims is the higher-concurrency queue check.
func TestAgentQueue_500ConcurrentClaims(t *testing.T) {
	testAgentQueueClaims(t, 500, 50)
}

func testAgentQueueClaims(t *testing.T, total, concurrency int) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()
	store := agentdata.NewStore(ts.data.DB())
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
	if err != nil || queued < int64(total) {
		t.Fatalf("queue stats before claims: queued=%d err=%v", queued, err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	attempts := map[string]int{}
	sem := make(chan struct{}, concurrency)
	for _, run := range runs {
		wg.Add(1)
		go func(id uint64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
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

// TestAgentApproval_Recover100Concurrent verifies bulk approval recovery:
// every waiting run is requeued exactly once under bounded concurrency.
func TestAgentApproval_Recover100Concurrent(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()
	store := agentdata.NewStore(ts.data.DB())

	const total = 100
	approvals := make([]*domain.AgentApproval, 0, total)
	for i := 0; i < total; i++ {
		run := &domain.AgentRun{
			EventID: fmt.Sprintf("evt-approval-load-%d-%d", time.Now().UnixNano(), i),
			AgentID: int64(9100 + i), ActorUserID: int64(9200 + i),
			Topic: fmt.Sprintf("p2p_%d_%d", 9200+i, 9100+i),
			MsgID: int64(i + 1), TopicSeq: 1, Status: domain.RunQueued,
		}
		if created, err := store.EnqueueRun(ctx, run); err != nil || !created {
			t.Fatalf("enqueue %d: created=%v err=%v", i, created, err)
		}
		if err := store.UpdateRun(ctx, run.ID, map[string]interface{}{"status": domain.RunWaitingApproval}); err != nil {
			t.Fatalf("park run %d: %v", i, err)
		}
		approval := &domain.AgentApproval{
			RunID: run.ID, ToolCallID: fmt.Sprintf("tool-load-%d", i), Tool: "send_message",
			ArgsSummary: "summary", ArgsHash: "hash", ApproverID: run.ActorUserID,
			Status: domain.ApprovalPending, ExpiresAt: time.Now().Add(time.Hour),
		}
		if err := store.CreateApproval(ctx, approval); err != nil {
			t.Fatalf("create approval %d: %v", i, err)
		}
		approvals = append(approvals, approval)
	}

	sem := make(chan struct{}, 25)
	var wg sync.WaitGroup
	for _, approval := range approvals {
		wg.Add(1)
		go func(item *domain.AgentApproval) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := store.DecideAndRequeue(ctx, item.ID, domain.ApprovalApproved, item.ApproverID); err != nil {
				t.Errorf("decide approval %d: %v", item.ID, err)
			}
		}(approval)
	}
	wg.Wait()

	for _, approval := range approvals {
		got, err := store.GetApproval(ctx, approval.ID)
		if err != nil || got.Status != domain.ApprovalApproved {
			t.Fatalf("approval %d status=%q err=%v", approval.ID, got.Status, err)
		}
		run, err := store.GetRun(ctx, approval.RunID)
		if err != nil || run.Status != domain.RunQueued {
			t.Fatalf("run %d status=%q err=%v", approval.RunID, run.Status, err)
		}
	}
}
