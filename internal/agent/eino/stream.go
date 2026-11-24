package eino

import (
	"context"

	"nonoka-im/internal/agent/port"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

// toolEventCallbacks emits transport-neutral tool progress. Raw arguments are
// never emitted; only the catalog summarizer's redacted summary is forwarded.
func toolEventCallbacks(sink port.EventSink, summarize func(toolName, args string) (string, string)) callbacks.Handler {
	if sink == nil {
		return callbacks.NewHandlerBuilder().Build()
	}
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			if info.Component != components.ComponentOfTool {
				return ctx
			}
			summary := ""
			if summarize != nil {
				args := ""
				if converted := tool.ConvCallbackInput(input); converted != nil {
					args = converted.ArgumentsInJSON
				}
				summary, _ = summarize(info.Name, args)
			}
			sink(port.StreamEvent{Type: port.EventToolStarted, Tool: info.Name, Summary: summary})
			return ctx
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackOutput) context.Context {
			if info.Component != components.ComponentOfTool {
				return ctx
			}
			sink(port.StreamEvent{Type: port.EventToolFinished, Tool: info.Name, Status: "ok"})
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			if info.Component != components.ComponentOfTool {
				return ctx
			}
			// approvalTool interrupts via compose/tool.Interrupt, which surfaces
			// as an error callback. The following approval_required event is the
			// real terminal state for this step, so do not report it as a tool
			// failure.
			if _, interrupted := compose.ExtractInterruptInfo(err); interrupted {
				return ctx
			}
			sink(port.StreamEvent{Type: port.EventToolFinished, Tool: info.Name, Status: "error"})
			return ctx
		}).
		Build()
}
