package service

import (
	"encoding/json"
	"time"

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
	Draft          *AgentDraftView   `json:"draft,omitempty"`
	Published      *AgentVersionView `json:"published,omitempty"`
}

type AgentDraftView struct {
	Instruction string          `json:"instruction"`
	Model       string          `json:"model"`
	Tools       json.RawMessage `json:"tools"`
	BaseURL     string          `json:"baseUrl"`
	HasAPIKey   bool            `json:"hasApiKey"`
	MaxTokens   int             `json:"maxTokens"`
	MCPBindings json.RawMessage `json:"mcpBindings"`
	Skills      json.RawMessage `json:"skills"`
	Revision    int64           `json:"revision"`
}

type AgentVersionView struct {
	ID          uint64          `json:"id"`
	AgentID     int64           `json:"agentId"`
	Version     int64           `json:"version"`
	Instruction string          `json:"instruction"`
	Model       string          `json:"model"`
	Tools       json.RawMessage `json:"tools"`
	BaseURL     string          `json:"baseUrl"`
	HasAPIKey   bool            `json:"hasApiKey"`
	MaxTokens   int             `json:"maxTokens"`
	MCPBindings json.RawMessage `json:"mcpBindings"`
	Skills      json.RawMessage `json:"skills"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// RunView mirrors the previous domain JSON contract without exposing the
// internal attempt token or raw error text.
type RunView struct {
	ID            uint64     `json:"id"`
	EventID       string     `json:"eventId"`
	AgentID       int64      `json:"agentId"`
	AgentVersion  int64      `json:"agentVersion"`
	SessionID     uint64     `json:"sessionId,omitempty"`
	ActorUserID   int64      `json:"actorUserId"`
	Topic         string     `json:"topic"`
	MsgID         int64      `json:"msgId"`
	TopicSeq      uint64     `json:"topicSeq"`
	Status        string     `json:"status"`
	ErrorCode     string     `json:"errorCode,omitempty"`
	ErrorClass    string     `json:"errorClass,omitempty"`
	Attempts      int        `json:"attempts"`
	LeaseUntil    *time.Time `json:"leaseUntil,omitempty"`
	NextAttemptAt *time.Time `json:"nextAttemptAt,omitempty"`
	ReplyMsgID    int64      `json:"replyMsgId,omitempty"`
	CheckpointID  string     `json:"checkpointId,omitempty"`
	SideEffectAt  *time.Time `json:"sideEffectAt,omitempty"`
	InputTokens   int        `json:"inputTokens"`
	OutputTokens  int        `json:"outputTokens"`
	TotalTokens   int        `json:"totalTokens"`
	CreatedAt     time.Time  `json:"createdAt"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
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

func agentView(row *domain.Agent, manager bool) AgentView {
	view := AgentView{
		ID: row.ID, BotUserID: row.BotUserID, OwnerUserID: row.OwnerUserID,
		Name: row.Name, Description: row.Description, Status: row.Status,
		CurrentVersion: row.CurrentVersion, CanManage: manager,
	}
	if manager {
		view.Draft = &AgentDraftView{
			Instruction: row.DraftInstruction, Model: row.DraftModel,
			Tools: rawJSON(row.DraftTools), BaseURL: row.DraftBaseURL,
			HasAPIKey: row.DraftAPIKeyEnc != "", MaxTokens: row.DraftMaxTokens,
			MCPBindings: rawJSON(row.DraftMCPBindings), Skills: rawJSON(row.DraftSkills),
			Revision: row.DraftRevision,
		}
	}
	return view
}

func versionView(v *domain.AgentVersion) AgentVersionView {
	return AgentVersionView{
		ID: v.ID, AgentID: v.AgentID, Version: v.Version, Instruction: v.Instruction,
		Model: v.Model, Tools: rawJSON(v.Tools), BaseURL: v.BaseURL,
		HasAPIKey: v.APIKeyEnc != "", MaxTokens: v.MaxTokens,
		MCPBindings: rawJSON(v.MCPBindings), Skills: rawJSON(v.Skills), CreatedAt: v.CreatedAt,
	}
}

func runView(run *domain.AgentRun) RunView {
	return RunView{
		ID: run.ID, EventID: run.EventID, AgentID: run.AgentID, AgentVersion: run.AgentVersion,
		SessionID: run.SessionID, ActorUserID: run.ActorUserID, Topic: run.Topic, MsgID: run.MsgID,
		TopicSeq: run.TopicSeq, Status: run.Status, ErrorCode: run.ErrorCode, ErrorClass: run.ErrorClass,
		Attempts: run.Attempts, LeaseUntil: run.LeaseUntil, NextAttemptAt: run.NextAttemptAt,
		ReplyMsgID: run.ReplyMsgID, CheckpointID: run.CheckpointID, SideEffectAt: run.SideEffectAt,
		InputTokens: run.InputTokens, OutputTokens: run.OutputTokens, TotalTokens: run.TotalTokens,
		CreatedAt: run.CreatedAt, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
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
