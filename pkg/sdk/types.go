package sdk

import "time"

// MessageType is the public message content type. It mirrors the IM enum but
// never exposes the generated protobuf type.
type MessageType int32

const (
	MessageTypeUnspecified MessageType = 0
	MessageTypeText        MessageType = 1
	MessageTypeImage       MessageType = 2
	MessageTypeFile        MessageType = 3
	MessageTypeVoice       MessageType = 4
	MessageTypeAgentCard   MessageType = 5
)

// MessageStatus is the client-side delivery state of a message.
type MessageStatus string

const (
	MessageStatusSending   MessageStatus = "sending"
	MessageStatusSent      MessageStatus = "sent"
	MessageStatusDelivered MessageStatus = "delivered"
	MessageStatusRead      MessageStatus = "read"
	MessageStatusFailed    MessageStatus = "failed"
	MessageStatusRecalled  MessageStatus = "recalled"
)

// Message is one IM message.
type Message struct {
	ID               int64         `json:"msgId"`
	Topic            string        `json:"topic"`
	SenderID         int64         `json:"senderId"`
	Type             MessageType   `json:"msgType"`
	Content          []byte        `json:"content"`
	Timestamp        int64         `json:"timestamp"`
	TopicSeq         uint64        `json:"topicSeq"`
	ClientMessageID  string        `json:"clientMsgId,omitempty"`
	MentionedUserIDs []int64       `json:"mentionedUserIds,omitempty"`
	Recalled         bool          `json:"recalled,omitempty"`
	Status           MessageStatus `json:"-"`
}

// MediaContent is the JSON payload stored in IMAGE/FILE messages. The SDK
// must send this shape, not a bare URL: the server derives file authorization
// and agent multimodal input from file_id.
type MediaContent struct {
	FileID string `json:"file_id,omitempty"`
	URL    string `json:"url,omitempty"`
	Name   string `json:"name,omitempty"`
	Mime   string `json:"mime,omitempty"`
	Size   int64  `json:"size,omitempty"`
}

// SendRequest creates one message. ClientMessageID is optional; the SDK
// generates and reuses one across retries.
type SendRequest struct {
	Topic            string
	Type             MessageType
	Content          []byte
	ClientMessageID  string
	MentionedUserIDs []int64
}

// SendResult is the server acknowledgement of a message. Confirmed is true
// only when the durable msg_id/topic_seq receipt was observed (or recovered
// through pull). HTTP-only acceptance can be successful with Confirmed=false.
type SendResult struct {
	ClientMessageID string
	MessageID       int64
	Timestamp       int64
	TopicSeq        uint64
	Confirmed       bool
}

// PullResult is one page of history or offline catch-up messages.
type PullResult struct {
	Messages []*Message
	HasMore  bool
	NextSeq  uint64
}

// PullRequest selects forward (LastSeq > 0) or backward (EndSeq > 0) paging.
type PullRequest struct {
	Topic   string
	LastSeq uint64
	EndSeq  uint64
	Limit   int32
}

// SearchRequest searches one conversation. Query may be empty only when
// HasFile is true.
type SearchRequest struct {
	Topic    string
	Query    string
	SenderID int64
	Since    int64
	Until    int64
	HasFile  bool
	Limit    int32
	Offset   int32
}

// SearchHit is one authorized message search result.
type SearchHit struct {
	MessageID int64
	Topic     string
	TopicSeq  uint64
	SenderID  int64
	Timestamp int64
	MsgType   MessageType
	HasFile   bool
	Snippet   string
}

type SearchResult struct {
	Hits    []*SearchHit
	HasMore bool
}

type User struct {
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
}

type UserPresence struct {
	UserID int64 `json:"userId"`
	Online bool  `json:"online"`
}

type Conversation struct {
	Topic          string `json:"topic"`
	Type           string `json:"type"`
	PeerID         int64  `json:"peerId"`
	PeerUsername   string `json:"peerUsername"`
	LastMsgPreview string `json:"lastMsgPreview,omitempty"`
	LastMsgAt      int64  `json:"lastMsgAt,omitempty"`
	LastSeq        uint64 `json:"lastSeq"`
	LastReadSeq    uint64 `json:"lastReadSeq"`
	UnreadCount    int32  `json:"unreadCount"`
}

type Group struct {
	GroupID   string `json:"groupId"`
	Topic     string `json:"topic"`
	Name      string `json:"name"`
	OwnerID   int64  `json:"ownerId"`
	CreatedAt int64  `json:"createdAt"`
}

type GroupMember struct {
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
}

