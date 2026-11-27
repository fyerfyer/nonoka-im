package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	agentdata "nonoka-im/internal/data/agent"
	"nonoka-im/internal/messaging"
)

type authorizerStub struct {
	mu     sync.Mutex
	denied int64
	calls  []int64
}

func (a *authorizerStub) CanAccessTopic(_ context.Context, uid int64, _ string) error {
	a.mu.Lock()
	a.calls = append(a.calls, uid)
	a.mu.Unlock()
	if uid == a.denied {
		return errors.New("denied")
	}
	return nil
}
func (*authorizerStub) CanReadFile(context.Context, int64, string) error { return nil }
func (*authorizerStub) CanExecuteTool(context.Context, int64, int64, string, string, string, string) error {
	return nil
}

func TestHandleResolvedBotAuthorization(t *testing.T) {
	t.Run("bot message ignored", func(t *testing.T) {
		az := &authorizerStub{}
		h := &Harness{Authorizer: az}
		err := h.HandleResolved(context.Background(), messaging.MessageEvent{SenderID: 99, Topic: "p2p_1_99"}, ResolvedAgent{ID: 7, BotID: 99}, nil, nil, nil, nil)
		if err != nil || len(az.calls) != 0 {
			t.Fatalf("err=%v auth calls=%v", err, az.calls)
		}
	})
	t.Run("group topic requires actor and bot access", func(t *testing.T) {
		az := &authorizerStub{denied: 99}
		h := &Harness{Authorizer: az}
		err := h.HandleResolved(context.Background(), messaging.MessageEvent{SenderID: 1, Topic: "grp_group", MentionedAgentIDs: []int64{99}}, ResolvedAgent{ID: 7, BotID: 99}, nil, nil, nil, nil)
		if err == nil {
			t.Fatal("expected bot membership denial")
		}
		if len(az.calls) != 2 || az.calls[0] != 1 || az.calls[1] != 99 {
			t.Fatalf("unexpected auth calls: %v", az.calls)
		}
	})
	t.Run("missing dependencies reported", func(t *testing.T) {
		h := &Harness{Authorizer: &authorizerStub{}}
		err := h.HandleResolved(context.Background(), messaging.MessageEvent{SenderID: 1, Topic: "p2p_1_99"}, ResolvedAgent{ID: 7, BotID: 99}, &domain.AgentRun{ID: 1, AttemptID: "attempt"}, &domain.AgentSession{}, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "agent harness dependencies unavailable") {
			t.Fatalf("unexpected err: %v", err)
		}
	})
}

// setupHarnessStore connects to a real PostgreSQL for harness behavior tests.
// Set NONOKA_AGENT_TEST_PG_DSN to override; skipped when no PostgreSQL is
// reachable (the integration suite covers the live runs).
func setupHarnessStore(t *testing.T) *agentdata.Store {
	t.Helper()
	dsns := []string{os.Getenv("NONOKA_AGENT_TEST_PG_DSN")}
	if dsns[0] == "" {
		dsns = dsns[:0]
	}
	dsns = append(dsns,
		"host=127.0.0.1 user=postgres password=root dbname=nonoka_im_test port=5433 sslmode=disable",
		"host=127.0.0.1 user=postgres password=root dbname=nonoka_im port=5432 sslmode=disable",
		"host=127.0.0.1 user=nonoka password=nonoka dbname=nonoka port=5432 sslmode=disable",
	)
	for _, dsn := range dsns {
		d, cleanup, err := data.NewData(&conf.Data{Database: &conf.Data_Database{Source: dsn}})
		if err != nil {
			continue
		}
		t.Cleanup(cleanup)
		return agentdata.NewStore(d.DB())
	}
	t.Skip("PostgreSQL not reachable; set NONOKA_AGENT_TEST_PG_DSN")
	return nil
}

type failingRuntime struct{ err error }

func (f failingRuntime) Run(context.Context, port.Input) (port.Result, error) {
	return port.Result{}, f.err
}

