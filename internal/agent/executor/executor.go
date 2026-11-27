// Package executor turns one claimed run into a runtime invocation. It owns
// the authorization, version resolution, approval consumption and heartbeat
// lifecycle; the worker package owns scheduling and concurrency.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/domain"
	agentmetrics "nonoka-im/internal/agent/metrics"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/agent/tools/catalog"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/messaging"
	"nonoka-im/internal/realtime"

	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Executor struct {
	Store      Store
	Registry   Registry
	Models     ModelResolver
	Authorizer Authorizer
	Harness    *app.Harness
	Reader     app.MessageReader
	Quota      *app.QuotaService
	Notifier   *app.QuotaNotifier

	MaxAttempts   int
	RunTimeout    time.Duration
	Lease         time.Duration
	Stream        realtime.RunEventSink
	Metrics       *agentmetrics.Metrics
	SearchEnabled bool
	Log           *log.Helper
}

// ExecuteRun claims one queued run and executes it under a heartbeat. It is
// safe to call concurrently: ClaimRun guarantees a single winner and every
// state write is fenced by the attempt token.
func (e *Executor) ExecuteRun(parent context.Context, runID uint64) {
	if e.Store == nil {
		return
	}
	run, ok, err := e.claim(parent, runID)
	if err != nil {
		var budgetErr *domain.BudgetError
		if errors.As(err, &budgetErr) && run != nil {
			if agent, aerr := e.Store.GetAgent(context.WithoutCancel(parent), run.AgentID); aerr == nil && e.Notifier != nil {
				e.Notifier.Notify(context.WithoutCancel(parent), run.AgentID, agent.BotUserID, run.ActorUserID, run.Topic, run.EventID, "budget_exceeded", budgetErr.Error())
			}
			e.log().Warnf("run %d blocked by budget: %v", runID, err)
			return
		}
		e.log().Errorf("claim run %d: %v", runID, err)
		return
	}
	if !ok {
		return
	}
	if e.Metrics != nil && !run.CreatedAt.IsZero() {
		e.Metrics.ObserveQueueWait(time.Since(run.CreatedAt).Seconds())
	}
	ctx, cancel := context.WithTimeout(parent, e.RunTimeout)
	defer cancel()
	ctx, span := otel.Tracer("nonoka-im/agent/worker").Start(ctx, "agent.run", trace.WithAttributes(
		attribute.Int64("agent.id", run.AgentID),
		attribute.String("agent.run_id", fmt.Sprintf("%d", run.ID)),
		attribute.String("agent.event_id", run.EventID),
		attribute.String("agent.attempt_id", run.AttemptID),
	))
	defer span.End()
	if sc := span.SpanContext(); sc.IsValid() {
		run.TraceID = sc.TraceID().String()
		if err := e.Store.UpdateRunFenced(ctx, run.ID, run.AttemptID, map[string]interface{}{"trace_id": run.TraceID}); err != nil {
			e.log().Warnf("record trace id for run %d: %v", run.ID, err)
		}
	}

	// Heartbeat while executing. Losing the lease cancels the run context so a
	// stale worker cannot keep spending model/tool resources.
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		ticker := time.NewTicker(e.heartbeatInterval())
		defer ticker.Stop()
		var firstFailure time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ok, err := e.Store.Heartbeat(ctx, run.ID, run.AttemptID, e.Lease)
				if err != nil {
					if firstFailure.IsZero() {
						firstFailure = time.Now()
					}
					e.log().Warnf("heartbeat run %d: %v", run.ID, err)
					// If the database stays unreachable for a full lease the
					// sweeper may requeue this run. Stop before another worker
					// can execute the same tools.
					if e.Lease > 0 && time.Since(firstFailure) >= e.Lease {
						e.log().Errorf("run %d could not heartbeat for %s; cancelling execution", run.ID, e.Lease)
						cancel()
						return
					}
					continue
				}
				firstFailure = time.Time{}
				if !ok {
					e.log().Warnf("run %d lost its lease; cancelling execution", run.ID)
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-hbDone }()

	execErr := e.Execute(ctx, run)
	if errors.Is(execErr, context.Canceled) && parent.Err() != nil {
		// Graceful shutdown: release a run that has not committed a side
		// effect so another worker can pick it up immediately. Runs with a
		// side effect/checkpoint stay fenced for the sweeper or an operator.
		releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		if ok, err := e.Store.ReleaseRun(releaseCtx, run.ID, run.AttemptID); err != nil {
			e.log().Warnf("release run %d on shutdown: %v", run.ID, err)
		} else if ok {
			e.log().Infof("released run %d on shutdown", run.ID)
		}
		releaseCancel()
	}
	if execErr != nil && !errors.Is(execErr, domain.ErrRunState) && !errors.Is(execErr, context.Canceled) {
		e.log().Errorf("execute run %d: %v", run.ID, execErr)
	}
	// A budget/quota failure discovered mid-run (for example by the usage
	// guard) should still produce one human-readable chat notice. Dispatch and
	// claim denials already notify; the durable dedupe key suppresses repeats.
	if current, gerr := e.Store.GetRun(context.WithoutCancel(parent), run.ID); gerr == nil && isQuotaErrorCode(current.ErrorCode) {
		if agent, aerr := e.Store.GetAgent(context.WithoutCancel(parent), current.AgentID); aerr == nil && e.Notifier != nil {
			e.Notifier.Notify(context.WithoutCancel(parent), current.AgentID, agent.BotUserID, current.ActorUserID, current.Topic, current.EventID, current.ErrorCode, app.PublicFailureMessage(current.ErrorCode, current.LastError))
		}
	}
	// An explicit cancel clears attempt_id, so the heartbeat cancels the
	// in-flight context and the harness cannot record a terminal event. Emit
	// it from the executor after checking the durable state.
	if current, err := e.Store.GetRun(context.WithoutCancel(parent), run.ID); err == nil && current.Status == domain.RunCancelled && current.ErrorCode == "cancelled" {
		if e.Stream != nil {
			_ = e.Stream.Publish(context.WithoutCancel(parent), realtime.RunEvent{
				RunID: current.ID, Type: realtime.RunCancelled, Topic: current.Topic,
				AgentID: current.AgentID, ActorUserID: current.ActorUserID,
				Status: domain.RunCancelled, ErrorCode: "cancelled", TimestampMs: time.Now().UnixMilli(),
			}, []int64{current.ActorUserID})
		}
	}
}

