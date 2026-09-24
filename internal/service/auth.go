package service

import (
	"context"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/biz"

	"github.com/go-kratos/kratos/v2/errors"
	jwtMiddleware "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	jwt5 "github.com/golang-jwt/jwt/v5"
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
	userID, token, refreshToken, err := s.uc.Login(ctx, req.Username, req.Password, req.DeviceId)
	if err != nil {
		return nil, err
	}

	return &pb.LoginReply{
		UserId:       userID,
		Token:        token,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.LoginReply, error) {
	if req.RefreshToken == "" {
		return nil, errors.BadRequest("AuthService.RefreshToken", "refresh_token is required")
	}

	userID, token, refreshToken, err := s.uc.Refresh(ctx, req.UserId, req.DeviceId, req.RefreshToken)
	if err != nil {
		return nil, err
	}

	return &pb.LoginReply{
		UserId:       userID,
		Token:        token,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, _ *pb.LogoutRequest) (*pb.LogoutReply, error) {
	userID := extractUserIDFromContext(ctx)
	if userID == 0 {
		return nil, errors.Unauthorized("UNAUTHORIZED", "authentication required")
	}

	if err := s.uc.Logout(ctx, userID, extractDeviceIDFromContext(ctx)); err != nil {
		return nil, err
	}
	return &pb.LogoutReply{Success: true}, nil
}

// extractDeviceIDFromContext extracts device_id from JWT claims in context,
// defaulting to "default" for tokens issued without a device.
func extractDeviceIDFromContext(ctx context.Context) string {
	claims, ok := jwtMiddleware.FromContext(ctx)
	if !ok {
		return "default"
	}
	if mapClaims, ok := claims.(jwt5.MapClaims); ok {
		if deviceID, ok := mapClaims["device_id"].(string); ok && deviceID != "" {
			return deviceID
		}
	}
	return "default"
}