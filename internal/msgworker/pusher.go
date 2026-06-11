package msgworker

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-kratos/kratos/v2/log"
	pb "nonoka-im/api/im/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// gatewayConn holds a connection to a single gateway node.
type gatewayConn struct {
	addr   string
	client pb.PushServiceClient
	conn   *grpc.ClientConn
}

// GatewayPusher pushes messages to online users via Gateway's gRPC PushService.
// It supports multiple gateway addresses; if a user is not on the first gateway,
// it falls back to trying others.
type GatewayPusher struct {
	conns   []*gatewayConn
	log     *log.Helper
	connMu  sync.RWMutex
}

// NewGatewayPusher creates a new GatewayPusher with support for multiple gateway addresses.
func NewGatewayPusher(gatewayAddrs []string, logger log.Logger) (*GatewayPusher, error) {
	if len(gatewayAddrs) == 0 {
		return nil, fmt.Errorf("at least one gateway address is required")
	}

	p := &GatewayPusher{
		log: log.NewHelper(logger),
	}

	for _, addr := range gatewayAddrs {
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("connect to gateway %s: %w", addr, err)
		}
		p.conns = append(p.conns, &gatewayConn{
			addr:   addr,
			client: pb.NewPushServiceClient(conn),
			conn:   conn,
		})
	}

	return p, nil
}

// PushToUser delivers a message to a single user.
// It tries each gateway in order until one succeeds or all fail.
func (p *GatewayPusher) PushToUser(ctx context.Context, userID int64, msg *pb.MessagePush) (int32, error) {
	req := &pb.PushToUserRequest{
		UserId:  userID,
		Message: msg,
	}

	p.connMu.RLock()
	conns := make([]*gatewayConn, len(p.conns))
	copy(conns, p.conns)
	p.connMu.RUnlock()

	for _, gc := range conns {
		resp, err := gc.client.PushToUser(ctx, req)
		if err != nil {
			p.log.Warnf("push to user %d via gateway %s failed: %v", userID, gc.addr, err)
			continue
		}
		if resp.GetDeliveredCount() > 0 {
			return resp.GetDeliveredCount(), nil
		}
	}

	return 0, fmt.Errorf("push to user %d failed on all gateways", userID)
}

// BatchPushToUsers delivers a message to multiple users.
// It sends the full batch to all gateways and collects results.
// A user is considered failed only if all gateways report failure.
func (p *GatewayPusher) BatchPushToUsers(ctx context.Context, userIDs []int64, msg *pb.MessagePush) (int32, []int64, error) {
	if len(userIDs) == 0 {
		return 0, nil, nil
	}

	p.connMu.RLock()
	conns := make([]*gatewayConn, len(p.conns))
	copy(conns, p.conns)
	p.connMu.RUnlock()

	if len(conns) == 0 {
		return 0, userIDs, fmt.Errorf("no gateway connections available")
	}

	// If only one gateway, use it directly.
	if len(conns) == 1 {
		req := &pb.BatchPushToUsersRequest{
			UserIds: userIDs,
			Message: msg,
		}
		resp, err := conns[0].client.BatchPushToUsers(ctx, req)
		if err != nil {
			return 0, userIDs, fmt.Errorf("batch push to %d users via gateway %s: %w", len(userIDs), conns[0].addr, err)
		}
		return resp.GetTotalDelivered(), resp.GetFailedUserIds(), nil
	}

	// Multiple gateways: track per-user success across gateways.
	userSuccess := make(map[int64]bool, len(userIDs))
	var totalDelivered int32

	for _, gc := range conns {
		req := &pb.BatchPushToUsersRequest{
			UserIds: userIDs,
			Message: msg,
		}
		resp, err := gc.client.BatchPushToUsers(ctx, req)
		if err != nil {
			p.log.Warnf("batch push via gateway %s failed: %v", gc.addr, err)
			continue
		}
		totalDelivered += resp.GetTotalDelivered()
		for _, uid := range resp.GetFailedUserIds() {
			// If already succeeded on another gateway, ignore this failure.
			if !userSuccess[uid] {
				userSuccess[uid] = false
			}
		}
		// Mark successfully delivered users
		for _, uid := range userIDs {
			found := false
			for _, fid := range resp.GetFailedUserIds() {
				if uid == fid {
					found = true
					break
				}
			}
			if !found {
				userSuccess[uid] = true
			}
		}
	}

	var failedUserIDs []int64
	for _, uid := range userIDs {
		if !userSuccess[uid] {
			failedUserIDs = append(failedUserIDs, uid)
		}
	}

	return totalDelivered, failedUserIDs, nil
}

