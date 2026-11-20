package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/agent/domain"
	agentdata "nonoka-im/internal/data/agent"
	imdata "nonoka-im/internal/data/im"
	platformdata "nonoka-im/internal/data/platform"
	"nonoka-im/internal/msgworker"

	"google.golang.org/protobuf/proto"
)

func TestAgentRun_DuplicateEvent_StaysQueued(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	store := agentdata.NewStore(ts.data.DB())
	ctx := context.Background()
	eventID := fmt.Sprintf("evt-dup-%d", time.Now().UnixNano())

	first := &domain.AgentRun{EventID: eventID, AgentID: 7, ActorUserID: 1, Topic: "p2p_1_7", MsgID: 1, TopicSeq: 1, Status: domain.RunQueued}
	created, err := store.EnqueueRun(ctx, first)
	if err != nil || !created {
		t.Fatalf("first enqueue: created=%v err=%v", created, err)
	}
	second := &domain.AgentRun{EventID: eventID, AgentID: 7, ActorUserID: 1, Topic: "p2p_1_7", MsgID: 2, TopicSeq: 2, Status: domain.RunQueued}
	created, err = store.EnqueueRun(ctx, second)
	if err != nil || created {
		t.Fatalf("duplicate enqueue must be a no-op: created=%v err=%v", created, err)
	}
	run, err := store.GetRun(ctx, first.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != domain.RunQueued || run.Attempts != 0 || run.AttemptID != "" {
		t.Fatalf("duplicate event mutated run: status=%q attempts=%d attempt=%q", run.Status, run.Attempts, run.AttemptID)
	}
}

func TestAgentRun_ConcurrentClaim_OneWinnerAndFencing(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	store := agentdata.NewStore(ts.data.DB())
	ctx := context.Background()
	run := &domain.AgentRun{EventID: fmt.Sprintf("evt-claim-%d", time.Now().UnixNano()), AgentID: 8, ActorUserID: 2, Topic: "p2p_2_8", MsgID: 1, TopicSeq: 1, Status: domain.RunQueued}
	if created, err := store.EnqueueRun(ctx, run); err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	var winner *domain.AgentRun
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, ok, err := store.ClaimRun(ctx, run.ID, time.Minute)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			if ok {
				mu.Lock()
				winners++
				winner = claimed
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if winners != 1 || winner == nil {
		t.Fatalf("expected exactly one claim winner, got %d", winners)
	}
	if winner.AttemptID == "" || winner.Status != domain.RunRunning || winner.Attempts != 1 {
		t.Fatalf("unexpected claimed run: %+v", winner)
	}

	// A second claim while the lease is valid must fail.
	if _, ok, err := store.ClaimRun(ctx, run.ID, time.Minute); err != nil || ok {
		t.Fatalf("second claim while leased: ok=%v err=%v", ok, err)
	}

	// Expire the lease; the sweeper requeues, and a new claim gets a new fence.
	if err := ts.data.DB().Model(&domain.AgentRun{}).Where("id = ?", run.ID).Update("lease_until", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	requeued, failed, err := store.SweepStaleRuns(ctx, 5)
	if err != nil || requeued != 1 || failed != 0 {
		t.Fatalf("sweep: requeued=%d failed=%d err=%v", requeued, failed, err)
	}
	second, ok, err := store.ClaimRun(ctx, run.ID, time.Minute)
	if err != nil || !ok || second.AttemptID == winner.AttemptID || second.Attempts != 2 {
		t.Fatalf("second claim after sweep: ok=%v run=%+v err=%v", ok, second, err)
	}
	// The stale attempt cannot write terminal state.
	if err := store.UpdateRunFenced(ctx, run.ID, winner.AttemptID, map[string]interface{}{"status": domain.RunCompleted}); err != domain.ErrRunState {
		t.Fatalf("stale fencing write: got %v, want ErrRunState", err)
	}
	if err := store.UpdateRunFenced(ctx, run.ID, second.AttemptID, map[string]interface{}{"status": domain.RunCompleted, "finished_at": time.Now()}); err != nil {
		t.Fatalf("current attempt write: %v", err)
	}
}

func TestAgentRun_WaitingApprovalBlocksSession(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	store := agentdata.NewStore(ts.data.DB())
	ctx := context.Background()
	agentID, actorID := int64(9), int64(3)
	topic := "p2p_3_9"
	base := time.Now().UnixNano()

	first := &domain.AgentRun{EventID: fmt.Sprintf("evt-block-1-%d", base), AgentID: agentID, ActorUserID: actorID, Topic: topic, MsgID: 1, TopicSeq: 1, Status: domain.RunQueued}
	second := &domain.AgentRun{EventID: fmt.Sprintf("evt-block-2-%d", base), AgentID: agentID, ActorUserID: actorID, Topic: topic, MsgID: 2, TopicSeq: 2, Status: domain.RunQueued}
	for _, run := range []*domain.AgentRun{first, second} {
		if created, err := store.EnqueueRun(ctx, run); err != nil || !created {
			t.Fatalf("enqueue %s: created=%v err=%v", run.EventID, created, err)
		}
	}
	claimed, ok, err := store.ClaimRun(ctx, first.ID, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim first: ok=%v err=%v", ok, err)
	}
	if err := store.UpdateRunFenced(ctx, first.ID, claimed.AttemptID, map[string]interface{}{
		"status": domain.RunWaitingApproval, "attempt_id": "", "lease_until": nil, "started_at": nil,
	}); err != nil {
		t.Fatalf("park first run: %v", err)
	}

	runs, err := store.QueuedRuns(ctx, 32)
	if err != nil {
		t.Fatalf("queued runs: %v", err)
	}
	for _, run := range runs {
		if run.ID == second.ID {
			t.Fatal("waiting_approval run must block the next run in the session")
		}
	}
	if _, ok, err := store.ClaimRun(ctx, second.ID, time.Minute); err != nil || ok {
		t.Fatalf("claim blocked run: ok=%v err=%v", ok, err)
	}

	if err := store.UpdateRun(ctx, first.ID, map[string]interface{}{"status": domain.RunCompleted}); err != nil {
		t.Fatalf("complete first run: %v", err)
	}
	if _, ok, err := store.ClaimRun(ctx, second.ID, time.Minute); err != nil || !ok {
		t.Fatalf("claim after unblock: ok=%v err=%v", ok, err)
	}
}

func TestAgentApproval_ExpiredRejected(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()
	tokenOwner, userOwner := registerAndLogin(t, "approval-expired", "123456")
	agentID := createPublishedAgent(t, tokenOwner, "approval-expired-agent", "test agent", nil)
	store := agentdata.NewStore(ts.data.DB())
	agent, err := store.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	topic := fmt.Sprintf("p2p_%d_%d", userOwner, agent.BotUserID)
	run := &domain.AgentRun{EventID: fmt.Sprintf("evt-expired-%d", time.Now().UnixNano()), AgentID: agentID, AgentVersion: 1, ActorUserID: userOwner, Topic: topic, MsgID: 1, Status: domain.RunWaitingApproval}
	if created, err := store.EnqueueRun(ctx, run); err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}
	approval := &domain.AgentApproval{
		RunID: run.ID, ToolCallID: "tool-1", Tool: "send_message", ArgsSummary: "summary", ArgsHash: "hash",
		ApproverID: userOwner, Status: domain.ApprovalPending, ExpiresAt: time.Now().Add(-time.Minute),
	}
	if err := store.CreateApproval(ctx, approval); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	code, body := approvalAct(t, approval.ID, "approve", tokenOwner)
	if code != http.StatusConflict || body["code"] != "APPROVAL_EXPIRED" {
		t.Fatalf("expired approve: code=%d body=%v", code, body)
	}
	row, err := store.GetApproval(ctx, approval.ID)
	if err != nil || row.Status != domain.ApprovalExpired {
		t.Fatalf("approval after expired decision: status=%q err=%v", row.Status, err)
	}
	if got, err := store.GetRun(ctx, run.ID); err != nil || got.Status != domain.RunWaitingApproval {
		t.Fatalf("run after expired decision: status=%q err=%v", got.Status, err)
	}
}

// TestAgentApproval_ConcurrentCreateOneRow verifies the unique
// (run_id, tool_call_id) index and the insert-or-select transaction: concurrent
// redeliveries of the same interrupt must all observe the same approval row.
func TestAgentApproval_ConcurrentCreateOneRow(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	store := agentdata.NewStore(ts.data.DB())
	ctx := context.Background()
	run := &domain.AgentRun{EventID: fmt.Sprintf("evt-approval-race-%d", time.Now().UnixNano()), AgentID: 11, ActorUserID: 5, Topic: "p2p_5_11", MsgID: 1, TopicSeq: 1, Status: domain.RunQueued}
	if created, err := store.EnqueueRun(ctx, run); err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}

	const workers = 8
	var wg sync.WaitGroup
	ids := make([]uint64, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			row := &domain.AgentApproval{
				RunID: run.ID, ToolCallID: "tool-call-1", Tool: "send_message",
				ArgsSummary: "summary", ArgsHash: "hash", ApproverID: 5,
				Status: domain.ApprovalPending, ExpiresAt: time.Now().Add(time.Hour),
			}
			errs[i] = store.CreateApproval(ctx, row)
			ids[i] = row.ID
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
		if ids[i] == 0 || ids[i] != ids[0] {
			t.Fatalf("workers saw different approval IDs: %v", ids)
		}
	}
	var count int64
	if err := ts.data.DB().Model(&domain.AgentApproval{}).Where("run_id = ? AND tool_call_id = ?", run.ID, "tool-call-1").Count(&count).Error; err != nil {
		t.Fatalf("count approvals: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one approval row, got %d", count)
	}
}

// TestAgentApproval_DecideAndRequeueAtomic verifies the decision and the run
// requeue commit together, and that a second decision is rejected.
func TestAgentApproval_DecideAndRequeueAtomic(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	store := agentdata.NewStore(ts.data.DB())
	ctx := context.Background()
	run := &domain.AgentRun{EventID: fmt.Sprintf("evt-approval-requeue-%d", time.Now().UnixNano()), AgentID: 12, ActorUserID: 6, Topic: "p2p_6_12", MsgID: 1, TopicSeq: 1, Status: domain.RunQueued}
	if created, err := store.EnqueueRun(ctx, run); err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}
	if err := store.UpdateRun(ctx, run.ID, map[string]interface{}{"status": domain.RunWaitingApproval}); err != nil {
		t.Fatalf("park run: %v", err)
	}
	approval := &domain.AgentApproval{
		RunID: run.ID, ToolCallID: "tool-call-2", Tool: "send_message", ArgsSummary: "summary", ArgsHash: "hash",
		ApproverID: 6, Status: domain.ApprovalPending, ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.CreateApproval(ctx, approval); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	if err := store.DecideAndRequeue(ctx, approval.ID, domain.ApprovalApproved, 6); err != nil {
		t.Fatalf("decide and requeue: %v", err)
	}
	gotApproval, err := store.GetApproval(ctx, approval.ID)
	if err != nil || gotApproval.Status != domain.ApprovalApproved {
		t.Fatalf("approval after decision: status=%q err=%v", gotApproval.Status, err)
	}
	gotRun, err := store.GetRun(ctx, run.ID)
	if err != nil || gotRun.Status != domain.RunQueued {
		t.Fatalf("run after decision: status=%q err=%v", gotRun.Status, err)
	}
	if err := store.DecideAndRequeue(ctx, approval.ID, domain.ApprovalRejected, 6); !errors.Is(err, domain.ErrApprovalGone) {
		t.Fatalf("second decision: got %v, want ErrApprovalGone", err)
	}
}

// TestToolInvocationLedger verifies the durable side-effect ledger against
// PostgreSQL: concurrent/duplicate begin calls share one row, completion is
// cached, and a started call is not silently re-executed.
func TestToolInvocationLedger(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	store := agentdata.NewStore(ts.data.DB())
	ctx := context.Background()
	row := &domain.ToolInvocation{
		RunID: 77, ToolCallID: "call-ledger-1", Tool: "send_message", ArgsHash: "hash",
		Status: domain.ToolInvocationStarted,
	}
	first, created, err := store.BeginToolInvocation(ctx, row)
	if err != nil || !created || first.ID == 0 {
		t.Fatalf("first begin: created=%v row=%+v err=%v", created, first, err)
	}
	secondRow := &domain.ToolInvocation{
		RunID: 77, ToolCallID: "call-ledger-1", Tool: "send_message", ArgsHash: "hash",
		Status: domain.ToolInvocationStarted,
	}
	second, created, err := store.BeginToolInvocation(ctx, secondRow)
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("duplicate begin: created=%v row=%+v err=%v", created, second, err)
	}
	if second.Status != domain.ToolInvocationStarted {
		t.Fatalf("unexpected status before completion: %q", second.Status)
	}
	if err := store.CompleteToolInvocation(ctx, 77, "call-ledger-1", `{"accepted":true}`); err != nil {
		t.Fatalf("complete: %v", err)
	}
	thirdRow := &domain.ToolInvocation{
		RunID: 77, ToolCallID: "call-ledger-1", Tool: "send_message", ArgsHash: "hash",
		Status: domain.ToolInvocationStarted,
	}
	third, created, err := store.BeginToolInvocation(ctx, thirdRow)
	if err != nil || created || third.Status != domain.ToolInvocationCompleted || third.ResultSummary != `{"accepted":true}` {
		t.Fatalf("completed replay: created=%v row=%+v err=%v", created, third, err)
	}
}

