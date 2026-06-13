package service

import (
	"context"
	"fmt"
	"os"
	"time"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/gateway"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

const (
	// Default dispatch strategy
	strategyConsistentHash     = "consistent_hash"
	strategyLeastConnections   = "least_connections"
	defaultHeartbeatInterval   = 10 * time.Second
	defaultNodeTTL             = 30 * time.Second
)

// DispatchService implements the gateway dispatch logic.
// It supports multiple dispatch strategies:
//   - consistent_hash: route the same user to the same gateway (sticky)
//   - least_connections: route to the gateway with fewest active connections
//
// Gateway nodes are discovered dynamically via Redis heartbeat,
// combined with static configuration as fallback.
type DispatchService struct {
	pb.UnimplementedDispatchServiceServer

	redis      redis.UniversalClient
	strategy   string
	localNode  *gateway.GatewayNode
	nodeTTL    time.Duration
	log        *log.Helper
}

// NewDispatchService creates a new dispatch service.
func NewDispatchService(redis redis.UniversalClient, dispatchConf *conf.Dispatch, logger log.Logger) *DispatchService {
	strategy := strategyConsistentHash
	nodeTTL := defaultNodeTTL

	if dispatchConf != nil {
		if dispatchConf.Strategy != "" {
			strategy = dispatchConf.Strategy
		}
		if dispatchConf.NodeTtl != nil {
			nodeTTL = dispatchConf.NodeTtl.AsDuration()
		}
	}

	// Build local node info from config or environment
	localNode := buildLocalNode(dispatchConf)

	s := &DispatchService{
		redis:     redis,
		strategy:  strategy,
		localNode: localNode,
		nodeTTL:   nodeTTL,
		log:       log.NewHelper(logger),
	}

	s.log.Infof("dispatch service created: strategy=%s local_node=%s url=%s",
		strategy, localNode.NodeID, localNode.URL)
	return s
}

// Gateway returns the optimal gateway URL for the client.
// It first discovers alive gateway nodes from Redis, then applies the
// configured dispatch strategy. Falls back to the local node if Redis
// is unavailable or no nodes are registered.
func (s *DispatchService) Gateway(ctx context.Context, req *pb.GetGatewayRequest) (*pb.GetGatewayReply, error) {
	// Discover alive nodes from Redis
	var nodes []*gateway.GatewayNode
	var err error

	if s.redis != nil {
		nodes, err = gateway.GetAliveNodes(ctx, s.redis, s.nodeTTL)
		if err != nil {
			s.log.Warnf("failed to discover gateway nodes from redis: %v", err)
		}
	}

	// Fallback: if no alive nodes discovered, use local node
	if len(nodes) == 0 {
		s.log.Debugf("no alive gateway nodes found, falling back to local node: %s", s.localNode.URL)
		return &pb.GetGatewayReply{GatewayUrl: s.localNode.URL}, nil
	}

	// Apply dispatch strategy
	selected := s.selectNode(nodes, req.GetUserId())
	if selected == nil {
		// Should not happen, but fallback just in case
		return &pb.GetGatewayReply{GatewayUrl: s.localNode.URL}, nil
	}

	s.log.Debugf("gateway dispatched: user_id=%d strategy=%s selected=%s url=%s",
		req.GetUserId(), s.strategy, selected.NodeID, selected.URL)

	return &pb.GetGatewayReply{GatewayUrl: selected.URL}, nil
}

// selectNode picks a gateway node according to the configured strategy.
func (s *DispatchService) selectNode(nodes []*gateway.GatewayNode, userID int64) *gateway.GatewayNode {
	switch s.strategy {
	case strategyLeastConnections:
		return gateway.SelectByLeastConnections(nodes)
	case strategyConsistentHash:
		fallthrough
	default:
		// Use user_id as the hash key for sticky routing.
		// If user_id is 0 (anonymous), fall through to least-connections.
		if userID != 0 {
			return gateway.SelectByConsistentHash(nodes, fmt.Sprintf("%d", userID))
		}
		return gateway.SelectByLeastConnections(nodes)
	}
}

// buildLocalNode constructs the local gateway node info.
// Priority: 1) config static gateways matching this node_id  2) HTTP addr from env  3) localhost default
func buildLocalNode(dispatchConf *conf.Dispatch) *gateway.GatewayNode {
	nodeID, _ := os.Hostname()
	if nodeID == "" {
		nodeID = "gateway-0"
	}

	// Check if this node is configured statically
	if dispatchConf != nil {
		for _, gw := range dispatchConf.Gateways {
			if gw.NodeId == nodeID && gw.GatewayUrl != "" {
				return &gateway.GatewayNode{
					NodeID: nodeID,
					URL:    gw.GatewayUrl,
				}
			}
		}
		// If static config exists but no match, use the first one as fallback
		if len(dispatchConf.Gateways) > 0 && dispatchConf.Gateways[0].GatewayUrl != "" {
			return &gateway.GatewayNode{
				NodeID: nodeID,
				URL:    dispatchConf.Gateways[0].GatewayUrl,
			}
		}
	}

	// Default: derive from HTTP addr or env
	wsAddr := os.Getenv("GATEWAY_WS_ADDR")
	if wsAddr == "" {
		wsAddr = "ws://localhost:8000/ws"
	}
	return &gateway.GatewayNode{
		NodeID: nodeID,
		URL:    wsAddr,
	}
}
