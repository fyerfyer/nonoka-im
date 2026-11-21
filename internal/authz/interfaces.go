package authz

import "context"

// AgentAuthorizer is the read/use side of agent authorization.
type AgentAuthorizer interface {
	CanViewAgent(ctx context.Context, userID, agentID int64) error
	CanUseAgent(ctx context.Context, userID, agentID int64) error
	CanManageAgent(ctx context.Context, userID, agentID int64) error
	CanAccessTopic(ctx context.Context, userID int64, topic string) error
}

// AgentAdminAuthorizer changes agent ownership, shares and tool bindings.
type AgentAdminAuthorizer interface {
	SetAgentOwner(ctx context.Context, agentID, userID int64) error
	SetAgentToolBinding(ctx context.Context, agentID int64, tool string, enabled bool) error
	SetAgentShare(ctx context.Context, agentID int64, kind, target string, enabled bool) error
	ListAgentShares(ctx context.Context, agentID int64) ([]AgentShare, error)
	DeleteAgentTuples(ctx context.Context, agentID int64, tools []string) error
	CanManageGroup(ctx context.Context, userID int64, groupID string) error
	SetGroupMember(ctx context.Context, groupID string, userID int64, enabled bool) error
}

// GroupAuthorizer is the authorization surface required by the group usecase.
type GroupAuthorizer interface {
	CanAccessTopic(ctx context.Context, userID int64, topic string) error
	CanManageGroup(ctx context.Context, userID int64, groupID string) error
	SetGroupOwner(ctx context.Context, groupID string, userID int64) error
	SetGroupMember(ctx context.Context, groupID string, userID int64, enabled bool) error
	SetTopicGroup(ctx context.Context, topic, groupID string) error
}

// FileAuthorizer is the authorization surface required by the file usecase.
type FileAuthorizer interface {
	CanReadFile(ctx context.Context, userID int64, fileID string) error
	SetFileOwner(ctx context.Context, fileID string, userID int64) error
	SetFileViewer(ctx context.Context, fileID string, userID int64, enabled bool) error
	SetFileSharedTopic(ctx context.Context, fileID, topic string, enabled bool) error
}
