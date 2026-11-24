package agentdata

import (
	"time"

	"nonoka-im/internal/agent/domain"
)

// Row types are the persistence shape. Domain types stay free of gorm tags;
// every column detail and every database-specific null/default lives here.

var (
	ErrDraftConflict   = domain.ErrDraftConflict
	ErrRunState        = domain.ErrRunState
	ErrAgentNotFound   = domain.ErrAgentNotFound
	ErrApprovalGone    = domain.ErrApprovalGone
	ErrApprovalExpired = domain.ErrApprovalExpired
)

type AgentRow struct {
	ID                     int64  `gorm:"column:id;primaryKey;autoIncrement"`
	BotUserID              int64  `gorm:"column:bot_user_id;not null"`
	OwnerUserID            int64  `gorm:"column:owner_user_id;not null"`
	Name                   string `gorm:"column:name;size:100;not null"`
	Description            string `gorm:"column:description;type:text"`
	Status                 string `gorm:"column:status;size:24;not null;default:draft"`
	CurrentVersion         int64  `gorm:"column:current_version"`
	DraftInstruction       string `gorm:"column:draft_instruction;type:text"`
	DraftTools             string `gorm:"column:draft_tools;type:text"`
	DraftModelServiceID    int64  `gorm:"column:draft_model_service_id"`
	DraftModelID           int64  `gorm:"column:draft_model_id"`
	DraftCapabilityPresets string `gorm:"column:draft_capability_presets;type:text"`
	DraftToolPolicies      string `gorm:"column:draft_tool_policies;type:text"`
	DraftMaxTokens         int    `gorm:"column:draft_max_tokens;not null;default:4096"`
	DraftMCPBindings       string `gorm:"column:draft_mcp_bindings;type:text"`
	DraftSkills            string `gorm:"column:draft_skills;type:text"`
	DraftRevision          int64  `gorm:"column:draft_revision;not null;default:1"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (AgentRow) TableName() string { return "agents" }

func (r AgentRow) toDomain() domain.Agent {
	return domain.Agent{
		ID: r.ID, BotUserID: r.BotUserID, OwnerUserID: r.OwnerUserID, Name: r.Name,
		Description: r.Description, Status: r.Status, CurrentVersion: r.CurrentVersion,
		DraftInstruction: r.DraftInstruction, DraftTools: r.DraftTools,
		DraftModelServiceID: r.DraftModelServiceID, DraftModelID: r.DraftModelID,
		DraftCapabilityPresets: r.DraftCapabilityPresets, DraftToolPolicies: r.DraftToolPolicies,
		DraftMaxTokens:   r.DraftMaxTokens,
		DraftMCPBindings: r.DraftMCPBindings, DraftSkills: r.DraftSkills, DraftRevision: r.DraftRevision,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type AgentVersionRow struct {
	ID                uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	AgentID           int64      `gorm:"column:agent_id;not null"`
	Version           int64      `gorm:"column:version;not null"`
	Instruction       string     `gorm:"column:instruction;type:text;not null"`
	ModelServiceID    int64      `gorm:"column:model_service_id"`
	ModelID           int64      `gorm:"column:model_id"`
	ModelKey          string     `gorm:"column:model_key;size:200;not null;default:''"`
	Tools             string     `gorm:"column:tools;type:text;not null"`
	CapabilityPresets string     `gorm:"column:capability_presets;type:text"`
	ToolPolicies      string     `gorm:"column:tool_policies;type:text"`
	MaxTokens         int        `gorm:"column:max_tokens;not null;default:4096"`
	MCPBindings       string     `gorm:"column:mcp_bindings;type:text"`
	Skills            string     `gorm:"column:skills;type:text"`
	PublishNote       string     `gorm:"column:publish_note;type:text;not null;default:''"`
	PublishedBy       int64      `gorm:"column:published_by;not null;default:0"`
	PublishedAt       *time.Time `gorm:"column:published_at"`
	SourceRevision    int64      `gorm:"column:source_revision;not null;default:0"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
}

func (AgentVersionRow) TableName() string { return "agent_versions" }

func (r AgentVersionRow) toDomain() domain.AgentVersion {
	return domain.AgentVersion{
		ID: r.ID, AgentID: r.AgentID, Version: r.Version, Instruction: r.Instruction,
		ModelServiceID: r.ModelServiceID, ModelID: r.ModelID, ModelKey: r.ModelKey,
		Tools: r.Tools, CapabilityPresets: r.CapabilityPresets, ToolPolicies: r.ToolPolicies,
		MaxTokens: r.MaxTokens, MCPBindings: r.MCPBindings, Skills: r.Skills,
		PublishNote: r.PublishNote, PublishedBy: r.PublishedBy, PublishedAt: r.PublishedAt,
		SourceRevision: r.SourceRevision, CreatedAt: r.CreatedAt,
	}
}

