package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"nonoka-im/internal/agent/domain"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type countingTool struct {
	calls  int
	result string
	err    error
}

func (t *countingTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "send_message"}, nil
}

func (t *countingTool) InvokableRun(context.Context, string, ...tool.Option) (string, error) {
	t.calls++
	return t.result, t.err
}

type ledgerStoreStub struct {
	rows     map[string]*domain.ToolInvocation
	marked   int
	complete int
	failed   int
}

func newLedgerStoreStub() *ledgerStoreStub {
	return &ledgerStoreStub{rows: map[string]*domain.ToolInvocation{}}
}

func ledgerKey(runID uint64, toolCallID string) string {
	return fmt.Sprintf("%d\x00%s", runID, toolCallID)
}

func (s *ledgerStoreStub) BeginToolInvocation(_ context.Context, row *domain.ToolInvocation) (*domain.ToolInvocation, bool, error) {
	key := ledgerKey(row.RunID, row.ToolCallID)
	if existing, ok := s.rows[key]; ok {
		copied := *existing
		return &copied, false, nil
	}
	copied := *row
	s.rows[key] = &copied
	return &copied, true, nil
}

func (s *ledgerStoreStub) CompleteToolInvocation(_ context.Context, runID uint64, toolCallID, result string) error {
	row := s.rows[ledgerKey(runID, toolCallID)]
	if row == nil {
		return errors.New("missing")
	}
	row.Status = domain.ToolInvocationCompleted
	row.ResultSummary = result
	s.complete++
	return nil
}

func (s *ledgerStoreStub) FailToolInvocation(_ context.Context, runID uint64, toolCallID, code string) error {
	row := s.rows[ledgerKey(runID, toolCallID)]
	if row == nil {
		return errors.New("missing")
	}
	row.Status = domain.ToolInvocationFailed
	row.ErrorCode = code
	s.failed++
	return nil
}

func TestLedgerToolExecutesOncePerToolCall(t *testing.T) {
	store := newLedgerStoreStub()
	inner := &countingTool{result: `{"accepted":true}`}
	mark := func(context.Context, RunContext) error { store.marked++; return nil }
	wrapped := NewLedgerTool(inner, store, mark)
	ctx := WithRunContext(context.Background(), RunContext{RunID: 9, ToolCallID: "call-1", EventID: "evt-1", Topic: "p2p_1_2"})

	first, err := wrapped.InvokableRun(ctx, `{"content":"hi"}`)
	if err != nil || first != `{"accepted":true}` {
		t.Fatalf("first run: result=%q err=%v", first, err)
	}
	second, err := wrapped.InvokableRun(ctx, `{"content":"hi"}`)
	if err != nil || second != first {
		t.Fatalf("replay: result=%q err=%v", second, err)
	}
	if inner.calls != 1 {
		t.Fatalf("tool executed %d times, want 1", inner.calls)
	}
	if store.marked != 1 || store.complete != 1 {
		t.Fatalf("mark=%d complete=%d", store.marked, store.complete)
	}
}

func TestLedgerToolFailsClosedWhenOutcomeUnknown(t *testing.T) {
	store := newLedgerStoreStub()
	store.rows[ledgerKey(9, "call-1")] = &domain.ToolInvocation{
		RunID: 9, ToolCallID: "call-1", Status: domain.ToolInvocationStarted,
	}
	inner := &countingTool{result: "ok"}
	wrapped := NewLedgerTool(inner, store, nil)
	ctx := WithRunContext(context.Background(), RunContext{RunID: 9, ToolCallID: "call-1"})
	if _, err := wrapped.InvokableRun(ctx, `{}`); err == nil {
		t.Fatal("started call must fail closed")
	}
	if inner.calls != 0 {
		t.Fatalf("tool executed %d times, want 0", inner.calls)
	}
}

func TestLedgerToolReplaysCachedFailure(t *testing.T) {
	store := newLedgerStoreStub()
	store.rows[ledgerKey(9, "call-1")] = &domain.ToolInvocation{
		RunID: 9, ToolCallID: "call-1", Status: domain.ToolInvocationFailed, ErrorCode: "tool_error",
	}
	inner := &countingTool{result: "ok"}
	wrapped := NewLedgerTool(inner, store, nil)
	ctx := WithRunContext(context.Background(), RunContext{RunID: 9, ToolCallID: "call-1"})
	if _, err := wrapped.InvokableRun(ctx, `{}`); err == nil {
		t.Fatal("failed call must not execute again")
	}
	if inner.calls != 0 {
		t.Fatalf("tool executed %d times, want 0", inner.calls)
	}
}
