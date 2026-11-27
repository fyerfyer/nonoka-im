package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	agentcontext "nonoka-im/internal/agent/context"
	"nonoka-im/internal/agent/domain"
	agentmetrics "nonoka-im/internal/agent/metrics"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/messaging"
	"nonoka-im/internal/realtime"

	"github.com/go-kratos/kratos/v2/log"
)

// RunStore is the persistence surface the harness needs. agentdata.Store
// implements it; the interface keeps orchestration independent from gorm.
type RunStore interface {
	CompleteRun(ctx context.Context, run *domain.AgentRun, turn *domain.AgentTurn, snapshot *domain.AgentContextSnapshot, values map[string]interface{}) error
	FailRun(ctx context.Context, run *domain.AgentRun, failure domain.Failure, maxAttempts int, update map[string]interface{}) error
	UpdateRunFenced(ctx context.Context, runID uint64, attemptID string, values map[string]interface{}) error
	ListRunCitations(ctx context.Context, runID uint64) ([]domain.AgentRunCitation, error)
	CreateApproval(ctx context.Context, row *domain.AgentApproval) error
	// ParkRunForApproval atomically records the approval and parks the run.
	ParkRunForApproval(ctx context.Context, run *domain.AgentRun, approval *domain.AgentApproval, checkpointID string) (*domain.AgentApproval, error)
}

type Harness struct {
	Store       RunStore
	Runtime     port.Runtime
	Authorizer  Authorizer
	Sender      ReplySender
	Builder     agentcontext.Builder
	Files       FileReader
	Quota       *QuotaService
	MaxAttempts int
	Stream      realtime.RunEventSink
	Audience    RunAudience
	Metrics     *agentmetrics.Metrics
	Log         *log.Helper
}

type ResolvedAgent struct {
	ID            int64
	BotID         int64
	Version       int64
	Instruction   string
	Model         port.ModelEndpoint
	Tools         []string
	MCPTools      []port.MCPTool
	Skills        []port.SkillBinding
	RestrictTools bool
	// ApprovalTools are tool names whose calls interrupt for human
	// confirmation (write-class MCP tools and future side-effect tools).
	ApprovalTools []string
}

// approvalTTL bounds how long a pending approval waits for the user.
const approvalTTL = 30 * time.Minute

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

