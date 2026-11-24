package sdk

import (
	"context"

	v1 "nonoka-im/api/im/v1"
)

type AuthResource struct {
	proto v1.AuthServiceHTTPClient
}

func (r *AuthResource) Register(ctx context.Context, username, password string) (int64, error) {
	reply, err := r.proto.Register(ctx, &v1.RegisterRequest{Username: username, Password: password})
	if err != nil {
		return 0, apiErrorFromKratos(err)
	}
	return reply.GetUserId(), nil
}

func (r *AuthResource) Login(ctx context.Context, username, password, deviceID string) (*TokenPair, error) {
	reply, err := r.proto.Login(ctx, &v1.LoginRequest{Username: username, Password: password, DeviceId: deviceID})
	if err != nil {
		return nil, apiErrorFromKratos(err)
	}
	return &TokenPair{UserID: reply.GetUserId(), Token: reply.GetToken(), RefreshToken: reply.GetRefreshToken()}, nil
}

func (r *AuthResource) Refresh(ctx context.Context, userID int64, deviceID, refreshToken string) (*TokenPair, error) {
	reply, err := r.proto.RefreshToken(ctx, &v1.RefreshTokenRequest{UserId: userID, DeviceId: deviceID, RefreshToken: refreshToken})
	if err != nil {
		return nil, apiErrorFromKratos(err)
	}
	return &TokenPair{UserID: reply.GetUserId(), Token: reply.GetToken(), RefreshToken: reply.GetRefreshToken()}, nil
}

func (r *AuthResource) Logout(ctx context.Context) error {
	if _, err := r.proto.Logout(ctx, &v1.LogoutRequest{}); err != nil {
		return apiErrorFromKratos(err)
	}
	return nil
}
