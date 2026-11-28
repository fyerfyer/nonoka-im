package gateway

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

const (
	// Default dispatch strategy
	strategyConsistentHash   = "consistent_hash"
	strategyLeastConnections = "least_connections"
	defaultHeartbeatInterval = 10 * time.Second
	defaultNodeTTL           = 30 * time.Second
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

	redis     redis.UniversalClient
	strategy  string
	localNode *GatewayNode
	nodeTTL   time.Duration
	log       *log.Helper

	cacheMu sync.RWMutex
	alive   []*GatewayNode
	aliveAt time.Time
	ring    *consistentHash
	ringKey string
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
	nodes := s.aliveNodes(ctx)
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

	// Build the full URL list so multi-gateway-aware clients can cache it.
	urls := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.URL != "" {
			urls = append(urls, n.URL)
		}
	}
	// Ensure the selected node is first.
	if len(urls) == 0 || urls[0] != selected.URL {
		urls = append([]string{selected.URL}, urls...)
	}

	s.log.Debugf("gateway dispatched: user_id=%d strategy=%s selected=%s urls=%v",
		req.GetUserId(), s.strategy, selected.NodeID, urls)

	return &pb.GetGatewayReply{GatewayUrl: selected.URL, GatewayUrls: urls}, nil
}

// aliveNodes returns a short-lived local snapshot of gateway nodes. The
// dispatch endpoint is hit on every client reconnect; without this cache a
// reconnect storm performs N Redis round-trips per client.
func (s *DispatchService) aliveNodes(ctx context.Context) []*GatewayNode {
	if s.redis == nil {
		return nil
	}
	ttl := s.nodeTTL / 2
	if ttl < time.Second {
		ttl = time.Second
	}
	now := time.Now()
	s.cacheMu.RLock()
	if s.aliveAt.After(now.Add(-ttl)) {
		nodes := s.alive
		s.cacheMu.RUnlock()
		return nodes
	}
	s.cacheMu.RUnlock()

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.aliveAt.After(now.Add(-ttl)) {
		return s.alive
	}
	nodes, err := GetAliveNodes(ctx, s.redis, s.nodeTTL)
	if err != nil {
		s.log.Warnf("failed to discover gateway nodes from redis: %v", err)
		// Keep the previous snapshot during a short Redis outage; if there is
		// none, the caller falls back to the local node.
		return s.alive
	}
	s.alive = nodes
	s.aliveAt = now
	return nodes
}

// selectNode picks a gateway node according to the configured strategy.
func (s *DispatchService) selectNode(nodes []*GatewayNode, userID int64) *GatewayNode {
	switch s.strategy {
	case strategyLeastConnections:
		return SelectByLeastConnections(nodes)
	case strategyConsistentHash:
		fallthrough
	default:
		// Use user_id as the hash key for sticky routing.
		// If user_id is 0 (anonymous), fall through to least-connections.
		if userID != 0 {
			return s.selectByRing(nodes, fmt.Sprintf("%d", userID))
		}
		return SelectByLeastConnections(nodes)
	}
}

// selectByRing reuses the consistent hash ring until node membership changes.
// The old implementation rebuilt and sorted 150 virtual nodes per request.
func (s *DispatchService) selectByRing(nodes []*GatewayNode, key string) *GatewayNode {
	if len(nodes) == 0 {
		return nil
	}
	if len(nodes) == 1 {
		return nodes[0]
	}
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.NodeID)
	}
	sort.Strings(ids)
	ringKey := strings.Join(ids, "\x00")

	s.cacheMu.Lock()
	if s.ring == nil || s.ringKey != ringKey {
		ring := newConsistentHash(DefaultVirtualReplicas)
		for _, id := range ids {
			ring.Add(id)
		}
		s.ring = ring
		s.ringKey = ringKey
	}
	ring := s.ring
	s.cacheMu.Unlock()

	selectedID := ring.Get(key)
	for _, n := range nodes {
		if n.NodeID == selectedID {
			return n
		}
	}
	return nodes[0]
}

// buildLocalNode constructs the local gateway node info.
// Priority: 1) config static gateways matching this node_id  2) HTTP addr from env  3) localhost default
func buildLocalNode(dispatchConf *conf.Dispatch) *GatewayNode {
	nodeID := os.Getenv("GATEWAY_NODE_ID")
	if nodeID == "" {
		nodeID, _ = os.Hostname()
	}
	if nodeID == "" {
		nodeID = "gateway-0"
	}

	// Check if this node is configured statically
	if dispatchConf != nil {
		for _, gw := range dispatchConf.Gateways {
			if gw.NodeId == nodeID && gw.GatewayUrl != "" {
				return &GatewayNode{
					NodeID: nodeID,
					URL:    gw.GatewayUrl,
				}
			}
		}
		// If static config exists but no match, use the first one as fallback
		if len(dispatchConf.Gateways) > 0 && dispatchConf.Gateways[0].GatewayUrl != "" {
			return &GatewayNode{
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
	return &GatewayNode{
		NodeID: nodeID,
		URL:    wsAddr,
	}
}
