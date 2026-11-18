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

// RunStore is the persistence surface the harness needs; *data.AgentStore
// implements it. An interface keeps HandleResolved testable without a database.
type RunStore interface {
	PrepareRun(ctx context.Context, agentID, actorID int64, topic string, candidate *data.AgentRun, topicSeq uint64) (*data.PreparedRun, error)
	UpdateRun(ctx context.Context, id uint64, values map[string]interface{}) error
	CompleteRun(ctx context.Context, agentID, actorID int64, topic string, turn *data.AgentTurn, snapshot *data.AgentContextSnapshot, runID uint64, values map[string]interface{}) error
	CreateApproval(ctx context.Context, row *data.AgentApproval) error
}

type Harness struct {
	Store      RunStore
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
	BaseURL       string
	APIKey        string
	MaxTokens     int
	Tools         []string
	MCPTools      []runtime.MCPTool
	Skills        []runtime.SkillBinding
	RestrictTools bool
	// ApprovalTools are tool names whose calls interrupt for human
	// confirmation (write-class MCP tools and future side-effect tools).
	ApprovalTools []string
}

// approvalTTL bounds how long a pending approval waits for the user.
const approvalTTL = 30 * time.Minute

// cardSender is implemented by senders that can post the agent approval card
// message type.
type cardSender interface {
	SendCard(context.Context, string, string, string) (int64, error)
}

