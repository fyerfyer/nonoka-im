package gateway

import (
	"sync"

	"github.com/go-kratos/kratos/v2/log"
	v1 "nonoka-im/api/im/v1"
)

// Manager manages all active WebSocket connections.
// It supports multi-device login: one user can have multiple connections.
type Manager struct {
	// userID -> connID -> *Connection
	users sync.Map

	// connID -> *Connection (for quick lookup by connection ID)
	conns sync.Map

	log *log.Helper
}

// NewManager creates a new connection manager.
func NewManager(logger log.Logger) *Manager {
	return &Manager{
		log: log.NewHelper(logger),
	}
}

// Add registers a connection to the manager.
// Should be called after authentication.
func (m *Manager) Add(c *Connection) {
	if c.UserID() == 0 {
		return
	}

	// Store in global conn map
	m.conns.Store(c.ConnID(), c)

	// Store in user map
	actual, _ := m.users.LoadOrStore(c.UserID(), &sync.Map{})
	userConns := actual.(*sync.Map)
	userConns.Store(c.ConnID(), c)

	m.log.Infof("connection added: user_id=%d conn_id=%s device_id=%s", c.UserID(), c.ConnID(), c.DeviceID())
}

// Remove removes a connection from the manager.
func (m *Manager) Remove(c *Connection) {
	m.conns.Delete(c.ConnID())

	if c.UserID() != 0 {
		if actual, ok := m.users.Load(c.UserID()); ok {
			userConns := actual.(*sync.Map)
			userConns.Delete(c.ConnID())

			// Clean up user map if no connections left
			empty := true
			userConns.Range(func(_, _ interface{}) bool {
				empty = false
				return false
			})
			if empty {
				m.users.Delete(c.UserID())
			}
		}
	}

	m.log.Infof("connection removed: user_id=%d conn_id=%s", c.UserID(), c.ConnID())
}

// Get returns a single connection by user ID.
// If multiple devices are online, returns any one of them.
func (m *Manager) Get(userID int64) *Connection {
	actual, ok := m.users.Load(userID)
	if !ok {
		return nil
	}

	userConns := actual.(*sync.Map)
	var result *Connection
	userConns.Range(func(_, value interface{}) bool {
		result = value.(*Connection)
		return false // break after first
	})
	return result
}

// GetByConnID returns a connection by its unique connID.
func (m *Manager) GetByConnID(connID string) *Connection {
	if value, ok := m.conns.Load(connID); ok {
		return value.(*Connection)
	}
	return nil
}

// GetAll returns all connections of a user.
func (m *Manager) GetAll(userID int64) []*Connection {
	actual, ok := m.users.Load(userID)
	if !ok {
		return nil
	}

	userConns := actual.(*sync.Map)
	var result []*Connection
	userConns.Range(func(_, value interface{}) bool {
		result = append(result, value.(*Connection))
		return true
	})
	return result
}

// BroadcastToUser sends a packet to all devices of a user.
// It deep-copies the payload for each connection to avoid race conditions.
func (m *Manager) BroadcastToUser(userID int64, packet *v1.Packet) int {
	conns := m.GetAll(userID)
	if len(conns) == 0 {
		return 0
	}

	sent := 0
	for _, c := range conns {
		// Deep copy payload to avoid race conditions across goroutines
		var payloadCopy []byte
		if len(packet.Payload) > 0 {
			payloadCopy = make([]byte, len(packet.Payload))
			copy(payloadCopy, packet.Payload)
		}
		p := &v1.Packet{
			Cmd:     packet.Cmd,
			Seq:     packet.Seq,
			Payload: payloadCopy,
		}
		if err := c.Send(p); err != nil {
			m.log.Warnf("broadcast to conn %s failed: %v", c.ConnID(), err)
			continue
		}
		sent++
	}
	return sent
}

// Range iterates over all connections.
func (m *Manager) Range(f func(c *Connection) bool) {
	m.conns.Range(func(_, value interface{}) bool {
		return f(value.(*Connection))
	})
}

// Count returns the total number of active connections.
func (m *Manager) Count() int {
	count := 0
	m.conns.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// UserCount returns the number of online users.
func (m *Manager) UserCount() int {
	count := 0
	m.users.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