func (f failingRuntime) Resume(context.Context, port.ResumeInput) (port.Result, error) {
	return port.Result{}, f.err
}

type stubRuntime struct{ result port.Result }

func (s stubRuntime) Run(context.Context, port.Input) (port.Result, error) {
	return s.result, nil
}

func (s stubRuntime) Resume(context.Context, port.ResumeInput) (port.Result, error) {
	return s.result, nil
}

type recordingSender struct {
	sent      []string
	receiptID int64
}

func (r *recordingSender) Send(_ context.Context, _, content, _ string) error {
	r.sent = append(r.sent, content)
	return nil
}
func (r *recordingSender) SendWithReceipt(_ context.Context, _, content, _ string) (int64, error) {
	r.sent = append(r.sent, content)
	return r.receiptID, nil
}
func (r *recordingSender) SendCard(context.Context, string, string, string) (int64, error) {
	return 1, nil
}

// claimRun enqueues and claims a run for the harness tests, returning the run
// plus the state the executor would have loaded.
func claimRun(t *testing.T, store *agentdata.Store, run *domain.AgentRun) (*domain.AgentRun, *domain.AgentSession, *domain.AgentContextSnapshot, []domain.AgentTurn) {
	t.Helper()
	ctx := context.Background()
	run.Status = domain.RunQueued
	if created, err := store.EnqueueRun(ctx, run); err != nil || !created {
		t.Fatalf("enqueue run: created=%v err=%v", created, err)
	}
	claimed, ok, err := store.ClaimRun(ctx, run.ID, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim run: ok=%v err=%v", ok, err)
	}
	session, snapshot, turns, err := store.LoadRunState(ctx, claimed)
	if err != nil {
		t.Fatalf("load run state: %v", err)
	}
	return claimed, session, snapshot, turns
}

func findRunByEvent(t *testing.T, store *agentdata.Store, agentID int64, eventID string) *domain.AgentRun {
	t.Helper()
	runs, err := store.ListRuns(context.Background(), agentID, 50)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	for i := range runs {
		if runs[i].EventID == eventID {
			return &runs[i]
		}
	}
	t.Fatalf("run with event %q not found", eventID)
	return nil
}

func turnsForRun(t *testing.T, store *agentdata.Store, runID uint64, agentID, actorID int64, topic string) []domain.AgentTurn {
	t.Helper()
	rows, err := store.RecentTurns(context.Background(), agentID, actorID, topic, 50)
	if err != nil {
		t.Fatalf("RecentTurns: %v", err)
	}
	out := make([]domain.AgentTurn, 0, 1)
	for _, row := range rows {
		if row.RunID == runID {
			out = append(out, row)
		}
	}
	return out
}

func TestHandleResolvedPhaseBFailureMarksRunFailed(t *testing.T) {
	store := setupHarnessStore(t)
	ctx := context.Background()
	agentID, botID, actorID := int64(987000011), int64(987000012), int64(987000013)
	topic := fmt.Sprintf("p2p_%d_%d_%d", actorID, botID, time.Now().UnixNano())
	eventID := fmt.Sprintf("evt-fail-%d", time.Now().UnixNano())

	run, session, snapshot, turns := claimRun(t, store, &domain.AgentRun{
		EventID: eventID, AgentID: agentID, AgentVersion: 1, ActorUserID: actorID,
		Topic: topic, MsgID: 1001, TopicSeq: 100, CreatedAt: time.Now(),
	})
	h := &Harness{
		Store:       store,
		Runtime:     failingRuntime{err: errors.New("model exploded")},
		Authorizer:  &authorizerStub{},
		Sender:      &recordingSender{receiptID: 4242},
		MaxAttempts: 1,
	}
	ev := messaging.MessageEvent{SchemaVersion: messaging.SchemaVersion, EventID: eventID, MsgID: 1001, TopicSeq: 100, Topic: topic, SenderID: actorID, Content: "hello agent"}
	err := h.HandleResolved(ctx, ev, ResolvedAgent{ID: agentID, BotID: botID, Version: 1, Instruction: "be helpful", Model: port.ModelEndpoint{BaseURL: "https://fixture.invalid/v1", APIKey: "fixture-key", Model: "deepseek-chat"}}, run, session, snapshot, turns)
	if err == nil || err.Error() != "model exploded" {
		t.Fatalf("expected original runtime error, got %v", err)
	}

	row := findRunByEvent(t, store, agentID, eventID)
	// The default MaxAttempts is 0, which FailRun treats as unlimited retries;
	// harness tests that assert the terminal state set MaxAttempts=1.
	if row.Status != domain.RunFailed {
		t.Fatalf("run must be failed after phase B error, got %q", row.Status)
	}
	if got := turnsForRun(t, store, row.ID, agentID, actorID, topic); len(got) != 0 {
		t.Fatalf("no turn must be written on phase B failure, got %d", len(got))
	}
}