func (e *Executor) claim(ctx context.Context, runID uint64) (*domain.AgentRun, bool, error) {
	if e.Quota != nil {
		return e.Quota.ClaimRun(ctx, runID, e.Lease)
	}
	return e.Store.ClaimRun(ctx, runID, e.Lease)
}

func (e *Executor) heartbeatInterval() time.Duration {
	if e.Lease <= 0 {
		return 45 * time.Second
	}
	interval := e.Lease / 3
	if interval <= 0 {
		interval = time.Second
	}
	return interval
}

// Execute runs one claimed run. All early failures are recorded on the run
// row through FailRun; only a lost lease returns ErrRunState.
func (e *Executor) Execute(ctx context.Context, run *domain.AgentRun) (err error) {
	// Last-resort failure boundary. Harness normally records its own failures;
	// anything that escapes is classified here. A run whose lease was lost or
	// whose worker is shutting down must not be failed, because another worker
	// or the sweeper owns recovery.
	defer func() {
		if err == nil {
			return
		}
		var recorded recordedError
		if errors.As(err, &recorded) {
			return
		}
		if errors.Is(err, domain.ErrRunState) || errors.Is(err, context.Canceled) {
			return
		}
		if ferr := e.recordFailure(ctx, run, err); ferr != nil {
			e.log().Errorf("record failure for run %d: %v", run.ID, ferr)
		}
	}()

	msg, err := e.Reader.MessageForUser(ctx, run.ActorUserID, run.Topic, run.MsgID)
	if err != nil {
		if errors.Is(err, app.ErrMessageNotFound) {
			return e.fail(ctx, run, port.ClassInvalidConfig, "message_unavailable", err, false, false)
		}
		return e.fail(ctx, run, port.ClassTransientInfra, "message_unavailable", err, true, false)
	}
	ev := messaging.MessageEvent{
		SchemaVersion: messaging.SchemaVersion,
		EventID:       run.EventID,
		MsgID:         run.MsgID,
		TopicSeq:      run.TopicSeq,
		Topic:         run.Topic,
		SenderID:      run.ActorUserID,
		Timestamp:     time.Now().Unix(),
		MsgType:       messaging.MessageTypeText,
		Content:       string(msg.Content),
	}
	switch msg.Type {
	case messaging.MessageTypeImage, messaging.MessageTypeFile:
		meta, ok := messaging.ParseMediaContent(msg.Content)
		if ok {
			ev.FileID, ev.FileName, ev.Mime, ev.Size = meta.FileID, meta.Name, meta.Mime, meta.Size
			if msg.Type == messaging.MessageTypeImage {
				ev.Content = "[image: " + displayName(meta.Name) + "]"
			} else {
				ev.Content = "[file: " + displayName(meta.Name) + "; file_id=" + meta.FileID + "]"
			}
		}
		ev.MsgType = msg.Type
	default:
		ev.MsgType = msg.Type
	}
	if err := e.Authorizer.CanAccessTopic(ctx, run.ActorUserID, run.Topic); err != nil {
		if errors.Is(err, authz.ErrDenied) {
			return e.cancel(ctx, run, "access_revoked")
		}
		return e.fail(ctx, run, port.ClassTransientInfra, "authz_unavailable", err, true, false)
	}
	agent, err := e.Store.GetAgent(ctx, run.AgentID)
	if err != nil {
		if errors.Is(err, domain.ErrAgentNotFound) {
			return e.fail(ctx, run, port.ClassInvalidConfig, "agent_missing", err, false, false)
		}
		return e.fail(ctx, run, port.ClassTransientInfra, "agent_unavailable", err, true, false)
	}
	if agent.Status != "active" {
		return e.cancel(ctx, run, "agent_disabled")
	}
	if err := e.Authorizer.CanUseAgent(ctx, run.ActorUserID, agent.ID); err != nil {
		if errors.Is(err, authz.ErrDenied) {
			return e.cancel(ctx, run, "access_revoked")
		}
		return e.fail(ctx, run, port.ClassTransientInfra, "authz_unavailable", err, true, false)
	}
	if err := e.Authorizer.CanAccessTopic(ctx, agent.BotUserID, run.Topic); err != nil {
		if errors.Is(err, authz.ErrDenied) {
			return e.cancel(ctx, run, "access_revoked")
		}
		return e.fail(ctx, run, port.ClassTransientInfra, "authz_unavailable", err, true, false)
	}

	versionNumber := agent.CurrentVersion
	if run.AgentVersion > 0 {
		versionNumber = run.AgentVersion
	}
	version, err := e.Store.GetVersion(ctx, agent.ID, versionNumber)
	if err != nil {
		if errors.Is(err, domain.ErrVersionNotFound) {
			return e.fail(ctx, run, port.ClassInvalidConfig, "version_missing", err, false, false)
		}
		return e.fail(ctx, run, port.ClassTransientInfra, "version_unavailable", err, true, false)
	}
	resolved, err := e.resolve(ctx, agent, version)
	if err != nil {
		if errors.Is(err, domain.ErrTransient) {
			return e.fail(ctx, run, port.ClassTransientInfra, "config_unavailable", err, true, false)
		}
		return e.fail(ctx, run, port.ClassInvalidConfig, "invalid_config", err, false, false)
	}

	if run.CheckpointID != "" {
		return e.resume(ctx, ev, run, resolved)
	}

	ctx, memSpan := otel.Tracer("nonoka-im/agent/worker").Start(ctx, "agent.memory.load")
	session, snapshot, turns, err := e.Store.LoadRunState(ctx, run)
	memSpan.End()
	if err != nil {
		if errors.Is(err, domain.ErrTransient) {
			return e.fail(ctx, run, port.ClassTransientInfra, "state_unavailable", err, true, false)
		}
		return e.fail(ctx, run, port.ClassInvalidConfig, "state_unavailable", err, false, false)
	}
	return e.Harness.HandleResolved(ctx, ev, resolved, run, session, snapshot, turns)
}

