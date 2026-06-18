package msgworker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

// Redis keys used by the gateway registry. They are duplicated here to avoid an
// import cycle between internal/msgworker and internal/gateway.
const (
	gatewayNodesKey      = "im:gateway:nodes"
	gatewayNodeKey       = "im:gateway:%s"
	sessionChangeChannel = "im:session:changes"
)

// sessionChangeEvent mirrors gateway.SessionChangeEvent without importing the
// gateway package (keeps the package DAG simple).
type sessionChangeEvent struct {
	UserID   int64  `json:"user_id"`
	DeviceID string `json:"device_id"`
	Action   string `json:"action"`
	NodeID   string `json:"node_id"`
	At       int64  `json:"at"`
}

// GatewayNodeInfo holds the discovery data for a single gateway node.
// It mirrors gateway.GatewayNode without importing the gateway package.
type GatewayNodeInfo struct {
	NodeID    string
	URL       string
	GrpcAddr  string
	ConnCount int
}

// sessionCacheEntry holds the locally cached node IDs for a single user.
type sessionCacheEntry struct {
	nodeIDs []string
	expires time.Time
}

// Router resolves online users to gateway nodes for targeted push delivery.
// It is implemented by GatewayRouter and can be mocked in tests.
type Router interface {
	ResolveUserNodes(ctx context.Context, userIDs []int64) (map[string][]int64, error)
	ResolveUserNodesWithNodes(ctx context.Context, userIDs []int64, aliveNodes []*GatewayNodeInfo) (map[string][]int64, error)
	GetAliveNodes(ctx context.Context) ([]*GatewayNodeInfo, error)
	GetAliveNodesMap(ctx context.Context) (map[string]*GatewayNodeInfo, error)
}

// GatewayRouter resolves online users to gateway nodes using the distributed
// session index stored in Redis. It allows MsgWorker to push messages only to
// the gateway nodes that actually host the target user's devices, instead of
// broadcasting to every gateway in the cluster.
type GatewayRouter struct {
	redis   redis.UniversalClient
	nodeTTL time.Duration
	log     *log.Helper

	// aliveNodeCache holds a local snapshot of gateway registry entries to
	// avoid hitting Redis on every push. It is refreshed when stale.
	aliveNodeCache    []*GatewayNodeInfo
	aliveNodeCachedAt time.Time
	cacheMu           sync.RWMutex

	// sessionCache holds a local L1 cache from userID -> node IDs.
	// It is invalidated via Redis Pub/Sub session change events and TTL.
	sessionCache   map[int64]*sessionCacheEntry
	sessionCacheMu sync.RWMutex
	sessionCacheTTL time.Duration

	// pubsub lifecycle
	pubsub     *redis.PubSub
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

var _ Router = (*GatewayRouter)(nil)

// NewGatewayRouter creates a new gateway router backed by Redis.
func NewGatewayRouter(redis redis.UniversalClient, nodeTTL time.Duration, logger log.Logger) *GatewayRouter {
	if nodeTTL <= 0 {
		nodeTTL = 30 * time.Second
	}
	return &GatewayRouter{
		redis:           redis,
		nodeTTL:         nodeTTL,
		log:             log.NewHelper(logger),
		sessionCache:    make(map[int64]*sessionCacheEntry),
		sessionCacheTTL: 5 * time.Second,
		stopCh:          make(chan struct{}),
	}
}

// SetSessionCacheTTL configures the L1 session route cache TTL. Must be called
// before Start.
func (r *GatewayRouter) SetSessionCacheTTL(d time.Duration) {
	if d > 0 {
		r.sessionCacheTTL = d
	}
}

// Start begins the Redis Pub/Sub listener for session change events.
// It is safe to call multiple times; only the first call has effect.
func (r *GatewayRouter) Start(ctx context.Context) error {
	if r.redis == nil {
		return nil
	}
	r.sessionCacheMu.Lock()
	if r.pubsub != nil {
		r.sessionCacheMu.Unlock()
		return nil
	}
	r.pubsub = r.redis.Subscribe(ctx, sessionChangeChannel)
	r.sessionCacheMu.Unlock()

	r.wg.Add(1)
	go r.pubsubLoop(ctx)
	return nil
}

// Stop shuts down the Pub/Sub listener and waits for the background goroutine.
func (r *GatewayRouter) Stop() error {
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})
	if r.pubsub != nil {
		_ = r.pubsub.Close()
	}
	r.wg.Wait()
	return nil
}