type AgentRunRow struct {
	ID             uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	EventID        string     `gorm:"column:event_id;size:128;not null"`
	AgentID        int64      `gorm:"column:agent_id;not null"`
	AgentVersion   int64      `gorm:"column:agent_version;not null;default:0"`
	SessionID      uint64     `gorm:"column:session_id"`
	ActorUserID    int64      `gorm:"column:actor_user_id;not null"`
	Topic          string     `gorm:"column:topic;size:128;not null"`
	MsgID          int64      `gorm:"column:msg_id;not null"`
	TopicSeq       uint64     `gorm:"column:topic_seq;not null;default:0"`
	Status         string     `gorm:"column:status;size:24;not null"`
	ErrorCode      string     `gorm:"column:error_code;size:64"`
	ErrorClass     string     `gorm:"column:error_class;size:32"`
	LastError      string     `gorm:"column:last_error;type:text"`
	Attempts       int        `gorm:"column:attempts;not null;default:0"`
	AttemptID      string     `gorm:"column:attempt_id;size:64"`
	LeaseUntil     *time.Time `gorm:"column:lease_until"`
	NextAttemptAt  *time.Time `gorm:"column:next_attempt_at"`
	ReplyMsgID     int64      `gorm:"column:reply_msg_id"`
	CheckpointID   string     `gorm:"column:checkpoint_id;size:64"`
	Kind           string     `gorm:"column:kind;size:16;not null;default:im"`
	InputText      string     `gorm:"column:input_text;type:text"`
	OutputText     string     `gorm:"column:output_text;type:text"`
	ConfigSnapshot string     `gorm:"column:config_snapshot;type:text"`
	SideEffectAt   *time.Time `gorm:"column:side_effect_at"`
	InputTokens    int        `gorm:"column:input_tokens"`
	OutputTokens   int        `gorm:"column:output_tokens"`
	TotalTokens    int        `gorm:"column:total_tokens"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	StartedAt      *time.Time `gorm:"column:started_at"`
	FinishedAt     *time.Time `gorm:"column:finished_at"`
}

func (AgentRunRow) TableName() string { return "agent_runs" }

func agentRunRowFromDomain(run domain.AgentRun) AgentRunRow {
	return AgentRunRow{
		ID: run.ID, EventID: run.EventID, AgentID: run.AgentID, AgentVersion: run.AgentVersion,
		SessionID: run.SessionID, ActorUserID: run.ActorUserID, Topic: run.Topic, MsgID: run.MsgID,
		TopicSeq: run.TopicSeq, Status: run.Status, ErrorCode: run.ErrorCode, ErrorClass: run.ErrorClass,
		LastError: run.LastError, Attempts: run.Attempts, AttemptID: run.AttemptID,
		LeaseUntil: run.LeaseUntil, NextAttemptAt: run.NextAttemptAt, ReplyMsgID: run.ReplyMsgID,
		CheckpointID: run.CheckpointID, Kind: run.Kind, InputText: run.InputText,
		OutputText: run.OutputText, ConfigSnapshot: run.ConfigSnapshot,
		SideEffectAt: run.SideEffectAt, InputTokens: run.InputTokens,
		OutputTokens: run.OutputTokens, TotalTokens: run.TotalTokens, CreatedAt: run.CreatedAt,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
}

func (r AgentRunRow) toDomain() domain.AgentRun {
	return domain.AgentRun{
		ID: r.ID, EventID: r.EventID, AgentID: r.AgentID, AgentVersion: r.AgentVersion,
		SessionID: r.SessionID, ActorUserID: r.ActorUserID, Topic: r.Topic, MsgID: r.MsgID,
		TopicSeq: r.TopicSeq, Status: r.Status, ErrorCode: r.ErrorCode, ErrorClass: r.ErrorClass,
		LastError: r.LastError, Attempts: r.Attempts, AttemptID: r.AttemptID,
		LeaseUntil: r.LeaseUntil, NextAttemptAt: r.NextAttemptAt, ReplyMsgID: r.ReplyMsgID,
		CheckpointID: r.CheckpointID, Kind: r.Kind, InputText: r.InputText,
		OutputText: r.OutputText, ConfigSnapshot: r.ConfigSnapshot,
		SideEffectAt: r.SideEffectAt, InputTokens: r.InputTokens,
		OutputTokens: r.OutputTokens, TotalTokens: r.TotalTokens, CreatedAt: r.CreatedAt,
		StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
}

type AgentRunStepRow struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	RunID      uint64    `gorm:"column:run_id;not null"`
	Seq        uint64    `gorm:"column:seq;not null"`
	Type       string    `gorm:"column:type;size:32;not null"`
	Tool       string    `gorm:"column:tool;size:200;not null;default:''"`
	ToolCallID string    `gorm:"column:tool_call_id;size:512;not null;default:''"`
	Summary    string    `gorm:"column:summary;type:text;not null;default:''"`
	Status     string    `gorm:"column:status;size:32;not null;default:''"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
}

