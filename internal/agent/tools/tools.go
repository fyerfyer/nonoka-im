// Package tools contains the narrow, authorization-aware IM tool surface
// exposed to an Agent. The backend implementations remain in the IM layer;
// these adapters never accept an actor ID from model arguments.
package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

type RunContext struct {
	ActorUserID int64
	AgentID     int64
	BotID       int64
	EventID     string
	Topic       string
}
type contextKey struct{}

func WithRunContext(ctx context.Context, r RunContext) context.Context {
	return context.WithValue(ctx, contextKey{}, r)
}
func GetRunContext(ctx context.Context) (RunContext, bool) {
	r, ok := ctx.Value(contextKey{}).(RunContext)
	return r, ok
}

type SearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}
type ContextRequest struct {
	Limit int `json:"limit,omitempty"`
}
type SendRequest struct {
	Content string `json:"content"`
}
type SearchResult struct {
	Results []string `json:"results"`
}
type ContextResult struct {
	Messages []string `json:"messages"`
}
type SendResult struct {
	Accepted bool `json:"accepted"`
}

type Backend interface {
	SearchMessages(context.Context, RunContext, string, int) ([]string, error)
	ConversationContext(context.Context, RunContext, int) ([]string, error)
	SendMessage(context.Context, RunContext, string) error
}

func New(backend Backend) []tool.InvokableTool {
	search := utils.NewTool(&schema.ToolInfo{Name: "search_messages", Desc: "Search messages by keyword in the current authorized conversation."}, func(ctx context.Context, in SearchRequest) (SearchResult, error) {
		r, ok := GetRunContext(ctx)
		if !ok {
			return SearchResult{}, fmt.Errorf("run context missing")
		}
		if in.Query == "" {
			return SearchResult{}, fmt.Errorf("query is required")
		}
		if in.Limit <= 0 || in.Limit > 20 {
			in.Limit = 10
		}
		v, e := backend.SearchMessages(ctx, r, in.Query, in.Limit)
		return SearchResult{Results: v}, e
	})
	conversation := utils.NewTool(&schema.ToolInfo{Name: "get_conversation_context", Desc: "Read recent messages from the current authorized conversation."}, func(ctx context.Context, in ContextRequest) (ContextResult, error) {
		r, ok := GetRunContext(ctx)
		if !ok {
			return ContextResult{}, fmt.Errorf("run context missing")
		}
		if in.Limit <= 0 || in.Limit > 50 {
			in.Limit = 20
		}
		v, e := backend.ConversationContext(ctx, r, in.Limit)
		return ContextResult{Messages: v}, e
	})
	send := utils.NewTool(&schema.ToolInfo{Name: "send_message", Desc: "Send a message to the current conversation only."}, func(ctx context.Context, in SendRequest) (SendResult, error) {
		r, ok := GetRunContext(ctx)
		if !ok {
			return SendResult{}, fmt.Errorf("run context missing")
		}
		if in.Content == "" {
			return SendResult{}, fmt.Errorf("content is required")
		}
		e := backend.SendMessage(ctx, r, in.Content)
		return SendResult{Accepted: e == nil}, e
	})
	return []tool.InvokableTool{search, conversation, send}
}
