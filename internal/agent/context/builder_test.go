package context

import (
	"testing"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
)

func TestBuildKeepsFullHistoryAndSnapshot(t *testing.T) {
	b := Builder{}
	turns := []domain.AgentTurn{
		{ID: 1, Input: "older", Output: "reply"},
		{ID: 2, Input: "recent", Output: "answer"},
	}
	in, err := b.Build("sys", "current", &domain.AgentContextSnapshot{LastTurnID: 0, Summary: "prior fact"}, turns)
	if err != nil {
		t.Fatal(err)
	}
	if in.Text != "current" || in.Instruction != "sys" || in.Summary != "prior fact" || len(in.History) != 2 || in.SummaryThroughTurnID != 2 {
		t.Fatalf("unexpected input: %+v", in)
	}
}

func TestFallbackKeepsRecentTurnsAndDisablesSummary(t *testing.T) {
	b := Builder{}
	in := b.Fallback(portInputWithTurns(6))
	if len(in.History) != 4 || !in.DisableSummary {
		t.Fatalf("fallback: %+v", in)
	}
}

func TestBuildRejectsEmptyInput(t *testing.T) {
	b := Builder{}
	if _, err := b.Build("sys", "  ", nil, nil); err == nil {
		t.Fatal("empty input accepted")
	}
}

func portInputWithTurns(n int) (out port.Input) {
	for i := 1; i <= n; i++ {
		out.History = append(out.History, port.Turn{ID: uint64(i), Input: "q", Output: "a"})
	}
	return out
}