// resume consumes the approval exactly once, then continues the interrupted
// runtime from its checkpoint. A second delivery sees a consumed approval and
// fails the run instead of executing the side effect twice.
func (e *Executor) resume(ctx context.Context, ev messaging.MessageEvent, run *domain.AgentRun, agent app.ResolvedAgent) error {
	approval, err := e.Store.ApprovalForRun(ctx, run.ID)
	if err != nil {
		return e.fail(ctx, run, port.ClassTransientInfra, "approval_unavailable", err, true, true)
	}
	if approval == nil {
		return e.fail(ctx, run, port.ClassInvalidConfig, "approval_missing", fmt.Errorf("run %d has no approval", run.ID), false, true)
	}
	if approval.Status == domain.ApprovalPending {
		// Not decided yet (for example a duplicate replay before the API
		// decision). Park the run again; the decision will requeue it.
		return e.Store.UpdateRunFenced(ctx, run.ID, run.AttemptID, map[string]interface{}{
			"status": domain.RunWaitingApproval, "started_at": nil, "attempt_id": "", "lease_until": nil,
		})
	}
	ctx, resumeSpan := otel.Tracer("nonoka-im/agent/worker").Start(ctx, "agent.approval.resume")
	decision, ok, err := e.Store.ConsumeApproval(ctx, approval.ID, run.AttemptID)
	resumeSpan.End()
	if err != nil {
		return e.fail(ctx, run, port.ClassTransientInfra, "approval_consume_failed", err, true, true)
	}
	if !ok {
		return e.fail(ctx, run, port.ClassInvalidConfig, "approval_consumed", fmt.Errorf("approval %d was already consumed", approval.ID), false, true)
	}
	if err := e.Harness.HandleResume(ctx, ev, agent, run, approval, decision); err != nil {
		// The side effect is consumed; never replay it automatically. Clear
		// the checkpoint so a manual retry starts a fresh run that asks for
		// approval again. Record the failure as terminal so the deferred
		// boundary cannot requeue a consumed approval.
		if errors.Is(err, domain.ErrRunState) {
			return err
		}
		return e.failAfterResume(ctx, run, err)
	}
	return nil
}

