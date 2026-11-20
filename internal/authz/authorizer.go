package authz

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"nonoka-im/internal/topic"
)

// Authorizer is the shared security boundary for topic and resource access.
// Implementations must fail closed when membership cannot be determined.
type Authorizer interface {
	CanAccessTopic(context.Context, int64, string) error
	CanReadFile(context.Context, int64, string) error
	// CanExecuteTool authorizes one agent tool call for the actor. resourceType
	// is "file" or "topic" (empty resourceRef means the current topic).
	CanExecuteTool(ctx context.Context, actorID, agentID int64, tool, currentTopic, resourceType, resourceRef string) error
}

// GroupMembership is the narrow authorization dependency: the authz module
// must not depend on the concrete PostgreSQL repository.
type GroupMembership interface {
	IsGroupMember(ctx context.Context, groupID string, userID int64) (bool, error)
}

// TopicAuthorizer uses the authoritative group membership store. File
// ownership is checked by FileService, which supplies the stored owner ID.
type TopicAuthorizer struct{ groups GroupMembership }

func NewTopicAuthorizer(groups GroupMembership) *TopicAuthorizer {
	return &TopicAuthorizer{groups: groups}
}

func (a *TopicAuthorizer) CanAccessTopic(ctx context.Context, userID int64, raw string) error {
	if userID <= 0 {
		return fmt.Errorf("authentication required")
	}
	t, err := topic.NormalizeTopic(raw)
	if err != nil {
		return err
	}
	switch topic.ParseType(t) {
	case topic.TypeP2P:
		x, y, err := topic.ExtractUserIDsFromP2PTopic(t)
		if err != nil {
			return err
		}
		if userID != x && userID != y {
			return fmt.Errorf("topic access denied")
		}
	case topic.TypeGroup:
		gid, err := topic.ExtractGroupID(t)
		if err != nil {
			return err
		}
		if a.groups == nil {
			return fmt.Errorf("group authorization unavailable")
		}
		member, err := a.groups.IsGroupMember(ctx, gid, userID)
		if err != nil {
			return fmt.Errorf("check group membership: %w", err)
		}
		if !member {
			return fmt.Errorf("topic access denied")
		}
	case topic.TypeSystem:
		// System topics are user scoped: sys_<user id>.
		owner, err := strconv.ParseInt(strings.TrimPrefix(t, topic.PrefixSystem), 10, 64)
		if err != nil || owner != userID {
			return fmt.Errorf("topic access denied")
		}
	default:
		return fmt.Errorf("invalid topic")
	}
	return nil
}

func (a *TopicAuthorizer) CanReadFile(_ context.Context, userID int64, fileID string) error {
	ownerID, err := strconv.ParseInt(fileID, 10, 64)
	if err != nil {
		return fmt.Errorf("legacy authorizer requires owner ID")
	}
	if userID <= 0 {
		return fmt.Errorf("authentication required")
	}
	if userID != ownerID {
		return fmt.Errorf("file access denied")
	}
	return nil
}

// CanExecuteTool is a migration shim for the legacy authorizer: tool bindings
// live in OpenFGA, so deployments using this authorizer cannot run tools.
func (a *TopicAuthorizer) CanExecuteTool(_ context.Context, _, _ int64, _, _, _, _ string) error {
	return fmt.Errorf("tool authorization requires OpenFGA")
}