// HandleResolved executes an already-claimed run. The caller (the worker
// executor) owns the lease and passes the state loaded after ClaimRun, so the
// harness never claims a run itself and never sees a duplicate event.
func (h *Harness) HandleResolved(ctx context.Context, ev messaging.MessageEvent, agent ResolvedAgent, run *domain.AgentRun, session *domain.AgentSession, snapshot *domain.AgentContextSnapshot, turns []domain.AgentTurn) error {
	ctx, span := otel.Tracer("nonoka-im/agent/harness").Start(ctx, "agent.handle_event",
		trace.WithAttributes(
			attribute.Int64("agent.id", agent.ID),
			attribute.String("agent.event_id", ev.EventID),
			attribute.String("messaging.topic", ev.Topic),
		))
	defer span.End()
	if run != nil {
		span.SetAttributes(attribute.String("agent.run_id", fmt.Sprintf("%d", run.ID)))
	}
	if ev.SenderID == agent.BotID {
		// A bot-authored event must never leave a claimed run stuck in running.
		return h.cancelRun(ctx, run, "bot_message")
	}
	if h.Authorizer == nil {
		return h.failRun(ctx, run, &port.Error{Class: port.ClassInvalidConfig, Err: fmt.Errorf("topic authorization unavailable")})
	}
	_, authSpan := otel.Tracer("nonoka-im/agent/harness").Start(ctx, "agent.authorize_topic")
	if err := h.Authorizer.CanAccessTopic(ctx, ev.SenderID, ev.Topic); err != nil {
		authSpan.End()
		return h.failAuth(ctx, run, err)
	}
	if err := h.Authorizer.CanAccessTopic(ctx, agent.BotID, ev.Topic); err != nil {
		authSpan.End()
		return h.failAuth(ctx, run, err)
	}
	authSpan.End()
	if h.Store == nil || h.Runtime == nil || h.Sender == nil || run == nil || session == nil {
		return h.failRun(ctx, run, &port.Error{Class: port.ClassInvalidConfig, Err: fmt.Errorf("agent harness dependencies unavailable")})
	}
	builder := h.Builder

	h.emitRunEvent(ctx, run, agent, realtime.RunEvent{Type: realtime.RunStarted, Status: domain.RunRunning}, true)
	stopHeartbeat := h.startHeartbeat(ctx, run, agent, 10*time.Second)
	defer stopHeartbeat()

	// Build the prompt and run the model without holding any database lock.
	in, err := builder.Build(agent.Instruction, ev.Content, snapshot, turns)
	if err != nil {
		return h.failRun(ctx, run, err)
	}
	in.EventID, in.AgentID, in.ActorUserID, in.BotID, in.Topic = ev.EventID, agent.ID, ev.SenderID, agent.BotID, ev.Topic
	in.RunID, in.AttemptID = run.ID, run.AttemptID
	in.Model = agent.Model
	in.AllowedTools, in.RestrictTools = agent.Tools, agent.RestrictTools
	in.MCPTools, in.Skills = agent.MCPTools, agent.Skills
	in.CheckPointID = checkpointIDForRun(run.ID)
	in.ApprovalTools = agent.ApprovalTools
	in.OnEvent = h.eventSink(ctx, run, agent)
	h.attachCurrentMessage(ctx, &in, ev, agent)
	if h.Quota != nil {
		guard, maxTurns, gerr := h.Quota.BeginRun(ctx, run, agent.Model)
		if gerr != nil {
			return h.failRun(ctx, run, gerr)
		}
		in.Guard = guard
		in.MaxToolTurns = maxTurns
	}
	res, err := h.Runtime.Run(ctx, in)
	if errors.Is(err, port.ErrSummary) {
		res, err = h.Runtime.Run(ctx, builder.Fallback(in))
	}
	if err != nil {
		return h.failRun(ctx, run, err)
	}
	if res.Status == port.StatusInterrupted && res.Interrupt != nil {
		return h.parkForApproval(ctx, ev, agent, run, res.Interrupt)
	}
	if strings.TrimSpace(res.Text) == "" {
		return h.failRun(ctx, run, fmt.Errorf("empty response"))
	}
	return h.finalize(ctx, ev, agent, run, res)
}

