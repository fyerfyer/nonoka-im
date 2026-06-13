package gateway

import (
	"context"
	"fmt"
	"hash/fnv"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

const (
	// Redis keys
	gatewayNodesKey = "im:gateway:nodes"
	gatewayNodeKey  = "im:gateway:%s"

	// DefaultVirtualReplicas is the default number of virtual replicas per node
	// on the consistent hash ring.
	DefaultVirtualReplicas = 150
)

// GatewayNode represents a discovered gateway instance.
type GatewayNode struct {
	NodeID    string
	URL       string
	GrpcAddr  string
	ConnCount int
	UpdatedAt time.Time
}

// GatewayRegistry manages this node's registration and discovers other nodes.
// It uses Redis as the shared registry with TTL-based heartbeat.
type GatewayRegistry struct {
	redis    redis.UniversalClient
	nodeID   string
	url      string
	grpcAddr string
	interval time.Duration
	ttl      time.Duration
	manager  *Manager
	log      *log.Helper

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewGatewayRegistry creates a new registry for this gateway node.
func NewGatewayRegistry(redis redis.UniversalClient, nodeID string, url string, grpcAddr string, interval time.Duration, ttl time.Duration, manager *Manager, logger log.Logger) *GatewayRegistry {
	return &GatewayRegistry{
		redis:    redis,
		nodeID:   nodeID,
		url:      url,
		grpcAddr: grpcAddr,
		interval: interval,
		ttl:      ttl,
		manager:  manager,
		log:      log.NewHelper(logger),
		stopCh:   make(chan struct{}),
	}
}

// StartHeartbeat begins the periodic heartbeat reporting in a background goroutine.
func (r *GatewayRegistry) StartHeartbeat() {
	r.wg.Go(func() {

		// Report immediately on start
		if err := r.reportOnce(); err != nil {
			r.log.Warnf("initial heartbeat failed: %v", err)
		}

		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := r.reportOnce(); err != nil {
					r.log.Warnf("heartbeat failed: %v", err)
				}
			case <-r.stopCh:
				return
			}
		}
	})
}

// Stop stops the heartbeat goroutine and removes this node from the registry.
func (r *GatewayRegistry) Stop(ctx context.Context) {
	close(r.stopCh)
	r.wg.Wait()

	// Remove this node from the registry
	pipe := r.redis.Pipeline()
	pipe.SRem(ctx, gatewayNodesKey, r.nodeID)
	pipe.Del(ctx, fmt.Sprintf(gatewayNodeKey, r.nodeID))
	if _, err := pipe.Exec(ctx); err != nil {
		r.log.Warnf("failed to unregister node %s: %v", r.nodeID, err)
	}
}

// reportOnce writes the current node state to Redis.
func (r *GatewayRegistry) reportOnce() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	connCount := 0
	if r.manager != nil {
		connCount = r.manager.Count()
	}

	nodeKey := fmt.Sprintf(gatewayNodeKey, r.nodeID)
	pipe := r.redis.Pipeline()
	pipe.SAdd(ctx, gatewayNodesKey, r.nodeID)
	pipe.HSet(ctx, nodeKey, map[string]interface{}{
		"url":        r.url,
		"grpc_addr":  r.grpcAddr,
		"conns":      connCount,
		"heartbeat":  time.Now().Unix(),
	})
	pipe.Expire(ctx, nodeKey, r.ttl)
	pipe.Expire(ctx, gatewayNodesKey, r.ttl*2)

	_, err := pipe.Exec(ctx)
	return err
}

// GetAliveNodes returns all gateway nodes whose heartbeat is within the TTL window.
func GetAliveNodes(ctx context.Context, redis redis.UniversalClient, nodeTTL time.Duration) ([]*GatewayNode, error) {
	nodeIDs, err := redis.SMembers(ctx, gatewayNodesKey).Result()
	if err != nil {
		return nil, fmt.Errorf("smembers failed: %w", err)
	}
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	now := time.Now()
	cutoff := now.Add(-nodeTTL).Unix()

	nodes := make([]*GatewayNode, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeKey := fmt.Sprintf(gatewayNodeKey, id)
		vals, err := redis.HGetAll(ctx, nodeKey).Result()
		if err != nil {
			continue
		}
		if len(vals) == 0 {
			continue
		}

		heartbeatStr := vals["heartbeat"]
		heartbeat, _ := strconv.ParseInt(heartbeatStr, 10, 64)
		if heartbeat < cutoff {
			// Stale node, skip
			continue
		}

		conns, _ := strconv.Atoi(vals["conns"])
		nodes = append(nodes, &GatewayNode{
			NodeID:    id,
			URL:       vals["url"],
			GrpcAddr:  vals["grpc_addr"],
			ConnCount: conns,
			UpdatedAt: time.Unix(heartbeat, 0),
		})
	}

	return nodes, nil
}