func (AgentRunStepRow) TableName() string { return "agent_run_steps" }

func (r AgentRunStepRow) toDomain() domain.AgentRunStep {
	return domain.AgentRunStep{
		ID: r.ID, RunID: r.RunID, Seq: r.Seq, Type: r.Type, Tool: r.Tool,
		ToolCallID: r.ToolCallID, Summary: r.Summary, Status: r.Status, CreatedAt: r.CreatedAt,
	}
}

type AgentTurnRow struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	AgentID      int64     `gorm:"column:agent_id;not null"`
	ActorUserID  int64     `gorm:"column:actor_user_id;not null"`
	Topic        string    `gorm:"column:topic;size:128;not null"`
	SessionID    uint64    `gorm:"column:session_id"`
	RunID        uint64    `gorm:"column:run_id;not null"`
	TriggerMsgID int64     `gorm:"column:trigger_msg_id"`
	ReplyMsgID   int64     `gorm:"column:reply_msg_id"`
	Input        string    `gorm:"column:input;type:text;not null"`
	Output       string    `gorm:"column:output;type:text"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (AgentTurnRow) TableName() string { return "agent_turns" }

func agentTurnRowFromDomain(t domain.AgentTurn) AgentTurnRow {
	return AgentTurnRow{
		ID: t.ID, AgentID: t.AgentID, ActorUserID: t.ActorUserID, Topic: t.Topic,
		SessionID: t.SessionID, RunID: t.RunID, TriggerMsgID: t.TriggerMsgID,
		ReplyMsgID: t.ReplyMsgID, Input: t.Input, Output: t.Output, CreatedAt: t.CreatedAt,
	}
}

func (r AgentTurnRow) toDomain() domain.AgentTurn {
	return domain.AgentTurn{
		ID: r.ID, AgentID: r.AgentID, ActorUserID: r.ActorUserID, Topic: r.Topic,
		SessionID: r.SessionID, RunID: r.RunID, TriggerMsgID: r.TriggerMsgID,
		ReplyMsgID: r.ReplyMsgID, Input: r.Input, Output: r.Output, CreatedAt: r.CreatedAt,
	}
}

type AgentSessionRow struct {
	ID          uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	AgentID     int64     `gorm:"column:agent_id;not null"`
	ActorUserID int64     `gorm:"column:actor_user_id;not null"`
	Topic       string    `gorm:"column:topic;size:128;not null"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (AgentSessionRow) TableName() string { return "agent_sessions" }

func (r AgentSessionRow) toDomain() domain.AgentSession {
	return domain.AgentSession{
		ID: r.ID, AgentID: r.AgentID, ActorUserID: r.ActorUserID, Topic: r.Topic,
		UpdatedAt: r.UpdatedAt, CreatedAt: r.CreatedAt,
	}
}

