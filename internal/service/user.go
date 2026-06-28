package service

import (
	"context"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/biz"
)

// UserService provides user-related HTTP APIs.
type UserService struct {
	pb.UnimplementedUserServiceServer

	uc *biz.AuthUsecase
}

// NewUserService creates a new UserService.
func NewUserService(uc *biz.AuthUsecase) *UserService {
	return &UserService{uc: uc}
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
