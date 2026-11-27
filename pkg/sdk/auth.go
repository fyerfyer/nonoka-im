package sdk

import (
	"context"
	"net/http"

	v1 "nonoka-im/api/im/v1"
)

type AuthResource struct {
	http *apiTransport
}

func (r *AuthResource) Register(ctx context.Context, username, password string) (int64, error) {
	var reply v1.RegisterReply
	if err := r.http.doProto(ctx, http.MethodPost, "/v1/auth/register", nil, &v1.RegisterRequest{Username: username, Password: password}, &reply); err != nil {
		return 0, err
	}
	return reply.GetUserId(), nil
}

func (r *AuthResource) Login(ctx context.Context, username, password, deviceID string) (*TokenPair, error) {
	var reply v1.LoginReply
	if err := r.http.doProto(ctx, http.MethodPost, "/v1/auth/login", nil, &v1.LoginRequest{Username: username, Password: password, DeviceId: deviceID}, &reply); err != nil {
		return nil, err
	}
	return &TokenPair{UserID: reply.GetUserId(), Token: reply.GetToken(), RefreshToken: reply.GetRefreshToken()}, nil
}

func (r *AuthResource) Refresh(ctx context.Context, userID int64, deviceID, refreshToken string) (*TokenPair, error) {
	var reply v1.LoginReply
	if err := r.http.doProto(ctx, http.MethodPost, "/v1/auth/refresh", nil, &v1.RefreshTokenRequest{UserId: userID, DeviceId: deviceID, RefreshToken: refreshToken}, &reply); err != nil {
		return nil, err
	}
	return &TokenPair{UserID: reply.GetUserId(), Token: reply.GetToken(), RefreshToken: reply.GetRefreshToken()}, nil
}

func (r *AuthResource) Logout(ctx context.Context) error {
	return r.http.doProto(ctx, http.MethodPost, "/v1/auth/logout", nil, &v1.LogoutRequest{}, nil)
}
