package gateway

import (
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/proto"
	v1 "nonoka-im/api/im/v1"
)

const shardCount = 32

// connShard holds a subset of connections, protected by its own RWMutex.
// This design reduces lock contention compared to a single sync.Map.
type connShard struct {
	mu    sync.RWMutex
	conns map[string]*Connection        // connID -> Connection
	users map[int64]map[string]struct{} // userID -> set of connIDs
}

func newConnShard() *connShard {
	return &connShard{
		conns: make(map[string]*Connection),
		users: make(map[int64]map[string]struct{}),
	}
}

// Manager manages all active WebSocket connections using sharded maps.
// It supports multi-device login: one user can have multiple connections.
type Manager struct {
	shards [shardCount]*connShard
	log    *log.Helper
}

// NewManager creates a new connection manager.
func NewManager(logger log.Logger) *Manager {
	m := &Manager{
		log: log.NewHelper(logger),
	}
	for i := range shardCount {
		m.shards[i] = newConnShard()
	}
	return m
}

func (m *Manager) getShard(userID int64) *connShard {
	if userID == 0 {
		return m.shards[0]
	}
	return m.shards[userID%shardCount]
}

func (m *Manager) getShardByConnID(connID string) *connShard {
	// Use FNV-like hash for connID distribution
	h := uint32(2166136261)
	for i := 0; i < len(connID); i++ {
		h ^= uint32(connID[i])
		h *= 16777619
	}
	return m.shards[h%shardCount]
}

// Add registers a connection to the manager.
// Should be called after authentication.
func (m *Manager) Add(c *Connection) {
	if c.UserID() == 0 {
		return
	}

	shard := m.getShard(c.UserID())
	shard.mu.Lock()
	defer shard.mu.Unlock()

	shard.conns[c.ConnID()] = c
	if shard.users[c.UserID()] == nil {
		shard.users[c.UserID()] = make(map[string]struct{})
	}
	shard.users[c.UserID()][c.ConnID()] = struct{}{}

	m.log.Infof("connection added: user_id=%d conn_id=%s device_id=%s", c.UserID(), c.ConnID(), c.DeviceID())
}

// Remove removes a connection from the manager.
func (m *Manager) Remove(c *Connection) {
	shard := m.getShard(c.UserID())
	shard.mu.Lock()
	defer shard.mu.Unlock()

	delete(shard.conns, c.ConnID())

	if c.UserID() != 0 {
		if userConns, ok := shard.users[c.UserID()]; ok {
			delete(userConns, c.ConnID())
			if len(userConns) == 0 {
				delete(shard.users, c.UserID())
			}
		}
	}

	m.log.Infof("connection removed: user_id=%d conn_id=%s", c.UserID(), c.ConnID())
}

// Get returns a single connection by user ID.
// If multiple devices are online, returns any one of them.
func (m *Manager) Get(userID int64) *Connection {
	shard := m.getShard(userID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	if userConns, ok := shard.users[userID]; ok {
		for connID := range userConns {
			if c, ok := shard.conns[connID]; ok {
				return c
			}
		}
	}
	return nil
}

// GetByConnID returns a connection by its unique connID.
func (m *Manager) GetByConnID(connID string) *Connection {
	shard := m.getShardByConnID(connID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	if c, ok := shard.conns[connID]; ok {
		return c
	}
	return nil
}

// GetAll returns all connections of a user.
func (m *Manager) GetAll(userID int64) []*Connection {
	shard := m.getShard(userID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	userConns, ok := shard.users[userID]
	if !ok || len(userConns) == 0 {
		return nil
	}

	result := make([]*Connection, 0, len(userConns))
	for connID := range userConns {
		if c, ok := shard.conns[connID]; ok {
			result = append(result, c)
		}
	}
	return result
}

// broadcastSendTimeout is the timeout for broadcast messages to each device.
const broadcastSendTimeout = 100 * time.Millisecond

// BroadcastToUser sends a packet to all devices of a user.
// It pre-serializes the packet once and copies the bytes for each connection,
// avoiding N repeated protobuf marshals.
func (m *Manager) BroadcastToUser(userID int64, packet *v1.Packet) int {
	data, err := proto.Marshal(packet)
	if err != nil {
		m.log.Warnf("marshal packet for broadcast failed: %v", err)
		return 0
	}
	return m.BroadcastToUserRaw(userID, data)
}

// BroadcastToUserRaw sends pre-marshaled data to all devices of a user.
// The caller must ensure data is not modified after this call.
func (m *Manager) BroadcastToUserRaw(userID int64, data []byte) int {
	conns := m.GetAll(userID)
	if len(conns) == 0 {
		return 0
	}

	sent := 0
	for _, c := range conns {
		// data is immutable during this call, skip per-connection copy
		if err := c.SendRawBytesWithTimeoutUnsafe(data, broadcastSendTimeout); err != nil {
			m.log.Warnf("broadcast to conn %s failed: %v", c.ConnID(), err)
			continue
		}
		sent++
	}
	return sent
}

// Range iterates over all connections.
func (m *Manager) Range(f func(c *Connection) bool) {
	for i := range shardCount {
		shard := m.shards[i]
		shard.mu.RLock()
		for _, c := range shard.conns {
			shard.mu.RUnlock()
			if !f(c) {
				return
			}
			shard.mu.RLock()
		}
		shard.mu.RUnlock()
	}
}

// Count returns the total number of active connections.
func (m *Manager) Count() int {
	count := 0
	for i := 0; i < shardCount; i++ {
		shard := m.shards[i]
		shard.mu.RLock()
		count += len(shard.conns)
		shard.mu.RUnlock()
	}
	return count
}

// UserCount returns the number of online users.
func (m *Manager) UserCount() int {
	count := 0
	for i := 0; i < shardCount; i++ {
		shard := m.shards[i]
		shard.mu.RLock()
		count += len(shard.users)
		shard.mu.RUnlock()
	}
	return count
}
