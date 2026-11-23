package eino

import (
	"context"
	"encoding/gob"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/agent/tools/catalog"
)

type spanContextKey struct{}

func init() {
	// Checkpoints gob-encode the interrupt payload and the resume decision;
	// both cross interface boundaries inside the Eino runtime state.
	gob.Register(approvalInterruptInfo{})
	gob.Register(approvalDecision{})
}

// ChatModelFactory builds the provider model for one resolved platform
// endpoint. The default uses the OpenAI-compatible adapter; tests can inject a
// fake without changing the runtime contract.
type ChatModelFactory func(ctx context.Context, endpoint port.ModelEndpoint, maxTokens int) (model.ToolCallingChatModel, error)

type Engine struct {
	tools    []tool.BaseTool
	newModel ChatModelFactory

	// mcpTools resolves per-run MCP bindings into guarded tools; nil disables
	// MCP tools. skillBackend builds the skill middleware backend for the
	// skills bound to the agent version; nil disables skills.
	mcpTools     func(ctx context.Context, bindings []port.MCPTool) ([]tool.BaseTool, error)
	skillBackend func(bindings []port.SkillBinding) (skill.Backend, error)

	// checkpoints persists Eino execution state across approval interrupts;
	// nil disables interrupt/resume (tools that need approval then fail).
	checkpoints adk.CheckPointStore

	// ledger makes side-effecting tool calls at-most-once per (run, tool call).
	// markSideEffect records the committed side effect on the run row.
	ledger         tools.ToolInvocationStore
	markSideEffect tools.SideEffectMarker
	// toolCatalog supplies side-effect classification and approval summaries.
	toolCatalog catalog.Catalog

	mu     sync.Mutex
	models map[modelCacheKey]model.ToolCallingChatModel
	order  []modelCacheKey // FIFO eviction order; capped at modelCacheLimit.
}

// Deps is the complete, immutable dependency set of an Eino engine. The model
// endpoint is supplied per run from the platform Model Service, not here.
type Deps struct {
	// Tools are the pre-built built-in tools for every run. Per-run MCP tools
	// are resolved separately by MCPToolProvider.
	Tools               []tool.BaseTool
	MCPToolProvider     func(ctx context.Context, bindings []port.MCPTool) ([]tool.BaseTool, error)
	SkillBackendFactory func(bindings []port.SkillBinding) (skill.Backend, error)
	Checkpoints         adk.CheckPointStore
	Ledger              tools.ToolInvocationStore
	MarkSideEffect      tools.SideEffectMarker
	ToolCatalog         catalog.Catalog
	NewChatModel        ChatModelFactory
}

// New constructs the Eino implementation of port.Runtime.
func New(_ context.Context, deps Deps) (*Engine, error) {
	return &Engine{
		tools: deps.Tools, newModel: deps.NewChatModel, mcpTools: deps.MCPToolProvider,
		skillBackend: deps.SkillBackendFactory, checkpoints: deps.Checkpoints,
		ledger: deps.Ledger, markSideEffect: deps.MarkSideEffect, toolCatalog: deps.ToolCatalog,
	}, nil
}

func runLoop(iterator *adk.AsyncIterator[*adk.AgentEvent], state *runState, summaryThrough uint64) (port.Result, error) {
	var final string
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			// Summary generation failures already wrap ErrSummary (see
			// summaryTracker), so the harness can route them to the fallback
			// via errors.Is. A model callback error is tagged as ErrModel so
			// the worker classifies it as a retryable model failure instead of
			// an internal error.
			if state.modelError.Load() {
				return port.Result{}, fmt.Errorf("%w: %v", port.ErrModel, event.Err)
			}
			return port.Result{}, event.Err
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			root := rootInterruptContext(event.Action.Interrupted.InterruptContexts)
			if root != nil {
				info, _ := root.Info.(approvalInterruptInfo)
				return port.Result{Status: port.StatusInterrupted, Interrupt: &port.InterruptInfo{ToolCallID: root.ID, Tool: info.Tool, Summary: info.Summary, ArgsHash: info.ArgsHash}, Summary: state.summary, SummaryThroughTurnID: summaryThrough, Usage: state.usage}, nil
			}
		}
		// GetMessage handles wrapped events and copies a streaming message
		// before concatenating it, so a future streaming consumer can still
		// forward the original stream.
		msg, _, err := adk.GetMessage(event)
		if err != nil {
			return port.Result{}, fmt.Errorf("read Eino output: %w", err)
		}
		if msg != nil && msg.Role == schema.Assistant && len(msg.ToolCalls) == 0 {
			final = msg.Content
		}
	}
	if strings.TrimSpace(final) == "" {
		return port.Result{}, fmt.Errorf("empty model response")
	}
	return port.Result{Text: strings.TrimSpace(final), Status: port.StatusCompleted, Summary: state.summary, SummaryThroughTurnID: summaryThrough, Usage: state.usage}, nil
}

