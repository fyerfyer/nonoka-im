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
	ID               uint64     `json:"id"`
	EventID          string     `json:"eventId"`
	AgentID          int64      `json:"agentId"`
	AgentVersion     int64      `json:"agentVersion"`
	SessionID        uint64     `json:"sessionId,omitempty"`
	ActorUserID      int64      `json:"actorUserId"`
	Topic            string     `json:"topic"`
	MsgID            int64      `json:"msgId"`
	TopicSeq         uint64     `json:"topicSeq"`
	Status           string     `json:"status"`
	ErrorCode        string     `json:"errorCode,omitempty"`
	ErrorClass       string     `json:"errorClass,omitempty"`
	Attempts         int        `json:"attempts"`
	LeaseUntil       *time.Time `json:"leaseUntil,omitempty"`
	NextAttemptAt    *time.Time `json:"nextAttemptAt,omitempty"`
	ReplyMsgID       int64      `json:"replyMsgId,omitempty"`
	ReplyClientMsgID string     `json:"replyClientMsgId,omitempty"`
	CheckpointID     string     `json:"checkpointId,omitempty"`
	SideEffectAt     *time.Time `json:"sideEffectAt,omitempty"`
	InputTokens      int        `json:"inputTokens"`
	OutputTokens     int        `json:"outputTokens"`
	TotalTokens      int        `json:"totalTokens"`
	ModelKey         string     `json:"modelKey,omitempty"`
	EstimatedCostUSD float64    `json:"estimatedCostUsd"`
	TraceID          string     `json:"traceId,omitempty"`
	RetryAfterAt     *time.Time `json:"retryAfterAt,omitempty"`
	LastError        string     `json:"lastError,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
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
		ReplyMsgID: run.ReplyMsgID, ReplyClientMsgID: run.ReplyClientMsgID,
		CheckpointID: run.CheckpointID, SideEffectAt: run.SideEffectAt,
		InputTokens: run.InputTokens, OutputTokens: run.OutputTokens, TotalTokens: run.TotalTokens,
		ModelKey: run.ModelKey, EstimatedCostUSD: run.EstimatedCostUSD, TraceID: run.TraceID,
		RetryAfterAt: run.RetryAfterAt, LastError: app.PublicFailureMessage(run.ErrorCode, run.LastError),
		CreatedAt: run.CreatedAt, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
}

func runViews(runs []domain.AgentRun) []RunView {
	out := make([]RunView, 0, len(runs))
	for i := range runs {
		out = append(out, runView(&runs[i]))
	}
	return out
}