type AgentContextSnapshotRow struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID     uint64    `gorm:"column:session_id;not null"`
	LastTurnID    uint64    `gorm:"column:last_turn_id;not null"`
	AgentVersion  int64     `gorm:"column:agent_version;not null"`
	Summary       string    `gorm:"column:summary;type:text;not null"`
	SummaryTokens int       `gorm:"column:summary_tokens"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (AgentContextSnapshotRow) TableName() string { return "agent_context_snapshots" }

func agentContextSnapshotRowFromDomain(s domain.AgentContextSnapshot) AgentContextSnapshotRow {
	return AgentContextSnapshotRow{
		ID: s.ID, SessionID: s.SessionID, LastTurnID: s.LastTurnID, AgentVersion: s.AgentVersion,
		Summary: s.Summary, SummaryTokens: s.SummaryTokens, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

func (r AgentContextSnapshotRow) toDomain() domain.AgentContextSnapshot {
	return domain.AgentContextSnapshot{
		ID: r.ID, SessionID: r.SessionID, LastTurnID: r.LastTurnID, AgentVersion: r.AgentVersion,
		Summary: r.Summary, SummaryTokens: r.SummaryTokens, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type AgentApprovalRow struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	RunID       uint64     `gorm:"column:run_id;not null"`
	ToolCallID  string     `gorm:"column:tool_call_id;size:512;not null"`
	Tool        string     `gorm:"column:tool;size:128;not null"`
	ArgsSummary string     `gorm:"column:args_summary;type:text;not null"`
	ArgsHash    string     `gorm:"column:args_hash;size:64;not null"`
	ApproverID  int64      `gorm:"column:approver_id;not null"`
	Status      string     `gorm:"column:status;size:24;not null"`
	ExpiresAt   time.Time  `gorm:"column:expires_at"`
	DecidedAt   *time.Time `gorm:"column:decided_at"`
	DecidedBy   int64      `gorm:"column:decided_by"`
	AttemptID   string     `gorm:"column:attempt_id;size:64"`
	ConsumedAt  *time.Time `gorm:"column:consumed_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (AgentApprovalRow) TableName() string { return "agent_approvals" }

func agentApprovalRowFromDomain(a domain.AgentApproval) AgentApprovalRow {
	return AgentApprovalRow{
		ID: a.ID, RunID: a.RunID, ToolCallID: a.ToolCallID, Tool: a.Tool,
		ArgsSummary: a.ArgsSummary, ArgsHash: a.ArgsHash, ApproverID: a.ApproverID,
		Status: a.Status, ExpiresAt: a.ExpiresAt, DecidedAt: a.DecidedAt, DecidedBy: a.DecidedBy,
		AttemptID: a.AttemptID, ConsumedAt: a.ConsumedAt, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func (r AgentApprovalRow) toDomain() domain.AgentApproval {
	return domain.AgentApproval{
		ID: r.ID, RunID: r.RunID, ToolCallID: r.ToolCallID, Tool: r.Tool,
		ArgsSummary: r.ArgsSummary, ArgsHash: r.ArgsHash, ApproverID: r.ApproverID,
		Status: r.Status, ExpiresAt: r.ExpiresAt, DecidedAt: r.DecidedAt, DecidedBy: r.DecidedBy,
		AttemptID: r.AttemptID, ConsumedAt: r.ConsumedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type AgentCheckpointRow struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	RunID        uint64    `gorm:"column:run_id;not null"`
	CheckpointID string    `gorm:"column:checkpoint_id;size:64;not null"`
	State        []byte    `gorm:"column:state;type:bytea;not null"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	ExpiresAt    time.Time `gorm:"column:expires_at;not null"`
}

func (AgentCheckpointRow) TableName() string { return "agent_checkpoints" }

type ToolInvocationRow struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	RunID         uint64    `gorm:"column:run_id;not null"`
	ToolCallID    string    `gorm:"column:tool_call_id;size:128;not null"`
	Tool          string    `gorm:"column:tool;size:128;not null"`
	ArgsHash      string    `gorm:"column:args_hash;size:64;not null"`
	Status        string    `gorm:"column:status;size:16;not null"`
	ResultSummary string    `gorm:"column:result_summary;type:text"`
	ErrorCode     string    `gorm:"column:error_code;size:64"`
	CreatedAt     time.Time `gorm:"column:created_at;not null"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null"`
}

func (ToolInvocationRow) TableName() string { return "agent_tool_invocations" }

func toolInvocationRowFromDomain(t domain.ToolInvocation) ToolInvocationRow {
	return ToolInvocationRow{
		ID: t.ID, RunID: t.RunID, ToolCallID: t.ToolCallID, Tool: t.Tool, ArgsHash: t.ArgsHash,
		Status: t.Status, ResultSummary: t.ResultSummary, ErrorCode: t.ErrorCode,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func (r ToolInvocationRow) toDomain() domain.ToolInvocation {
	return domain.ToolInvocation{
		ID: r.ID, RunID: r.RunID, ToolCallID: r.ToolCallID, Tool: r.Tool, ArgsHash: r.ArgsHash,
		Status: r.Status, ResultSummary: r.ResultSummary, ErrorCode: r.ErrorCode,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
