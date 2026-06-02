package service

import (
	"context"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/biz"

	"github.com/go-kratos/kratos/v2/errors"
)

type AuthService struct {
	pb.UnimplementedAuthServiceServer

	uc *biz.AuthUsecase
}

func NewAuthService(uc *biz.AuthUsecase) *AuthService {
	return &AuthService{uc: uc}
}

func (s *AuthService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterReply, error) {
	if req.Username == "" || req.Password == "" {
		return nil, errors.BadRequest("AuthService.Register", "Username or password is required")
	}

	user, err := s.uc.Register(ctx, req.Username, req.Password)
	if err != nil {
		return nil, err
	}

	return &pb.RegisterReply{
		UserId: user.ID,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginReply, error) {
	userID, token, err := s.uc.Login(ctx, req.Username, req.Password, req.DeviceId)
	if err != nil {
		return nil, err
	}

	return &pb.LoginReply{
		UserId: userID,
		Token:  token,
	}, nil
}