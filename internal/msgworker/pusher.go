package msgworker

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	pb "nonoka-im/api/im/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GatewayPusher pushes messages to online users via Gateway's gRPC PushService.
type GatewayPusher struct {
	client pb.PushServiceClient
	conn   *grpc.ClientConn
	log    *log.Helper
}

// NewGatewayPusher creates a new GatewayPusher.
func NewGatewayPusher(gatewayAddr string, logger log.Logger) (*GatewayPusher, error) {
	conn, err := grpc.NewClient(gatewayAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to gateway %s: %w", gatewayAddr, err)
	}

	client := pb.NewPushServiceClient(conn)
	return &GatewayPusher{
		client: client,
		conn:   conn,
		log:    log.NewHelper(logger),
	}, nil
}

// PushToUser delivers a message to a single user.
func (p *GatewayPusher) PushToUser(ctx context.Context, userID int64, msg *pb.MessagePush) (int32, error) {
	req := &pb.PushToUserRequest{
		UserId:  userID,
		Message: msg,
	}
	resp, err := p.client.PushToUser(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("push to user %d: %w", userID, err)
	}
	return resp.GetDeliveredCount(), nil
}

// BatchPushToUsers delivers a message to multiple users.
func (p *GatewayPusher) BatchPushToUsers(ctx context.Context, userIDs []int64, msg *pb.MessagePush) (int32, []int64, error) {
	req := &pb.BatchPushToUsersRequest{
		UserIds: userIDs,
		Message: msg,
	}
	resp, err := p.client.BatchPushToUsers(ctx, req)
	if err != nil {
		return 0, nil, fmt.Errorf("batch push to %d users: %w", len(userIDs), err)
	}
	return resp.GetTotalDelivered(), resp.GetFailedUserIds(), nil
}

// Close closes the gRPC connection.
func (p *GatewayPusher) Close() error {
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}