type UploadedFile struct {
	FileID string `json:"file_id"`
	URL    string `json:"url"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Mime   string `json:"mime"`
}

type TokenPair struct {
	UserID       int64  `json:"userId"`
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
}

// ---------- Agent platform ----------

type Model struct {
	ID                         int64   `json:"id"`
	ModelKey                   string  `json:"modelKey"`
	Provider                   string  `json:"provider"`
	GatewayModelKey            string  `json:"gatewayModelKey"`
	DisplayName                string  `json:"displayName"`
	Icon                       string  `json:"icon,omitempty"`
	ContextWindow              int     `json:"contextWindow"`
	MaxOutputTokens            int     `json:"maxOutputTokens"`
	SupportsTools              bool    `json:"supportsTools"`
	SupportsVision             bool    `json:"supportsVision"`
	SupportsReasoning          bool    `json:"supportsReasoning"`
	InputPricePerMtok          float64 `json:"inputPricePerMtok"`
	OutputPricePerMtok         float64 `json:"outputPricePerMtok"`
	InputCacheReadPricePerMtok float64 `json:"inputCacheReadPricePerMtok"`
	Enabled                    bool    `json:"enabled"`
	Curated                    bool    `json:"curated"`
	Recommended                bool    `json:"recommended"`
	SortOrder                  int     `json:"sortOrder"`
}

type ModelCuration struct {
	DisplayName       string `json:"displayName,omitempty"`
	Icon              string `json:"icon,omitempty"`
	SupportsTools     *bool  `json:"supportsTools,omitempty"`
	SupportsVision    *bool  `json:"supportsVision,omitempty"`
	SupportsReasoning *bool  `json:"supportsReasoning,omitempty"`
	Enabled           *bool  `json:"enabled,omitempty"`
	Recommended       *bool  `json:"recommended,omitempty"`
	SortOrder         *int   `json:"sortOrder,omitempty"`
}

type CapabilityPreset struct {
	Key            string   `json:"key"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	DefaultEnabled bool     `json:"defaultEnabled"`
	Tools          []string `json:"tools"`
}

type ToolDefinition struct {
	Key            string         `json:"key"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Category       string         `json:"category"`
	Risk           string         `json:"risk"`
	DefaultPolicy  string         `json:"defaultPolicy"`
	DefaultEnabled bool           `json:"defaultEnabled"`
	System         bool           `json:"system"`
	Hidden         bool           `json:"hidden"`
	Advanced       bool           `json:"advanced"`
	SideEffect     bool           `json:"sideEffect"`
	Presets        []string       `json:"presets"`
	InputSchema    map[string]any `json:"inputSchema,omitempty"`
}

type AgentDraft struct {
	Instruction       string            `json:"instruction"`
	ModelID           int64             `json:"modelId"`
	Tools             []string          `json:"tools"`
	CapabilityPresets []string          `json:"capabilityPresets"`
	ToolPolicies      map[string]string `json:"toolPolicies"`
	MaxTokens         int               `json:"maxTokens"`
	MCPBindings       []MCPBinding      `json:"mcpBindings"`
	Skills            []SkillBinding    `json:"skills"`
	Revision          int64             `json:"revision"`
}

type MCPBinding struct {
	ServerID   uint64 `json:"serverId"`
	Server     string `json:"server,omitempty"`
	Tool       string `json:"tool"`
	Exposed    string `json:"exposed,omitempty"`
	SchemaHash string `json:"schemaHash,omitempty"`
	Policy     string `json:"policy,omitempty"`
}

type SkillBinding struct {
	SkillID  uint64 `json:"skillId"`
	Name     string `json:"name,omitempty"`
	Version  int64  `json:"version"`
	Checksum string `json:"checksum,omitempty"`
}

type AgentVersion struct {
	ID                uint64            `json:"id"`
	AgentID           int64             `json:"agentId"`
	Version           int64             `json:"version"`
	Instruction       string            `json:"instruction"`
	ModelID           int64             `json:"modelId"`
	ModelKey          string            `json:"modelKey"`
	Tools             []string          `json:"tools"`
	CapabilityPresets []string          `json:"capabilityPresets"`
	ToolPolicies      map[string]string `json:"toolPolicies"`
	MaxTokens         int               `json:"maxTokens"`
	MCPBindings       []MCPBinding      `json:"mcpBindings"`
	Skills            []SkillBinding    `json:"skills"`
	PublishNote       string            `json:"publishNote"`
	PublishedBy       int64             `json:"publishedBy"`
	PublishedAt       *time.Time        `json:"publishedAt,omitempty"`
	SourceRevision    int64             `json:"sourceRevision"`
	CreatedAt         time.Time         `json:"createdAt"`
}

type AgentVersionSummary struct {
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

type Agent struct {
	ID             int64         `json:"id"`
	BotUserID      int64         `json:"botUserId"`
	OwnerUserID    int64         `json:"ownerUserId"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	Status         string        `json:"status"`
	CurrentVersion int64         `json:"currentVersion"`
	CanManage      bool          `json:"canManage"`
	CanOwn         bool          `json:"canOwn"`
	DraftChanged   bool          `json:"draftChanged"`
	Draft          *AgentDraft   `json:"draft,omitempty"`
	Published      *AgentVersion `json:"published,omitempty"`
}

