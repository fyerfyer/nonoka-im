package msgworker

import (
	"context"
	"fmt"
	"sync"

	"github.com/bwmarrin/snowflake"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

const defaultSeqBatchSize = 100

// seqRange holds a locally cached range of sequence numbers for a topic.
type seqRange struct {
	next uint64 // next available seq
	max  uint64 // inclusive upper bound of the cached range
	mu   sync.Mutex
}

// SeqGenerator generates topic-scoped sequence numbers using Redis INCR.
// It caches sequence number ranges locally to reduce Redis round-trips.
type SeqGenerator struct {
	redis     redis.UniversalClient
	log       *log.Helper
	batchSize int
	// ranges caches pre-allocated seq ranges per topic: topic -> *seqRange
	ranges sync.Map
}

// NewSeqGenerator creates a new SeqGenerator.
func NewSeqGenerator(redis redis.UniversalClient, logger log.Logger) *SeqGenerator {
	return &SeqGenerator{
		redis:     redis,
		log:       log.NewHelper(logger),
		batchSize: defaultSeqBatchSize,
	}
}

// NextSeq generates the next sequence number for a given topic.
// It first tries to allocate from the local cache; if exhausted, it fetches
// a new batch from Redis.
func (g *SeqGenerator) NextSeq(ctx context.Context, topic string) (uint64, error) {
	raw, _ := g.ranges.LoadOrStore(topic, &seqRange{})
	sr := raw.(*seqRange)

	sr.mu.Lock()
	defer sr.mu.Unlock()

	if sr.next > 0 && sr.next <= sr.max {
		seq := sr.next
		sr.next++
		return seq, nil
	}

	// Local cache exhausted: fetch a new batch from Redis.
	key := fmt.Sprintf("im:seq:%s", topic)
	endSeq, err := g.redis.IncrBy(ctx, key, int64(g.batchSize)).Result()
	if err != nil {
		return 0, fmt.Errorf("redis incrby failed for topic %s: %w", topic, err)
	}

	sr.max = uint64(endSeq)
	sr.next = sr.max - uint64(g.batchSize) + 1
	seq := sr.next
	sr.next++
	return seq, nil
}

// NextSeqBatch generates a batch of sequence numbers for a topic.
func (g *SeqGenerator) NextSeqBatch(ctx context.Context, topic string, count int) ([]uint64, error) {
	if count <= 0 {
		return nil, nil
	}
	key := fmt.Sprintf("im:seq:%s", topic)
	endSeq, err := g.redis.IncrBy(ctx, key, int64(count)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis incrby failed for topic %s: %w", topic, err)
	}
	seqs := make([]uint64, count)
	startSeq := endSeq - int64(count) + 1
	for i := 0; i < count; i++ {
		seqs[i] = uint64(startSeq + int64(i))
	}
	return seqs, nil
}

// IDGenerator wraps bwmarrin/snowflake for lock-free unique ID generation.
type IDGenerator struct {
	node *snowflake.Node
}

// NewSnowflake creates a new Snowflake ID generator using the bwmarrin/snowflake
// library, which provides a high-performance lock-free implementation.
func NewSnowflake(nodeID int64) *IDGenerator {
	// Normalize to [0, 1023]. Go's % operator can return negative values,
	// so we adjust to avoid mapping different physical nodes to the same ID.
	normalized := nodeID % 1024
	if normalized < 0 {
		normalized += 1024
	}
	node, err := snowflake.NewNode(normalized)
	if err != nil {
		// Fallback to node 0 if the requested nodeID is invalid.
		node, _ = snowflake.NewNode(0)
	}
	return &IDGenerator{node: node}
}

// NextID generates the next unique message ID (lock-free).
func (s *IDGenerator) NextID() int64 {
	return s.node.Generate().Int64()
}
