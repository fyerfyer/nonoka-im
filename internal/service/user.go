package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/errors"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/biz"
)

// UserService provides user-related HTTP APIs.
type UserService struct {
	pb.UnimplementedUserServiceServer

	uc       *biz.AuthUsecase
	presence PresenceChecker
}

// PresenceChecker hides the Redis/gateway implementation from the API layer.
type PresenceChecker interface {
	IsUserOnline(context.Context, int64) (bool, error)
}

// NewUserService creates a new UserService.
func NewUserService(uc *biz.AuthUsecase) *UserService {
	return &UserService{uc: uc}
}

// SetPresenceChecker wires optional distributed presence into the service.
func (s *UserService) SetPresenceChecker(checker PresenceChecker) {
	s.presence = checker
}

func (s *UserService) GetUserPresence(ctx context.Context, req *pb.GetUserPresenceRequest) (*pb.GetUserPresenceReply, error) {
	if req.GetUserId() <= 0 {
		return nil, errors.BadRequest("INVALID_USER_ID", "user id is required")
	}
	online := false
	var err error
	if s.presence != nil {
		online, err = s.presence.IsUserOnline(ctx, req.GetUserId())
		if err != nil {
			return nil, err
		}
	}
	return &pb.GetUserPresenceReply{UserId: req.GetUserId(), Online: online}, nil
}

// SearchUsers searches users by username prefix. Authentication is required.
func (s *UserService) SearchUsers(ctx context.Context, req *pb.SearchUsersRequest) (*pb.SearchUsersReply, error) {
	prefix := req.GetUsername()
	if prefix == "" {
		return &pb.SearchUsersReply{Users: []*pb.User{}}, nil
	}

	limit := req.GetLimit()
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	users, err := s.uc.Repo().SearchUsersByPrefix(ctx, prefix, limit)
	if err != nil {
		return nil, err
	}

	reply := make([]*pb.User, len(users))
	for i, u := range users {
		reply[i] = &pb.User{
			UserId:   u.ID,
			Username: u.Username,
		}
	}
	return &pb.SearchUsersReply{Users: reply}, nil
}