// pubsubLoop receives session change events and invalidates the local cache.
func (r *GatewayRouter) pubsubLoop(ctx context.Context) {
	defer r.wg.Done()
	if r.pubsub == nil {
		return
	}
	ch := r.pubsub.Channel()
	for {
		select {
		case <-r.stopCh:
			return
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var ev sessionChangeEvent
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
				continue
			}
			r.invalidateSessionCache(ev.UserID)
		}
	}
}

// invalidateSessionCache removes a user's entry from the local session cache.
func (r *GatewayRouter) invalidateSessionCache(userID int64) {
	r.sessionCacheMu.Lock()
	delete(r.sessionCache, userID)
	r.sessionCacheMu.Unlock()
}

// getCachedSession returns the cached node IDs for a user if still valid.
func (r *GatewayRouter) getCachedSession(userID int64) ([]string, bool) {
	r.sessionCacheMu.RLock()
	entry, ok := r.sessionCache[userID]
	r.sessionCacheMu.RUnlock()
	if !ok || entry == nil {
		return nil, false
	}
	if time.Now().After(entry.expires) {
		r.invalidateSessionCache(userID)
		return nil, false
	}
	return entry.nodeIDs, true
}

// setCachedSession stores the node IDs for a user in the local cache.
func (r *GatewayRouter) setCachedSession(userID int64, nodeIDs []string) {
	r.sessionCacheMu.Lock()
	r.sessionCache[userID] = &sessionCacheEntry{
		nodeIDs: nodeIDs,
		expires: time.Now().Add(r.sessionCacheTTL),
	}
	r.sessionCacheMu.Unlock()
}

// cacheTTL returns how long the local alive-node cache remains valid.
// Using nodeTTL/2 keeps the cache fresh while reducing Redis round-trips.
func (r *GatewayRouter) cacheTTL() time.Duration {
	ttl := r.nodeTTL / 2
	if ttl < 1*time.Second {
		return 1 * time.Second
	}
	return ttl
}

// ResolveUserNodes returns a mapping from gateway node ID to the list of user
// IDs that have at least one active device session on that node.
// Users that are offline (no session entry) are not included in the result.
// Sessions hosted on gateway nodes that have missed their heartbeat TTL are
// treated as offline to avoid pushing to crashed nodes.
func (r *GatewayRouter) ResolveUserNodes(ctx context.Context, userIDs []int64) (map[string][]int64, error) {
	if r.redis == nil || len(userIDs) == 0 {
		return map[string][]int64{}, nil
	}

	aliveNodes, err := r.GetAliveNodes(ctx)
	if err != nil {
		r.log.Warnf("resolve user nodes failed: get alive nodes err=%v", err)
		return nil, err
	}
	return r.ResolveUserNodesWithNodes(ctx, userIDs, aliveNodes)
}