// failAfterResume marks a consumed-approval run terminal. The side effect may
// already have happened, so a retry must be an explicit operator decision.
func (e *Executor) failAfterResume(parent context.Context, run *domain.AgentRun, cause error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer cancel()
	if run.CheckpointID != "" {
		_ = e.Store.DeleteCheckpoint(ctx, run.CheckpointID)
	}
	ferr := e.Store.FailRun(ctx, run, domain.Failure{
		Class: string(port.ClassInvalidConfig), Code: "resume_failed", Message: cause.Error(), Retryable: false,
	}, e.MaxAttempts, map[string]interface{}{"checkpoint_id": ""})
	if errors.Is(ferr, domain.ErrRunState) {
		return recordedError{err: cause}
	}
	if ferr != nil {
		return recordedError{err: fmt.Errorf("%w (record failure: %v)", cause, ferr)}
	}
	return recordedError{err: cause}
}

func (e *Executor) resolve(ctx context.Context, agent *domain.Agent, version *domain.AgentVersion) (app.ResolvedAgent, error) {
	var allowed []string
	if version.Tools != "" {
		if err := json.Unmarshal([]byte(version.Tools), &allowed); err != nil {
			return app.ResolvedAgent{}, fmt.Errorf("%w: decode tool list: %v", port.ErrInvalidConfig, err)
		}
	}
	policies := map[string]string{}
	if version.ToolPolicies != "" {
		if err := json.Unmarshal([]byte(version.ToolPolicies), &policies); err != nil {
			return app.ResolvedAgent{}, fmt.Errorf("%w: decode tool policies: %v", port.ErrInvalidConfig, err)
		}
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = true
	}
	for _, name := range catalog.SystemToolNames() {
		if !allowedSet[name] {
			allowed = append(allowed, name)
			allowedSet[name] = true
		}
	}
	var approvalTools []string
	for _, def := range catalog.BuiltinDefinitions() {
		if !allowedSet[def.Name] {
			continue
		}
		policy := catalog.Policy(def, policies)
		if policy == catalog.PolicyDeny && !def.System {
			delete(allowedSet, def.Name)
			continue
		}
		if policy == catalog.PolicyAsk && !def.System {
			approvalTools = append(approvalTools, def.Name)
		}
	}
	filteredAllowed := allowed[:0]
	for _, name := range allowed {
		if allowedSet[name] {
			filteredAllowed = append(filteredAllowed, name)
		}
	}
	allowed = filteredAllowed
	if !e.SearchEnabled {
		// Search is an optional backend. Do not advertise a tool that can only
		// fail with SEARCH_UNAVAILABLE; the agent still has conversation context.
		filteredAllowed = allowed[:0]
		for _, name := range allowed {
			if name != "search_messages" {
				filteredAllowed = append(filteredAllowed, name)
			}
		}
		allowed = filteredAllowed
		filteredApproval := approvalTools[:0]
		for _, name := range approvalTools {
			if name != "search_messages" {
				filteredApproval = append(filteredApproval, name)
			}
		}
		approvalTools = filteredApproval
	}
	if e.Models == nil {
		return app.ResolvedAgent{}, fmt.Errorf("%w: model resolver unavailable", port.ErrInvalidConfig)
	}
	endpoint, err := e.Models.ResolveForRun(ctx, agent.ID, version.ModelID)
	if err != nil {
		return app.ResolvedAgent{}, fmt.Errorf("%w: resolve model: %v", port.ErrInvalidConfig, err)
	}
	if version.MaxTokens > 0 {
		endpoint.MaxTokens = version.MaxTokens
	}
	var mcpBindings []domain.MCPBinding
	if version.MCPBindings != "" {
		if err := json.Unmarshal([]byte(version.MCPBindings), &mcpBindings); err != nil {
			return app.ResolvedAgent{}, fmt.Errorf("%w: decode mcp bindings: %v", port.ErrInvalidConfig, err)
		}
	}
	var skillBindings []domain.SkillBinding
	if version.Skills != "" {
		if err := json.Unmarshal([]byte(version.Skills), &skillBindings); err != nil {
			return app.ResolvedAgent{}, fmt.Errorf("%w: decode skill bindings: %v", port.ErrInvalidConfig, err)
		}
	}
	var mcpTools []port.MCPTool
	var sideEffectTools []string
	for _, binding := range mcpBindings {
		server, err := e.Registry.GetServerByID(ctx, binding.ServerID)
		if err != nil {
			if errors.Is(err, domain.ErrMCPNotFound) {
				return app.ResolvedAgent{}, fmt.Errorf("%w: mcp server %d not found", port.ErrInvalidConfig, binding.ServerID)
			}
			return app.ResolvedAgent{}, err
		}
		if !server.Enabled {
			return app.ResolvedAgent{}, fmt.Errorf("%w: mcp server %q is disabled", port.ErrInvalidConfig, server.Name)
		}
		if server.Visibility != domain.VisibilityPrivate && server.ReviewStatus != domain.ReviewApproved {
			return app.ResolvedAgent{}, fmt.Errorf("%w: mcp server %q is not approved", port.ErrInvalidConfig, server.Name)
		}
		tool, err := e.Registry.GetTool(ctx, binding.ServerID, binding.Tool)
		if err != nil {
			return app.ResolvedAgent{}, fmt.Errorf("%w: mcp tool %q is unavailable", port.ErrInvalidConfig, binding.Tool)
		}
		if binding.SchemaHash != "" && tool.SchemaHash != binding.SchemaHash {
			return app.ResolvedAgent{}, fmt.Errorf("%w: mcp tool %q schema changed", port.ErrInvalidConfig, binding.Tool)
		}
		policy := binding.Policy
		if policy == "" {
			policy = catalog.PolicyAuto
			if tool.Risk == domain.MCPRiskWrite {
				policy = catalog.PolicyAsk
			}
		}
		if policy == catalog.PolicyDeny {
			continue
		}
		exposed := binding.Exposed
		if exposed == "" {
			exposed = domain.MCPExposedName(binding.ServerID, binding.Tool)
		}
		mcpTools = append(mcpTools, port.MCPTool{
			ServerID: binding.ServerID, Server: server.Name, Tool: binding.Tool,
			Exposed: exposed, SchemaHash: binding.SchemaHash, Policy: policy,
		})
		if tool.Risk == domain.MCPRiskWrite {
			// Ledger all MCP writes even when the owner chose auto policy.
			sideEffectTools = append(sideEffectTools, exposed)
		}
		if policy == catalog.PolicyAsk {
			approvalTools = append(approvalTools, exposed)
		}
	}
	skills := make([]port.SkillBinding, 0, len(skillBindings))
	skills = append(skills, skillBindings...)
	return app.ResolvedAgent{
		ID: agent.ID, BotID: agent.BotUserID, Version: version.Version,
		Instruction: version.Instruction, Model: endpoint, Tools: allowed, MCPTools: mcpTools,
		Skills: skills, RestrictTools: true, ApprovalTools: approvalTools,
		SideEffectTools: sideEffectTools,
	}, nil
}

