package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// SessionChangeChannel is the Redis Pub/Sub channel used to broadcast
	// session registration/unregistration events. Consumers (e.g. MsgWorker
	// routers) can subscribe to this channel to invalidate local caches.
	SessionChangeChannel = "im:session:changes"
)

// SessionChangeEvent describes a change in the distributed session index.
type SessionChangeEvent struct {
	UserID   int64  `json:"user_id"`
	DeviceID string `json:"device_id"`
	Action   string `json:"action"`   // "set", "del", "expire"
	NodeID   string `json:"node_id"`
	At       int64  `json:"at"`
}

// publishSessionChange publishes a session change event to Redis Pub/Sub.
// Errors are logged but not returned: this is a best-effort cache invalidation
// mechanism and should not block the hot path.
func (s *SessionManager) publishSessionChange(ctx context.Context, userID int64, deviceID, action string) {
	if s.redis == nil {
		return
	}
	event := SessionChangeEvent{
		UserID:   userID,
		DeviceID: deviceID,
		Action:   action,
		NodeID:   s.nodeID,
		At:       time.Now().Unix(),
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_ = s.redis.Publish(ctx, SessionChangeChannel, data)
}

// SessionManager manages distributed session index using Redis.
// It tracks which gateway node each user is connected to.
type SessionManager struct {
	redis redis.UniversalClient
	// nodeID is the unique identifier of this gateway node.
	nodeID string
}

// NewSessionManager creates a new session manager.
func NewSessionManager(redis redis.UniversalClient, nodeID string) *SessionManager {
	return &SessionManager{
		redis:  redis,
		nodeID: nodeID,
	}
}

// userSessionKey returns the Redis key for a user's session.
func (s *SessionManager) userSessionKey(userID int64) string {
	return fmt.Sprintf("im:session:%d", userID)
}

// deviceSessionKey returns the Redis key for a user's device session index.
func (s *SessionManager) deviceSessionKey(userID int64) string {
	return fmt.Sprintf("im:session:%d:devices", userID)
}

// SetSession registers a user's session to this node with TTL.
func (s *SessionManager) SetSession(ctx context.Context, userID int64, deviceID string, ttl time.Duration) error {
	key := s.userSessionKey(userID)
	// Use a Redis Hash to store device -> node mapping for multi-device support
	deviceKey := s.deviceSessionKey(userID)

	pipe := s.redis.Pipeline()
	pipe.Set(ctx, key, s.nodeID, ttl)
	pipe.HSet(ctx, deviceKey, deviceID, s.nodeID)
	pipe.Expire(ctx, deviceKey, ttl)
	_, err := pipe.Exec(ctx)
	if err == nil {
		s.publishSessionChange(ctx, userID, deviceID, "set")
	}
	return err
}

// GetSession returns the node ID where the user is connected.
func (s *SessionManager) GetSession(ctx context.Context, userID int64) (string, error) {
	key := s.userSessionKey(userID)
	return s.redis.Get(ctx, key).Result()
}

// GetDevices returns all devices of a user and their node IDs.
func (s *SessionManager) GetDevices(ctx context.Context, userID int64) (map[string]string, error) {
	deviceKey := s.deviceSessionKey(userID)
	return s.redis.HGetAll(ctx, deviceKey).Result()
}

// DelSession removes a user's session if it belongs to this node.
func (s *SessionManager) DelSession(ctx context.Context, userID int64, deviceID string) error {
	defer s.publishSessionChange(ctx, userID, deviceID, "del")
	userKey := s.userSessionKey(userID)
	deviceKey := s.deviceSessionKey(userID)

	// Use Lua script to ensure we only delete if the session belongs to this node
	const delSessionScript = `
		local user_key = KEYS[1]
		local device_key = KEYS[2]
		local node_id = ARGV[1]
		local device_id = ARGV[2]

		-- Remove device from hash
		redis.call("hdel", device_key, device_id)

		-- Check if user session belongs to this node
		local current = redis.call("get", user_key)
		if current == node_id then
			-- Check if there are other devices on this node
			local devices = redis.call("hgetall", device_key)
			local has_local = false
			for i = 2, #devices, 2 do
				if devices[i] == node_id then
					has_local = true
					break
				end
			end
			if not has_local then
				redis.call("del", user_key)
			end
		end
		return 1
	`

	err := s.redis.Eval(ctx, delSessionScript, []string{userKey, deviceKey}, s.nodeID, deviceID).Err()
	if err == redis.Nil {
		return nil
	}
	return err
}

// ExpireSession refreshes the TTL of a user's session.
// Expiration does not change the routing mapping, so it does not publish a
// session change event; subscribers rely on TTL for cache entries anyway.
func (s *SessionManager) ExpireSession(ctx context.Context, userID int64, ttl time.Duration) error {
	userKey := s.userSessionKey(userID)
	deviceKey := s.deviceSessionKey(userID)

	pipe := s.redis.Pipeline()
	pipe.Expire(ctx, userKey, ttl)
	pipe.Expire(ctx, deviceKey, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// IsOnline checks if a user is online.
func (s *SessionManager) IsOnline(ctx context.Context, userID int64) bool {
	_, err := s.GetSession(ctx, userID)
	return err == nil
}
