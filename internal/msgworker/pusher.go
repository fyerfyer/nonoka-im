package msgworker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	pb "nonoka-im/api/im/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// maxDynamicConnAge is the maximum lifetime of a dynamically discovered gateway
	// connection. Older connections are closed and recreated to prevent fd leaks
	// when gateway nodes are replaced frequently.
	maxDynamicConnAge = 30 * time.Minute
)

// gatewayConn holds a connection to a single gateway node.
type gatewayConn struct {
	addr      string
	client    pb.PushServiceClient
	conn      *grpc.ClientConn
	createdAt time.Time
}

// GatewayPusher pushes messages to online users via Gateway's gRPC PushService.
// It supports two modes:
//   1. Static address list (backward compatible): tries every configured gateway.
//   2. Dynamic routing: when a Router is configured, it resolves each user
//      to the gateway node hosting their device sessions and pushes only to those
//      nodes, avoiding O(#gateways) broadcast amplification.
type GatewayPusher struct {
	// staticConns are pre-configured gateway connections used when no router is set.
	staticConns []*gatewayConn

	// router resolves user -> gateway node using Redis session index.
	router Router

	// dynamicConns are connections to gateway nodes discovered via the router.
	dynamicConns map[string]*gatewayConn // addr -> conn
	dynamicMu    sync.RWMutex

	log    *log.Helper
	connMu sync.RWMutex
}

// NewGatewayPusher creates a new GatewayPusher with support for multiple gateway addresses.
// The provided addresses are used as static fallback when no GatewayRouter is configured.
func NewGatewayPusher(gatewayAddrs []string, logger log.Logger) (*GatewayPusher, error) {
	if len(gatewayAddrs) == 0 {
		return nil, fmt.Errorf("at least one gateway address is required")
	}

	p := &GatewayPusher{
		log:          log.NewHelper(logger),
		dynamicConns: make(map[string]*gatewayConn),
	}

	for _, addr := range gatewayAddrs {
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("connect to gateway %s: %w", addr, err)
		}
		p.staticConns = append(p.staticConns, &gatewayConn{
			addr:      addr,
			client:    pb.NewPushServiceClient(conn),
			conn:      conn,
			createdAt: time.Now(),
		})
	}

	return p, nil
}

// SetRouter enables dynamic user-to-node routing. Must be called before the first push.
func (p *GatewayPusher) SetRouter(router Router) {
	p.dynamicMu.Lock()
	defer p.dynamicMu.Unlock()
	p.router = router
}

// PushToUser delivers a message to a single user.
// When a router is configured, it pushes only to the node hosting the user's session.
// If the router fails (e.g., Redis unavailable), it falls back to the static gateway list.
func (p *GatewayPusher) PushToUser(ctx context.Context, userID int64, msg *pb.MessagePush) (int32, error) {
	if p.router != nil {
		nodeMap, err := p.router.ResolveUserNodes(ctx, []int64{userID})
		if err != nil {
			p.log.Warnf("router resolve failed, falling back to static gateways: %v", err)
			return p.pushToUserStatic(ctx, userID, msg)
		}
		aliveMap, err := p.router.GetAliveNodesMap(ctx)
		if err != nil {
			p.log.Warnf("router alive nodes failed, falling back to static gateways: %v", err)
			return p.pushToUserStatic(ctx, userID, msg)
		}
		for nodeID := range nodeMap {
			node, ok := aliveMap[nodeID]
			if !ok || node.GrpcAddr == "" {
				continue
			}
			gc, err := p.getOrCreateDynamicConn(node.GrpcAddr)
			if err != nil {
				p.log.Warnf("connect to gateway %s failed: %v", node.GrpcAddr, err)
				continue
			}
			resp, err := gc.client.PushToUser(ctx, &pb.PushToUserRequest{
				UserId:  userID,
				Message: msg,
			})
			if err != nil {
				p.log.Warnf("push to user %d via gateway %s failed: %v", userID, node.GrpcAddr, err)
				continue
			}
			if resp.GetDeliveredCount() > 0 {
				return resp.GetDeliveredCount(), nil
			}
		}
		return 0, fmt.Errorf("push to user %d failed: no online session or reachable gateway", userID)
	}

	return p.pushToUserStatic(ctx, userID, msg)
}

