package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	agevent "nonoka-im/internal/agent/event"
	"nonoka-im/internal/agent/runtime"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
)

type authorizerStub struct {
	denied int64
	calls  []int64
}

func (a *authorizerStub) CanAccessTopic(_ context.Context, uid int64, _ string) error {
	a.calls = append(a.calls, uid)
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
		err := h.HandleResolved(context.Background(), agevent.MessageEvent{SenderID: 99, Topic: "p2p_1_99"}, ResolvedAgent{ID: 7, BotID: 99})
		if err != nil || len(az.calls) != 0 {
			t.Fatalf("err=%v auth calls=%v", err, az.calls)
		}
	})
	t.Run("group topic requires actor and bot access", func(t *testing.T) {
		az := &authorizerStub{denied: 99}
		h := &Harness{Authorizer: az}
		err := h.HandleResolved(context.Background(), agevent.MessageEvent{SenderID: 1, Topic: "grp_group", MentionedAgentIDs: []int64{99}}, ResolvedAgent{ID: 7, BotID: 99})
		if err == nil {
			t.Fatal("expected bot membership denial")
		}
		if len(az.calls) != 2 || az.calls[0] != 1 || az.calls[1] != 99 {
			t.Fatalf("unexpected auth calls: %v", az.calls)
		}
	})
	t.Run("missing dependencies reported", func(t *testing.T) {
		h := &Harness{Authorizer: &authorizerStub{}}
		err := h.HandleResolved(context.Background(), agevent.MessageEvent{SenderID: 1, Topic: "p2p_1_99"}, ResolvedAgent{ID: 7, BotID: 99})
		if err == nil || err.Error() != "agent harness dependencies unavailable" {
			t.Fatalf("unexpected err: %v", err)
		}
	})
}

// setupHarnessStore connects to a real PostgreSQL for harness behavior tests.
// Set NONOKA_AGENT_TEST_PG_DSN to override; skipped when no PostgreSQL is
// reachable (the integration suite covers the live runs).
func setupHarnessStore(t *testing.T) *data.AgentStore {
	t.Helper()
	dsns := []string{os.Getenv("NONOKA_AGENT_TEST_PG_DSN")}
	if dsns[0] == "" {
		dsns = dsns[:0]
	}
	dsns = append(dsns,
		"host=127.0.0.1 user=postgres password=root dbname=nonoka_im port=5432 sslmode=disable",
		"host=127.0.0.1 user=nonoka password=nonoka dbname=nonoka port=5432 sslmode=disable",
	)
	for _, dsn := range dsns {
		d, cleanup, err := data.NewData(&conf.Data{Database: &conf.Data_Database{Source: dsn}})
		if err != nil {
			continue
		}
		t.Cleanup(cleanup)
		return data.NewAgentStore(d)
	}
	t.Skip("PostgreSQL not reachable; set NONOKA_AGENT_TEST_PG_DSN")
	return nil
}

type failingRuntime struct{ err error }

func (f failingRuntime) Run(context.Context, runtime.Input) (runtime.Result, error) {
	return runtime.Result{}, f.err
}

func (f failingRuntime) Resume(context.Context, runtime.ResumeInput) (runtime.Result, error) {
	return runtime.Result{}, f.err
}

type stubRuntime struct{ result runtime.Result }

func (s stubRuntime) Run(context.Context, runtime.Input) (runtime.Result, error) {
	return s.result, nil
}