type AgentShare struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Role   string `json:"role,omitempty"`
}

type AgentRun struct {
	ID               uint64           `json:"id"`
	EventID          string           `json:"eventId"`
	AgentID          int64            `json:"agentId"`
	AgentVersion     int64            `json:"agentVersion"`
	SessionID        uint64           `json:"sessionId,omitempty"`
	ActorUserID      int64            `json:"actorUserId"`
	Topic            string           `json:"topic"`
	MsgID            int64            `json:"msgId"`
	TopicSeq         uint64           `json:"topicSeq"`
	Status           string           `json:"status"`
	ErrorCode        string           `json:"errorCode,omitempty"`
	ErrorClass       string           `json:"errorClass,omitempty"`
	Attempts         int              `json:"attempts"`
	ReplyMsgID       int64            `json:"replyMsgId,omitempty"`
	ReplyClientMsgID string           `json:"replyClientMsgId,omitempty"`
	Steps            []AgentRunStep   `json:"steps,omitempty"`
	ToolInvocations  []ToolInvocation `json:"toolInvocations,omitempty"`
	InputTokens      int              `json:"inputTokens"`
	OutputTokens     int              `json:"outputTokens"`
	TotalTokens      int              `json:"totalTokens"`
	TraceID          string           `json:"traceId,omitempty"`
	CreatedAt        time.Time        `json:"createdAt"`
	StartedAt        *time.Time       `json:"startedAt,omitempty"`
	FinishedAt       *time.Time       `json:"finishedAt,omitempty"`
}

type AgentRunStep struct {
	ID         uint64    `json:"id"`
	Seq        uint64    `json:"seq"`
	Type       string    `json:"type"`
	Tool       string    `json:"tool,omitempty"`
	ToolCallID string    `json:"toolCallId,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Status     string    `json:"status,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ToolInvocation struct {
	ID            uint64    `json:"id"`
	ToolCallID    string    `json:"toolCallId"`
	Tool          string    `json:"tool"`
	Status        string    `json:"status"`
	ResultSummary string    `json:"resultSummary,omitempty"`
	ErrorCode     string    `json:"errorCode,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type AgentApproval struct {
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

type MCPTool struct {
	ID          uint64 `json:"id"`
	ServerID    uint64 `json:"serverId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Risk        string `json:"risk"`
	SchemaHash  string `json:"schemaHash"`
	InputSchema string `json:"inputSchema,omitempty"`
	Annotations string `json:"annotations,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type MCPServer struct {
	ID            uint64     `json:"id"`
	OwnerUserID   int64      `json:"ownerUserId"`
	Name          string     `json:"name"`
	DisplayName   string     `json:"displayName"`
	Description   string     `json:"description"`
	URL           string     `json:"url"`
	Transport     string     `json:"transport"`
	AuthType      string     `json:"authType"`
	HasCredential bool       `json:"hasCredential"`
	Visibility    string     `json:"visibility"`
	ReviewStatus  string     `json:"reviewStatus"`
	ReviewReason  string     `json:"reviewReason,omitempty"`
	HealthStatus  string     `json:"healthStatus"`
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
	Enabled       bool       `json:"enabled"`
	ToolsRevision int64      `json:"toolsRevision"`
	Tools         []MCPTool  `json:"tools,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type MCPConnection struct {
	ID            uint64     `json:"id"`
	ServerID      uint64     `json:"serverId"`
	AuthType      string     `json:"authType"`
	Status        string     `json:"status"`
	HasCredential bool       `json:"hasCredential"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt    *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type SkillVersion struct {
	ID           uint64     `json:"id"`
	SkillID      uint64     `json:"skillId"`
	Version      int64      `json:"version"`
	Checksum     string     `json:"checksum"`
	Status       string     `json:"status"`
	ReviewStatus string     `json:"reviewStatus"`
	ReviewReason string     `json:"reviewReason,omitempty"`
	CreatedBy    int64      `json:"createdBy"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	Content      string     `json:"content,omitempty"`
	ReviewAt     *time.Time `json:"reviewAt,omitempty"`
}

type Skill struct {
	ID            uint64         `json:"id"`
	OwnerUserID   int64          `json:"ownerUserId"`
	Slug          string         `json:"slug"`
	DisplayName   string         `json:"displayName"`
	Description   string         `json:"description"`
	Visibility    string         `json:"visibility"`
	ReviewStatus  string         `json:"reviewStatus"`
	LatestVersion int64          `json:"latestVersion"`
	Versions      []SkillVersion `json:"versions,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}