// TestAgentRun_SideEffectBlocksAutomaticRetry verifies that once a write tool
// committed an external effect, a retryable failure is forced terminal and a
// retry requires an explicit acknowledgement.
func TestAgentRun_SideEffectBlocksAutomaticRetry(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	store := agentdata.NewStore(ts.data.DB())
	ctx := context.Background()
	run := &domain.AgentRun{EventID: fmt.Sprintf("evt-side-effect-%d", time.Now().UnixNano()), AgentID: 13, ActorUserID: 7, Topic: "p2p_7_13", MsgID: 1, TopicSeq: 1, Status: domain.RunQueued}
	if created, err := store.EnqueueRun(ctx, run); err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}
	claimed, ok, err := store.ClaimRun(ctx, run.ID, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if ok, err := store.MarkRunSideEffect(ctx, claimed.ID, claimed.AttemptID); err != nil || !ok {
		t.Fatalf("mark side effect: ok=%v err=%v", ok, err)
	}
	if err := store.FailRun(ctx, claimed, domain.Failure{
		Class: "transient_infra", Code: "infra", Message: "temporary", Retryable: true,
	}, 5, nil); err != nil {
		t.Fatalf("fail run: %v", err)
	}
	got, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != domain.RunFailed || got.ErrorClass != domain.FailureClassSideEffect || got.ErrorCode != domain.FailureCodeManualRetry {
		t.Fatalf("side effect must force terminal: status=%q class=%q code=%q", got.Status, got.ErrorClass, got.ErrorCode)
	}
	if err := store.SetRunAction(ctx, run.ID, "retry", false); !errors.Is(err, domain.ErrSideEffectCommitted) {
		t.Fatalf("retry without ack: got %v, want ErrSideEffectCommitted", err)
	}
	if err := store.SetRunAction(ctx, run.ID, "retry", true); err != nil {
		t.Fatalf("retry with ack: %v", err)
	}
	got, err = store.GetRun(ctx, run.ID)
	if err != nil || got.Status != domain.RunQueued || got.SideEffectAt != nil {
		t.Fatalf("acknowledged retry: status=%q sideEffectAt=%v err=%v", got.Status, got.SideEffectAt, err)
	}
}