func (s stubRuntime) Resume(context.Context, runtime.ResumeInput) (runtime.Result, error) {
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

// findRunByEvent returns the newest run for the agent with the given event ID.
func findRunByEvent(t *testing.T, store *data.AgentStore, agentID int64, eventID string) *data.AgentRun {
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

// turnsForRun filters RecentTurns down to the given run so leftovers from
// earlier manual runs in the shared dev database do not affect assertions.
func turnsForRun(t *testing.T, store *data.AgentStore, runID uint64, agentID, actorID int64, topic string) []data.AgentTurn {
	t.Helper()
	rows, err := store.RecentTurns(context.Background(), agentID, actorID, topic, 50)
	if err != nil {
		t.Fatalf("RecentTurns: %v", err)
	}
	out := make([]data.AgentTurn, 0, 1)
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
	// Unique topic per test run isolates the session from leftovers of
	// interrupted runs in the shared development database.
	topic := fmt.Sprintf("p2p_%d_%d_%d", actorID, botID, time.Now().UnixNano())
	eventID := fmt.Sprintf("evt-fail-%d", time.Now().UnixNano())

	h := &Harness{
		Store:      store,
		Runtime:    failingRuntime{err: errors.New("model exploded")},
		Authorizer: &authorizerStub{},
		Sender:     &recordingSender{receiptID: 4242},
	}
	ev := agevent.MessageEvent{SchemaVersion: agevent.SchemaVersion, EventID: eventID, MsgID: 1001, TopicSeq: 100, Topic: topic, SenderID: actorID, Content: "hello agent"}
	err := h.HandleResolved(ctx, ev, ResolvedAgent{ID: agentID, BotID: botID, Version: 1, Instruction: "be helpful", Model: "deepseek-chat"})
	if err == nil || err.Error() != "model exploded" {
		t.Fatalf("expected original runtime error, got %v", err)
	}

	run := findRunByEvent(t, store, agentID, eventID)
	if run.Status != "failed" {
		t.Fatalf("run must be failed after phase B error, got %q", run.Status)
	}
	if turns := turnsForRun(t, store, run.ID, agentID, actorID, topic); len(turns) != 0 {
		t.Fatalf("no turn must be written on phase B failure, got %d", len(turns))
	}
}

func TestHandleResolvedCompletesRunAndTurn(t *testing.T) {
	store := setupHarnessStore(t)
	ctx := context.Background()
	agentID, botID, actorID := int64(987000021), int64(987000022), int64(987000023)
	topic := fmt.Sprintf("p2p_%d_%d_%d", actorID, botID, time.Now().UnixNano())
	eventID := fmt.Sprintf("evt-ok-%d", time.Now().UnixNano())

	sender := &recordingSender{receiptID: 5150}
	h := &Harness{
		Store:      store,
		Runtime:    stubRuntime{result: runtime.Result{Text: "hi there", Usage: runtime.Usage{InputTokens: 3, OutputTokens: 5, TotalTokens: 8}}},
		Authorizer: &authorizerStub{},
		Sender:     sender,
	}
	ev := agevent.MessageEvent{SchemaVersion: agevent.SchemaVersion, EventID: eventID, MsgID: 2002, TopicSeq: 0, Topic: topic, SenderID: actorID, Content: "hello agent"}
	if err := h.HandleResolved(ctx, ev, ResolvedAgent{ID: agentID, BotID: botID, Version: 1, Instruction: "be helpful", Model: "deepseek-chat"}); err != nil {
		t.Fatalf("HandleResolved: %v", err)
	}
	if len(sender.sent) != 1 || sender.sent[0] != "hi there" {
		t.Fatalf("reply not sent: %v", sender.sent)
	}
	run := findRunByEvent(t, store, agentID, eventID)
	if run.Status != "completed" || run.ReplyMsgID != 5150 {
		t.Fatalf("unexpected run state: %+v", run)
	}
	turns := turnsForRun(t, store, run.ID, agentID, actorID, topic)
	if len(turns) != 1 || turns[0].Output != "hi there" || turns[0].ReplyMsgID != 5150 {
		t.Fatalf("unexpected turns: %+v", turns)
	}
}

// fakeRunStore stubs the harness persistence for retry tests: CompleteRun
// fails completeFails times before succeeding and records committed turns.
type fakeRunStore struct {
	prepared      *data.PreparedRun
	completeCalls int
	completeFails int
	turns         []data.AgentTurn
}

func (f *fakeRunStore) PrepareRun(context.Context, int64, int64, string, *data.AgentRun, uint64) (*data.PreparedRun, error) {
	return f.prepared, nil
}
func (f *fakeRunStore) UpdateRun(context.Context, uint64, map[string]interface{}) error { return nil }
func (f *fakeRunStore) CreateApproval(_ context.Context, row *data.AgentApproval) error {
	row.ID = 1
	return nil
}
func (f *fakeRunStore) CompleteRun(_ context.Context, _, _ int64, _ string, turn *data.AgentTurn, _ *data.AgentContextSnapshot, _ uint64, _ map[string]interface{}) error {
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
	store := &fakeRunStore{
		prepared:      &data.PreparedRun{Claimed: true, Run: &data.AgentRun{ID: 11}, Session: &data.AgentSession{ID: 22}},
		completeFails: 2,
	}
	sender := &recordingSender{receiptID: 77}
	h := &Harness{Store: store, Runtime: stubRuntime{result: runtime.Result{Text: "ok"}}, Authorizer: &authorizerStub{}, Sender: sender}
	ev := agevent.MessageEvent{SchemaVersion: agevent.SchemaVersion, EventID: "evt-retry", MsgID: 1, Topic: "p2p_1_2", SenderID: 1, Content: "hi"}
	if err := h.HandleResolved(context.Background(), ev, ResolvedAgent{ID: 7, BotID: 2, Version: 1}); err != nil {
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
	store := &fakeRunStore{
		prepared:      &data.PreparedRun{Claimed: true, Run: &data.AgentRun{ID: 11}, Session: &data.AgentSession{ID: 22}},
		completeFails: 99,
	}
	h := &Harness{Store: store, Runtime: stubRuntime{result: runtime.Result{Text: "ok"}}, Authorizer: &authorizerStub{}, Sender: &recordingSender{}}
	ev := agevent.MessageEvent{SchemaVersion: agevent.SchemaVersion, EventID: "evt-giveup", MsgID: 1, Topic: "p2p_1_2", SenderID: 1, Content: "hi"}
	if err := h.HandleResolved(context.Background(), ev, ResolvedAgent{ID: 7, BotID: 2, Version: 1}); err == nil {
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
