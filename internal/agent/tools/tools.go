// Package tools contains the narrow, authorization-aware IM tool surface
// exposed to an Agent. The backend implementations remain in the IM layer;
// these adapters never accept an actor ID from model arguments and every call
// is authorized against the run context before it reaches the backend.
package tools

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

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

// toolCallTimeout bounds a single tool execution so one stuck backend call
// (e.g. a hung MCP request) cannot eat the whole run budget. When the run
// context already carries an earlier deadline, context.WithTimeout keeps the
// earlier one automatically, so no extra check is needed.
const toolCallTimeout = 20 * time.Second

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
	// Since filters to messages sent at or after this unix time (seconds).
	// Zero means no time filter.
	Since int64 `json:"since,omitempty"`
	// SenderID filters to messages sent by this user ID. Zero means no
	// sender filter.
	SenderID int64 `json:"sender_id,omitempty"`
}

// SearchFilter carries the optional search_messages filters in parsed form.
type SearchFilter struct {
	Since    int64
	SenderID int64
}

func (r SearchRequest) filter() SearchFilter {
	return SearchFilter{Since: r.Since, SenderID: r.SenderID}
}

type ContextRequest struct {
	Limit int `json:"limit,omitempty"`
}
type SendRequest struct {
	Content string `json:"content"`
}
type ReadFileRequest struct {
	FileID string `json:"file_id"`
}
type GroupMembersRequest struct {
	GroupID string `json:"group_id"`
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

func New(backend Backend, guard Guard, budgets ...int) []tool.InvokableTool {
	resultBudgetTokens := 8192
	if len(budgets) > 0 {
		resultBudgetTokens = budgets[0]
	}
	if resultBudgetTokens <= 0 {
		resultBudgetTokens = 8192
	}
	search := utils.NewTool(&schema.ToolInfo{Name: "search_messages", Desc: `Search the current conversation's message history for messages containing a keyword or phrase.

Use this tool when the user asks about something discussed earlier, wants you to find a specific message, or references facts you do not have in your current context. Results are the most recent matching messages.

Args:
- query: keyword or phrase to search for (required).
- limit: max results, 1-20 (default 10).
- since: optional unix timestamp (seconds); only messages sent at or after this time are returned.
- sender_id: optional user ID; only messages sent by this user are returned.`}, func(ctx context.Context, in SearchRequest) (SearchResult, error) {
		r, ok := GetRunContext(ctx)
		if !ok {
			return SearchResult{}, fmt.Errorf("run context missing")
		}
		ctx, cancel := context.WithTimeout(ctx, toolCallTimeout)
		defer cancel()
		if in.Query == "" {
			return SearchResult{}, fmt.Errorf("query is required")
		}
		if in.Limit <= 0 || in.Limit > 20 {
			in.Limit = 10
		}
		if err := authorize(ctx, guard, r, "search_messages", "topic", ""); err != nil {
			return SearchResult{}, err
		}
		v, e := backend.SearchMessages(ctx, r, in.Query, in.Limit, in.filter())
		return SearchResult{Results: limitItems(v, resultBudgetTokens)}, e
	})
	conversation := utils.NewTool(&schema.ToolInfo{Name: "get_conversation_context", Desc: `Read the most recent messages of the current conversation, oldest first.

Use this tool to catch up on what happened before you were called, or to quote earlier messages accurately. Do not use it when you only need a keyword lookup — prefer search_messages for that.

Args:
- limit: max messages, 1-50 (default 20).`}, func(ctx context.Context, in ContextRequest) (ContextResult, error) {
		r, ok := GetRunContext(ctx)
		if !ok {
			return ContextResult{}, fmt.Errorf("run context missing")
		}
		ctx, cancel := context.WithTimeout(ctx, toolCallTimeout)
		defer cancel()
		if in.Limit <= 0 || in.Limit > 50 {
			in.Limit = 20
		}
		if err := authorize(ctx, guard, r, "get_conversation_context", "topic", ""); err != nil {
			return ContextResult{}, err
		}
		v, e := backend.ConversationContext(ctx, r, in.Limit)
		return ContextResult{Messages: limitItems(v, resultBudgetTokens)}, e
	})
	send := utils.NewTool(&schema.ToolInfo{Name: "send_message", Desc: `Send a message to the current conversation as the agent. The message is delivered to every participant immediately.

Use this tool when the user explicitly asks you to post or announce something. Your normal final reply is already delivered to the conversation, so do not use this tool to repeat your answer.

Args:
- content: message text (required).`}, func(ctx context.Context, in SendRequest) (SendResult, error) {
		r, ok := GetRunContext(ctx)
		if !ok {
			return SendResult{}, fmt.Errorf("run context missing")
		}
		ctx, cancel := context.WithTimeout(ctx, toolCallTimeout)
		defer cancel()
		if in.Content == "" {
			return SendResult{}, fmt.Errorf("content is required")
		}
		if err := authorize(ctx, guard, r, "send_message", "topic", ""); err != nil {
			return SendResult{}, err
		}
		e := backend.SendMessage(ctx, r, in.Content)
		return SendResult{Accepted: e == nil}, e
	})
	readFile := utils.NewTool(&schema.ToolInfo{Name: "read_file", Desc: `Read the content of an attachment (file or image) by its file ID, as long as the user who triggered you is allowed to see it.

Use this tool when the user asks about the contents of a file shared in this conversation, e.g. "what does the attached log say". Text files are returned as text; other files return metadata only.

Args:
- file_id: the attachment ID from the message that shared the file (required).`}, func(ctx context.Context, in ReadFileRequest) (ReadFileResult, error) {
		r, ok := GetRunContext(ctx)
		if !ok {
			return ReadFileResult{}, fmt.Errorf("run context missing")
		}
		ctx, cancel := context.WithTimeout(ctx, toolCallTimeout)
		defer cancel()
		if in.FileID == "" {
			return ReadFileResult{}, fmt.Errorf("file_id is required")
		}
		if err := authorize(ctx, guard, r, "read_file", "file", in.FileID); err != nil {
			return ReadFileResult{}, err
		}
		const maxBytes = 256 << 10
		name, mime, content, truncated, err := backend.ReadFile(ctx, r, in.FileID, maxBytes)
		if err != nil {
			return ReadFileResult{}, err
		}
		result := ReadFileResult{Name: name, Mime: mime, Truncated: truncated}
		if isTextMime(mime) {
			result.Content = string(content)
		} else {
			result.Content = fmt.Sprintf("[%s attachment: %d bytes, content not shown]", mime, len(content))
		}
		return result, nil
	})
	topicInfo := utils.NewTool(&schema.ToolInfo{Name: "get_topic_info", Desc: `Get metadata about the current conversation: whether it is a direct chat or a group, the group's name and member list, or the direct chat counterpart.

Use this tool when you need to know who is in this conversation or what group this is, e.g. before addressing someone by name. Takes no arguments.`}, func(ctx context.Context, _ struct{}) (string, error) {
		r, ok := GetRunContext(ctx)
		if !ok {
			return "", fmt.Errorf("run context missing")
		}
		ctx, cancel := context.WithTimeout(ctx, toolCallTimeout)
		defer cancel()
		if err := authorize(ctx, guard, r, "get_topic_info", "topic", ""); err != nil {
			return "", err
		}
		return backend.TopicInfo(ctx, r)
	})
	myInfo := utils.NewTool(&schema.ToolInfo{Name: "get_my_info", Desc: `Get the current user's own profile: user ID, username and account creation time.

Use this tool when the user asks who they are, what their account is, or what their own user ID or username is. Returns only the profile of the user you are talking to. Takes no arguments.`}, func(ctx context.Context, _ struct{}) (string, error) {
		r, ok := GetRunContext(ctx)
		if !ok {
			return "", fmt.Errorf("run context missing")
		}
		ctx, cancel := context.WithTimeout(ctx, toolCallTimeout)
		defer cancel()
		if err := authorize(ctx, guard, r, "get_my_info", "external", ""); err != nil {
			return "", err
		}
		return backend.MyInfo(ctx, r)
	})
	myGroups := utils.NewTool(&schema.ToolInfo{Name: "list_my_groups", Desc: `List the groups the current user has joined: group name, group ID and member count for each (up to 50 groups).

Use this tool when the user asks which groups they are in, or before using get_group_members to find a group ID. Only returns groups the current user belongs to. Takes no arguments.`}, func(ctx context.Context, _ struct{}) (string, error) {
		r, ok := GetRunContext(ctx)
		if !ok {
			return "", fmt.Errorf("run context missing")
		}
		ctx, cancel := context.WithTimeout(ctx, toolCallTimeout)
		defer cancel()
		if err := authorize(ctx, guard, r, "list_my_groups", "external", ""); err != nil {
			return "", err
		}
		return backend.MyGroups(ctx, r)
	})
	groupMembers := utils.NewTool(&schema.ToolInfo{Name: "get_group_members", Desc: `List the members of a group by its group ID: usernames and user IDs (up to 200 members).

Use this tool when the user asks who is in a specific group. You can only look up groups the current user belongs to; for other group IDs the tool fails without revealing whether the group exists.

Args:
- group_id: the ID of the group (from list_my_groups or the current conversation) (required).`}, func(ctx context.Context, in GroupMembersRequest) (string, error) {
		r, ok := GetRunContext(ctx)
		if !ok {
			return "", fmt.Errorf("run context missing")
		}
		ctx, cancel := context.WithTimeout(ctx, toolCallTimeout)
		defer cancel()
		if in.GroupID == "" {
			return "", fmt.Errorf("group_id is required")
		}
		if err := authorize(ctx, guard, r, "get_group_members", "external", ""); err != nil {
			return "", err
		}
		return backend.GroupMembers(ctx, r, in.GroupID)
	})
	return []tool.InvokableTool{search, conversation, send, readFile, topicInfo, myInfo, myGroups, groupMembers}
}

// authorize fails closed when no guard is configured, so a miswired worker
// cannot silently run tools without authorization.
func authorize(ctx context.Context, guard Guard, r RunContext, tool, resourceType, resourceRef string) error {
	if guard == nil {
		return fmt.Errorf("tool authorization unavailable")
	}
	if err := guard.CanExecuteTool(ctx, r.ActorUserID, r.AgentID, tool, r.Topic, resourceType, resourceRef); err != nil {
		return fmt.Errorf("tool %s not authorized", tool)
	}
	return nil
}

func isTextMime(mime string) bool {
	return mime == "" || mime == "text/plain" || mime == "text/markdown" || mime == "application/json" ||
		len(mime) > 5 && (mime[:5] == "text/" || mime == "application/xml" || mime == "application/javascript" || mime == "application/x-yaml")
}

func limitItems(items []string, tokenBudget int) []string {
	remaining := tokenBudget * 4
	if remaining <= 0 {
		return nil
	}
	result := make([]string, 0, len(items))
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