// recordedError marks an error whose run state has already been written. The
// deferred failure boundary must not try to record it again.
type recordedError struct{ err error }

func (e recordedError) Error() string { return e.err.Error() }
func (e recordedError) Unwrap() error { return e.err }

// fail records a classified failure. clearCheckpoint is used when a consumed
// approval must not be replayed by an automatic retry.
func (e *Executor) fail(parent context.Context, run *domain.AgentRun, class port.ErrorClass, code string, err error, retryable, clearCheckpoint bool) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer cancel()
	update := map[string]interface{}{}
	if clearCheckpoint {
		update["checkpoint_id"] = ""
	}
	ferr := e.Store.FailRun(ctx, run, domain.Failure{
		Class: string(class), Code: code, Message: err.Error(), Retryable: retryable,
	}, e.MaxAttempts, update)
	if errors.Is(ferr, domain.ErrRunState) {
		return recordedError{err: err}
	}
	if ferr != nil {
		return recordedError{err: fmt.Errorf("%w (record failure: %v)", err, ferr)}
	}
	return recordedError{err: err}
}

// recordFailure is the deferred last-resort classifier.
func (e *Executor) recordFailure(parent context.Context, run *domain.AgentRun, err error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer cancel()
	class, retryable := port.Classify(err)
	if errors.Is(err, domain.ErrTransient) {
		class, retryable = port.ClassTransientInfra, true
	}
	ferr := e.Store.FailRun(ctx, run, domain.Failure{
		Class: string(class), Code: failureCode(class), Message: err.Error(), Retryable: retryable,
		RetryAfter: retryAfter(err),
	}, e.MaxAttempts, nil)
	if errors.Is(ferr, domain.ErrRunState) {
		return nil
	}
	return ferr
}