// cardPayload is the versioned JSON body of MSG_TYPE_AGENT_CARD. It carries
// only a human-readable summary; raw tool arguments never leave the
// checkpoint/approval record.
type cardPayload struct {
	Version     int    `json:"version"`
	RunID       uint64 `json:"run_id"`
	ApprovalID  uint64 `json:"approval_id"`
	Tool        string `json:"tool"`
	Summary     string `json:"summary"`
	Target      string `json:"target"`
	Status      string `json:"status"`
	RequestedBy int64  `json:"requested_by"`
	ExpiresAt   int64  `json:"expires_at"` // Unix seconds
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

	var runErr error
	// fail records the original error, marks the run failed on a detached
	// context, and returns the state-update error only when that write fails.
	// The detached context is created per call: a model call longer than the
	// window must not poison the terminal write.
	fail := func(err error) error {
		runErr = err
		fctx, fcancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer fcancel()
		if uerr := h.Store.UpdateRun(fctx, run.ID, map[string]interface{}{"status": "failed", "error_code": classifyRunError(err), "finished_at": time.Now()}); uerr != nil {
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
	in.BaseURL, in.APIKey, in.MaxTokens = agent.BaseURL, agent.APIKey, agent.MaxTokens
	in.AllowedTools, in.RestrictTools = agent.Tools, agent.RestrictTools
	in.MCPTools, in.Skills = agent.MCPTools, agent.Skills
	in.CheckPointID = checkpointIDForRun(run.ID)
	in.ApprovalTools = agent.ApprovalTools
	res, err := h.Runtime.Run(ctx, in)
	if errors.Is(err, eino.ErrSummary) {
		res, err = h.Runtime.Run(ctx, builder.Fallback(in))
	}
	if err != nil {
		return failRun(err)
	}
	if res.Status == runtime.StatusInterrupted && res.Interrupt != nil {
		return h.parkForApproval(ctx, ev, agent, run, res.Interrupt)
	}
	if strings.TrimSpace(res.Text) == "" {
		return failRun(fmt.Errorf("empty response"))
	}
	return h.finalize(ctx, ev, agent, run, session.ID, res)
}

// parkForApproval records the pending approval, posts the card message and
// moves the run to waiting_approval. The Kafka event is already consumed and
// the worker is free; the run resumes via the approval action API. All store
// writes run on their own detached context: the incoming handler context may
// already be expired after a long model call.
func (h *Harness) parkForApproval(parent context.Context, ev agevent.MessageEvent, agent ResolvedAgent, run *data.AgentRun, interrupt *runtime.InterruptInfo) error {
	if h.Store == nil {
		return fmt.Errorf("agent harness store unavailable")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer cancel()
	approval := &data.AgentApproval{
		RunID:       run.ID,
		ToolCallID:  interrupt.ToolCallID,
		Tool:        interrupt.Tool,
		ArgsSummary: interrupt.Summary,
		ArgsHash:    fmt.Sprintf("%x", sha256.Sum256([]byte(interrupt.Summary))),
		ApproverID:  ev.SenderID,
		Status:      "pending",
		ExpiresAt:   time.Now().Add(approvalTTL),
		CreatedAt:   time.Now(),
	}
	if err := h.Store.CreateApproval(ctx, approval); err != nil {
		return err
	}
	// Card delivery failure must not lose the approval: the run still moves to
	// waiting_approval and the user can act via the approvals API; a failed
	// card only degrades the experience.
	if sender, ok := h.Sender.(cardSender); ok {
		card, err := json.Marshal(cardPayload{Version: 1, RunID: run.ID, ApprovalID: approval.ID, Tool: interrupt.Tool, Summary: interrupt.Summary, Target: ev.Topic, Status: "pending", RequestedBy: ev.SenderID, ExpiresAt: approval.ExpiresAt.Unix()})
		if err == nil {
			sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			_, _ = sender.SendCard(tools.WithRunContext(sendCtx, tools.RunContext{AgentID: agent.ID, BotID: agent.BotID, ActorUserID: ev.SenderID, Topic: ev.Topic, EventID: ev.EventID}), ev.Topic, string(card), stableCardClientID(ev.EventID, agent.ID, approval.ID))
		}
	}
	return h.Store.UpdateRun(ctx, run.ID, map[string]interface{}{"status": "waiting_approval", "checkpoint_id": checkpointIDForRun(run.ID), "error_code": ""})
}

// HandleResume continues a waiting_approval run after the user approved or
// rejected the pending tool call. The caller (worker execute path) loads the
// run and approval and re-checks eligibility before invoking.
func (h *Harness) HandleResume(ctx context.Context, ev agevent.MessageEvent, agent ResolvedAgent, run *data.AgentRun, approval *data.AgentApproval) error {
	ctx, span := otel.Tracer("nonoka-im/agent/harness").Start(ctx, "agent.handle_resume")
	defer span.End()
	if run.CheckpointID == "" || approval.RunID != run.ID {
		return fmt.Errorf("resume mismatch: run %d approval %d", run.ID, approval.ID)
	}
	if h.Authorizer == nil || h.Store == nil || h.Runtime == nil || h.Sender == nil {
		return fmt.Errorf("agent harness dependencies unavailable")
	}
	if err := h.Authorizer.CanAccessTopic(ctx, run.ActorUserID, run.Topic); err != nil {
		return err
	}
	if err := h.Authorizer.CanAccessTopic(ctx, agent.BotID, run.Topic); err != nil {
		return err
	}
	if ext, ok := h.Authorizer.(interface {
		CanUseAgent(context.Context, int64, int64) error
	}); ok {
		if err := ext.CanUseAgent(ctx, run.ActorUserID, agent.ID); err != nil {
			return err
		}
	}
	fail := func(err error) error {
		fctx, fcancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer fcancel()
		if uerr := h.Store.UpdateRun(fctx, run.ID, map[string]interface{}{"status": "failed", "error_code": classifyRunError(err), "finished_at": time.Now()}); uerr != nil {
			return uerr
		}
		return err
	}
	approved := approval.Status == "approved"
	res, err := h.Runtime.Resume(ctx, runtime.ResumeInput{
		Input: runtime.Input{
			Instruction:   agent.Instruction,
			Model:         agent.Model,
			BaseURL:       agent.BaseURL,
			APIKey:        agent.APIKey,
			MaxTokens:     agent.MaxTokens,
			AllowedTools:  agent.Tools,
			RestrictTools: agent.RestrictTools,
			MCPTools:      agent.MCPTools,
			Skills:        agent.Skills,
			ApprovalTools: agent.ApprovalTools,
			EventID:       run.EventID,
			AgentID:       agent.ID,
			ActorUserID:   run.ActorUserID,
			BotID:         agent.BotID,
			Topic:         run.Topic,
			CheckPointID:  run.CheckpointID,
		},
		InterruptID: approval.ToolCallID,
		Approved:    approved,
	})
	if err != nil {
		return fail(err)
	}
	if res.Status == runtime.StatusInterrupted && res.Interrupt != nil {
		return h.parkForApproval(ctx, ev, agent, run, res.Interrupt)
	}
	if strings.TrimSpace(res.Text) == "" {
		return fail(fmt.Errorf("empty response"))
	}
	return h.finalize(ctx, ev, agent, run, run.SessionID, res)
}

// finalize submits the model reply and commits the turn, snapshot and terminal
// run state. Shared by fresh runs and approval resumes. All post-reply store
// writes run on a fresh detached context created here: the incoming context
// may already be expired after a long model call, and the commit must succeed
// regardless.
func (h *Harness) finalize(ctx context.Context, ev agevent.MessageEvent, agent ResolvedAgent, run *data.AgentRun, sessionID uint64, res runtime.Result) error {
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer finishCancel()
	clientID := stableClientID(ev.EventID, agent.ID)
	sendCtx := tools.WithRunContext(ctx, tools.RunContext{AgentID: agent.ID, BotID: agent.BotID, ActorUserID: ev.SenderID, Topic: ev.Topic, EventID: ev.EventID})
	sendCtx, sendCancel := context.WithTimeout(context.WithoutCancel(sendCtx), 10*time.Second)
	defer sendCancel()
	var replyMsgID int64
	var err error
	if sender, ok := h.Sender.(interface {
		SendWithReceipt(context.Context, string, string, string) (int64, error)
	}); ok {
		replyMsgID, err = sender.SendWithReceipt(sendCtx, ev.Topic, res.Text, clientID)
	} else {
		err = h.Sender.Send(sendCtx, ev.Topic, res.Text, clientID)
	}
	if err != nil {
		if uerr := h.Store.UpdateRun(finishCtx, run.ID, map[string]interface{}{"status": "failed", "error_code": classifyRunError(err), "finished_at": time.Now()}); uerr != nil {
			return uerr
		}
		return err
	}

	// Phase C (short transaction, session lock re-acquired): commit the turn,
	// snapshot and terminal run state together.
	var snapshot *data.AgentContextSnapshot
	if res.Summary != "" {
		snapshot = &data.AgentContextSnapshot{SessionID: sessionID, LastTurnID: res.SummaryThroughTurnID, AgentVersion: agent.Version, Summary: res.Summary, SummaryTokens: agentcontext.EstimateTokens(res.Summary)}
	}
	// The reply is already delivered at this point; losing the commit would
	// drop the turn/snapshot and leave the run stuck in "running", so
	// transient failures are retried with backoff. finishCtx is detached from
	// the handler context, so the retries survive handler cancellation.
	completeErr := withRetry(finishCtx, func(ctx context.Context) error {
		return h.Store.CompleteRun(ctx, agent.ID, ev.SenderID, ev.Topic,
			&data.AgentTurn{AgentID: agent.ID, ActorUserID: ev.SenderID, Topic: ev.Topic, SessionID: sessionID, RunID: run.ID, TriggerMsgID: ev.MsgID, ReplyMsgID: replyMsgID, Input: ev.Content, Output: res.Text, CreatedAt: time.Now()},
			snapshot, run.ID,
			map[string]interface{}{"status": "completed", "reply_msg_id": replyMsgID, "input_tokens": res.Usage.InputTokens, "output_tokens": res.Usage.OutputTokens, "total_tokens": res.Usage.TotalTokens, "finished_at": time.Now()})
	})
	if completeErr != nil {
		return completeErr
	}
	return nil
}

// completeRunAttempts and completeRunBackoff bound the Phase C commit retries.
var completeRunBackoff = []time.Duration{100 * time.Millisecond, 400 * time.Millisecond, time.Second}

const completeRunAttempts = 3

// withRetry runs op until it succeeds or completeRunAttempts is exhausted,
// sleeping completeRunBackoff[attempt] between attempts. It stops early when
// ctx is done and returns the last op error.
func withRetry(ctx context.Context, op func(context.Context) error) error {
	var err error
	for attempt := 0; attempt < completeRunAttempts; attempt++ {
		if err = op(ctx); err == nil {
			return nil
		}
		if attempt < len(completeRunBackoff) {
			select {
			case <-ctx.Done():
				return err
			case <-time.After(completeRunBackoff[attempt]):
			}
		}
	}
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
func stableClientID(eventID string, agentID int64) string {
	h := sha256.Sum256(fmt.Appendf(nil, "%s:%d", eventID, agentID))
	return "agent-" + hex.EncodeToString(h[:16])
}

// checkpointIDForRun derives the Eino checkpoint ID scoped to a run.
func checkpointIDForRun(runID uint64) string {
	return fmt.Sprintf("agent-run-%d", runID)
}

// stableCardClientID deterministically identifies the card message of one
// approval so redelivery cannot post duplicate cards.
func stableCardClientID(eventID string, agentID int64, approvalID uint64) string {
	h := sha256.Sum256(fmt.Appendf(nil, "%s:%d:card:%d", eventID, agentID, approvalID))
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

// SendCard posts an MSG_TYPE_AGENT_CARD message (the approval card).
func (s *HTTPSender) SendCard(ctx context.Context, topic, content, clientID string) (int64, error) {
	return s.send(ctx, topic, content, clientID, 5)
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
	return s.send(ctx, topic, content, clientID, 0)
}

func (s *HTTPSender) send(ctx context.Context, topic, content, clientID string, msgType int32) (int64, error) {
	// json.Marshal keeps the payload valid even when the model output
	// contains quotes or invalid UTF-8.
	body, err := json.Marshal(sendReplyRequest{Topic: topic, ClientMsgID: clientID, MsgType: msgType, Content: base64.StdEncoding.EncodeToString([]byte(content))})
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
