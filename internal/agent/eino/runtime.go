package eino

import (
	"context"
	"crypto/sha256"
	"encoding/gob"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	agentcontext "nonoka-im/internal/agent/context"
	"nonoka-im/internal/agent/runtime"
	"nonoka-im/internal/agent/tools"
)

type spanContextKey struct{}

var ErrSummary = errors.New("summarization failed")

func init() {
	// Checkpoints gob-encode the interrupt payload and the resume decision;
	// both cross interface boundaries inside the Eino runtime state.
	gob.Register(approvalInterruptInfo{})
	gob.Register(approvalDecision{})
}

type Runtime struct {
	model                      model.ToolCallingChatModel
	tools                      []tool.InvokableTool
	apiKey, baseURL, modelName string
	maxTokens                  int

	// mcpTools resolves per-run MCP bindings into guarded tools; nil disables
	// MCP tools. skillBackend builds the skill middleware backend for the
	// skills bound to the agent version; nil disables skills.
	mcpTools     func(ctx context.Context, bindings []runtime.MCPTool) ([]tool.BaseTool, error)
	skillBackend func(bindings []runtime.SkillBinding) (skill.Backend, error)

	// checkpoints persists Eino execution state across approval interrupts;
	// nil disables interrupt/resume (tools that need approval then fail).
	checkpoints adk.CheckPointStore

	mu     sync.Mutex
	models map[modelCacheKey]model.ToolCallingChatModel
	order  []modelCacheKey // FIFO eviction order; capped at modelCacheLimit.
}

const modelCacheLimit = 64

type modelCacheKey struct {
	baseURL    string
	apiKeyHash [32]byte
	modelName  string
	maxTokens  int
}

// NewRuntime creates the OpenAI-compatible Eino runtime without tools. Use
// NewRuntimeWithTools in production when authorized IM tool backends are wired.
func NewRuntime(ctx context.Context, apiKey, baseURL, modelName string) (*Runtime, error) {
	return NewRuntimeWithTools(ctx, apiKey, baseURL, modelName, nil, nil)
}

// NewRuntimeWithTools builds a runtime whose tools are authorized through the
// guard on every call. A nil guard is only safe when backend is nil.
func NewRuntimeWithTools(ctx context.Context, apiKey, baseURL, modelName string, backend tools.Backend, guard tools.Guard) (*Runtime, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("model API key is required")
	}
	maxTokens := 4096
	m, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{APIKey: apiKey, BaseURL: baseURL, Model: modelName, MaxCompletionTokens: &maxTokens})
	if err != nil {
		return nil, err
	}
	r := &Runtime{model: m, apiKey: apiKey, baseURL: baseURL, modelName: modelName, maxTokens: maxTokens}
	if backend != nil {
		r.tools = tools.New(backend, guard, 8192)
	}
	return r, nil
}

// SetMCPToolProvider installs the resolver for per-run MCP tool bindings.
func (r *Runtime) SetMCPToolProvider(p func(ctx context.Context, bindings []runtime.MCPTool) ([]tool.BaseTool, error)) {
	r.mcpTools = p
}

// SetSkillBackendFactory installs the factory building skill middleware
// backends from the skills bound to the agent version.
func (r *Runtime) SetSkillBackendFactory(f func(bindings []runtime.SkillBinding) (skill.Backend, error)) {
	r.skillBackend = f
}

// SetCheckPointStore installs the store used to persist execution state when
// approval-requiring tools interrupt a run.
func (r *Runtime) SetCheckPointStore(store adk.CheckPointStore) {
	r.checkpoints = store
}