// ResolveUserNodesWithNodes is like ResolveUserNodes but uses the provided
// alive node list instead of fetching it from Redis. This avoids repeated
// Redis round-trips when the caller already has the node list.
// It returns an error if the Redis pipeline fails so callers can distinguish
// "all users offline" from "routing lookup failed".
func (r *GatewayRouter) ResolveUserNodesWithNodes(ctx context.Context, userIDs []int64, aliveNodes []*GatewayNodeInfo) (map[string][]int64, error) {
	if r.redis == nil || len(userIDs) == 0 {
		return map[string][]int64{}, nil
	}

	aliveSet := make(map[string]struct{}, len(aliveNodes))
	for _, n := range aliveNodes {
		aliveSet[n.NodeID] = struct{}{}
	}

	// Split users into cache hits and misses.
	result := make(map[string][]int64)
	missing := make([]int64, 0, len(userIDs))
	for _, uid := range userIDs {
		if nodeIDs, ok := r.getCachedSession(uid); ok {
			added := false
			for _, nodeID := range nodeIDs {
				if _, ok := aliveSet[nodeID]; ok {
					result[nodeID] = append(result[nodeID], uid)
					added = true
				}
			}
			// If all cached nodes are dead, fall through to refresh from Redis.
			if added {
				continue
			}
		}
		missing = append(missing, uid)
	}

	if len(missing) == 0 {
		return result, nil
	}

	// Pipeline HGetAll for missing users to reduce Redis round-trips.
	pipe := r.redis.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(missing))
	for i, uid := range missing {
		deviceKey := fmt.Sprintf("im:session:%d:devices", uid)
		cmds[i] = pipe.HGetAll(ctx, deviceKey)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		r.log.Warnf("resolve user nodes pipeline failed: err=%v", err)
		return nil, fmt.Errorf("resolve user nodes pipeline: %w", err)
	}

	for i, uid := range missing {
		devices, err := cmds[i].Result()
		if err != nil {
			r.log.Warnf("resolve user nodes failed: user_id=%d, err=%v", uid, err)
			continue
		}
		if len(devices) == 0 {
			// Cache negative result briefly to avoid hammering Redis for offline users.
			r.setCachedSession(uid, nil)
			continue
		}

		seen := make(map[string]struct{})
		var cached []string
		for _, nodeID := range devices {
			if _, ok := seen[nodeID]; ok {
				continue
			}
			if _, ok := aliveSet[nodeID]; !ok {
				// Gateway node missed heartbeats; treat session as offline.
				continue
			}
			seen[nodeID] = struct{}{}
			cached = append(cached, nodeID)
			result[nodeID] = append(result[nodeID], uid)
		}
		r.setCachedSession(uid, cached)
	}

	return result, nil
}

// GetAliveNodes returns all gateway nodes whose heartbeat is within the TTL window.
// It caches the result locally for a short duration to reduce Redis load.
func (r *GatewayRouter) GetAliveNodes(ctx context.Context) ([]*GatewayNodeInfo, error) {
	if r.redis == nil {
		return nil, nil
	}

	// Fast path: return cached snapshot if still fresh.
	r.cacheMu.RLock()
	cached := r.aliveNodeCache
	cachedAt := r.aliveNodeCachedAt
	r.cacheMu.RUnlock()
	if cached != nil && time.Since(cachedAt) < r.cacheTTL() {
		return cached, nil
	}

	// Slow path: refresh from Redis.
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	// Double-check after acquiring write lock.
	if r.aliveNodeCache != nil && time.Since(r.aliveNodeCachedAt) < r.cacheTTL() {
		return r.aliveNodeCache, nil
	}

	nodes, err := r.fetchAliveNodes(ctx)
	if err != nil {
		return nil, err
	}

	r.aliveNodeCache = nodes
	r.aliveNodeCachedAt = time.Now()
	return nodes, nil
}

// fetchAliveNodes loads alive gateway nodes from Redis using a single SMembers
// followed by a pipeline of HGetAll for all node metadata hashes.
func (r *GatewayRouter) fetchAliveNodes(ctx context.Context) ([]*GatewayNodeInfo, error) {
	nodeIDs, err := r.redis.SMembers(ctx, gatewayNodesKey).Result()
	if err != nil {
		return nil, fmt.Errorf("smembers failed: %w", err)
	}
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	now := time.Now()
	cutoff := now.Add(-r.nodeTTL).Unix()

	// Fetch all node metadata in one pipeline to avoid N round-trips.
	pipe := r.redis.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(nodeIDs))
	for i, id := range nodeIDs {
		nodeKey := fmt.Sprintf(gatewayNodeKey, id)
		cmds[i] = pipe.HGetAll(ctx, nodeKey)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("pipeline hgetall failed: %w", err)
	}

	nodes := make([]*GatewayNodeInfo, 0, len(nodeIDs))
	for i, id := range nodeIDs {
		vals, err := cmds[i].Result()
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

// GetAliveNodesMap returns alive nodes as a map from node ID to node info.
// Useful for callers that need O(1) node lookups during batch routing.
func (r *GatewayRouter) GetAliveNodesMap(ctx context.Context) (map[string]*GatewayNodeInfo, error) {
	nodes, err := r.GetAliveNodes(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*GatewayNodeInfo, len(nodes))
	for _, n := range nodes {
		m[n.NodeID] = n
	}
	return m, nil
}

// GetNodeGRPCAddr returns the gRPC address of an alive node by its node ID.
// If the node is not found or has no gRPC address, it returns an empty string.
// Note: this fetches the full alive node list each time; prefer
// GetAliveNodesMap when making repeated lookups in a batch.
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
