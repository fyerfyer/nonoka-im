package eino

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"nonoka-im/internal/agent/port"
)

const modelCacheLimit = 64

// modelRetryMax is the number of in-process ChatModel retries. It covers
// provider 429/5xx and network errors without replaying the whole run (and its
// already-executed tools). Run-level retry remains for infra/crash recovery.
const modelRetryMax = 2

type modelCacheKey struct {
	baseURL    string
	apiKeyHash [32]byte
	modelName  string
	maxTokens  int
}

func (r *Engine) chatModel(ctx context.Context, in port.Input) (model.ToolCallingChatModel, error) {
	endpoint := in.Model
	baseURL := strings.TrimSpace(endpoint.BaseURL)
	modelName := strings.TrimSpace(endpoint.Model)
	if r.newModel == nil && (baseURL == "" || modelName == "") {
		return nil, fmt.Errorf("model endpoint is not configured")
	}
	apiKey := endpoint.APIKey
	maxTokens := endpoint.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
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
	var m model.ToolCallingChatModel
	var err error
	if r.newModel != nil {
		m, err = r.newModel(ctx, port.ModelEndpoint{BaseURL: baseURL, APIKey: apiKey, Model: modelName, MaxTokens: maxTokens}, maxTokens)
	} else {
		m, err = openai.NewChatModel(ctx, &openai.ChatModelConfig{APIKey: apiKey, BaseURL: baseURL, Model: modelName, MaxCompletionTokens: &maxTokens})
	}
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

func retryableModelError(_ context.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch code := apiErr.HTTPStatusCode; {
		case code == 408, code == 409, code == 429, code >= 500:
			return true
		case code >= 400:
			return false
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	// Unknown provider errors are treated as transient by default, matching
	// Eino's own default retry policy.
	return true
}

// modelSpanHandler records model execution timing and failure status without
// capturing prompts, outputs, tool arguments, or provider error text. Usage is
// only counted for ChatModel component events: the runner also reports the
// final message at the agent level, which would double-count tokens.
func modelSpanHandler(usage *port.Usage, modelError *atomic.Bool) callbacks.Handler {
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
			// A subsequent successful attempt clears the error flag; otherwise
			// a model retry that eventually succeeds would leave a stale marker
			// that misclassifies a later tool error as a model error.
			modelError.Store(false)
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
			modelError.Store(true)
			if span, ok := ctx.Value(spanContextKey{}).(trace.Span); ok {
				span.SetStatus(codes.Error, "model generation failed")
				span.End()
			}
			return ctx
		}).Build()
}