type authzDeniedStub struct{ err error }

func (a authzDeniedStub) CanAccessTopic(context.Context, int64, string) error { return a.err }
func (authzDeniedStub) CanReadFile(context.Context, int64, string) error      { return nil }
func (authzDeniedStub) CanExecuteTool(context.Context, int64, int64, string, string, string, string) error {
	return nil
}
func (a authzDeniedStub) IsDenied(err error) bool { return errors.Is(err, port.ErrAuthDenied) }

func TestHandleResolvedAuthDeniedMarksRunTerminal(t *testing.T) {
	store := setupHarnessStore(t)
	ctx := context.Background()
	agentID, botID, actorID := int64(987000031), int64(987000032), int64(987000033)
	topic := fmt.Sprintf("p2p_%d_%d_%d", actorID, botID, time.Now().UnixNano())
	eventID := fmt.Sprintf("evt-auth-denied-%d", time.Now().UnixNano())
	run, session, snapshot, turns := claimRun(t, store, &domain.AgentRun{
		EventID: eventID, AgentID: agentID, AgentVersion: 1, ActorUserID: actorID,
		Topic: topic, MsgID: 1002, TopicSeq: 101, CreatedAt: time.Now(),
	})
	h := &Harness{
		Store: store, Runtime: stubRuntime{result: port.Result{Text: "unused"}},
		Authorizer: authzDeniedStub{err: port.ErrAuthDenied}, Sender: &recordingSender{},
		MaxAttempts: 1,
	}
	ev := messaging.MessageEvent{SchemaVersion: messaging.SchemaVersion, EventID: eventID, MsgID: 1002, TopicSeq: 101, Topic: topic, SenderID: actorID, Content: "hello"}
	if err := h.HandleResolved(ctx, ev, ResolvedAgent{ID: agentID, BotID: botID, Version: 1}, run, session, snapshot, turns); err == nil {
		t.Fatal("expected authorization denial")
	}
	row := findRunByEvent(t, store, agentID, eventID)
	if row.Status != domain.RunFailed || row.ErrorClass != string(port.ClassAuthDenied) {
		t.Fatalf("auth denial must be terminal: status=%q class=%q", row.Status, row.ErrorClass)
	}
}

