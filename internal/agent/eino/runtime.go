package eino

import (
	"context"
	"fmt"
	"strings"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"nonoka-im/internal/agent/runtime"
	"nonoka-im/internal/agent/tools"
)

type spanContextKey struct{}

type Runtime struct {
	model model.ToolCallingChatModel
	tools []tool.InvokableTool
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
	maxTokens := 1024
	m, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{APIKey: apiKey, BaseURL: baseURL, Model: modelName, MaxCompletionTokens: &maxTokens})
	if err != nil {
		return nil, err
	}
	r := &Runtime{model: m}
	if backend != nil {
		r.tools = tools.New(backend)
	}
	return r, nil
}

func (r *Runtime) Run(ctx context.Context, in runtime.Input) (runtime.Result, error) {
	msgs := make([]adk.Message, 0, len(in.History)*2+1)
	for _, t := range in.History {
		msgs = append(msgs, schema.UserMessage(t.Input), schema.AssistantMessage(t.Output, nil))
	}
	msgs = append(msgs, schema.UserMessage(in.Text))
	toolCtx := tools.WithRunContext(ctx, tools.RunContext{ActorUserID: in.ActorUserID, AgentID: in.AgentID, BotID: in.BotID, EventID: in.EventID, Topic: in.Topic})
	agent, err := adk.NewChatModelAgent(toolCtx, &adk.ChatModelAgentConfig{
		Name:          "nonoka-assistant",
		Description:   "Nonoka IM conversation assistant",
		Instruction:   "You are the Nonoka IM assistant. Use concise, helpful answers. Use tools only when needed. Never claim a tool action succeeded unless it returned success.",
		Model:         r.model,
		MaxIterations: 5,
		ToolsConfig:   adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: baseTools(r.tools)}},
	})
	if err != nil {
		return runtime.Result{}, fmt.Errorf("create Eino agent: %w", err)
	}
	runner := adk.NewRunner(toolCtx, adk.RunnerConfig{Agent: agent, EnableStreaming: false})
	iterator := runner.Run(toolCtx, msgs, adk.WithCallbacks(modelSpanHandler()))
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
	return runtime.Result{Text: strings.TrimSpace(final)}, nil
}

func baseTools(ts []tool.InvokableTool) []tool.BaseTool {
	result := make([]tool.BaseTool, len(ts))
	for i := range ts {
		result[i] = ts[i]
	}
	return result
}

// modelSpanHandler records model execution timing and failure status without
// capturing prompts, outputs, tool arguments, or provider error text.
func modelSpanHandler() callbacks.Handler {
	tracer := otel.Tracer("nonoka-im/agent/eino")
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
			ctx, span := tracer.Start(ctx, "agent.chat_model.generate", trace.WithAttributes(attribute.String("gen_ai.operation.name", "chat")))
			return context.WithValue(ctx, spanContextKey{}, span)
		}).
		OnEndFn(func(ctx context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackOutput) context.Context {
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
