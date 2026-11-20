package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"unicode/utf8"

	"nonoka-im/internal/agent/domain"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// ToolInvocationStore is the durable ledger for side-effecting tool calls. The
// data layer implements it; the wrapper only depends on this narrow interface.
type ToolInvocationStore interface {
	BeginToolInvocation(ctx context.Context, row *domain.ToolInvocation) (*domain.ToolInvocation, bool, error)
	CompleteToolInvocation(ctx context.Context, runID uint64, toolCallID, resultSummary string) error
	FailToolInvocation(ctx context.Context, runID uint64, toolCallID, errorCode string) error
}

// SideEffectMarker records on the run that an external effect already happened.
type SideEffectMarker func(ctx context.Context, rc RunContext) error

// ledgerTool makes one side-effecting tool call at-most-once for a given
// (run, tool call) pair. A completed call returns its cached result; a started
// call fails closed because its outcome is unknown; a failed call returns the
// cached failure.
type ledgerTool struct {
	wrapped tool.InvokableTool
	store   ToolInvocationStore
	mark    SideEffectMarker
}

// NewLedgerTool wraps a side-effecting tool. A nil store leaves the tool
// unwrapped (only safe in tests).
func NewLedgerTool(wrapped tool.InvokableTool, store ToolInvocationStore, mark SideEffectMarker) tool.InvokableTool {
	if store == nil {
		return wrapped
	}
	return &ledgerTool{wrapped: wrapped, store: store, mark: mark}
}

func (t *ledgerTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.wrapped.Info(ctx)
}

func (t *ledgerTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	rc, ok := GetRunContext(ctx)
	if !ok {
		return "", fmt.Errorf("run context missing")
	}
	callID := compose.GetToolCallID(ctx)
	if callID == "" {
		callID = rc.ToolCallID
	}
	if callID == "" {
		return "", fmt.Errorf("tool call id missing")
	}
	rc.ToolCallID = callID
	ctx = WithRunContext(ctx, rc)

	name := "tool"
	if info, err := t.wrapped.Info(ctx); err == nil && info != nil {
		name = info.Name
	}
	sum := sha256.Sum256([]byte(argumentsInJSON))
	row, created, err := t.store.BeginToolInvocation(ctx, &domain.ToolInvocation{
		RunID: rc.RunID, ToolCallID: callID, Tool: name,
		ArgsHash: hex.EncodeToString(sum[:]), Status: domain.ToolInvocationStarted,
	})
	if err != nil {
		return "", err
	}
	if !created {
		switch row.Status {
		case domain.ToolInvocationCompleted:
			return row.ResultSummary, nil
		case domain.ToolInvocationFailed:
			return "", fmt.Errorf("tool call previously failed (%s); not retrying automatically", row.ErrorCode)
		default:
			return "", fmt.Errorf("tool call outcome is unknown; manual retry required")
		}
	}

	result, err := t.wrapped.InvokableRun(ctx, argumentsInJSON, opts...)
	if err != nil {
		_ = t.store.FailToolInvocation(ctx, rc.RunID, callID, "tool_error")
		return result, err
	}
	if err := t.store.CompleteToolInvocation(ctx, rc.RunID, callID, boundResult(result)); err != nil {
		return "", err
	}
	if t.mark != nil {
		_ = t.mark(ctx, rc)
	}
	return result, nil
}

const maxLedgerResult = 4096

func boundResult(s string) string {
	if len(s) <= maxLedgerResult {
		return s
	}
	s = s[:maxLedgerResult]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "…"
}
