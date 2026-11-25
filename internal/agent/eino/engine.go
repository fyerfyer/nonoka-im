package eino

import (
	"context"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentmetrics "nonoka-im/internal/agent/metrics"
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
	metrics     *agentmetrics.Metrics

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
	Metrics             *agentmetrics.Metrics
}

// New constructs the Eino implementation of port.Runtime.
func New(_ context.Context, deps Deps) (*Engine, error) {
	return &Engine{
		tools: deps.Tools, newModel: deps.NewChatModel, mcpTools: deps.MCPToolProvider,
		skillBackend: deps.SkillBackendFactory, checkpoints: deps.Checkpoints,
		ledger: deps.Ledger, markSideEffect: deps.MarkSideEffect, toolCatalog: deps.ToolCatalog,
		metrics: deps.Metrics,
	}, nil
}

func runLoop(iterator *adk.AsyncIterator[*adk.AgentEvent], state *runState, summaryThrough uint64, sink port.EventSink) (port.Result, error) {
	var final string
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if err := state.usageError(); err != nil {
			return port.Result{}, err
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
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		var msg *schema.Message
		if mv.IsStreaming {
			streams := mv.MessageStream.Copy(2)
			readStream, concatStream := streams[0], streams[1]
			for {
				chunk, err := readStream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					readStream.Close()
					concatStream.Close()
					return port.Result{}, fmt.Errorf("read Eino stream: %w", err)
				}
				if chunk != nil && chunk.Role == schema.Assistant && chunk.Content != "" && sink != nil {
					sink(port.StreamEvent{Type: port.EventDelta, Delta: chunk.Content})
				}
			}
			readStream.Close()
			var err error
			msg, err = schema.ConcatMessageStream(concatStream)
			if err != nil {
				return port.Result{}, fmt.Errorf("concat Eino stream: %w", err)
			}
		} else {
			msg = mv.Message
		}
		if msg != nil && msg.Role == schema.Assistant && len(msg.ToolCalls) == 0 {
			final = msg.Content
		}
	}
	if strings.TrimSpace(final) == "" {
		return port.Result{}, fmt.Errorf("empty model response")
	}
	if err := state.usageError(); err != nil {
		return port.Result{}, err
	}
	return port.Result{Text: strings.TrimSpace(final), Status: port.StatusCompleted, Summary: state.summary, SummaryThroughTurnID: summaryThrough, Usage: state.usage}, nil
}

func userMessage(text string, attachments []port.Attachment) *schema.Message {
	images := make([]port.Attachment, 0, len(attachments))
	for _, a := range attachments {
		if a.Kind == "image" && len(a.Data) > 0 {
			images = append(images, a)
		}
	}
	if len(images) == 0 {
		return schema.UserMessage(text)
	}
	if strings.TrimSpace(text) == "" {
		text = "Please look at the attached image."
	}
	parts := make([]schema.MessageInputPart, 0, len(images)+1)
	parts = append(parts, schema.MessageInputPart{Type: schema.ChatMessagePartTypeText, Text: text})
	for _, a := range images {
		data := base64.StdEncoding.EncodeToString(a.Data)
		mime := a.Mime
		if mime == "" {
			mime = "image/png"
		}
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{Base64Data: &data, MIMEType: mime},
				Detail:            schema.ImageURLDetailAuto,
			},
		})
	}
	return &schema.Message{Role: schema.User, UserInputMultiContent: parts}
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
	msgs = append(msgs, userMessage(in.Text, in.Attachments))
	toolCtx := tools.WithRunContext(ctx, tools.RunContext{ActorUserID: in.ActorUserID, AgentID: in.AgentID, BotID: in.BotID, RunID: in.RunID, AttemptID: in.AttemptID, EventID: in.EventID, Topic: in.Topic})
	runnerCfg := adk.RunnerConfig{Agent: agent, EnableStreaming: true}
	if r.checkpoints != nil && in.CheckPointID != "" {
		runnerCfg.CheckPointStore = r.checkpoints
	}
	runner := adk.NewRunner(toolCtx, runnerCfg)
	opts := []adk.AgentRunOption{adk.WithCallbacks(
		modelSpanHandler(&state.usage, &state.modelError, state),
		toolEventCallbacks(in.OnEvent, r.summarizeArgs),
	)}
	if in.CheckPointID != "" {
		opts = append(opts, adk.WithCheckPointID(in.CheckPointID))
	}
	return runLoop(runner.Run(toolCtx, msgs, opts...), state, in.SummaryThroughTurnID, in.OnEvent)
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
	runnerCfg := adk.RunnerConfig{Agent: agent, EnableStreaming: true}
	if r.checkpoints != nil {
		runnerCfg.CheckPointStore = r.checkpoints
	}
	runner := adk.NewRunner(toolCtx, runnerCfg)
	iterator, err := runner.ResumeWithParams(toolCtx, in.CheckPointID, &adk.ResumeParams{
		Targets: map[string]any{in.InterruptID: approvalDecision{Approved: in.Approved}},
	}, adk.WithCallbacks(
		modelSpanHandler(&state.usage, &state.modelError, state),
		toolEventCallbacks(in.OnEvent, r.summarizeArgs),
	))
	if err != nil {
		return port.Result{}, fmt.Errorf("resume run: %w", err)
	}
	return runLoop(iterator, state, in.SummaryThroughTurnID, in.OnEvent)
}

// summaryTracker wraps the model used by the summarization middleware and
// tags genuine summary generation failures by wrapping them in ErrSummary, so
// the run loop and the harness can recognize them with errors.Is instead of a