// pushToUserStatic pushes to a single user using the static gateway list.
func (p *GatewayPusher) pushToUserStatic(ctx context.Context, userID int64, msg *pb.MessagePush) (int32, error) {

	req := &pb.PushToUserRequest{
		UserId:  userID,
		Message: msg,
	}

	p.connMu.RLock()
	conns := make([]*gatewayConn, len(p.staticConns))
	copy(conns, p.staticConns)
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
// With a router, it groups users by gateway node and pushes only to nodes that
// host at least one target user. Without a router, it falls back to broadcasting
// to all statically configured gateways.
func (p *GatewayPusher) BatchPushToUsers(ctx context.Context, userIDs []int64, msg *pb.MessagePush) (int32, []int64, error) {
	if len(userIDs) == 0 {
		return 0, nil, nil
	}

	if p.router != nil {
		return p.batchPushWithRouting(ctx, userIDs, msg)
	}

	return p.batchPushStatic(ctx, userIDs, msg)
}

// batchPushWithRouting groups users by the gateway node that hosts their session
// and sends one batch request per node. Users with no online session are returned
// as failed so the retry queue can handle them later.
// If the router fails (e.g., Redis unavailable), it falls back to static broadcast.
func (p *GatewayPusher) batchPushWithRouting(ctx context.Context, userIDs []int64, msg *pb.MessagePush) (int32, []int64, error) {
	aliveNodes, err := p.router.GetAliveNodes(ctx)
	if err != nil {
		p.log.Warnf("router get alive nodes failed, falling back to static gateways: %v", err)
		return p.batchPushStatic(ctx, userIDs, msg)
	}

	nodeMap, err := p.router.ResolveUserNodesWithNodes(ctx, userIDs, aliveNodes)
	if err != nil {
		p.log.Warnf("router resolve user nodes failed, falling back to static gateways: %v", err)
		return p.batchPushStatic(ctx, userIDs, msg)
	}

	aliveMap := make(map[string]*GatewayNodeInfo, len(aliveNodes))
	for _, n := range aliveNodes {
		aliveMap[n.NodeID] = n
	}

	// Build a set of online users for quick lookup.
	onlineUserSet := make(map[int64]struct{}, len(userIDs))
	for _, ids := range nodeMap {
		for _, uid := range ids {
			onlineUserSet[uid] = struct{}{}
		}
	}

	var totalDelivered int32
	var failedUserIDs []int64

	for nodeID, ids := range nodeMap {
		node, ok := aliveMap[nodeID]
		if !ok || node.GrpcAddr == "" {
			p.log.Warnf("no grpc address for gateway node %s, marking %d users failed", nodeID, len(ids))
			failedUserIDs = append(failedUserIDs, ids...)
			continue
		}

		gc, err := p.getOrCreateDynamicConn(node.GrpcAddr)
		if err != nil {
			p.log.Warnf("connect to gateway %s failed: %v", node.GrpcAddr, err)
			failedUserIDs = append(failedUserIDs, ids...)
			continue
		}

		resp, err := gc.client.BatchPushToUsers(ctx, &pb.BatchPushToUsersRequest{
			UserIds: ids,
			Message: msg,
		})
		if err != nil {
			p.log.Warnf("batch push via gateway %s failed: %v", node.GrpcAddr, err)
			failedUserIDs = append(failedUserIDs, ids...)
			continue
		}

		totalDelivered += resp.GetTotalDelivered()
		failedUserIDs = append(failedUserIDs, resp.GetFailedUserIds()...)
	}

	// Any input user not present in an online session is offline -> failed.
	for _, uid := range userIDs {
		if _, ok := onlineUserSet[uid]; !ok {
			failedUserIDs = append(failedUserIDs, uid)
		}
	}

	return totalDelivered, failedUserIDs, nil
}

// batchPushStatic broadcasts the full user list to every statically configured gateway.
// A user is considered failed only if all gateways report failure.
func (p *GatewayPusher) batchPushStatic(ctx context.Context, userIDs []int64, msg *pb.MessagePush) (int32, []int64, error) {
	p.connMu.RLock()
	conns := make([]*gatewayConn, len(p.staticConns))
	copy(conns, p.staticConns)
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
		failedSet := make(map[int64]struct{}, len(resp.GetFailedUserIds()))
		for _, uid := range resp.GetFailedUserIds() {
			failedSet[uid] = struct{}{}
		}
		for _, uid := range userIDs {
			if _, failed := failedSet[uid]; !failed {
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

// getOrCreateDynamicConn returns an existing gRPC connection to addr or creates one.
// It evicts connections that are shut down or older than maxDynamicConnAge to
// avoid leaking fds when gateway nodes churn.
func (p *GatewayPusher) getOrCreateDynamicConn(addr string) (*gatewayConn, error) {
	p.dynamicMu.RLock()
	gc, ok := p.dynamicConns[addr]
	p.dynamicMu.RUnlock()
	if ok && p.isDynamicConnUsable(gc) {
		return gc, nil
	}

	p.dynamicMu.Lock()
	defer p.dynamicMu.Unlock()

	// Double-check after acquiring write lock.
	if gc, ok := p.dynamicConns[addr]; ok {
		if p.isDynamicConnUsable(gc) {
			return gc, nil
		}
		// Evict stale/dead connection.
		_ = gc.conn.Close()
		delete(p.dynamicConns, addr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial gateway %s: %w", addr, err)
	}

	gc = &gatewayConn{
		addr:      addr,
		client:    pb.NewPushServiceClient(conn),
		conn:      conn,
		createdAt: time.Now(),
	}
	p.dynamicConns[addr] = gc
	return gc, nil
}

// isDynamicConnUsable reports whether a dynamic connection is healthy enough to reuse.
func (p *GatewayPusher) isDynamicConnUsable(gc *gatewayConn) bool {
	if gc == nil || gc.conn == nil {
		return false
	}
	state := gc.conn.GetState()
	if state == connectivity.Shutdown {
		return false
	}
	if time.Since(gc.createdAt) > maxDynamicConnAge {
		return false
	}
	return true
}

// PushReceiptToUser delivers a send receipt to a single user.
// If the router is unavailable, it falls back to the static gateway list.
func (p *GatewayPusher) PushReceiptToUser(ctx context.Context, userID int64, receipt *pb.SendReceipt) (int32, error) {
	if p.router != nil {
		nodeMap, err := p.router.ResolveUserNodes(ctx, []int64{userID})
		if err != nil {
			p.log.Warnf("router resolve failed, falling back to static gateways: %v", err)
			return p.pushReceiptToUserStatic(ctx, userID, receipt)
		}
		aliveMap, err := p.router.GetAliveNodesMap(ctx)
		if err != nil {
			p.log.Warnf("router alive nodes failed, falling back to static gateways: %v", err)
			return p.pushReceiptToUserStatic(ctx, userID, receipt)
		}
		for nodeID := range nodeMap {
			node, ok := aliveMap[nodeID]
			if !ok || node.GrpcAddr == "" {
				continue
			}
			gc, err := p.getOrCreateDynamicConn(node.GrpcAddr)
			if err != nil {
				p.log.Warnf("connect to gateway %s failed: %v", node.GrpcAddr, err)
				continue
			}
			resp, err := gc.client.PushReceiptToUser(ctx, &pb.PushReceiptToUserRequest{
				UserId:  userID,
				Receipt: receipt,
			})
			if err != nil {
				p.log.Warnf("push receipt to user %d via gateway %s failed: %v", userID, node.GrpcAddr, err)
				continue
			}
			if resp.GetDeliveredCount() > 0 {
				return resp.GetDeliveredCount(), nil
			}
		}
		return 0, fmt.Errorf("push receipt to user %d failed: no online session or reachable gateway", userID)
	}

	return p.pushReceiptToUserStatic(ctx, userID, receipt)
}

// pushReceiptToUserStatic pushes a receipt to a single user using static gateways.
func (p *GatewayPusher) pushReceiptToUserStatic(ctx context.Context, userID int64, receipt *pb.SendReceipt) (int32, error) {
	req := &pb.PushReceiptToUserRequest{
		UserId:  userID,
		Receipt: receipt,
	}

	p.connMu.RLock()
	conns := make([]*gatewayConn, len(p.staticConns))
	copy(conns, p.staticConns)
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

// BatchPushReceiptToUsers delivers send receipts to multiple users concurrently.
func (p *GatewayPusher) BatchPushReceiptToUsers(ctx context.Context, userIDs []int64, receipt *pb.SendReceipt) (int32, []int64, error) {
	if len(userIDs) == 0 {
		return 0, nil, nil
	}

	if p.router != nil {
		return p.batchPushReceiptsWithRouting(ctx, userIDs, receipt)
	}

	return p.batchPushReceiptsStatic(ctx, userIDs, receipt)
}

// batchPushReceiptsWithRouting routes receipt batches to the gateway nodes that
// host the target users' sessions.
// If the router fails (e.g., Redis unavailable), it falls back to static broadcast.
func (p *GatewayPusher) batchPushReceiptsWithRouting(ctx context.Context, userIDs []int64, receipt *pb.SendReceipt) (int32, []int64, error) {
	aliveNodes, err := p.router.GetAliveNodes(ctx)
	if err != nil {
		p.log.Warnf("router get alive nodes failed, falling back to static gateways: %v", err)
		return p.batchPushReceiptsStatic(ctx, userIDs, receipt)
	}

	nodeMap, err := p.router.ResolveUserNodesWithNodes(ctx, userIDs, aliveNodes)
	if err != nil {
		p.log.Warnf("router resolve user nodes failed, falling back to static gateways: %v", err)
		return p.batchPushReceiptsStatic(ctx, userIDs, receipt)
	}

	aliveMap := make(map[string]*GatewayNodeInfo, len(aliveNodes))
	for _, n := range aliveNodes {
		aliveMap[n.NodeID] = n
	}

	onlineUserSet := make(map[int64]struct{}, len(userIDs))
	for _, ids := range nodeMap {
		for _, uid := range ids {
			onlineUserSet[uid] = struct{}{}
		}
	}

	var totalDelivered int32
	var failedUserIDs []int64

	for nodeID, ids := range nodeMap {
		node, ok := aliveMap[nodeID]
		if !ok || node.GrpcAddr == "" {
			failedUserIDs = append(failedUserIDs, ids...)
			continue
		}

		gc, err := p.getOrCreateDynamicConn(node.GrpcAddr)
		if err != nil {
			p.log.Warnf("connect to gateway %s failed: %v", node.GrpcAddr, err)
			failedUserIDs = append(failedUserIDs, ids...)
			continue
		}

		resp, err := gc.client.BatchPushReceiptToUsers(ctx, &pb.BatchPushReceiptToUsersRequest{
			UserIds: ids,
			Receipt: receipt,
		})
		if err != nil {
			p.log.Warnf("batch push receipts via gateway %s failed: %v", node.GrpcAddr, err)
			failedUserIDs = append(failedUserIDs, ids...)
			continue
		}

		totalDelivered += resp.GetTotalDelivered()
		failedUserIDs = append(failedUserIDs, resp.GetFailedUserIds()...)
	}

	for _, uid := range userIDs {
		if _, ok := onlineUserSet[uid]; !ok {
			failedUserIDs = append(failedUserIDs, uid)
		}
	}

	return totalDelivered, failedUserIDs, nil
}

// batchPushReceiptsStatic broadcasts receipt batches to every statically configured gateway.
func (p *GatewayPusher) batchPushReceiptsStatic(ctx context.Context, userIDs []int64, receipt *pb.SendReceipt) (int32, []int64, error) {
	p.connMu.RLock()
	conns := make([]*gatewayConn, len(p.staticConns))
	copy(conns, p.staticConns)
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
		failedSet := make(map[int64]struct{}, len(resp.GetFailedUserIds()))
		for _, uid := range resp.GetFailedUserIds() {
			failedSet[uid] = struct{}{}
		}
		for _, uid := range userIDs {
			if _, failed := failedSet[uid]; !failed {
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
	for _, gc := range p.staticConns {
		if gc.conn != nil {
			_ = gc.conn.Close()
		}
	}
	p.staticConns = nil
	p.connMu.Unlock()

	p.dynamicMu.Lock()
	for _, gc := range p.dynamicConns {
		if gc.conn != nil {
			_ = gc.conn.Close()
		}
	}
	p.dynamicConns = make(map[string]*gatewayConn)
	p.dynamicMu.Unlock()

	return nil
}
