package service

import (
	"context"
	stderrors "errors"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/im/app"

	"github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GroupService provides group-related HTTP APIs.
type GroupService struct {
	pb.UnimplementedGroupServiceServer
	uc *app.GroupService
}

func NewGroupService(uc *app.GroupService) *GroupService { return &GroupService{uc: uc} }

func (s *GroupService) CreateGroup(ctx context.Context, req *pb.CreateGroupRequest) (*pb.CreateGroupReply, error) {
	group, err := s.uc.Create(ctx, extractUserIDFromContext(ctx), req.GetName(), req.GetMemberIds())
	if err != nil {
		return nil, groupError(err)
	}
	return &pb.CreateGroupReply{GroupId: group.GroupID, Topic: group.Topic}, nil
}

func (s *GroupService) ListMyGroups(ctx context.Context, _ *emptypb.Empty) (*pb.ListGroupsReply, error) {
	groups, err := s.uc.ListMine(ctx, extractUserIDFromContext(ctx))
	if err != nil {
		return nil, groupError(err)
	}
	reply := make([]*pb.Group, 0, len(groups))
	for _, group := range groups {
		reply = append(reply, &pb.Group{GroupId: group.GroupID, Topic: group.Topic, Name: group.Name, OwnerId: group.OwnerID, CreatedAt: group.CreatedAt})
	}
	return &pb.ListGroupsReply{Groups: reply}, nil
}

func (s *GroupService) GetGroup(ctx context.Context, req *pb.GetGroupRequest) (*pb.Group, error) {
	group, err := s.uc.Get(ctx, extractUserIDFromContext(ctx), req.GetGroupId())
	if err != nil {
		return nil, groupError(err)
	}
	return &pb.Group{GroupId: group.GroupID, Topic: group.Topic, Name: group.Name, OwnerId: group.OwnerID, CreatedAt: group.CreatedAt}, nil
}

func (s *GroupService) AddGroupMember(ctx context.Context, req *pb.AddGroupMemberRequest) (*emptypb.Empty, error) {
	if err := s.uc.AddMember(ctx, extractUserIDFromContext(ctx), req.GetGroupId(), req.GetUserId()); err != nil {
		return nil, groupError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GroupService) RemoveGroupMember(ctx context.Context, req *pb.RemoveGroupMemberRequest) (*emptypb.Empty, error) {
	if err := s.uc.RemoveMember(ctx, extractUserIDFromContext(ctx), req.GetGroupId(), req.GetUserId()); err != nil {
		return nil, groupError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GroupService) ListGroupMembers(ctx context.Context, req *pb.ListGroupMembersRequest) (*pb.ListGroupMembersReply, error) {
	members, err := s.uc.ListMembers(ctx, extractUserIDFromContext(ctx), req.GetGroupId())
	if err != nil {
		return nil, groupError(err)
	}
	reply := make([]*pb.GroupMember, 0, len(members))
	for _, member := range members {
		reply = append(reply, &pb.GroupMember{UserId: member.UserID, Username: member.Username})
	}
	return &pb.ListGroupMembersReply{Members: reply}, nil
}

func groupError(err error) error {
	switch {
	case stderrors.Is(err, app.ErrAuthRequired):
		return errors.Unauthorized("AUTH_REQUIRED", "authentication required")
	case stderrors.Is(err, app.ErrInvalidGroupName):
		return errors.BadRequest("INVALID_NAME", "group name is required")
	case stderrors.Is(err, app.ErrGroupNotFound):
		return errors.NotFound("GROUP_NOT_FOUND", "group not found")
	case stderrors.Is(err, app.ErrGroupForbidden):
		return errors.Forbidden("GROUP_MEMBER_REQUIRED", "group membership required")
	default:
		return err
	}
}