// chatModel resolves the model for a run. Agent-owned base URL / API key /
// model / max tokens override the runtime defaults; overrides are cached with
// a bounded FIFO cache keyed on the full combination.
func (r *Runtime) chatModel(ctx context.Context, in runtime.Input) (model.ToolCallingChatModel, error) {
	// No agent-owned overrides: use the runtime default model as-is.
	if in.BaseURL == "" && in.APIKey == "" && (in.Model == "" || in.Model == r.modelName) {
		return r.model, nil
	}
	baseURL := in.BaseURL
	if baseURL == "" {
		baseURL = r.baseURL
	}
	apiKey := in.APIKey
	if apiKey == "" {
		apiKey = r.apiKey
	}
	modelName := in.Model
	if modelName == "" {
		modelName = r.modelName
	}
	maxTokens := in.MaxTokens
	if maxTokens <= 0 {
		maxTokens = in.OutputTokenBudget
	}
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("model API key is required")
	}
	key := modelCacheKey{baseURL: baseURL, apiKeyHash: sha256.Sum256([]byte(apiKey)), modelName: modelName, maxTokens: maxTokens}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.models == nil {
		r.models = make(map[modelCacheKey]model.ToolCallingChatModel)
	}
	if cached, ok := r.models[key]; ok {
		return cached, nil
	}
	m, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{APIKey: apiKey, BaseURL: baseURL, Model: modelName, MaxCompletionTokens: &maxTokens})
	if err != nil {
		return nil, fmt.Errorf("create model %q: %w", modelName, err)
	}
	r.models[key] = m
	r.order = append(r.order, key)
	for len(r.order) > modelCacheLimit {
		delete(r.models, r.order[0])
		r.order = r.order[1:]
	}
	return m, nil
}

// approvalInterruptInfo is the user-facing payload an approval-carrying tool
// raises when it interrupts. It surfaces in the AgentEvent interrupt contexts
// and lets the harness build the approval card without touching checkpoint
// internals.
type approvalInterruptInfo struct {
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
}

// approvalDecision is handed back to the interrupted tool on resume.
type approvalDecision struct {
	Approved bool `json:"approved"`
}

// approvalTool wraps a tool whose calls require human confirmation. The first
// execution interrupts with the call summary; the resume delivers the user's
// decision, after which the wrapped tool either runs or reports a rejection.
type approvalTool struct {
	wrapped tool.InvokableTool
}

func (t *approvalTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.wrapped.Info(ctx)
}

func (t *approvalTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	wasInterrupted, _, _ := tool.GetInterruptState[any](ctx)
	if !wasInterrupted {
		name := "tool"
		if info, err := t.wrapped.Info(ctx); err == nil && info != nil {
			name = info.Name
		}
		return "", tool.Interrupt(ctx, approvalInterruptInfo{Tool: name, Summary: summarizeArgs(args)})
	}
	isTarget, _, decision := tool.GetResumeContext[approvalDecision](ctx)
	if !isTarget {
		// A different interrupt point is being resumed; preserve our state.
		return "", tool.Interrupt(ctx, nil)
	}
	if !decision.Approved {
		return "The user rejected this tool call.", nil
	}
	return t.wrapped.InvokableRun(ctx, args, opts...)
}

// summarizeArgs renders tool arguments for the approval card. The raw
// arguments stay inside the checkpoint only; the card never carries them.
func summarizeArgs(args string) string {
	const maxSummary = 300
	compact := strings.Join(strings.Fields(args), " ")
	if len(compact) > maxSummary {
		compact = compact[:maxSummary] + "…"
	}
	return compact
}

// runState collects cross-phase outputs of one execution.
type runState struct {
	summary string
	usage   runtime.Usage
}