// parkForApproval records the pending approval, posts the card message and
// moves the run to waiting_approval in one transaction. The transaction keeps
// the approval row and the run state consistent; the card is sent only after
// the commit and uses the approval's stable ID.
func (h *Harness) parkForApproval(parent context.Context, ev messaging.MessageEvent, agent ResolvedAgent, run *domain.AgentRun, interrupt *port.InterruptInfo) error {
	if h.Store == nil {
		return h.failRun(parent, run, &port.Error{Class: port.ClassInvalidConfig, Err: fmt.Errorf("agent harness store unavailable")})
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer cancel()
	now := time.Now()
	approval := &domain.AgentApproval{
		RunID:       run.ID,
		ToolCallID:  interrupt.ToolCallID,
		Tool:        interrupt.Tool,
		ArgsSummary: interrupt.Summary,
		ArgsHash:    interrupt.ArgsHash,
		ApproverID:  ev.SenderID,
		Status:      domain.ApprovalPending,
		ExpiresAt:   now.Add(approvalTTL),
		AttemptID:   run.AttemptID,
		CreatedAt:   now,
	}
	parkCtx, parkSpan := otel.Tracer("nonoka-im/agent/harness").Start(ctx, "agent.approval.park")
	saved, err := h.Store.ParkRunForApproval(parkCtx, run, approval, checkpointIDForRun(run.ID))
	parkSpan.End()
	if err != nil {
		return err
	}
	h.emitRunEvent(ctx, run, agent, realtime.RunEvent{
		Type: realtime.RunApprovalNeeded, ApprovalID: saved.ID, Tool: interrupt.Tool,
		Summary: interrupt.Summary, Status: domain.ApprovalPending,
	}, false)
	// Card delivery failure must not lose the approval: the run is already
	// waiting_approval and the user can act via the approvals API.
	card, err := json.Marshal(cardPayload{Version: 1, RunID: run.ID, ApprovalID: saved.ID, Tool: interrupt.Tool, Summary: interrupt.Summary, Target: ev.Topic, Status: domain.ApprovalPending, RequestedBy: ev.SenderID, ExpiresAt: saved.ExpiresAt.Unix()})
	if err == nil {
		sendCtx := tools.WithRunContext(ctx, tools.RunContext{AgentID: agent.ID, BotID: agent.BotID, ActorUserID: ev.SenderID, Topic: ev.Topic, EventID: ev.EventID})
		sendCtx, sendCancel := context.WithTimeout(context.WithoutCancel(sendCtx), 10*time.Second)
		defer sendCancel()
		_, _ = h.Sender.SendCard(sendCtx, ev.Topic, string(card), stableCardClientID(ev.EventID, agent.ID, saved.ID))
	}
	return nil
}

// HandleResume continues a waiting_approval run. The approval has already
// been consumed by the executor, so a duplicate delivery cannot execute the
// interrupted side effect twice. decision is the consumed status:
// approved/rejected/expired; expired behaves as a rejection.
func (h *Harness) HandleResume(ctx context.Context, ev messaging.MessageEvent, agent ResolvedAgent, run *domain.AgentRun, approval *domain.AgentApproval, decision string) error {
	if run == nil {
		return fmt.Errorf("resume mismatch: run %d approval %v", 0, approval)
	}
	ctx, span := otel.Tracer("nonoka-im/agent/harness").Start(ctx, "agent.handle_resume",
		trace.WithAttributes(
			attribute.Int64("agent.id", agent.ID),
			attribute.String("agent.event_id", ev.EventID),
			attribute.String("messaging.topic", ev.Topic),
			attribute.String("agent.run_id", fmt.Sprintf("%d", run.ID)),
		))
	defer span.End()
	if run.CheckpointID == "" || approval == nil || approval.RunID != run.ID || approval.ToolCallID == "" {
		return h.failRun(ctx, run, &port.Error{Class: port.ClassInvalidConfig, Err: fmt.Errorf("resume mismatch: run %d approval %v", run.ID, approval)})
	}
	if h.Authorizer == nil || h.Store == nil || h.Runtime == nil || h.Sender == nil {
		return h.failRun(ctx, run, &port.Error{Class: port.ClassInvalidConfig, Err: fmt.Errorf("agent harness dependencies unavailable")})
	}
	if err := h.Authorizer.CanAccessTopic(ctx, run.ActorUserID, run.Topic); err != nil {
		return h.failAuth(ctx, run, err)
	}
	if err := h.Authorizer.CanAccessTopic(ctx, agent.BotID, run.Topic); err != nil {
		return h.failAuth(ctx, run, err)
	}
	if ext, ok := h.Authorizer.(interface {
		CanUseAgent(context.Context, int64, int64) error
	}); ok {
		if err := ext.CanUseAgent(ctx, run.ActorUserID, agent.ID); err != nil {
			return h.failAuth(ctx, run, err)
		}
	}
	approved := decision == domain.ApprovalApproved
	h.emitRunEvent(ctx, run, agent, realtime.RunEvent{
		Type: realtime.RunApprovalResolved, ApprovalID: approval.ID, Tool: approval.Tool, Status: decision,
	}, false)
	h.emitRunEvent(ctx, run, agent, realtime.RunEvent{Type: realtime.RunStarted, Status: domain.RunRunning}, true)
	stopHeartbeat := h.startHeartbeat(ctx, run, agent, 10*time.Second)
	defer stopHeartbeat()
	resumeIn := port.Input{
		Instruction:   agent.Instruction,
		Model:         agent.Model,
		AllowedTools:  agent.Tools,
		RestrictTools: agent.RestrictTools,
		MCPTools:      agent.MCPTools,
		Skills:        agent.Skills,
		ApprovalTools: agent.ApprovalTools,
		EventID:       run.EventID,
		RunID:         run.ID,
		AttemptID:     run.AttemptID,
		AgentID:       agent.ID,
		ActorUserID:   run.ActorUserID,
		BotID:         agent.BotID,
		Topic:         run.Topic,
		CheckPointID:  run.CheckpointID,
		OnEvent:       h.eventSink(ctx, run, agent),
	}
	if h.Quota != nil {
		guard, maxTurns, gerr := h.Quota.BeginRun(ctx, run, agent.Model)
		if gerr != nil {
			return h.failRun(ctx, run, gerr)
		}
		resumeIn.Guard = guard
		resumeIn.MaxToolTurns = maxTurns
	}
	res, err := h.Runtime.Resume(ctx, port.ResumeInput{
		Input:       resumeIn,
		InterruptID: approval.ToolCallID,
		Approved:    approved,
	})
	if err != nil {
		return h.failRun(ctx, run, err)
	}
	if res.Status == port.StatusInterrupted && res.Interrupt != nil {
		return h.parkForApproval(ctx, ev, agent, run, res.Interrupt)
	}
	if strings.TrimSpace(res.Text) == "" {
		return h.failRun(ctx, run, fmt.Errorf("empty response"))
	}
	return h.finalize(ctx, ev, agent, run, res)
}

// failRun records a classified failure on a detached context. The run row
// carries the attempt fence, so a stale worker cannot overwrite a newer
// attempt.
func (h *Harness) failRun(parent context.Context, run *domain.AgentRun, err error) error {
	if err == nil {
		return nil
	}
	if run == nil || run.AttemptID == "" || h.Store == nil {
		return err
	}
	// A shutdown or lease-loss cancellation is recovered by the sweeper, not
	// recorded as a terminal failure.
	if errors.Is(err, context.Canceled) {
		return err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer cancel()
	class, retryable := port.Classify(err)
	if errors.Is(err, domain.ErrTransient) {
		class, retryable = port.ClassTransientInfra, true
	}
	code := "run_failed"
	switch class {
	case port.ClassCanceled:
		code = "canceled"
	case port.ClassTimeout:
		code = "timeout"
	case port.ClassAuthDenied:
		code = "access_denied"
	case port.ClassInvalidConfig:
		code = "invalid_config"
	case port.ClassTransientInfra:
		code = "infra_unavailable"
	case port.ClassModel:
		code = "model_error"
	case port.ClassTool:
		code = "tool_error"
	case port.ClassSideEffect:
		code = domain.FailureCodeManualRetry
	case port.ClassQuota:
		code = "quota_exceeded"
	case port.ClassBudget:
		code = "budget_exceeded"
	case port.ClassRateLimit:
		code = "rate_limited"
	}
	if h.Metrics != nil {
		h.Metrics.ObserveRunFailure(string(class), code)
	}
	if ferr := h.Store.FailRun(ctx, run, domain.Failure{
		Class: string(class), Code: code, Message: err.Error(), Retryable: retryable,
		RetryAfter: failureRetryAfter(err),
	}, h.MaxAttempts, nil); ferr != nil {
		if errors.Is(ferr, domain.ErrRunState) {
			return err
		}
		return fmt.Errorf("%w (record failure: %v)", err, ferr)
	}
	h.emitRunEvent(ctx, run, ResolvedAgent{ID: run.AgentID}, realtime.RunEvent{
		Type: realtime.RunFailed, ErrorCode: code, ErrorClass: string(class),
		ErrorMessage: PublicFailureMessage(code, err.Error()), RetryAfterAt: failureRetryAt(err), Retryable: retryable, Status: domain.RunFailed,
	}, false)
	return err
}

// failAuth distinguishes a real authorization denial from an OpenFGA outage.
// A denial is terminal; an outage is retryable infrastructure noise.
func (h *Harness) failAuth(ctx context.Context, run *domain.AgentRun, err error) error {
	if IsDenied(h.Authorizer, err) {
		return h.failRun(ctx, run, fmt.Errorf("%w: %v", port.ErrAuthDenied, err))
	}
	return h.failRun(ctx, run, &port.Error{Class: port.ClassTransientInfra, Err: err})
}

// cancelRun moves a claimed run to a terminal cancelled state. It is used for
// events that should never execute (for example a bot-authored trigger).
func (h *Harness) cancelRun(ctx context.Context, run *domain.AgentRun, code string) error {
	if run == nil || run.AttemptID == "" || h.Store == nil {
		return nil
	}
	err := h.Store.UpdateRunFenced(ctx, run.ID, run.AttemptID, map[string]interface{}{
		"status": domain.RunCancelled, "error_code": code, "finished_at": time.Now(),
		"attempt_id": "", "lease_until": nil,
	})
	if errors.Is(err, domain.ErrRunState) {
		return nil
	}
	if err == nil {
		h.emitRunEvent(ctx, run, ResolvedAgent{ID: run.AgentID}, realtime.RunEvent{
			Type: realtime.RunCancelled, ErrorCode: code, Status: domain.RunCancelled,
		}, false)
	}
	return err
}

// finalize submits the model reply and commits the turn, snapshot and terminal
// run state with the attempt fence. All post-reply store writes use a detached
// context: the handler context may already be expired after a long model call.
func (h *Harness) finalize(ctx context.Context, ev messaging.MessageEvent, agent ResolvedAgent, run *domain.AgentRun, res port.Result) error {
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer finishCancel()
	replyMsgID := int64(0)
	clientID := stableClientID(ev.EventID, agent.ID)
	sendCtx := tools.WithRunContext(ctx, tools.RunContext{AgentID: agent.ID, BotID: agent.BotID, ActorUserID: ev.SenderID, Topic: ev.Topic, EventID: ev.EventID})
	sendCtx, sendCancel := context.WithTimeout(context.WithoutCancel(sendCtx), 10*time.Second)
	defer sendCancel()
	replyCtx, replySpan := otel.Tracer("nonoka-im/agent/harness").Start(sendCtx, "agent.reply.submit")
	replyMsgID, err := h.Sender.SendWithReceipt(replyCtx, ev.Topic, res.Text, clientID)
	replySpan.End()
	if err != nil {
		class, retryable := port.Classify(err)
		_ = h.Store.FailRun(finishCtx, run, domain.Failure{
			Class: string(class), Code: "reply_failed", Message: err.Error(), Retryable: retryable,
		}, h.MaxAttempts, nil)
		return err
	}

	var snapshot *domain.AgentContextSnapshot
	if res.Summary != "" {
		snapshot = &domain.AgentContextSnapshot{SessionID: run.SessionID, LastTurnID: res.SummaryThroughTurnID, AgentVersion: agent.Version, Summary: res.Summary, SummaryTokens: agentcontext.EstimateTokens(res.Summary)}
	}
	values := map[string]interface{}{
		"status": domain.RunCompleted, "reply_msg_id": replyMsgID, "reply_client_msg_id": clientID,
		"model_key": agent.Model.ModelKey, "finished_at": time.Now(),
	}
	// The reply is already delivered at this point; losing the commit would
	// drop the turn/snapshot and leave the run stuck in "running", so
	// transient failures are retried with backoff.
	completeErr := withRetry(finishCtx, func(ctx context.Context) error {
		return h.Store.CompleteRun(ctx, run,
			&domain.AgentTurn{AgentID: agent.ID, ActorUserID: ev.SenderID, Topic: ev.Topic, SessionID: run.SessionID, RunID: run.ID, TriggerMsgID: ev.MsgID, ReplyMsgID: replyMsgID, Input: ev.Content, Output: res.Text, CreatedAt: time.Now()},
			snapshot, values)
	})
	if completeErr != nil {
		return completeErr
	}
	completed := realtime.RunEvent{
		Type: realtime.RunCompleted, ReplyMsgID: replyMsgID, Status: domain.RunCompleted,
		InputTokens: res.Usage.InputTokens, OutputTokens: res.Usage.OutputTokens,
		ClientMessageID: clientID,
	}
	if citations, err := h.Store.ListRunCitations(finishCtx, run.ID); err == nil {
		completed.Citations = toRunCitations(citations)
	}
	if h.Metrics != nil {
		cost := (float64(res.Usage.InputTokens)*agent.Model.InputPricePerMTok +
			float64(res.Usage.OutputTokens)*agent.Model.OutputPricePerMTok) / 1_000_000
		h.Metrics.ObserveUsage(agent.ID, agent.Model.ModelKey, res.Usage.InputTokens, res.Usage.OutputTokens, cost)
	}
	h.emitRunEvent(ctx, run, agent, completed, false)
	return nil
}

// completeRunAttempts and completeRunBackoff bound the final commit retries.
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

const maxInlineImageBytes = 4 << 20

// attachCurrentMessage turns the triggering IMAGE message into a multimodal
// part. Non-vision models get an explicit degradation note and no image bytes.
func (h *Harness) attachCurrentMessage(ctx context.Context, in *port.Input, ev messaging.MessageEvent, agent ResolvedAgent) {
	if ev.MsgType != messaging.MessageTypeImage || ev.FileID == "" {
		return
	}
	if !agent.Model.SupportsVision {
		in.Text = strings.TrimSpace(in.Text + "\n[The user sent an image (" + ev.FileName + "), but the selected model cannot view images. Do not claim to see it; tell the user you cannot view images.]")
		return
	}
	if h.Files == nil || h.Authorizer == nil {
		in.Text += "\n[Image unavailable: attachment reader is not configured.]"
		return
	}
	if err := h.Authorizer.CanReadFile(ctx, ev.SenderID, ev.FileID); err != nil {
		in.Text += "\n[Image unavailable: the sender is not allowed to read this attachment.]"
		return
	}
	if ev.Size > maxInlineImageBytes {
		in.Text += "\n[Image skipped: the attachment is larger than the 4MB inline image limit.]"
		return
	}
	if ev.Mime != "" && !strings.HasPrefix(strings.ToLower(ev.Mime), "image/") {
		in.Text += "\n[Attachment skipped: it is not an image.]"
		return
	}
	file, err := h.Files.ReadFile(ctx, ev.FileID)
	if err != nil {
		in.Text += "\n[Image unavailable: failed to read the attachment.]"
		return
	}
	if len(file.Data) > maxInlineImageBytes {
		in.Text += "\n[Image skipped: the attachment is larger than the 4MB inline image limit.]"
		return
	}
	in.Attachments = append(in.Attachments, port.Attachment{
		Kind: "image", FileID: ev.FileID, Name: file.Name, Mime: file.Mime, Data: file.Data,
	})
}

func failureRetryAfter(err error) time.Duration {
	var quotaErr *domain.QuotaError
	if errors.As(err, &quotaErr) {
		return quotaErr.RetryAfter
	}
	var budgetErr *domain.BudgetError
	if errors.As(err, &budgetErr) {
		return budgetErr.RetryAfter
	}
	var rateErr *domain.RateLimitError
	if errors.As(err, &rateErr) {
		return rateErr.RetryAfter
	}
	return 0
}

func failureRetryAt(err error) string {
	if d := failureRetryAfter(err); d > 0 {
		return time.Now().Add(d).UTC().Format(time.RFC3339)
	}
	return ""
}

func toRunCitations(rows []domain.AgentRunCitation) []realtime.RunCitation {
	if len(rows) == 0 {
		return nil
	}
	out := make([]realtime.RunCitation, 0, len(rows))
	for _, row := range rows {
		out = append(out, realtime.RunCitation{
			MsgID: row.MsgID, Topic: row.Topic, TopicSeq: row.TopicSeq,
			SenderID: row.SenderID, Snippet: row.Snippet,
		})
	}
	return out
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