// PushReceiptToUser delivers a send receipt to a single user.
func (p *GatewayPusher) PushReceiptToUser(ctx context.Context, userID int64, receipt *pb.SendReceipt) (int32, error) {
	req := &pb.PushReceiptToUserRequest{
		UserId:  userID,
		Receipt: receipt,
	}

	p.connMu.RLock()
	conns := make([]*gatewayConn, len(p.conns))
	copy(conns, p.conns)
	p.connMu.RUnlock()

	for _, gc := range conns {
		resp, err := gc.client.PushReceiptToUser(ctx, req)
		if err != nil {
			p.log.Warnf("push receipt to user %d via gateway %s failed: %v", userID, gc.addr, err)
			continue
		}
		if resp.GetDeliveredCount() > 0 {
			return resp.GetDeliveredCount(), nil
		}
	}

	return 0, fmt.Errorf("push receipt to user %d failed on all gateways", userID)
}

// BatchPushReceiptToUsers delivers send receipts to multiple users.
func (p *GatewayPusher) BatchPushReceiptToUsers(ctx context.Context, userIDs []int64, receipt *pb.SendReceipt) (int32, []int64, error) {
	if len(userIDs) == 0 {
		return 0, nil, nil
	}

	p.connMu.RLock()
	conns := make([]*gatewayConn, len(p.conns))
	copy(conns, p.conns)
	p.connMu.RUnlock()

	if len(conns) == 0 {
		return 0, userIDs, fmt.Errorf("no gateway connections available")
	}

	// If only one gateway, use it directly.
	if len(conns) == 1 {
		req := &pb.BatchPushReceiptToUsersRequest{
			UserIds: userIDs,
			Receipt: receipt,
		}
		resp, err := conns[0].client.BatchPushReceiptToUsers(ctx, req)
		if err != nil {
			return 0, userIDs, fmt.Errorf("batch push receipts to %d users via gateway %s: %w", len(userIDs), conns[0].addr, err)
		}
		return resp.GetTotalDelivered(), resp.GetFailedUserIds(), nil
	}

	// Multiple gateways: track per-user success across gateways.
	userSuccess := make(map[int64]bool, len(userIDs))
	var totalDelivered int32

	for _, gc := range conns {
		req := &pb.BatchPushReceiptToUsersRequest{
			UserIds: userIDs,
			Receipt: receipt,
		}
		resp, err := gc.client.BatchPushReceiptToUsers(ctx, req)
		if err != nil {
			p.log.Warnf("batch push receipts via gateway %s failed: %v", gc.addr, err)
			continue
		}
		totalDelivered += resp.GetTotalDelivered()
		for _, uid := range resp.GetFailedUserIds() {
			// If already succeeded on another gateway, ignore this failure.
			if !userSuccess[uid] {
				userSuccess[uid] = false
			}
		}
		// Mark successfully delivered users
		for _, uid := range userIDs {
			found := false
			for _, fid := range resp.GetFailedUserIds() {
				if uid == fid {
					found = true
					break
				}
			}
			if !found {
				userSuccess[uid] = true
			}
		}
	}

	var failedUserIDs []int64
	for _, uid := range userIDs {
		if !userSuccess[uid] {
			failedUserIDs = append(failedUserIDs, uid)
		}
	}

	return totalDelivered, failedUserIDs, nil
}

// Close closes all gRPC connections.
func (p *GatewayPusher) Close() error {
	p.connMu.Lock()
	defer p.connMu.Unlock()

	var firstErr error
	for _, gc := range p.conns {
		if gc.conn != nil {
			if err := gc.conn.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	p.conns = nil
	return firstErr
}