// buildAgent assembles the ChatModelAgent for a run or resume. withSummary
// enables the summarization middleware (fresh runs only; a resumed execution
// restores its compacted state from the checkpoint).
func (r *Runtime) buildAgent(ctx context.Context, in runtime.Input, withSummary bool) (adk.Agent, *runState, error) {
	chatModel, err := r.chatModel(ctx, in)
	if err != nil {
		return nil, nil, err
	}
	toolCtx := tools.WithRunContext(ctx, tools.RunContext{ActorUserID: in.ActorUserID, AgentID: in.AgentID, BotID: in.BotID, EventID: in.EventID, Topic: in.Topic})
	state := &runState{}
	var handlers []adk.ChatModelAgentMiddleware
	if withSummary && in.NeedsSummary && !in.DisableSummary && len(in.History) > 0 {
		trigger := in.TriggerTokens
		if trigger <= 0 {
			trigger = 38000
		}
		mw, err := summarization.New(toolCtx, &summarization.Config{
			Model:                summaryTracker{model: chatModel},
			PreserveUserMessages: &summarization.PreserveUserMessages{Enabled: false},
			Trigger:              &summarization.TriggerCondition{ContextTokens: trigger},
			TokenCounter: func(_ context.Context, input *summarization.TokenCounterInput) (int, error) {
				if state.summary != "" {
					return 0, nil
				}
				// Count the actually assembled messages (including the system
				// instruction) so the middleware triggers on real content size,
				// matching the builder's coarse budget estimate.
				total := 0
				for _, msg := range input.Messages {
					total += agentcontext.EstimateTokens(messageText(msg))
				}
				return total, nil
			},
			GenModelInput: func(_ context.Context, system, user adk.Message, original []adk.Message) ([]adk.Message, error) {
				// The final message is the current input and must not enter the snapshot.
				request := append([]adk.Message{system}, original[:len(original)-1]...)
				return append(request, user), nil
			},
			Finalize: func(_ context.Context, original []adk.Message, compacted adk.Message) ([]adk.Message, error) {
				result := make([]adk.Message, 0, 3)
				for _, msg := range original {
					if msg.Role == schema.System {
						result = append(result, msg)
					}
				}
				return append(result, compacted, original[len(original)-1]), nil
			},
			Callback: func(_ context.Context, _, after adk.ChatModelAgentState) error {
				for _, msg := range after.Messages {
					if len(msg.UserInputMultiContent) == 0 {
						continue
					}
					state.summary = msg.UserInputMultiContent[0].Text
					break
				}
				return nil
			},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("create summarization middleware: %w", err)
		}
		handlers = append(handlers, mw)
	}
	instruction := in.Instruction
	if instruction == "" {
		instruction = "You are the Nonoka IM assistant. Use concise, helpful answers. Use tools only when needed. Never claim a tool action succeeded unless it returned success."
	}
	if len(in.Skills) > 0 && r.skillBackend != nil {
		backend, err := r.skillBackend(in.Skills)
		if err != nil {
			return nil, nil, fmt.Errorf("load skills: %w", err)
		}
		mw, err := skill.NewMiddleware(toolCtx, &skill.Config{Backend: backend})
		if err != nil {
			return nil, nil, fmt.Errorf("create skill middleware: %w", err)
		}
		handlers = append(handlers, mw)
	}
	agentTools, err := r.resolveTools(toolCtx, in)
	if err != nil {
		return nil, nil, err
	}
	agent, err := adk.NewChatModelAgent(toolCtx, &adk.ChatModelAgentConfig{
		Name:          "nonoka-assistant",
		Description:   "Nonoka IM conversation assistant",
		Instruction:   instruction,
		Model:         chatModel,
		Handlers:      handlers,
		MaxIterations: 5,
		ToolsConfig:   adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: agentTools}},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create Eino agent: %w", err)
	}
	return agent, state, nil
}

// resolveTools builds the tool list for a run: authorized local tools, the
// run's MCP bindings, and approval wrappers around every tool that requires
// human confirmation.
func (r *Runtime) resolveTools(toolCtx context.Context, in runtime.Input) ([]tool.BaseTool, error) {
	agentTools := allowedTools(toolCtx, r.tools, in.AllowedTools, in.RestrictTools)
	if len(in.MCPTools) > 0 {
		if r.mcpTools == nil {
			return nil, fmt.Errorf("mcp tools not available")
		}
		extra, err := r.mcpTools(toolCtx, in.MCPTools)
		if err != nil {
			return nil, fmt.Errorf("resolve mcp tools: %w", err)
		}
		agentTools = append(agentTools, extra...)
	}
	if len(in.ApprovalTools) == 0 {
		return agentTools, nil
	}
	approval := make(map[string]bool, len(in.ApprovalTools))
	for _, name := range in.ApprovalTools {
		approval[name] = true
	}
	wrapped := make([]tool.BaseTool, 0, len(agentTools))
	for _, t := range agentTools {
		info, err := t.Info(toolCtx)
		if err == nil && info != nil && approval[info.Name] {
			if inv, ok := t.(tool.InvokableTool); ok {
				wrapped = append(wrapped, &approvalTool{wrapped: inv})
				continue
			}
		}
		wrapped = append(wrapped, t)
	}
	return wrapped, nil
}

