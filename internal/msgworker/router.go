package msgworker

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

// Redis keys used by the gateway registry. They are duplicated here to avoid an
// import cycle between internal/msgworker and internal/gateway.
const (
	gatewayNodesKey = "im:gateway:nodes"
	gatewayNodeKey  = "im:gateway:%s"
)

// GatewayNodeInfo holds the discovery data for a single gateway node.
// It mirrors gateway.GatewayNode without importing the gateway package.
type GatewayNodeInfo struct {
	NodeID    string
	URL       string
	GrpcAddr  string
	ConnCount int
}

// GatewayRouter resolves online users to gateway nodes using the distributed
// session index stored in Redis. It allows MsgWorker to push messages only to
// the gateway nodes that actually host the target user's devices, instead of
// broadcasting to every gateway in the cluster.
type GatewayRouter struct {
	redis   redis.UniversalClient
	nodeTTL time.Duration
	log     *log.Helper
}

// NewGatewayRouter creates a new gateway router backed by Redis.
func NewGatewayRouter(redis redis.UniversalClient, nodeTTL time.Duration, logger log.Logger) *GatewayRouter {
	if nodeTTL <= 0 {
		nodeTTL = 30 * time.Second
	}
	return &GatewayRouter{
		redis:   redis,
		nodeTTL: nodeTTL,
		log:     log.NewHelper(logger),
	}
}

// ResolveUserNodes returns a mapping from gateway node ID to the list of user
// IDs that have at least one active device session on that node.
// Users that are offline (no session entry) are not included in the result.
func (r *GatewayRouter) ResolveUserNodes(ctx context.Context, userIDs []int64) (map[string][]int64, error) {
	if r.redis == nil || len(userIDs) == 0 {
		return map[string][]int64{}, nil
	}

	result := make(map[string][]int64)
	for _, uid := range userIDs {
		deviceKey := fmt.Sprintf("im:session:%d:devices", uid)
		devices, err := r.redis.HGetAll(ctx, deviceKey).Result()
		if err != nil {
			r.log.Warnf("resolve user nodes failed: user_id=%d, err=%v", uid, err)
			continue
		}
		if len(devices) == 0 {
			continue
		}

		seen := make(map[string]struct{})
		for _, nodeID := range devices {
			if _, ok := seen[nodeID]; ok {
				continue
			}
			seen[nodeID] = struct{}{}
			result[nodeID] = append(result[nodeID], uid)
		}
	}

	return result, nil
}

// GetAliveNodes returns all gateway nodes whose heartbeat is within the TTL window.
func (r *GatewayRouter) GetAliveNodes(ctx context.Context) ([]*GatewayNodeInfo, error) {
	nodeIDs, err := r.redis.SMembers(ctx, gatewayNodesKey).Result()
	if err != nil {
		return nil, fmt.Errorf("smembers failed: %w", err)
	}
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	now := time.Now()
	cutoff := now.Add(-r.nodeTTL).Unix()

	nodes := make([]*GatewayNodeInfo, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeKey := fmt.Sprintf(gatewayNodeKey, id)
		vals, err := r.redis.HGetAll(ctx, nodeKey).Result()
		if err != nil || len(vals) == 0 {
			continue
		}

		heartbeatStr := vals["heartbeat"]
		heartbeat, _ := strconv.ParseInt(heartbeatStr, 10, 64)
		if heartbeat < cutoff {
			continue
		}

		conns, _ := strconv.Atoi(vals["conns"])
		nodes = append(nodes, &GatewayNodeInfo{
			NodeID:    id,
			URL:       vals["url"],
			GrpcAddr:  vals["grpc_addr"],
			ConnCount: conns,
		})
	}

	return nodes, nil
}

// GetNodeGRPCAddr returns the gRPC address of an alive node by its node ID.
// If the node is not found or has no gRPC address, it returns an empty string.
func (r *GatewayRouter) GetNodeGRPCAddr(ctx context.Context, nodeID string) string {
	nodes, err := r.GetAliveNodes(ctx)
	if err != nil {
		r.log.Warnf("get alive nodes failed: %v", err)
		return ""
	}
	for _, n := range nodes {
		if n.NodeID == nodeID {
			return n.GrpcAddr
		}
	}
	return ""
}
