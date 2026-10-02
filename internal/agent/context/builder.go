package context

import (
	"errors"
	"slices"
	"strings"

	"nonoka-im/internal/agent/runtime"
	"nonoka-im/internal/data"
)

var ErrInputTooLarge = errors.New("agent input exceeds model budget")

type TokenCounter func(string) int

// EstimateTokens matches Eino v0.8.4's default byte-based estimate.
func EstimateTokens(s string) int { return (len(s) + 3) / 4 }

type Builder struct {
	Count         TokenCounter
	ContextWindow int
	OutputReserve int
	ToolReserve   int
	SafetyReserve int
	TriggerTokens int
}

func DefaultBuilder() Builder {
	return Builder{Count: EstimateTokens, ContextWindow: 65536, OutputReserve: 4096, ToolReserve: 8192, SafetyReserve: 4096, TriggerTokens: 38000}
}

func (b Builder) Build(instruction, current string, snapshot *data.AgentContextSnapshot, turns []data.AgentTurn) (runtime.Input, error) {
	if b.Count == nil {
		b.Count = EstimateTokens
	}
	if b.ContextWindow <= 0 {
		b = DefaultBuilder()
	}
	budget := b.ContextWindow - b.OutputReserve - b.ToolReserve - b.SafetyReserve
	currentCost := b.Count(current) + b.Count(instruction)
	if strings.TrimSpace(current) == "" || currentCost >= budget {
		return runtime.Input{}, ErrInputTooLarge
	}
	out := runtime.Input{Text: current, Instruction: instruction, TokenBudget: budget, OutputTokenBudget: b.OutputReserve, ToolResultTokenBudget: b.ToolReserve, TriggerTokens: b.TriggerTokens}
	used := currentCost
	needsSummary := false
	if snapshot != nil && snapshot.Summary != "" {
		cost := b.Count(snapshot.Summary)
		if used+cost < budget {
			out.Summary = snapshot.Summary
			used += cost
		}
	}
	for _, t := range slices.Backward(turns) {
		cost := b.Count(t.Input) + b.Count(t.Output)
		if used+cost >= budget {
			needsSummary = true
			continue
		}
		out.History = append(out.History, runtime.Turn{ID: t.ID, Input: t.Input, Output: t.Output})
		used += cost
	}
	for i, j := 0, len(out.History)-1; i < j; i, j = i+1, j-1 {
		out.History[i], out.History[j] = out.History[j], out.History[i]
	}
	if needsSummary {
		// Give Eino the complete retained history so its summarizer can preserve
		// facts from turns that do not fit the normal recent-turn budget.
		out.History = make([]runtime.Turn, 0, len(turns))
		for _, t := range turns {
			out.History = append(out.History, runtime.Turn{ID: t.ID, Input: t.Input, Output: t.Output})
		}
		out.NeedsSummary = true
	}
	if len(out.History) > 0 {
		out.SummaryThroughTurnID = out.History[len(out.History)-1].ID
	}
	if used > b.TriggerTokens {
		out.NeedsSummary = true
	}
	return out, nil
}

func (b Builder) Fallback(in runtime.Input) runtime.Input {
	if b.Count == nil {
		b.Count = EstimateTokens
	}
	in.DisableSummary = true
	in.NeedsSummary = false
	if len(in.History) > 4 {
		in.History = in.History[len(in.History)-4:]
	}
	used := b.Count(in.Instruction) + b.Count(in.Text) + b.Count(in.Summary)
	for _, turn := range in.History {
		used += b.Count(turn.Input) + b.Count(turn.Output)
	}
	for used >= in.TokenBudget && len(in.History) > 0 {
		used -= b.Count(in.History[0].Input) + b.Count(in.History[0].Output)
		in.History = in.History[1:]
	}
	return in
}