// runLoop drains the runner event stream. An approval interrupt returns with
// StatusInterrupted and the root-cause interrupt details; model and tool
// errors propagate, with summary failures already tagged by summaryTracker.
func runLoop(iterator *adk.AsyncIterator[*adk.AgentEvent], state *runState, summaryThrough uint64) (runtime.Result, error) {
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
			// via errors.Is; every other event error propagates as-is.
			return runtime.Result{}, event.Err
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			root := rootInterruptContext(event.Action.Interrupted.InterruptContexts)
			if root != nil {
				info, _ := root.Info.(approvalInterruptInfo)
				return runtime.Result{Status: runtime.StatusInterrupted, Interrupt: &runtime.InterruptInfo{ToolCallID: root.ID, Tool: info.Tool, Summary: info.Summary}, Summary: state.summary, SummaryThroughTurnID: summaryThrough, Usage: state.usage}, nil
			}
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		msg, err := event.Output.MessageOutput.GetMessage()
		if err != nil {
			return runtime.Result{}, fmt.Errorf("read Eino output: %w", err)
		}
		if msg != nil && msg.Role == schema.Assistant && len(msg.ToolCalls) == 0 {
			final = msg.Content
		}
	}
	if strings.TrimSpace(final) == "" {
		return runtime.Result{}, fmt.Errorf("empty model response")
	}
	return runtime.Result{Text: strings.TrimSpace(final), Status: runtime.StatusCompleted, Summary: state.summary, SummaryThroughTurnID: summaryThrough, Usage: state.usage}, nil
}

func rootInterruptContext(contexts []*adk.InterruptCtx) *adk.InterruptCtx {
	for _, c := range contexts {
		if c != nil && c.IsRootCause {
			return c
		}
	}
	return nil
}

func (r *Runtime) Run(ctx context.Context, in runtime.Input) (runtime.Result, error) {
	agent, state, err := r.buildAgent(ctx, in, true)
	if err != nil {
		return runtime.Result{}, err
	}
	msgs := make([]adk.Message, 0, len(in.History)*2+2)
	if in.Summary != "" {
		msgs = append(msgs, schema.UserMessage("Previous conversation summary:\n"+in.Summary))
	}
	for _, t := range in.History {
		msgs = append(msgs, schema.UserMessage(t.Input), schema.AssistantMessage(t.Output, nil))
	}
	msgs = append(msgs, schema.UserMessage(in.Text))
	toolCtx := tools.WithRunContext(ctx, tools.RunContext{ActorUserID: in.ActorUserID, AgentID: in.AgentID, BotID: in.BotID, EventID: in.EventID, Topic: in.Topic})
	runnerCfg := adk.RunnerConfig{Agent: agent, EnableStreaming: false}
	if r.checkpoints != nil && in.CheckPointID != "" {
		runnerCfg.CheckPointStore = r.checkpoints
	}
	runner := adk.NewRunner(toolCtx, runnerCfg)
	opts := []adk.AgentRunOption{adk.WithCallbacks(modelSpanHandler(&state.usage))}
	if in.CheckPointID != "" {
		opts = append(opts, adk.WithCheckPointID(in.CheckPointID))
	}
	return runLoop(runner.Run(toolCtx, msgs, opts...), state, in.SummaryThroughTurnID)
}

// Resume continues an interrupted run from its checkpoint. The agent is
// rebuilt from the same frozen version snapshot so the resumed tool set
// matches the interrupted execution; conversation state comes from the
// checkpoint, not from history.
func (r *Runtime) Resume(ctx context.Context, in runtime.ResumeInput) (runtime.Result, error) {
	if in.CheckPointID == "" || in.InterruptID == "" {
		return runtime.Result{}, fmt.Errorf("resume requires checkpoint and interrupt IDs")
	}
	agent, state, err := r.buildAgent(ctx, in.Input, false)
	if err != nil {
		return runtime.Result{}, err
	}
	toolCtx := tools.WithRunContext(ctx, tools.RunContext{ActorUserID: in.ActorUserID, AgentID: in.AgentID, BotID: in.BotID, EventID: in.EventID, Topic: in.Topic})
	runnerCfg := adk.RunnerConfig{Agent: agent, EnableStreaming: false}
	if r.checkpoints != nil {
		runnerCfg.CheckPointStore = r.checkpoints
	}
	runner := adk.NewRunner(toolCtx, runnerCfg)
	iterator, err := runner.ResumeWithParams(toolCtx, in.CheckPointID, &adk.ResumeParams{
		Targets: map[string]any{in.InterruptID: approvalDecision{Approved: in.Approved}},
	}, adk.WithCallbacks(modelSpanHandler(&state.usage)))
	if err != nil {
		return runtime.Result{}, fmt.Errorf("resume run: %w", err)
	}
	return runLoop(iterator, state, in.SummaryThroughTurnID)
}

