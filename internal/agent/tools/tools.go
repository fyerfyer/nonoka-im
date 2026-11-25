// Package tools contains the narrow, authorization-aware IM tool surface
// exposed to an Agent. The backend implementations remain in the IM layer;
// these adapters never accept an actor ID from model arguments and every call
// is authorized against the run context before it reaches the backend.
package tools

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/authz"
)

type RunContext struct {
	ActorUserID int64
	AgentID     int64
	BotID       int64
	RunID       uint64
	AttemptID   string
	ToolCallID  string
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
	Query string `json:"query" jsonschema:"required" jsonschema_description:"Keyword or phrase to search for."`
	Limit int    `json:"limit,omitempty" jsonschema_description:"Maximum results (1-20, default 10)."`
	// Since filters to messages sent at or after this unix time (seconds).
	// Zero means no time filter.
	Since int64 `json:"since,omitempty" jsonschema_description:"Only messages at or after this Unix timestamp (seconds)."`
	// SenderID filters to messages sent by this user ID. Zero means no
	// sender filter.
	SenderID int64 `json:"sender_id,omitempty" jsonschema_description:"Only messages sent by this user ID."`
	// Until filters to messages sent at or before this unix time (seconds).
	// Zero means no upper time bound.
	Until int64 `json:"until,omitempty" jsonschema_description:"Only messages at or before this Unix timestamp (seconds)."`
	// HasFile filters to image/file messages.
	HasFile bool `json:"has_file,omitempty" jsonschema_description:"Only messages with an image or file attachment."`
}

// SearchFilter carries the optional search_messages filters in parsed form.
type SearchFilter struct {
	Since    int64
	Until    int64
	SenderID int64
	HasFile  bool
}

// Filter returns the optional filters in parsed form.
func (r SearchRequest) Filter() SearchFilter {
	return SearchFilter{Since: r.Since, Until: r.Until, SenderID: r.SenderID, HasFile: r.HasFile}
}

type ContextRequest struct {
	Limit int `json:"limit,omitempty" jsonschema_description:"Maximum recent messages (1-50, default 20)."`
}
type SendRequest struct {
	Content string `json:"content" jsonschema:"required" jsonschema_description:"Message text to post."`
}
type ReadFileRequest struct {
	FileID string `json:"file_id" jsonschema:"required" jsonschema_description:"Attachment file ID."`
}
type GroupMembersRequest struct {
	GroupID string `json:"group_id" jsonschema:"required" jsonschema_description:"Group ID to list members for."`
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
type ReadFileResult struct {
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Backend performs the actual IM operations. Implementations must enforce
// authorization for rc.ActorUserID on every call; the tool adapters only
// validate argument shape and bound result size.
type Backend interface {
	SearchMessages(context.Context, RunContext, string, int, SearchFilter) ([]string, error)
	ConversationContext(context.Context, RunContext, int) ([]string, error)
	SendMessage(context.Context, RunContext, string) error
	// ReadFile returns the attachment content when the actor may read it.
	// maxBytes bounds the returned content; the backend reports truncation.
	ReadFile(context.Context, RunContext, string, int) (name, mime string, content []byte, truncated bool, err error)
	// TopicInfo returns a human-readable description of the current
	// conversation: type, group name and members, or the P2P peer.
	TopicInfo(context.Context, RunContext) (string, error)
	// MyInfo returns a human-readable description of the actor's own user
	// profile. Implementations must only ever return rc.ActorUserID's data.
	MyInfo(context.Context, RunContext) (string, error)
	// MyGroups returns a human-readable, bounded list of the groups the
	// actor belongs to (names, IDs and member counts).
	MyGroups(context.Context, RunContext) (string, error)
	// GroupMembers returns a human-readable member list of the given group.
	// Implementations must deny (with an existence-ambiguous error) when the
	// actor is not a member of that group.
	GroupMembers(context.Context, RunContext, string) (string, error)
}

// Guard authorizes every tool call against the run context before it reaches
// the backend. It is the only authorization entry point for tools: adapters
// hold no ACLs of their own.
type Guard interface {
	CanExecuteTool(ctx context.Context, actorID, agentID int64, tool, currentTopic, resourceType, resourceRef string) error
}

// Authorize fails closed when no guard is configured, so a miswired worker
// cannot silently run tools without authorization. A real denial is reported as
// a normal tool refusal; an OpenFGA outage is surfaced as transient infra so
// the worker retries instead of telling the model the tool does not exist.
func Authorize(ctx context.Context, guard Guard, r RunContext, tool, resourceType, resourceRef string) error {
	if guard == nil {
		return &port.Error{Class: port.ClassInvalidConfig, Err: fmt.Errorf("tool authorization unavailable")}
	}
	if err := guard.CanExecuteTool(ctx, r.ActorUserID, r.AgentID, tool, r.Topic, resourceType, resourceRef); err != nil {
		if errors.Is(err, authz.ErrDenied) {
			return fmt.Errorf("tool %s not authorized", tool)
		}
		return &port.Error{Class: port.ClassTransientInfra, Err: err}
	}
	return nil
}

// ExposedMCPName is the model-facing name of an MCP tool. Keeping the prefix
// in one place prevents the management API and the runtime from drifting.
func ExposedMCPName(server, toolName string) string {
	return "mcp_" + server + "_" + toolName
}

// BoundStrings caps a list of textual tool results to maxChars while keeping
// each item on a valid UTF-8 boundary. It is shared by the built-in tools and
// the app-level tool backend so both apply the same budget rules.
func BoundStrings(items []string, maxChars int) []string {
	if maxChars <= 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	remaining := maxChars
	for _, item := range items {
		if remaining <= 0 {
			break
		}
		if len(item) > remaining {
			item = item[:remaining]
			for len(item) > 0 && !utf8.ValidString(item) {
				item = item[:len(item)-1]
			}
		}
		result = append(result, item)
		remaining -= len(item)
	}
	return result
}
