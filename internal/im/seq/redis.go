// Package seq generates per-topic sequence numbers and globally unique IDs.
package seq

import (
	"context"
	"fmt"
	"sync"

	"github.com/bwmarrin/snowflake"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

const defaultBatchSize = 100

type seqRange struct {
	next uint64
	max  uint64
	mu   sync.Mutex
}

// Generator allocates topic-scoped sequence numbers from Redis with a local
// range cache to reduce round-trips.
type Generator struct {
	redis     redis.UniversalClient
	log       *log.Helper
	batchSize int
	ranges    sync.Map
}

func New(redis redis.UniversalClient, logger log.Logger) *Generator {
	return &Generator{redis: redis, log: log.NewHelper(logger), batchSize: defaultBatchSize}
}

func (g *Generator) NextSeq(ctx context.Context, topic string) (uint64, error) {
	raw, _ := g.ranges.LoadOrStore(topic, &seqRange{})
	sr := raw.(*seqRange)
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.next > 0 && sr.next <= sr.max {
		value := sr.next
		sr.next++
		return value, nil
	}
	key := fmt.Sprintf("im:seq:%s", topic)
	end, err := g.redis.IncrBy(ctx, key, int64(g.batchSize)).Result()
	if err != nil {
		return 0, fmt.Errorf("redis incrby failed for topic %s: %w", topic, err)
	}
	sr.max = uint64(end)
	sr.next = sr.max - uint64(g.batchSize) + 1
	value := sr.next
	sr.next++
	return value, nil
}

func (g *Generator) NextSeqBatch(ctx context.Context, topic string, count int) ([]uint64, error) {
	if count <= 0 {
		return nil, nil
	}
	key := fmt.Sprintf("im:seq:%s", topic)
	end, err := g.redis.IncrBy(ctx, key, int64(count)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis incrby failed for topic %s: %w", topic, err)
	}
	start := end - int64(count) + 1
	out := make([]uint64, count)
	for i := 0; i < count; i++ {
		out[i] = uint64(start + int64(i))
	}
	return out, nil
}

// IDs wraps bwmarrin/snowflake.
type IDs struct{ node *snowflake.Node }

func NewIDs(nodeID int64) *IDs {
	normalized := nodeID % 1024
	if normalized < 0 {
		normalized += 1024
	}
	node, err := snowflake.NewNode(normalized)
	if err != nil {
		node, _ = snowflake.NewNode(0)
	}
	return &IDs{node: node}
}

func (s *IDs) NextID() int64 { return s.node.Generate().Int64() }