// -------------------- Consistent Hash --------------------

// consistentHash implements a simple consistent hash ring.
type consistentHash struct {
	replicas int
	ring     []uint64          // sorted hash ring
	nodes    map[uint64]string // hash -> nodeID
	nodeSet  map[string]struct{}
	mu       sync.RWMutex
}

// newConsistentHash creates a new consistent hash with the given virtual replicas per node.
func newConsistentHash(replicas int) *consistentHash {
	if replicas <= 0 {
		replicas = DefaultVirtualReplicas
	}
	return &consistentHash{
		replicas: replicas,
		ring:     make([]uint64, 0),
		nodes:    make(map[uint64]string),
		nodeSet:  make(map[string]struct{}),
	}
}

// Add adds a node to the hash ring.
func (ch *consistentHash) Add(node string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if _, exists := ch.nodeSet[node]; exists {
		return
	}
	ch.nodeSet[node] = struct{}{}

	for i := 0; i < ch.replicas; i++ {
		key := fmt.Sprintf("%s:%d", node, i)
		h := xxhash.Sum64String(key)
		ch.ring = append(ch.ring, h)
		ch.nodes[h] = node
	}
	slices.Sort(ch.ring)
}

// Remove removes a node from the hash ring.
func (ch *consistentHash) Remove(node string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if _, exists := ch.nodeSet[node]; !exists {
		return
	}
	delete(ch.nodeSet, node)

	newRing := make([]uint64, 0, len(ch.ring))
	for _, h := range ch.ring {
		if ch.nodes[h] != node {
			newRing = append(newRing, h)
		} else {
			delete(ch.nodes, h)
		}
	}
	ch.ring = newRing
}

// Get returns the node for the given key.
func (ch *consistentHash) Get(key string) string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.ring) == 0 {
		return ""
	}

	h := xxhash.Sum64String(key)
	// Binary search for the first hash >= h
	idx := sort.Search(len(ch.ring), func(i int) bool {
		return ch.ring[i] >= h
	})
	if idx == len(ch.ring) {
		idx = 0
	}
	return ch.nodes[ch.ring[idx]]
}

// -------------------- Simple Hash --------------------

// SelectByHash selects a node by hashing the key and using modulo.
// This is a simpler alternative to consistent hash when node churn is low.
func SelectByHash(nodes []*GatewayNode, key string) *GatewayNode {
	if len(nodes) == 0 {
		return nil
	}
	if len(nodes) == 1 {
		return nodes[0]
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	idx := h.Sum64() % uint64(len(nodes))
	return nodes[idx]
}

// SelectByLeastConnections selects the node with the fewest active connections.
func SelectByLeastConnections(nodes []*GatewayNode) *GatewayNode {
	if len(nodes) == 0 {
		return nil
	}
	if len(nodes) == 1 {
		return nodes[0]
	}

	best := nodes[0]
	for _, n := range nodes[1:] {
		if n.ConnCount < best.ConnCount {
			best = n
		}
	}
	return best
}

// SelectByConsistentHash selects a node using a consistent hash ring.
// It builds a temporary ring from the provided nodes, which is correct but
// not optimal for very high throughput; callers may cache the ring and rebuild
// it only when node membership changes.
func SelectByConsistentHash(nodes []*GatewayNode, key string) *GatewayNode {
	if len(nodes) == 0 {
		return nil
	}
	if len(nodes) == 1 {
		return nodes[0]
	}

	ch := newConsistentHash(DefaultVirtualReplicas)
	for _, n := range nodes {
		ch.Add(n.NodeID)
	}

	selectedID := ch.Get(key)
	for _, n := range nodes {
		if n.NodeID == selectedID {
			return n
		}
	}
	// Fallback to the first node if the ring returns an unexpected ID.
	return nodes[0]
}
