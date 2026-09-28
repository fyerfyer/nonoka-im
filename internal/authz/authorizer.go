package authz

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"nonoka-im/internal/data"
	"nonoka-im/internal/topic"
)

// Authorizer is the shared security boundary for topic and resource access.
// Implementations must fail closed when membership cannot be determined.
type Authorizer interface {
	CanAccessTopic(context.Context, int64, string) error
	CanReadFile(context.Context, int64, int64) error
}

// TopicAuthorizer uses the authoritative PostgreSQL group repository. File
// ownership is checked by FileService, which supplies the stored owner ID.
type TopicAuthorizer struct{ groups data.GroupRepo }

func NewTopicAuthorizer(groups data.GroupRepo) *TopicAuthorizer {
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
		members, err := a.groups.ListMembers(ctx, gid)
		if err != nil {
			return fmt.Errorf("check group membership: %w", err)
		}
		found := false
		for _, m := range members {
			if m != nil && m.UserId == userID {
				found = true
				break
			}
		}
		if !found {
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

func (a *TopicAuthorizer) CanReadFile(_ context.Context, userID, ownerID int64) error {
	if userID <= 0 {
		return fmt.Errorf("authentication required")
	}
	if userID != ownerID {
		return fmt.Errorf("file access denied")
	}
	return nil
}
