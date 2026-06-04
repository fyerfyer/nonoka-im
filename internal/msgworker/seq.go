package msgworker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

// SeqGenerator generates topic-scoped sequence numbers using Redis INCR.
type SeqGenerator struct {
	redis redis.UniversalClient
	log   *log.Helper
}

// NewSeqGenerator creates a new SeqGenerator.
func NewSeqGenerator(redis redis.UniversalClient, logger log.Logger) *SeqGenerator {
	return &SeqGenerator{
		redis: redis,
		log:   log.NewHelper(logger),
	}
}

// NextSeq generates the next sequence number for a given topic.
func (g *SeqGenerator) NextSeq(ctx context.Context, topic string) (uint64, error) {
	key := fmt.Sprintf("im:seq:%s", topic)
	seq, err := g.redis.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis incr failed for topic %s: %w", topic, err)
	}
	return uint64(seq), nil
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

// Snowflake generates a unique message ID.
type Snowflake struct {
	nodeID   int64
	sequence int64
	lastTime int64
	mu       sync.Mutex
}

// NewSnowflake creates a new Snowflake ID generator.
func NewSnowflake(nodeID int64) *Snowflake {
	return &Snowflake{nodeID: nodeID}
}

// NextID generates the next unique message ID.
func (s *Snowflake) NextID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()
	if now == s.lastTime {
		s.sequence = (s.sequence + 1) & 0xFFF
		if s.sequence == 0 {
			for now <= s.lastTime {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		s.sequence = 0
	}
	s.lastTime = now
	return ((now - 1609459200000) << 22) | ((s.nodeID & 0x3FF) << 12) | s.sequence
}