func rootInterruptContext(contexts []*adk.InterruptCtx) *adk.InterruptCtx {
	for _, c := range contexts {
		if c != nil && c.IsRootCause {
			return c
		}
	}
	return nil
}

func (r *Engine) Run(ctx context.Context, in port.Input) (port.Result, error) {
	agent, state, err := r.buildAgent(ctx, in, true)
	if err != nil {
		return port.Result{}, err
	}
	msgs := make([]adk.Message, 0, len(in.History)*2+2)
	if in.Summary != "" {
		msgs = append(msgs, schema.UserMessage("Previous conversation summary:\n"+in.Summary))
	}
	for _, t := range in.History {
		msgs = append(msgs, schema.UserMessage(t.Input), schema.AssistantMessage(t.Output, nil))
	}
	msgs = append(msgs, schema.UserMessage(in.Text))
	toolCtx := tools.WithRunContext(ctx, tools.RunContext{ActorUserID: in.ActorUserID, AgentID: in.AgentID, BotID: in.BotID, RunID: in.RunID, AttemptID: in.AttemptID, EventID: in.EventID, Topic: in.Topic})
	runnerCfg := adk.RunnerConfig{Agent: agent, EnableStreaming: false}
	if r.checkpoints != nil && in.CheckPointID != "" {
		runnerCfg.CheckPointStore = r.checkpoints
	}
	runner := adk.NewRunner(toolCtx, runnerCfg)
	opts := []adk.AgentRunOption{adk.WithCallbacks(modelSpanHandler(&state.usage, &state.modelError))}
	if in.CheckPointID != "" {
		opts = append(opts, adk.WithCheckPointID(in.CheckPointID))
	}
	return runLoop(runner.Run(toolCtx, msgs, opts...), state, in.SummaryThroughTurnID)
}

// Resume continues an interrupted run from its checkpoint. The agent is
// rebuilt from the same frozen version snapshot so the resumed tool set
// matches the interrupted execution; conversation state comes from the
// checkpoint, not from history.
func (r *Engine) Resume(ctx context.Context, in port.ResumeInput) (port.Result, error) {
	if in.CheckPointID == "" || in.InterruptID == "" {
		return port.Result{}, fmt.Errorf("resume requires checkpoint and interrupt IDs")
	}
	agent, state, err := r.buildAgent(ctx, in.Input, false)
	if err != nil {
		return port.Result{}, err
	}
	toolCtx := tools.WithRunContext(ctx, tools.RunContext{ActorUserID: in.ActorUserID, AgentID: in.AgentID, BotID: in.BotID, RunID: in.RunID, AttemptID: in.AttemptID, EventID: in.EventID, Topic: in.Topic})
	runnerCfg := adk.RunnerConfig{Agent: agent, EnableStreaming: false}
	if r.checkpoints != nil {
		runnerCfg.CheckPointStore = r.checkpoints
	}
	runner := adk.NewRunner(toolCtx, runnerCfg)
	iterator, err := runner.ResumeWithParams(toolCtx, in.CheckPointID, &adk.ResumeParams{
		Targets: map[string]any{in.InterruptID: approvalDecision{Approved: in.Approved}},
	}, adk.WithCallbacks(modelSpanHandler(&state.usage, &state.modelError)))
	if err != nil {
		return port.Result{}, fmt.Errorf("resume run: %w", err)
	}
	return runLoop(iterator, state, in.SummaryThroughTurnID)
}

// summaryTracker wraps the model used by the summarization middleware and
// tags genuine summary generation failures by wrapping them in ErrSummary, so
// the run loop and the harness can recognize them with errors.Is instead of a
