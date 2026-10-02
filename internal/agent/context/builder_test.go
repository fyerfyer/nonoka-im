package context

import (
	"errors"
	"testing"

	"nonoka-im/internal/data"
)

func TestBuildBudgetAndSessionSnapshot(t *testing.T) {
	b := Builder{Count: func(s string) int { return len(s) }, ContextWindow: 80, OutputReserve: 10, ToolReserve: 10, SafetyReserve: 10, TriggerTokens: 20}
	turns := []data.AgentTurn{{ID: 1, Input: "older", Output: "reply"}, {ID: 2, Input: "recent", Output: "answer"}}
	in, err := b.Build("system", "current", &data.AgentContextSnapshot{Summary: "prior fact"}, turns)
	if err != nil {
		t.Fatal(err)
	}
	if in.Text != "current" || in.Instruction != "system" || in.Summary != "prior fact" || len(in.History) != 2 || !in.NeedsSummary || in.SummaryThroughTurnID != 2 {
		t.Fatalf("unexpected context: %+v", in)
	}
	if _, err := b.Build("system", "", nil, turns); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("empty input must fail: %v", err)
	}
	other, err := b.Build("system", "other", nil, nil)
	if err != nil || other.Summary != "" || len(other.History) != 0 {
		t.Fatalf("context crossed session boundary: %+v err=%v", other, err)
	}
}

func TestFallbackKeepsCurrentInputWithinBudget(t *testing.T) {
	b := Builder{Count: func(s string) int { return len(s) }}
	in, err := (Builder{Count: b.Count, ContextWindow: 50, OutputReserve: 10, ToolReserve: 10, SafetyReserve: 5, TriggerTokens: 5}).Build("sys", "current", nil, []data.AgentTurn{{ID: 1, Input: "older", Output: "reply"}, {ID: 2, Input: "recent", Output: "answer"}})
	if err != nil {
		t.Fatal(err)
	}
	out := b.Fallback(in)
	if out.Text != "current" || out.NeedsSummary || !out.DisableSummary {
		t.Fatalf("invalid fallback: %+v", out)
	}
}