func displayName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "attachment"
	}
	return name
}

func isQuotaErrorCode(code string) bool {
	switch code {
	case "quota_exceeded", "budget_exceeded", "rate_limited":
		return true
	default:
		return false
	}
}

func retryAfter(err error) time.Duration {
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

func failureCode(class port.ErrorClass) string {
	switch class {
	case port.ClassCanceled:
		return "canceled"
	case port.ClassTimeout:
		return "timeout"
	case port.ClassAuthDenied:
		return "access_denied"
	case port.ClassInvalidConfig:
		return "invalid_config"
	case port.ClassTransientInfra:
		return "infra_unavailable"
	case port.ClassModel:
		return "model_error"
	case port.ClassTool:
		return "tool_error"
	case port.ClassSideEffect:
		return domain.FailureCodeManualRetry
	case port.ClassQuota:
		return "quota_exceeded"
	case port.ClassBudget:
		return "budget_exceeded"
	case port.ClassRateLimit:
		return "rate_limited"
	default:
		return "run_failed"
	}
}

func (e *Executor) cancel(ctx context.Context, run *domain.AgentRun, code string) error {
	err := e.Store.UpdateRunFenced(ctx, run.ID, run.AttemptID, map[string]interface{}{
		"status": domain.RunCancelled, "error_code": code, "finished_at": time.Now(),
		"attempt_id": "", "lease_until": nil,
	})
	if errors.Is(err, domain.ErrRunState) {
		return recordedError{err: err}
	}
	if err != nil {
		return recordedError{err: err}
	}
	return recordedError{err: domain.ErrRunState}
}

func (e *Executor) log() *log.Helper {
	if e.Log != nil {
		return e.Log
	}
	return log.NewHelper(log.DefaultLogger)
}
