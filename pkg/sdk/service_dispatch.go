package sdk

import (
	"context"
	"fmt"

	v1 "nonoka-im/api/im/v1"
)

// DispatchService provides gateway dispatch APIs.
type DispatchService struct {
	client v1.DispatchServiceHTTPClient
}

// newDispatchService creates a new DispatchService.
func newDispatchService(svc *serviceClient) *DispatchService {
	return &DispatchService{
		client: v1.NewDispatchServiceHTTPClient(svc.httpCli),
	}
}

// GetGateway returns the WebSocket gateway URL for the given user.
func (s *DispatchService) GetGateway(ctx context.Context, userID int64) (string, error) {
	reply, err := s.client.Gateway(ctx, &v1.GetGatewayRequest{
		UserId: userID,
	})
	if err != nil {
		return "", fmt.Errorf("get gateway failed: %w", err)
	}
	return reply.GatewayUrl, nil
}