func TestHandleResolvedAuthOutageIsRetryable(t *testing.T) {
	store := setupHarnessStore(t)
	ctx := context.Background()
	agentID, botID, actorID := int64(987000041), int64(987000042), int64(987000043)
	topic := fmt.Sprintf("p2p_%d_%d_%d", actorID, botID, time.Now().UnixNano())
	eventID := fmt.Sprintf("evt-auth-outage-%d", time.Now().UnixNano())
	run, session, snapshot, turns := claimRun(t, store, &domain.AgentRun{
		EventID: eventID, AgentID: agentID, AgentVersion: 1, ActorUserID: actorID,
		Topic: topic, MsgID: 1003, TopicSeq: 102, CreatedAt: time.Now(),
	})
	h := &Harness{
		Store: store, Runtime: stubRuntime{result: port.Result{Text: "unused"}},
		Authorizer: authzDeniedStub{err: errors.New("openfga unavailable")}, Sender: &recordingSender{},
		MaxAttempts: 5,
	}
	ev := messaging.MessageEvent{SchemaVersion: messaging.SchemaVersion, EventID: eventID, MsgID: 1003, TopicSeq: 102, Topic: topic, SenderID: actorID, Content: "hello"}
	if err := h.HandleResolved(ctx, ev, ResolvedAgent{ID: agentID, BotID: botID, Version: 1}, run, session, snapshot, turns); err == nil {
		t.Fatal("expected authorization outage error")
	}
	row := findRunByEvent(t, store, agentID, eventID)
	if row.Status != domain.RunQueued || row.ErrorClass != string(port.ClassTransientInfra) {
		t.Fatalf("auth outage must be retryable: status=%q class=%q", row.Status, row.ErrorClass)
	}
	if row.NextAttemptAt == nil {
		t.Fatal("retryable failure must set next_attempt_at")
	}
}

func TestHandleResolvedCompletesRunAndTurn(t *testing.T) {
	store := setupHarnessStore(t)
	ctx := context.Background()
	agentID, botID, actorID := int64(987000021), int64(987000022), int64(987000023)
	topic := fmt.Sprintf("p2p_%d_%d_%d", actorID, botID, time.Now().UnixNano())
	eventID := fmt.Sprintf("evt-ok-%d", time.Now().UnixNano())

	run, session, snapshot, turns := claimRun(t, store, &domain.AgentRun{
		EventID: eventID, AgentID: agentID, AgentVersion: 1, ActorUserID: actorID,
		Topic: topic, MsgID: 2002, TopicSeq: 1, CreatedAt: time.Now(),
	})
	sender := &recordingSender{receiptID: 5150}
	h := &Harness{
		Store:      store,
		Runtime:    stubRuntime{result: port.Result{Text: "hi there", Usage: port.Usage{InputTokens: 3, OutputTokens: 5, TotalTokens: 8}}},
		Authorizer: &authorizerStub{},
		Sender:     sender,
	}
	ev := messaging.MessageEvent{SchemaVersion: messaging.SchemaVersion, EventID: eventID, MsgID: 2002, TopicSeq: 1, Topic: topic, SenderID: actorID, Content: "hello agent"}
	if err := h.HandleResolved(ctx, ev, ResolvedAgent{ID: agentID, BotID: botID, Version: 1, Instruction: "be helpful", Model: port.ModelEndpoint{BaseURL: "https://fixture.invalid/v1", APIKey: "fixture-key", Model: "deepseek-chat"}}, run, session, snapshot, turns); err != nil {
		t.Fatalf("HandleResolved: %v", err)
	}
	if len(sender.sent) != 1 || sender.sent[0] != "hi there" {
		t.Fatalf("reply not sent: %v", sender.sent)
	}
	row := findRunByEvent(t, store, agentID, eventID)
	if row.Status != domain.RunCompleted || row.ReplyMsgID != 5150 {
		t.Fatalf("unexpected run state: %+v", row)
	}
	got := turnsForRun(t, store, row.ID, agentID, actorID, topic)
	if len(got) != 1 || got[0].Output != "hi there" || got[0].ReplyMsgID != 5150 {
		t.Fatalf("unexpected turns: %+v", got)
	}
}

// fakeRunStore stubs the harness persistence for retry tests: CompleteRun
// fails completeFails times before succeeding and records committed turns.
type fakeRunStore struct {
	completeCalls int
	completeFails int
	turns         []domain.AgentTurn
}

