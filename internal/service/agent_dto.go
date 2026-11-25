package service

import (
	"encoding/json"
	"time"

	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/domain"
)

// AgentView is the public Agent response shape. Domain types stay free of
// transport tags; this DTO is the single place that defines camelCase JSON.
type AgentView struct {
	ID             int64             `json:"id"`
	BotUserID      int64             `json:"botUserId"`
	OwnerUserID    int64             `json:"ownerUserId"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Status         string            `json:"status"`
	CurrentVersion int64             `json:"currentVersion"`
	CanManage      bool              `json:"canManage"`
	CanOwn         bool              `json:"canOwn"`
	DraftChanged   bool              `json:"draftChanged"`
	Draft          *AgentDraftView   `json:"draft,omitempty"`
	Published      *AgentVersionView `json:"published,omitempty"`
}

type AgentDraftView struct {
	Instruction       string            `json:"instruction"`
	ModelID           int64             `json:"modelId"`
	Tools             json.RawMessage   `json:"tools"`
	CapabilityPresets json.RawMessage   `json:"capabilityPresets"`
	ToolPolicies      map[string]string `json:"toolPolicies"`
	MaxTokens         int               `json:"maxTokens"`
	MCPBindings       json.RawMessage   `json:"mcpBindings"`
	Skills            json.RawMessage   `json:"skills"`
	Revision          int64             `json:"revision"`
}

type AgentVersionView struct {
	ID                uint64            `json:"id"`
	AgentID           int64             `json:"agentId"`
	Version           int64             `json:"version"`
	Instruction       string            `json:"instruction"`
	ModelID           int64             `json:"modelId"`
	ModelKey          string            `json:"modelKey"`
	Tools             json.RawMessage   `json:"tools"`
	CapabilityPresets json.RawMessage   `json:"capabilityPresets"`
	ToolPolicies      map[string]string `json:"toolPolicies"`
	MaxTokens         int               `json:"maxTokens"`
	MCPBindings       json.RawMessage   `json:"mcpBindings"`
	Skills            json.RawMessage   `json:"skills"`
	PublishNote       string            `json:"publishNote"`
	PublishedBy       int64             `json:"publishedBy"`
	PublishedAt       *time.Time        `json:"publishedAt,omitempty"`
	SourceRevision    int64             `json:"sourceRevision"`
	CreatedAt         time.Time         `json:"createdAt"`
}

// AgentVersionSummaryView is the list shape; it omits the full instruction and
// tool snapshot to keep the versions endpoint light.
type AgentVersionSummaryView struct {
	ID             uint64     `json:"id"`
	Version        int64      `json:"version"`
	ModelID        int64      `json:"modelId"`
	ModelKey       string     `json:"modelKey"`
	PublishNote    string     `json:"publishNote"`
	PublishedBy    int64      `json:"publishedBy"`
	PublishedAt    *time.Time `json:"publishedAt,omitempty"`
	SourceRevision int64      `json:"sourceRevision"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// RunView mirrors the previous domain JSON contract without exposing the
// internal attempt token or raw error text.
type RunView struct {
	ID               uint64               `json:"id"`
	EventID          string               `json:"eventId"`
	AgentID          int64                `json:"agentId"`
	AgentVersion     int64                `json:"agentVersion"`
	SessionID        uint64               `json:"sessionId,omitempty"`
	ActorUserID      int64                `json:"actorUserId"`
	Topic            string               `json:"topic"`
	MsgID            int64                `json:"msgId"`
	TopicSeq         uint64               `json:"topicSeq"`
	Status           string               `json:"status"`
	ErrorCode        string               `json:"errorCode,omitempty"`
	ErrorClass       string               `json:"errorClass,omitempty"`
	Attempts         int                  `json:"attempts"`
	LeaseUntil       *time.Time           `json:"leaseUntil,omitempty"`
	NextAttemptAt    *time.Time           `json:"nextAttemptAt,omitempty"`
	ReplyMsgID       int64                `json:"replyMsgId,omitempty"`
	CheckpointID     string               `json:"checkpointId,omitempty"`
	Steps            []AgentRunStepView   `json:"steps,omitempty"`
	ToolInvocations  []ToolInvocationView `json:"toolInvocations,omitempty"`
	Citations        []CitationView       `json:"citations,omitempty"`
	SideEffectAt     *time.Time           `json:"sideEffectAt,omitempty"`
	InputTokens      int                  `json:"inputTokens"`
	OutputTokens     int                  `json:"outputTokens"`
	TotalTokens      int                  `json:"totalTokens"`
	ModelKey         string               `json:"modelKey,omitempty"`
	EstimatedCostUSD float64              `json:"estimatedCostUsd"`
	TraceID          string               `json:"traceId,omitempty"`
	RetryAfterAt     *time.Time           `json:"retryAfterAt,omitempty"`
	LastError        string               `json:"lastError,omitempty"`
	CreatedAt        time.Time            `json:"createdAt"`
	StartedAt        *time.Time           `json:"startedAt,omitempty"`
	FinishedAt       *time.Time           `json:"finishedAt,omitempty"`
}

type AgentRunStepView struct {
	ID         uint64    `json:"id"`
	Seq        uint64    `json:"seq"`
	Type       string    `json:"type"`
	Tool       string    `json:"tool,omitempty"`
	ToolCallID string    `json:"toolCallId,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Status     string    `json:"status,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type CitationView struct {
	MsgID    int64  `json:"msgId"`
	Topic    string `json:"topic"`
	TopicSeq uint64 `json:"topicSeq"`
	SenderID int64  `json:"senderId"`
	Snippet  string `json:"snippet,omitempty"`
}

type ToolInvocationView struct {
	ID            uint64    `json:"id"`
	ToolCallID    string    `json:"toolCallId"`
	Tool          string    `json:"tool"`
	Status        string    `json:"status"`
	ResultSummary string    `json:"resultSummary,omitempty"`
	ErrorCode     string    `json:"errorCode,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type ApprovalView struct {
	ID          uint64     `json:"id"`
	RunID       uint64     `json:"runId"`
	ToolCallID  string     `json:"toolCallId"`
	Tool        string     `json:"tool"`
	ArgsSummary string     `json:"argsSummary"`
	ArgsHash    string     `json:"argsHash"`
	ApproverID  int64      `json:"approverId"`
	Status      string     `json:"status"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	DecidedAt   *time.Time `json:"decidedAt,omitempty"`
	DecidedBy   int64      `json:"decidedBy,omitempty"`
	ConsumedAt  *time.Time `json:"consumedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func agentView(row *domain.Agent, manager, owner bool) AgentView {
	view := AgentView{
		ID: row.ID, BotUserID: row.BotUserID, OwnerUserID: row.OwnerUserID,
		Name: row.Name, Description: row.Description, Status: row.Status,
		CurrentVersion: row.CurrentVersion, CanManage: manager, CanOwn: owner,
	}
	if manager {
		view.Draft = &AgentDraftView{
			Instruction: row.DraftInstruction,
			ModelID:     row.DraftModelID, Tools: rawJSON(row.DraftTools),
			CapabilityPresets: rawJSON(row.DraftCapabilityPresets),
			ToolPolicies:      parsePolicies(row.DraftToolPolicies), MaxTokens: row.DraftMaxTokens,
			MCPBindings: rawJSON(row.DraftMCPBindings), Skills: rawJSON(row.DraftSkills),
			Revision: row.DraftRevision,
		}
	}
	return view
}

func versionView(v *domain.AgentVersion) AgentVersionView {
	return AgentVersionView{
		ID: v.ID, AgentID: v.AgentID, Version: v.Version, Instruction: v.Instruction,
		ModelID: v.ModelID, ModelKey: v.ModelKey,
		Tools: rawJSON(v.Tools), CapabilityPresets: rawJSON(v.CapabilityPresets),
		ToolPolicies: parsePolicies(v.ToolPolicies), MaxTokens: v.MaxTokens,
		MCPBindings: rawJSON(v.MCPBindings), Skills: rawJSON(v.Skills),
		PublishNote: v.PublishNote, PublishedBy: v.PublishedBy, PublishedAt: v.PublishedAt,
		SourceRevision: v.SourceRevision, CreatedAt: v.CreatedAt,
	}
}

func versionSummaryView(v *domain.AgentVersion) AgentVersionSummaryView {
	return AgentVersionSummaryView{
		ID: v.ID, Version: v.Version, ModelID: v.ModelID,
		ModelKey: v.ModelKey, PublishNote: v.PublishNote, PublishedBy: v.PublishedBy,
		PublishedAt: v.PublishedAt, SourceRevision: v.SourceRevision, CreatedAt: v.CreatedAt,
	}
}

func parsePolicies(raw string) map[string]string {
	out := map[string]string{}
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func runView(run *domain.AgentRun) RunView {
	return RunView{
		ID: run.ID, EventID: run.EventID, AgentID: run.AgentID, AgentVersion: run.AgentVersion,
		SessionID: run.SessionID, ActorUserID: run.ActorUserID, Topic: run.Topic, MsgID: run.MsgID,
		TopicSeq: run.TopicSeq, Status: run.Status, ErrorCode: run.ErrorCode, ErrorClass: run.ErrorClass,
		Attempts: run.Attempts, LeaseUntil: run.LeaseUntil, NextAttemptAt: run.NextAttemptAt,
		ReplyMsgID: run.ReplyMsgID, CheckpointID: run.CheckpointID, SideEffectAt: run.SideEffectAt,
		InputTokens: run.InputTokens, OutputTokens: run.OutputTokens, TotalTokens: run.TotalTokens,
		ModelKey: run.ModelKey, EstimatedCostUSD: run.EstimatedCostUSD, TraceID: run.TraceID,
		RetryAfterAt: run.RetryAfterAt, LastError: app.PublicFailureMessage(run.ErrorCode, run.LastError),
		CreatedAt: run.CreatedAt, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
}

func citationViews(rows []domain.AgentRunCitation) []CitationView {
	out := make([]CitationView, 0, len(rows))
	for i := range rows {
		out = append(out, CitationView{
			MsgID: rows[i].MsgID, Topic: rows[i].Topic, TopicSeq: rows[i].TopicSeq,
			SenderID: rows[i].SenderID, Snippet: rows[i].Snippet,
		})
	}
	return out
}

func runStepView(s *domain.AgentRunStep) AgentRunStepView {
	return AgentRunStepView{
		ID: s.ID, Seq: s.Seq, Type: s.Type, Tool: s.Tool, ToolCallID: s.ToolCallID,
		Summary: s.Summary, Status: s.Status, CreatedAt: s.CreatedAt,
	}
}

func runStepViews(rows []domain.AgentRunStep) []AgentRunStepView {
	out := make([]AgentRunStepView, 0, len(rows))
	for i := range rows {
		out = append(out, runStepView(&rows[i]))
	}
	return out
}

func toolInvocationView(t *domain.ToolInvocation) ToolInvocationView {
	return ToolInvocationView{
		ID: t.ID, ToolCallID: t.ToolCallID, Tool: t.Tool, Status: t.Status,
		ResultSummary: t.ResultSummary, ErrorCode: t.ErrorCode,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func toolInvocationViews(rows []domain.ToolInvocation) []ToolInvocationView {
	out := make([]ToolInvocationView, 0, len(rows))
	for i := range rows {
		out = append(out, toolInvocationView(&rows[i]))
	}
	return out
}

func approvalView(a *domain.AgentApproval) ApprovalView {
	return ApprovalView{
		ID: a.ID, RunID: a.RunID, ToolCallID: a.ToolCallID, Tool: a.Tool,
		ArgsSummary: a.ArgsSummary, ArgsHash: a.ArgsHash, ApproverID: a.ApproverID,
		Status: a.Status, ExpiresAt: a.ExpiresAt, DecidedAt: a.DecidedAt, DecidedBy: a.DecidedBy,
		ConsumedAt: a.ConsumedAt, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func runViews(runs []domain.AgentRun) []RunView {
	out := make([]RunView, 0, len(runs))
	for i := range runs {
		out = append(out, runView(&runs[i]))
	}
	return out
}

func approvalViews(rows []domain.AgentApproval) []ApprovalView {
	out := make([]ApprovalView, 0, len(rows))
	for i := range rows {
		out = append(out, approvalView(&rows[i]))
	}
	return out
}