// TestAgentDetail_ViewerDoesNotSeePublishedConfig verifies that sharing an
// agent as a viewer does not expose the owner's system prompt or model URL.
func TestAgentDetail_ViewerDoesNotSeePublishedConfig(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	tokenOwner, _ := registerAndLogin(t, "viewer-owner", "123456")
	tokenViewer, viewerID := registerAndLogin(t, "viewer-user", "123456")
	agentID := createPublishedAgent(t, tokenOwner, "viewer-agent", "SENSITIVE-PROMPT", nil)

	share := httpPutAuth(t, fmt.Sprintf("%s/v1/agents/%d/shares", testBaseURL, agentID), tokenOwner, map[string]any{"kind": "user", "target": fmt.Sprintf("%d", viewerID), "enabled": true})
	share.Body.Close()
	if share.StatusCode != http.StatusOK {
		t.Fatalf("share viewer: %d", share.StatusCode)
	}
	resp := httpGet(t, fmt.Sprintf("%s/v1/agents/%d", testBaseURL, agentID), tokenViewer)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("viewer detail: %d: %s", resp.StatusCode, body)
	}
	var view map[string]any
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if _, ok := view["published"]; ok {
		t.Fatalf("viewer received published config: %s", body)
	}
	if _, ok := view["draft"]; ok {
		t.Fatalf("viewer received draft config: %s", body)
	}
	if strings.Contains(string(body), "SENSITIVE-PROMPT") {
		t.Fatalf("viewer response leaked instruction: %s", body)
	}
}

