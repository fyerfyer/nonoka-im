package eino

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/callbacks"
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

type Runtime struct {
	model                      model.ToolCallingChatModel
	tools                      []tool.InvokableTool
	apiKey, baseURL, modelName string

	mu     sync.Mutex
	models map[string]model.ToolCallingChatModel
}

// NewRuntime creates the OpenAI-compatible Eino runtime without tools. Use
// NewRuntimeWithTools in production when authorized IM tool backends are wired.
func NewRuntime(ctx context.Context, apiKey, baseURL, modelName string) (*Runtime, error) {
	return NewRuntimeWithTools(ctx, apiKey, baseURL, modelName, nil)
}

func NewRuntimeWithTools(ctx context.Context, apiKey, baseURL, modelName string, backend tools.Backend) (*Runtime, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("model API key is required")
	}
	maxTokens := 4096
	m, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{APIKey: apiKey, BaseURL: baseURL, Model: modelName, MaxCompletionTokens: &maxTokens})
	if err != nil {
		return nil, err
	}
	r := &Runtime{model: m, apiKey: apiKey, baseURL: baseURL, modelName: modelName}
	if backend != nil {
		r.tools = tools.New(backend, 8192)
	}
	return r, nil
}

// chatModel returns the runtime's default model or a cached per-request model.
// The completion-token budget of the first creation is kept for the lifetime
// of the cached model.
func (r *Runtime) chatModel(ctx context.Context, modelName string, maxTokens int) (model.ToolCallingChatModel, error) {
	if modelName == "" || modelName == r.modelName {
		return r.model, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.models == nil {
		r.models = make(map[string]model.ToolCallingChatModel)
	}
	if cached, ok := r.models[modelName]; ok {
		return cached, nil
	}
	m, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{APIKey: r.apiKey, BaseURL: r.baseURL, Model: modelName, MaxCompletionTokens: &maxTokens})
	if err != nil {
		return nil, fmt.Errorf("create model %q: %w", modelName, err)
	}
	r.models[modelName] = m
	return m, nil
}

func (r *Runtime) Run(ctx context.Context, in runtime.Input) (runtime.Result, error) {
	maxTokens := in.OutputTokenBudget
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	chatModel, err := r.chatModel(ctx, in.Model, maxTokens)
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
	var handlers []adk.ChatModelAgentMiddleware
	var summary string
	var summaryGenFailed bool
	var usage runtime.Usage
	if in.NeedsSummary && !in.DisableSummary && len(in.History) > 0 {
		trigger := in.TriggerTokens
		if trigger <= 0 {
			trigger = 38000
		}
		mw, err := summarization.New(toolCtx, &summarization.Config{
			Model:                summaryTracker{model: chatModel, failed: &summaryGenFailed},
			PreserveUserMessages: &summarization.PreserveUserMessages{Enabled: false},
			Trigger:              &summarization.TriggerCondition{ContextTokens: trigger},
			TokenCounter: func(_ context.Context, input *summarization.TokenCounterInput) (int, error) {
				if summary != "" {
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
					summary = msg.UserInputMultiContent[0].Text
					break
				}
				return nil
			},
		})
		if err != nil {
			return runtime.Result{}, fmt.Errorf("create summarization middleware: %w", err)
		}
		handlers = append(handlers, mw)
	}
	instruction := in.Instruction
	if instruction == "" {
		instruction = "You are the Nonoka IM assistant. Use concise, helpful answers. Use tools only when needed. Never claim a tool action succeeded unless it returned success."
	}
	agent, err := adk.NewChatModelAgent(toolCtx, &adk.ChatModelAgentConfig{
		Name:          "nonoka-assistant",
		Description:   "Nonoka IM conversation assistant",
		Instruction:   instruction,
		Model:         chatModel,
		Handlers:      handlers,
		MaxIterations: 5,
		ToolsConfig:   adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: allowedTools(toolCtx, r.tools, in.AllowedTools, in.RestrictTools)}},
	})
	if err != nil {
		return runtime.Result{}, fmt.Errorf("create Eino agent: %w", err)
	}
	runner := adk.NewRunner(toolCtx, adk.RunnerConfig{Agent: agent, EnableStreaming: false})
	iterator := runner.Run(toolCtx, msgs, adk.WithCallbacks(modelSpanHandler(&usage)))
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
			if summaryGenFailed && summary == "" {
				return runtime.Result{}, fmt.Errorf("%w: %v", ErrSummary, event.Err)
			}
			return runtime.Result{}, event.Err
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
	return runtime.Result{Text: strings.TrimSpace(final), Summary: summary, SummaryThroughTurnID: in.SummaryThroughTurnID, Usage: usage}, nil
}

// summaryTracker wraps the model used by the summarization middleware and
// records whether the summary generation call itself failed, so the run loop
// only maps genuine summary failures to ErrSummary instead of every event
// error that happens after a summary was attempted.
type summaryTracker struct {
	model  model.BaseChatModel
	failed *bool
}

func (t summaryTracker) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	msg, err := t.model.Generate(ctx, input, opts...)
	if err != nil {
		*t.failed = true
	}
	return msg, err
}

func (t summaryTracker) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, err := t.model.Stream(ctx, input, opts...)
	if err != nil {
		*t.failed = true
	}
	return reader, err
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
// capturing prompts, outputs, tool arguments, or provider error text.
func modelSpanHandler(usage *runtime.Usage) callbacks.Handler {
	tracer := otel.Tracer("nonoka-im/agent/eino")
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
			ctx, span := tracer.Start(ctx, "agent.chat_model.generate", trace.WithAttributes(attribute.String("gen_ai.operation.name", "chat")))
			return context.WithValue(ctx, spanContextKey{}, span)
		}).
		OnEndFn(func(ctx context.Context, _ *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
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
		OnErrorFn(func(ctx context.Context, _ *callbacks.RunInfo, _ error) context.Context {
			if span, ok := ctx.Value(spanContextKey{}).(trace.Span); ok {
				span.SetStatus(codes.Error, "model generation failed")
				span.End()
			}
			return ctx
		}).Build()
}
