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

// GetGatewayURLs returns the discovered gateway URL list for the given user.
// The first element is the recommended gateway. If the server returns no list,
// a single-element list containing GatewayUrl is returned.
func (s *DispatchService) GetGatewayURLs(ctx context.Context, userID int64) ([]string, error) {
	reply, err := s.client.Gateway(ctx, &v1.GetGatewayRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("get gateway failed: %w", err)
	}
	if len(reply.GatewayUrls) > 0 {
		return reply.GatewayUrls, nil
	}
	if reply.GatewayUrl != "" {
		return []string{reply.GatewayUrl}, nil
	}
	return nil, fmt.Errorf("get gateway returned empty url")
}