// TestIdempotencyPruneExpired verifies the retention TTL actually removes
// completed idempotency records instead of growing forever.
func TestIdempotencyPruneExpired(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	store := platformdata.NewIdempotencyStore(ts.data.DB())
	store.TTL = 10 * time.Millisecond
	ctx := context.Background()
	key := fmt.Sprintf("idem-ttl-%d", time.Now().UnixNano())
	if _, acquired, err := store.Begin(ctx, 99, key, "fp", time.Now().Add(time.Minute)); err != nil || !acquired {
		t.Fatalf("begin: acquired=%v err=%v", acquired, err)
	}
	if err := store.Complete(ctx, 99, key, "fp", 200, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("complete: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	pruned, err := store.PruneExpired(ctx, time.Now(), 100)
	if err != nil || pruned == 0 {
		t.Fatalf("prune: n=%d err=%v", pruned, err)
	}
	var count int64
	if err := ts.data.DB().Model(&platformdata.IdempotencyKey{}).Where("scope_user_id = ? AND key = ?", 99, key).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("expired key must be gone: count=%d err=%v", count, err)
	}
}

func TestAgentApproval_ConsumeOnce(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	store := agentdata.NewStore(ts.data.DB())
	ctx := context.Background()
	run := &domain.AgentRun{EventID: fmt.Sprintf("evt-consume-%d", time.Now().UnixNano()), AgentID: 10, ActorUserID: 4, Topic: "p2p_4_10", MsgID: 1, Status: domain.RunWaitingApproval}
	if created, err := store.EnqueueRun(ctx, run); err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}
	approval := &domain.AgentApproval{
		RunID: run.ID, ToolCallID: "tool-1", Tool: "send_message", ArgsSummary: "summary", ArgsHash: "hash",
		ApproverID: 4, Status: domain.ApprovalApproved, ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.CreateApproval(ctx, approval); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	decision, ok, err := store.ConsumeApproval(ctx, approval.ID, "attempt-1")
	if err != nil || !ok || decision != domain.ApprovalApproved {
		t.Fatalf("first consume: decision=%q ok=%v err=%v", decision, ok, err)
	}
	decision, ok, err = store.ConsumeApproval(ctx, approval.ID, "attempt-2")
	if err != nil || ok {
		t.Fatalf("second consume must fail: decision=%q ok=%v err=%v", decision, ok, err)
	}
}

// recordingPusher captures push target batches for the bot-filter test.
type recordingPusher struct {
	mu      sync.Mutex
	batches [][]int64
}

func (p *recordingPusher) record(ids []int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.batches = append(p.batches, append([]int64(nil), ids...))
}

func (p *recordingPusher) PushToUser(_ context.Context, id int64, _ *v1.MessagePush) (int32, error) {
	p.record([]int64{id})
	return 1, nil
}
func (p *recordingPusher) BatchPushToUsers(_ context.Context, ids []int64, _ *v1.MessagePush) (int32, []int64, error) {
	p.record(ids)
	return int32(len(ids)), nil, nil
}
func (p *recordingPusher) PushReceiptToUser(context.Context, int64, *v1.SendReceipt) (int32, error) {
	return 1, nil
}
func (p *recordingPusher) BatchPushReceiptToUsers(context.Context, []int64, *v1.SendReceipt) (int32, []int64, error) {
	return 1, nil, nil
}
func (p *recordingPusher) BatchPushReceiptsToUsers(context.Context, []*v1.ReceiptBatchItem) (int32, []*v1.ReceiptBatchItem, error) {
	return 0, nil, nil
}
func (p *recordingPusher) Close() error { return nil }

func TestMsgWorker_BotRecipientsNotPushed(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tokenOwner, ownerID := registerAndLogin(t, "bot-push-owner", "123456")
	_, memberID := registerAndLogin(t, "bot-push-member", "123456")
	agentID := createPublishedAgent(t, tokenOwner, "bot-push-agent", "test agent", nil)
	store := agentdata.NewStore(ts.data.DB())
	agent, err := store.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}

	groups := imdata.NewGroupRepo(ts.data.DB(), testLogger)
	group, err := groups.CreateGroup(ctx, "bot push group", ownerID, []int64{memberID, agent.BotUserID})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	pusher := &recordingPusher{}
	seqGen := msgworker.NewSeqGenerator(ts.redis, testLogger)
	storage := msgworker.NewMessageStorage(ts.mongoDB, testLogger)
	if err := storage.EnsureIndexes(ctx); err != nil {
		t.Fatalf("ensure indexes: %v", err)
	}
	worker := msgworker.NewMsgWorker(nil, seqGen, msgworker.NewSnowflake(1), storage, pusher, testLogger)
	worker.SetGroupMemberService(msgworker.NewPersistentGroupMemberService(imdata.NewGroupMemberRepo(ts.data.DB(), testLogger), ts.redis, testLogger))
	botIndex := msgworker.NewBotIndexWithQuery(ts.data.ListBotUserIDs, ts.data.IsBotUser, testLogger)
	if err := botIndex.Refresh(ctx); err != nil {
		t.Fatalf("refresh bot index: %v", err)
	}
	worker.SetBotIndex(botIndex)
	if err := worker.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	defer worker.Stop()

	upstream := &v1.UpstreamMessage{
		Topic: group.Topic, SenderId: ownerID, MsgType: int32(v1.MsgType_MSG_TYPE_TEXT),
		Content: []byte("hello group"), ClientMsgId: "bot-push-1", Timestamp: time.Now().Unix(),
	}
	raw, _ := proto.Marshal(upstream)
	if err := worker.HandleMessage(ctx, nil, raw, nil); err != nil {
		t.Fatalf("handle group message: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pusher.mu.Lock()
		seen := len(pusher.batches) > 0
		pusher.mu.Unlock()
		if seen {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	pusher.mu.Lock()
	defer pusher.mu.Unlock()
	if len(pusher.batches) == 0 {
		t.Fatal("expected at least one group push batch")
	}
	foundMember := false
	for _, batch := range pusher.batches {
		for _, id := range batch {
			if id == agent.BotUserID {
				t.Fatalf("bot user %d must not be a push target: %v", agent.BotUserID, pusher.batches)
			}
			if id == memberID {
				foundMember = true
			}
		}
	}
	if !foundMember {
		t.Fatalf("human group member was not pushed: %v", pusher.batches)
	}
}

// TestAgentApprovalList verifies the approval inbox contract added in Phase 0.
func TestAgentApprovalList(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()
	tokenOwner, userOwner := registerAndLogin(t, "approval-list", "123456")
	agentID := createPublishedAgent(t, tokenOwner, "approval-list-agent", "test agent", nil)
	store := agentdata.NewStore(ts.data.DB())
	agent, err := store.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	topic := fmt.Sprintf("p2p_%d_%d", userOwner, agent.BotUserID)
	run := &domain.AgentRun{EventID: fmt.Sprintf("evt-list-%d", time.Now().UnixNano()), AgentID: agentID, AgentVersion: 1, ActorUserID: userOwner, Topic: topic, MsgID: 1, Status: domain.RunWaitingApproval}
	if created, err := store.EnqueueRun(ctx, run); err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}
	approval := &domain.AgentApproval{RunID: run.ID, ToolCallID: "tool-1", Tool: "send_message", ArgsSummary: "summary", ArgsHash: "hash", ApproverID: userOwner, Status: domain.ApprovalPending, ExpiresAt: time.Now().Add(time.Hour)}
	if err := store.CreateApproval(ctx, approval); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	resp := httpGet(t, testBaseURL+"/v1/agent-approvals?status=pending", tokenOwner)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list approvals: %d: %s", resp.StatusCode, body)
	}
	var page struct {
		Items []struct {
			ID     uint64 `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode page: %v (%s)", err, body)
	}
	if len(page.Items) == 0 || page.Items[0].ID != approval.ID || page.Items[0].Status != domain.ApprovalPending {
		t.Fatalf("unexpected approvals page: %+v", page.Items)
	}
}
