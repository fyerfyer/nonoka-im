package sdk

import (
	"context"

	v1 "nonoka-im/api/im/v1"
)

// AuthService provides authentication-related HTTP APIs.
// It can be used independently without WebSocket.
type AuthService struct {
	client v1.AuthServiceHTTPClient
}

// newAuthService creates a new AuthService.
func newAuthService(svc *serviceClient) *AuthService {
	return &AuthService{
		client: v1.NewAuthServiceHTTPClient(svc.httpCli),
	}
}

// Register creates a new user account.
func (s *AuthService) Register(ctx context.Context, username, password string) (*RegisterResult, error) {
	reply, err := s.client.Register(ctx, &v1.RegisterRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, err
	}
	return &RegisterResult{UserID: reply.UserId}, nil
}

// Login authenticates a user and returns a token.
func (s *AuthService) Login(ctx context.Context, username, password string) (*LoginResult, error) {
	reply, err := s.client.Login(ctx, &v1.LoginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, err
	}
	return &LoginResult{
		Token:  reply.Token,
		UserID: reply.UserId,
	}, nil
}

// RegisterResult is returned after successful registration.
type RegisterResult struct {
	UserID int64
}

// LoginResult is returned after successful login.
type LoginResult struct {
	Token  string
	UserID int64
}
