package service

import (
	"context"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/data"

	"github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

// groupMemberIDs returns membership using the existing group repository API.
// Keeping this check in the service makes every group read/write endpoint
// enforce the same authorization policy without introducing another component.
func (s *GroupService) groupMemberIDs(ctx context.Context, groupID string) (map[int64]struct{}, error) {
	members, err := s.repo.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	ids := make(map[int64]struct{}, len(members))
	for _, member := range members {
		if member != nil {
			ids[member.UserId] = struct{}{}
		}
	}
	return ids, nil
}

func (s *GroupService) requireMember(ctx context.Context, groupID string, userID int64) (*data.Group, error) {
	group, err := s.repo.GetGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	members, err := s.groupMemberIDs(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if _, ok := members[userID]; !ok {
		return nil, errors.Forbidden("GROUP_MEMBER_REQUIRED", "group membership required")
	}
	return group, nil
}

// GroupService provides group-related HTTP APIs.
type GroupService struct {
	pb.UnimplementedGroupServiceServer

	repo data.GroupRepo
}

// NewGroupService creates a new GroupService.
func NewGroupService(repo data.GroupRepo) *GroupService {
	return &GroupService{repo: repo}
}

// CreateGroup creates a new group and adds the creator as a member.
func (s *GroupService) CreateGroup(ctx context.Context, req *pb.CreateGroupRequest) (*pb.CreateGroupReply, error) {
	userID := extractUserIDFromContext(ctx)
	if userID == 0 {
		return nil, errors.Unauthorized("AUTH_REQUIRED", "authentication required")
	}

	name := req.GetName()
	if name == "" {
		return nil, errors.BadRequest("INVALID_NAME", "group name is required")
	}

	group, err := s.repo.CreateGroup(ctx, name, userID, req.GetMemberIds())
	if err != nil {
		return nil, err
	}

	return &pb.CreateGroupReply{
		GroupId: group.GroupID,
		Topic:   group.Topic,
	}, nil
}

// ListMyGroups returns groups the authenticated user is a member of.
func (s *GroupService) ListMyGroups(ctx context.Context, _ *emptypb.Empty) (*pb.ListGroupsReply, error) {
	userID := extractUserIDFromContext(ctx)
	if userID == 0 {
		return nil, errors.Unauthorized("AUTH_REQUIRED", "authentication required")
	}

	groups, err := s.repo.ListMyGroups(ctx, userID)
	if err != nil {
		return nil, err
	}

	reply := make([]*pb.Group, len(groups))
	for i, g := range groups {
		reply[i] = &pb.Group{
			GroupId:   g.GroupID,
			Topic:     g.Topic,
			Name:      g.Name,
			OwnerId:   g.OwnerID,
			CreatedAt: g.CreatedAt,
		}
	}

	return &pb.ListGroupsReply{Groups: reply}, nil
}

// GetGroup returns details of a specific group.
func (s *GroupService) GetGroup(ctx context.Context, req *pb.GetGroupRequest) (*pb.Group, error) {
	userID := extractUserIDFromContext(ctx)
	if userID == 0 {
		return nil, errors.Unauthorized("AUTH_REQUIRED", "authentication required")
	}

	groupID := req.GetGroupId()
	if groupID == "" {
		return nil, errors.BadRequest("INVALID_GROUP_ID", "group id is required")
	}

	group, err := s.requireMember(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}

	return &pb.Group{
		GroupId:   group.GroupID,
		Topic:     group.Topic,
		Name:      group.Name,
		OwnerId:   group.OwnerID,
		CreatedAt: group.CreatedAt,
	}, nil
}

// AddGroupMember adds a user to a group.
func (s *GroupService) AddGroupMember(ctx context.Context, req *pb.AddGroupMemberRequest) (*emptypb.Empty, error) {
	userID := extractUserIDFromContext(ctx)
	if userID == 0 {
		return nil, errors.Unauthorized("AUTH_REQUIRED", "authentication required")
	}

	groupID := req.GetGroupId()
	memberID := req.GetUserId()
	if groupID == "" || memberID == 0 {
		return nil, errors.BadRequest("INVALID_PARAMS", "group id and user id are required")
	}

	group, err := s.repo.GetGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	members, err := s.groupMemberIDs(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group.OwnerID != userID {
		if _, ok := members[userID]; !ok {
			return nil, errors.Forbidden("GROUP_MEMBER_REQUIRED", "only group members can add members")
		}
	}
	if err := s.repo.AddMember(ctx, groupID, memberID); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// RemoveGroupMember removes a user from a group.
func (s *GroupService) RemoveGroupMember(ctx context.Context, req *pb.RemoveGroupMemberRequest) (*emptypb.Empty, error) {
	userID := extractUserIDFromContext(ctx)
	if userID == 0 {
		return nil, errors.Unauthorized("AUTH_REQUIRED", "authentication required")
	}

	groupID := req.GetGroupId()
	memberID := req.GetUserId()
	if groupID == "" || memberID == 0 {
		return nil, errors.BadRequest("INVALID_PARAMS", "group id and user id are required")
	}

	group, err := s.repo.GetGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group.OwnerID != userID && memberID != userID {
		return nil, errors.Forbidden("GROUP_REMOVE_FORBIDDEN", "only the group owner or the member can remove this user")
	}
	if memberID == group.OwnerID && userID != group.OwnerID {
		return nil, errors.Forbidden("GROUP_OWNER_REQUIRED", "the group owner cannot be removed by another member")
	}
	if err := s.repo.RemoveMember(ctx, groupID, memberID); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ListGroupMembers returns members of a group.
func (s *GroupService) ListGroupMembers(ctx context.Context, req *pb.ListGroupMembersRequest) (*pb.ListGroupMembersReply, error) {
	userID := extractUserIDFromContext(ctx)
	if userID == 0 {
		return nil, errors.Unauthorized("AUTH_REQUIRED", "authentication required")
	}

	groupID := req.GetGroupId()
	if groupID == "" {
		return nil, errors.BadRequest("INVALID_GROUP_ID", "group id is required")
	}

	if _, err := s.requireMember(ctx, groupID, userID); err != nil {
		return nil, err
	}
	members, err := s.repo.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}

	return &pb.ListGroupMembersReply{Members: members}, nil
}