// summaryTracker wraps the model used by the summarization middleware and
// tags genuine summary generation failures by wrapping them in ErrSummary, so
// the run loop and the harness can recognize them with errors.Is instead of a
// global "did a summary fail" flag that would misattribute unrelated errors.
type summaryTracker struct {
	model model.BaseChatModel
}

func (t summaryTracker) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	msg, err := t.model.Generate(ctx, input, opts...)
	if err != nil {
		return msg, fmt.Errorf("%w: %v", ErrSummary, err)
	}
	return msg, nil
}

func (t summaryTracker) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, err := t.model.Stream(ctx, input, opts...)
	if err != nil {
		return reader, fmt.Errorf("%w: %v", ErrSummary, err)
	}
	return reader, nil
}

// messageText extracts the textual content of a message, including multimodal
// user input parts.
func messageText(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(msg.Content)
	for _, part := range msg.UserInputMultiContent {
		b.WriteString(part.Text)
	}
	return b.String()
}

func baseTools(ts []tool.InvokableTool) []tool.BaseTool {
	result := make([]tool.BaseTool, len(ts))
	for i := range ts {
		result[i] = ts[i]
	}
	return result
}

func allowedTools(ctx context.Context, ts []tool.InvokableTool, names []string, restrict bool) []tool.BaseTool {
	if !restrict {
		return baseTools(ts)
	}
	result := make([]tool.BaseTool, 0, len(ts))
	for _, t := range ts {
		info, err := t.Info(ctx)
		if err == nil && info != nil && slices.Contains(names, info.Name) {
			result = append(result, t)
		}
	}
	return result
}

// modelSpanHandler records model execution timing and failure status without
// capturing prompts, outputs, tool arguments, or provider error text. Usage is
// only counted for ChatModel component events: the runner also reports the
// final message at the agent level, which would double-count tokens.
func modelSpanHandler(usage *runtime.Usage) callbacks.Handler {
	tracer := otel.Tracer("nonoka-im/agent/eino")
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
			if info.Component != components.ComponentOfChatModel {
				return ctx
			}
			ctx, span := tracer.Start(ctx, "agent.chat_model.generate", trace.WithAttributes(attribute.String("gen_ai.operation.name", "chat")))
			return context.WithValue(ctx, spanContextKey{}, span)
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			if info.Component != components.ComponentOfChatModel {
				return ctx
			}
			if converted := model.ConvCallbackOutput(output); converted != nil {
				if tokens := converted.TokenUsage; tokens != nil {
					usage.InputTokens += tokens.PromptTokens
					usage.OutputTokens += tokens.CompletionTokens
					usage.TotalTokens += tokens.TotalTokens
				} else if converted.Message != nil && converted.Message.ResponseMeta != nil && converted.Message.ResponseMeta.Usage != nil {
					tokens := converted.Message.ResponseMeta.Usage
					usage.InputTokens += tokens.PromptTokens
					usage.OutputTokens += tokens.CompletionTokens
					usage.TotalTokens += tokens.TotalTokens
				}
			}
			if span, ok := ctx.Value(spanContextKey{}).(trace.Span); ok {
				span.End()
			}
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, _ error) context.Context {
			if info.Component != components.ComponentOfChatModel {
				return ctx
			}
			if span, ok := ctx.Value(spanContextKey{}).(trace.Span); ok {
				span.SetStatus(codes.Error, "model generation failed")
				span.End()
			}
			return ctx
		}).Build()
}