func (f *fakeRunStore) UpdateRunFenced(context.Context, uint64, string, map[string]interface{}) error {
	return nil
}
func (f *fakeRunStore) ListRunCitations(context.Context, uint64) ([]domain.AgentRunCitation, error) {
	return nil, nil
}
func (f *fakeRunStore) FailRun(context.Context, *domain.AgentRun, domain.Failure, int, map[string]interface{}) error {
	return nil
}
func (f *fakeRunStore) CreateApproval(_ context.Context, row *domain.AgentApproval) error {
	row.ID = 1
	return nil
}
func (f *fakeRunStore) ParkRunForApproval(_ context.Context, _ *domain.AgentRun, row *domain.AgentApproval, _ string) (*domain.AgentApproval, error) {
	if row.ID == 0 {
		row.ID = 1
	}
	return row, nil
}
func (f *fakeRunStore) CompleteRun(_ context.Context, _ *domain.AgentRun, turn *domain.AgentTurn, _ *domain.AgentContextSnapshot, _ map[string]interface{}) error {
	f.completeCalls++
	if f.completeCalls <= f.completeFails {
		return errors.New("transient commit failure")
	}
	if turn != nil {
		f.turns = append(f.turns, *turn)
	}
	return nil
}

func TestHandleResolvedRetriesCompleteRun(t *testing.T) {
	store := &fakeRunStore{completeFails: 2}
	sender := &recordingSender{receiptID: 77}
	h := &Harness{Store: store, Runtime: stubRuntime{result: port.Result{Text: "ok"}}, Authorizer: &authorizerStub{}, Sender: sender}
	ev := messaging.MessageEvent{SchemaVersion: messaging.SchemaVersion, EventID: "evt-retry", MsgID: 1, Topic: "p2p_1_2", SenderID: 1, Content: "hi"}
	run := &domain.AgentRun{ID: 11, AttemptID: "attempt-1", SessionID: 22, EventID: "evt-retry", ActorUserID: 1, Topic: "p2p_1_2"}
	if err := h.HandleResolved(context.Background(), ev, ResolvedAgent{ID: 7, BotID: 2, Version: 1}, run, &domain.AgentSession{ID: 22}, nil, nil); err != nil {
		t.Fatalf("transient CompleteRun failures must be retried: %v", err)
	}
	if store.completeCalls != 3 {
		t.Fatalf("expected 3 CompleteRun attempts, got %d", store.completeCalls)
	}
	if len(store.turns) != 1 || store.turns[0].ReplyMsgID != 77 {
		t.Fatalf("turn not committed after retries: %+v", store.turns)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("reply must be sent exactly once: %v", sender.sent)
	}
}

func TestHandleResolvedCompleteRunExhaustsRetries(t *testing.T) {
	store := &fakeRunStore{completeFails: 99}
	h := &Harness{Store: store, Runtime: stubRuntime{result: port.Result{Text: "ok"}}, Authorizer: &authorizerStub{}, Sender: &recordingSender{}}
	ev := messaging.MessageEvent{SchemaVersion: messaging.SchemaVersion, EventID: "evt-giveup", MsgID: 1, Topic: "p2p_1_2", SenderID: 1, Content: "hi"}
	run := &domain.AgentRun{ID: 11, AttemptID: "attempt-1", SessionID: 22, EventID: "evt-giveup", ActorUserID: 1, Topic: "p2p_1_2"}
	if err := h.HandleResolved(context.Background(), ev, ResolvedAgent{ID: 7, BotID: 2, Version: 1}, run, &domain.AgentSession{ID: 22}, nil, nil); err == nil {
		t.Fatal("expected error when CompleteRun keeps failing")
	}
	if store.completeCalls != 3 {
		t.Fatalf("expected exactly 3 attempts, got %d", store.completeCalls)
	}
	if len(store.turns) != 0 {
		t.Fatalf("no turn must be committed when CompleteRun never succeeds: %+v", store.turns)
	}
}

func TestStableClientIDDeterministic(t *testing.T) {
	a := stableClientID("evt-1", 7)
	if a != stableClientID("evt-1", 7) {
		t.Fatal("stableClientID must be deterministic")
	}
	if a == stableClientID("evt-2", 7) || a == stableClientID("evt-1", 8) {
		t.Fatal("stableClientID must differ per event/agent")
	}
	if len(a) != len("agent-")+32 {
		t.Fatalf("unexpected client ID format: %q", a)
	}
}
