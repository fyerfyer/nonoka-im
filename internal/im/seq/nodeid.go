package seq

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

const (
	// snowflakeNodeCount is the bwmarrin/snowflake node space.
	snowflakeNodeCount = 1024
	nodeLeaseTTL       = 15 * time.Minute
	nodeLeasePrefix    = "im:msgid:node:"
)

// NodeLease is a Redis-backed Snowflake node allocation. Two msgworker
// replicas must never generate IDs with the same node ID; a fixed node ID in
// source code is only safe for a single process.
type NodeLease struct {
	ID     int64
	key    string
	holder string
	redis  redis.UniversalClient
	log    *log.Helper
}

const refreshNodeLeaseScript = `
if redis.call('get', KEYS[1]) ~= ARGV[1] then
  return 0
end
redis.call('pexpire', KEYS[1], ARGV[2])
return 1
`

// AcquireNodeID leases a free Snowflake node ID. Callers should run KeepAlive
// for the lifetime of the process and Release during graceful shutdown.
func AcquireNodeID(ctx context.Context, redisClient redis.UniversalClient, holder string, logger log.Logger) (*NodeLease, error) {
	if redisClient == nil {
		return nil, fmt.Errorf("redis is required to lease a snowflake node id")
	}
	lease := &NodeLease{holder: holder, redis: redisClient, log: log.NewHelper(logger)}
	for i := int64(0); i < snowflakeNodeCount; i++ {
		key := fmt.Sprintf("%s%d", nodeLeasePrefix, i)
		ok, err := redisClient.SetNX(ctx, key, holder, nodeLeaseTTL).Result()
		if err != nil {
			return nil, fmt.Errorf("lease snowflake node %d: %w", i, err)
		}
		if ok {
			lease.ID = i
			lease.key = key
			return lease, nil
		}
	}
	return nil, fmt.Errorf("all %d snowflake node ids are leased", snowflakeNodeCount)
}

// KeepAlive refreshes the lease until ctx is canceled. If the lease is lost,
// generation cannot be made safe automatically; the caller should treat the
// process as unhealthy and restart it to acquire a fresh node ID.
func (l *NodeLease) KeepAlive(ctx context.Context, interval time.Duration) {
	if l == nil || l.redis == nil || l.key == "" {
		return
	}
	if interval <= 0 {
		interval = nodeLeaseTTL / 3
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ok, err := l.redis.Eval(ctx, refreshNodeLeaseScript, []string{l.key}, l.holder, nodeLeaseTTL.Milliseconds()).Int()
			if err != nil {
				l.log.Warnf("refresh snowflake node lease %d: %v", l.ID, err)
				continue
			}
			if ok == 0 {
				l.log.Errorf("snowflake node lease %d was lost; restart the worker before generating more ids", l.ID)
			}
		}
	}
}

// Release deletes the lease when this process still owns it.
func (l *NodeLease) Release(ctx context.Context) {
	if l == nil || l.redis == nil || l.key == "" {
		return
	}
	_, _ = l.redis.Eval(ctx, `
if redis.call('get', KEYS[1]) == ARGV[1] then
  return redis.call('del', KEYS[1])
end
return 0
`, []string{l.key}, l.holder).Result()
}
