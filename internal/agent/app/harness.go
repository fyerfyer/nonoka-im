package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	agevent "nonoka-im/internal/agent/event"
	"nonoka-im/internal/agent/runtime"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/data"
	"nonoka-im/internal/topic"
)

type ReplySender interface {
	Send(context.Context, string, string, string) error
}
type Harness struct {
	AgentID         int64
	BotID           int64
	Store           *data.AgentStore
	Runtime         runtime.Runtime
	Authorizer      authz.Authorizer
	Sender          ReplySender
	HistoryLimit    int
	MaxContextChars int
}

func (h *Harness) Handle(ctx context.Context, ev agevent.MessageEvent) error {
	ctx, span := otel.Tracer("nonoka-im/agent/harness").Start(ctx, "agent.handle_event")
	defer span.End()
	if !h.ShouldHandle(ev) {
		return nil
	}
	if h.Authorizer == nil {
		return fmt.Errorf("topic authorization unavailable")
	}
	_, authSpan := otel.Tracer("nonoka-im/agent/harness").Start(ctx, "agent.authorize_topic")
	if err := h.Authorizer.CanAccessTopic(ctx, ev.SenderID, ev.Topic); err != nil {
		authSpan.End()
		return err
	}
	if err := h.Authorizer.CanAccessTopic(ctx, h.BotID, ev.Topic); err != nil {
		authSpan.End()
		return err
	}
	authSpan.End()
	if h.Store == nil || h.Runtime == nil || h.Sender == nil {
		return fmt.Errorf("agent harness dependencies unavailable")
	}
	run, claimed, err := h.Store.ClaimRun(ctx, ev.EventID, &data.AgentRun{EventID: ev.EventID, AgentID: h.AgentID, ActorUserID: ev.SenderID, Topic: ev.Topic, MsgID: ev.MsgID, Status: "running", Attempts: 1, CreatedAt: time.Now()})
	if err != nil {
		return err
	}
	if !claimed && (run.Status == "completed" || run.Status == "running") {
		return nil
	}
	if err := h.Store.TouchSession(ctx, h.AgentID, ev.SenderID, ev.Topic); err != nil {
		return h.fail(ctx, run, err)
	}
	start := time.Now()
	_ = h.Store.UpdateRun(ctx, run.ID, map[string]interface{}{"status": "running", "started_at": start})
	limit := h.HistoryLimit
	if limit <= 0 {
		limit = 8
	}
	_, contextSpan := otel.Tracer("nonoka-im/agent/harness").Start(ctx, "agent.load_context")
	rows, err := h.Store.RecentTurns(ctx, h.AgentID, ev.SenderID, ev.Topic, limit)
	contextSpan.End()
	if err != nil {
		return h.fail(ctx, run, err)
	}
	hist := make([]runtime.Turn, 0, len(rows))
	budget := h.MaxContextChars
	if budget <= 0 {
		budget = 12000
	}
	currentText := truncateRunText(ev.Content, budget)
	used := len(currentText)
	for _, row := range slices.Backward(rows) {
		cost := len(row.Input) + len(row.Output)
		if used+cost > budget {
			break
		}
		hist = append(hist, runtime.Turn{Input: row.Input, Output: row.Output})
		used += cost
	}
	res, err := h.Runtime.Run(ctx, runtime.Input{Text: currentText, EventID: ev.EventID, History: hist, AgentID: h.AgentID, ActorUserID: ev.SenderID, BotID: h.BotID, Topic: ev.Topic})
	if err != nil {
		return h.fail(ctx, run, err)
	}
	if strings.TrimSpace(res.Text) == "" {
		return h.fail(ctx, run, fmt.Errorf("empty response"))
	}
	clientID := stableClientID(ev.EventID, h.AgentID)
	_, replySpan := otel.Tracer("nonoka-im/agent/harness").Start(ctx, "agent.submit_reply")
	if err := h.Sender.Send(ctx, ev.Topic, res.Text, clientID); err != nil {
		replySpan.End()
		return h.fail(ctx, run, err)
	}
	replySpan.End()
	if err := h.Store.AddTurn(ctx, &data.AgentTurn{AgentID: h.AgentID, ActorUserID: ev.SenderID, Topic: ev.Topic, RunID: run.ID, Input: ev.Content, Output: res.Text, CreatedAt: time.Now()}); err != nil {
		return err
	}
	return h.Store.UpdateRun(ctx, run.ID, map[string]interface{}{"status": "completed", "finished_at": time.Now()})
}

// ShouldHandle performs the side-effect-free trigger check before the worker
// loads message content from MongoDB.
func (h *Harness) ShouldHandle(ev agevent.MessageEvent) bool {
	if ev.SenderID == h.BotID {
		return false
	}
	switch topic.ParseType(ev.Topic) {
	case topic.TypeGroup:
		return contains(ev.MentionedAgentIDs, h.BotID)
	case topic.TypeP2P:
		a, b, err := topic.ExtractUserIDsFromP2PTopic(ev.Topic)
		return err == nil && (a == h.BotID || b == h.BotID)
	default:
		return false
	}
}
func (h *Harness) fail(ctx context.Context, run *data.AgentRun, err error) error {
	_ = h.Store.UpdateRun(ctx, run.ID, map[string]interface{}{"status": "failed", "error_code": classifyRunError(err), "finished_at": time.Now()})
	return err
}
func classifyRunError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "run_failed"
}
func contains(xs []int64, v int64) bool {
	return slices.Contains(xs, v)
}
func truncateRunText(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) > maxChars {
		return string(runes[:maxChars])
	}
	return s
}
func stableClientID(eventID string, agentID int64) string {
	h := sha256.Sum256(fmt.Appendf(nil, "%s:%d", eventID, agentID))
	return "agent-" + hex.EncodeToString(h[:16])
}

type HTTPSender struct {
	BaseURL, Token string
	Client         *http.Client
}

func (s *HTTPSender) Send(ctx context.Context, topic, content, clientID string) error {
	body := fmt.Sprintf(`{"topic":%q,"client_msg_id":%q,"msg_type":0,"content":%q}`, topic, clientID, content)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.BaseURL, "/")+"/v1/message/send", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.Token)
	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("send reply status %s", resp.Status)
	}
	return nil
}
