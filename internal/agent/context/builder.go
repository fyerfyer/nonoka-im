package context

import (
	"errors"
	"strings"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
)

var ErrEmptyInput = errors.New("agent input is empty")

// EstimateTokens is a coarse preflight/display estimate. Context compaction
// is owned by Eino's summarization middleware; this helper is only used for
// the stored summary-token metric and a minimal empty-input guard.
func EstimateTokens(s string) int { return (len(s) + 3) / 4 }

// Builder assembles the durable session state into the runtime input. Budget
// trimming and summarization are delegated to Eino's summarization middleware.
type Builder struct{}

func (b Builder) Build(instruction, current string, snapshot *domain.AgentContextSnapshot, turns []domain.AgentTurn) (port.Input, error) {
	if strings.TrimSpace(current) == "" {
		return port.Input{}, ErrEmptyInput
	}
	out := port.Input{Text: current, Instruction: instruction}
	if snapshot != nil {
		out.Summary = snapshot.Summary
	}
	out.History = make([]port.Turn, 0, len(turns))
	for _, turn := range turns {
		out.History = append(out.History, port.Turn{ID: turn.ID, Input: turn.Input, Output: turn.Output})
	}
	if len(out.History) > 0 {
		out.SummaryThroughTurnID = out.History[len(out.History)-1].ID
	}
	return out, nil
}

// Fallback is used only after Eino's summarization middleware fails. It keeps
// the most recent turns and disables another summary attempt for this run.
func (b Builder) Fallback(in port.Input) port.Input {
	in.DisableSummary = true
	if len(in.History) > 4 {
		in.History = in.History[len(in.History)-4:]
	}
	return in
}
