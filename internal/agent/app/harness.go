package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	agentcontext "nonoka-im/internal/agent/context"
	"nonoka-im/internal/agent/eino"
	agevent "nonoka-im/internal/agent/event"
	"nonoka-im/internal/agent/runtime"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/data"
)

type ReplySender interface {
	Send(context.Context, string, string, string) error
}
type Harness struct {
	Store      *data.AgentStore
	Runtime    runtime.Runtime
	Authorizer authz.Authorizer
	Sender     ReplySender
	Builder    agentcontext.Builder
}

type ResolvedAgent struct {
	ID            int64
	BotID         int64
	Version       int64
	Instruction   string
	Model         string
	Tools         []string
	RestrictTools bool
}

func (h *Harness) HandleResolved(ctx context.Context, ev agevent.MessageEvent, agent ResolvedAgent) error {
	ctx, span := otel.Tracer("nonoka-im/agent/harness").Start(ctx, "agent.handle_event")
	defer span.End()
	if ev.SenderID == agent.BotID {
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
	if err := h.Authorizer.CanAccessTopic(ctx, agent.BotID, ev.Topic); err != nil {
		authSpan.End()
		return err
	}
	authSpan.End()
	if h.Store == nil || h.Runtime == nil || h.Sender == nil {
		return fmt.Errorf("agent harness dependencies unavailable")
	}
	builder := h.Builder
	if builder.ContextWindow == 0 {
		builder = agentcontext.DefaultBuilder()
	}
	// Phase A (short transaction, session lock held): claim the run and load
	// the conversation state.
	prepared, err := h.Store.PrepareRun(ctx, agent.ID, ev.SenderID, ev.Topic, &data.AgentRun{EventID: ev.EventID, AgentID: agent.ID, AgentVersion: agent.Version, ActorUserID: ev.SenderID, Topic: ev.Topic, MsgID: ev.MsgID, TopicSeq: ev.TopicSeq, Status: "running", Attempts: 1, CreatedAt: time.Now()}, ev.TopicSeq)
	if err != nil {
		return err
	}
	if !prepared.Claimed || prepared.Queued {
		return nil
	}
	run, session := prepared.Run, prepared.Session

	// Terminal state writes must survive cancellation of the handler context
	// (the Kafka handler deadline may fire right after the model call), so
	// they run on a detached context with their own timeout.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer finishCancel()

	var runErr error
	// fail records the original error, marks the run failed on a detached
	// context, and returns the state-update error only when that write fails.
	fail := func(err error) error {
		runErr = err
		if uerr := h.Store.UpdateRun(finishCtx, run.ID, map[string]interface{}{"status": "failed", "error_code": classifyRunError(err), "finished_at": time.Now()}); uerr != nil {
			return uerr
		}
		return nil
	}
	failRun := func(err error) error {
		if ferr := fail(err); ferr != nil {
			return ferr
		}
		return runErr
	}

	// Phase B (no lock, no transaction): build the prompt, run the model and
	// submit the reply.
	in, err := builder.Build(agent.Instruction, ev.Content, prepared.Snapshot, prepared.Turns)
	if err != nil {
		return failRun(err)
	}
	in.EventID, in.AgentID, in.ActorUserID, in.BotID, in.Topic = ev.EventID, agent.ID, ev.SenderID, agent.BotID, ev.Topic
	in.Model = agent.Model
	in.AllowedTools, in.RestrictTools = agent.Tools, agent.RestrictTools
	res, err := h.Runtime.Run(ctx, in)
	if errors.Is(err, eino.ErrSummary) {
		res, err = h.Runtime.Run(ctx, builder.Fallback(in))
	}
	if err != nil {
		return failRun(err)
	}
	if strings.TrimSpace(res.Text) == "" {
		return failRun(fmt.Errorf("empty response"))
	}
	clientID := stableClientID(ev.EventID, agent.ID)
	sendCtx := tools.WithRunContext(ctx, tools.RunContext{AgentID: agent.ID, BotID: agent.BotID, ActorUserID: ev.SenderID, Topic: ev.Topic, EventID: ev.EventID})
	sendCtx, sendCancel := context.WithTimeout(context.WithoutCancel(sendCtx), 10*time.Second)
	defer sendCancel()
	var replyMsgID int64
	if sender, ok := h.Sender.(interface {
		SendWithReceipt(context.Context, string, string, string) (int64, error)
	}); ok {
		replyMsgID, err = sender.SendWithReceipt(sendCtx, ev.Topic, res.Text, clientID)
	} else {
		err = h.Sender.Send(sendCtx, ev.Topic, res.Text, clientID)
	}
	if err != nil {
		return failRun(err)
	}

	// Phase C (short transaction, session lock re-acquired): commit the turn,
	// snapshot and terminal run state together.
	var snapshot *data.AgentContextSnapshot
	if res.Summary != "" {
		snapshot = &data.AgentContextSnapshot{SessionID: session.ID, LastTurnID: res.SummaryThroughTurnID, AgentVersion: agent.Version, Summary: res.Summary, SummaryTokens: agentcontext.EstimateTokens(res.Summary)}
	}
	completeErr := h.Store.CompleteRun(finishCtx, agent.ID, ev.SenderID, ev.Topic,
		&data.AgentTurn{AgentID: agent.ID, ActorUserID: ev.SenderID, Topic: ev.Topic, SessionID: session.ID, RunID: run.ID, TriggerMsgID: ev.MsgID, ReplyMsgID: replyMsgID, Input: ev.Content, Output: res.Text, CreatedAt: time.Now()},
		snapshot, run.ID,
		map[string]interface{}{"status": "completed", "reply_msg_id": replyMsgID, "input_tokens": res.Usage.InputTokens, "output_tokens": res.Usage.OutputTokens, "total_tokens": res.Usage.TotalTokens, "finished_at": time.Now()})
	if completeErr != nil {
		return completeErr
	}
	return runErr
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
func stableClientID(eventID string, agentID int64) string {
	h := sha256.Sum256(fmt.Appendf(nil, "%s:%d", eventID, agentID))
	return "agent-" + hex.EncodeToString(h[:16])
}

type HTTPSender struct {
	BaseURL, Token string
	Client         *http.Client
	TokenIssuer    func(agentID, botID int64, topic string) (string, error)
}

func (s *HTTPSender) Send(ctx context.Context, topic, content, clientID string) error {
	_, err := s.SendWithReceipt(ctx, topic, content, clientID)
	return err
}

type sendReplyRequest struct {
	Topic       string `json:"topic"`
	ClientMsgID string `json:"client_msg_id"`
	MsgType     int32  `json:"msg_type"`
	// Content is base64 because the proto field is bytes and the HTTP codec
	// unmarshals JSON via protojson, which expects base64 for bytes.
	Content string `json:"content"`
}

func (s *HTTPSender) SendWithReceipt(ctx context.Context, topic, content, clientID string) (int64, error) {
	// json.Marshal keeps the payload valid even when the model output
	// contains quotes or invalid UTF-8.
	body, err := json.Marshal(sendReplyRequest{Topic: topic, ClientMsgID: clientID, MsgType: 0, Content: base64.StdEncoding.EncodeToString([]byte(content))})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.BaseURL, "/")+"/v1/message/send", strings.NewReader(string(body)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	token := s.Token
	if s.TokenIssuer != nil {
		rc, ok := tools.GetRunContext(ctx)
		if !ok {
			return 0, fmt.Errorf("run context missing for bot credential")
		}
		token, err = s.TokenIssuer(rc.AgentID, rc.BotID, topic)
		if err != nil {
			return 0, err
		}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("send reply status %s", resp.Status)
	}
	var ack struct {
		// The HTTP codec marshals proto int64 as a JSON string.
		MsgID int64 `json:"msgId,string"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		return 0, err
	}
	return ack.MsgID, nil
}
