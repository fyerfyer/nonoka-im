// Package executor turns one claimed run into a runtime invocation. It owns
// the authorization, version resolution, approval consumption and heartbeat
// lifecycle; the worker package owns scheduling and concurrency.
package executor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/credential"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/messaging"

	"github.com/go-kratos/kratos/v2/log"
)

type Executor struct {
	Store      Store
	Registry   Registry
	Authorizer Authorizer
	Harness    *app.Harness
	Reader     app.MessageReader
	SealKey    []byte

	MaxAttempts int
	RunTimeout  time.Duration
	Lease       time.Duration
	Log         *log.Helper
}

// ExecuteRun claims one queued run and executes it under a heartbeat. It is
// safe to call concurrently: ClaimRun guarantees a single winner and every
// state write is fenced by the attempt token.
func (e *Executor) ExecuteRun(parent context.Context, runID uint64) {
	if e.Store == nil {
		return
	}
	run, ok, err := e.Store.ClaimRun(parent, runID, e.Lease)
	if err != nil {
		e.log().Errorf("claim run %d: %v", runID, err)
		return
	}
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(parent, e.RunTimeout)
	defer cancel()

	// Heartbeat while executing. Losing the lease cancels the run context so a
	// stale worker cannot keep spending model/tool resources.
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		ticker := time.NewTicker(e.heartbeatInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ok, err := e.Store.Heartbeat(ctx, run.ID, run.AttemptID, e.Lease)
				if err != nil {
					e.log().Warnf("heartbeat run %d: %v", run.ID, err)
					continue
				}
				if !ok {
					e.log().Warnf("run %d lost its lease; cancelling execution", run.ID)
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-hbDone }()

	if err := e.Execute(ctx, run); err != nil && !errors.Is(err, domain.ErrRunState) && !errors.Is(err, context.Canceled) {
		e.log().Errorf("execute run %d: %v", run.ID, err)
	}
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
		Content:       string(msg.Content),
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

	session, snapshot, turns, err := e.Store.LoadRunState(ctx, run)
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
	decision, ok, err := e.Store.ConsumeApproval(ctx, approval.ID, run.AttemptID)
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
	apiKey := ""
	if version.APIKeyEnc != "" {
		raw, err := base64.StdEncoding.DecodeString(version.APIKeyEnc)
		if err != nil {
			return app.ResolvedAgent{}, fmt.Errorf("%w: decode sealed model key: %v", port.ErrInvalidConfig, err)
		}
		if len(e.SealKey) < 32 {
			return app.ResolvedAgent{}, fmt.Errorf("%w: AGENT_SECRET_KEY is required to unseal model keys", port.ErrInvalidConfig)
		}
		plain, err := credential.Open(e.SealKey, raw)
		if err != nil {
			return app.ResolvedAgent{}, fmt.Errorf("%w: unseal model key: %v", port.ErrInvalidConfig, err)
		}
		apiKey = string(plain)
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
	var approvalTools []string
	for _, binding := range mcpBindings {
		mcpTools = append(mcpTools, port.MCPTool{Server: binding.Server, Tool: binding.Tool})
		server, err := e.Registry.GetServer(ctx, binding.Server)
		if err != nil {
			if errors.Is(err, domain.ErrMCPNotFound) {
				return app.ResolvedAgent{}, fmt.Errorf("%w: mcp server %q not found", port.ErrInvalidConfig, binding.Server)
			}
			return app.ResolvedAgent{}, err
		}
		if !server.Enabled {
			return app.ResolvedAgent{}, fmt.Errorf("%w: mcp server %q is disabled", port.ErrInvalidConfig, binding.Server)
		}
		if server.RiskClass == "write" {
			approvalTools = append(approvalTools, binding.Exposed)
		}
	}
	skills := make([]port.SkillBinding, 0, len(skillBindings))
	skills = append(skills, skillBindings...)
	return app.ResolvedAgent{
		ID: agent.ID, BotID: agent.BotUserID, Version: version.Version,
		Instruction: version.Instruction, Model: version.Model, BaseURL: version.BaseURL,
		APIKey: apiKey, MaxTokens: version.MaxTokens, Tools: allowed, MCPTools: mcpTools,
		Skills: skills, RestrictTools: true, ApprovalTools: approvalTools,
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
	}, e.MaxAttempts, nil)
	if errors.Is(ferr, domain.ErrRunState) {
		return nil
	}
	return ferr
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
