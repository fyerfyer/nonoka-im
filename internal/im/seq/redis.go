// Package seq generates per-topic sequence numbers and globally unique IDs.
package seq

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/bwmarrin/snowflake"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

const defaultBatchSize = 100

// maxCachedRanges bounds per-topic range state. P2P topics are unbounded in a
// long-lived process; dropping a cached range only skips the unused tail of
// that range (sequence gaps are allowed) and allocates a new unique range.
const maxCachedRanges = 200000

type seqRange struct {
	next uint64
	max  uint64
	mu   sync.Mutex
}

// Generator allocates topic-scoped sequence numbers from Redis with a local
// range cache to reduce round-trips.
// RangeFloor supplies the durable lower bound for a topic. It is read once
// per range refill (one Mongo read per 100 messages) so a lost or evicted
// Redis key can never restart the sequence below persisted messages.
type RangeFloor func(ctx context.Context, topic string) (uint64, error)

// RangeAllocator is a durable, process-safe range allocator (Mongo in
// production). It must never return the same range twice, even if Redis or a
// local process cache is lost.
type RangeAllocator interface {
	AllocateRange(ctx context.Context, topic string, size int, floor uint64) (first, last uint64, err error)
}

const allocateRangeScript = `
local created = 0
if redis.call('exists', KEYS[1]) == 0 then created = 1 end
local endv = redis.call('incrby', KEYS[1], ARGV[1])
return {created, endv}
`

const resetRangeScript = `
local current = tonumber(redis.call('get', KEYS[1]) or '0')
local floor = tonumber(ARGV[1])
if current < floor then
  redis.call('set', KEYS[1], floor)
end
return redis.call('incrby', KEYS[1], ARGV[2])
`

type Generator struct {
	redis     redis.UniversalClient
	log       *log.Helper
	batchSize int
	ranges    sync.Map
	rangeN    atomic.Int64
	pruneMu   sync.Mutex
	floor     RangeFloor
	allocator RangeAllocator
}

func New(redis redis.UniversalClient, logger log.Logger) *Generator {
	return &Generator{redis: redis, log: log.NewHelper(logger), batchSize: defaultBatchSize}
}

// SetFloor installs the durable fallback used when a Redis sequence key is
// missing. Production wires this to MongoDB; tests may leave it nil.
func (g *Generator) SetFloor(floor RangeFloor) {
	g.floor = floor
}

// SetAllocator switches range allocation from Redis to a durable allocator.
// Production uses MongoDB so a lost Redis key cannot race a live local cache.
func (g *Generator) SetAllocator(allocator RangeAllocator) {
	g.allocator = allocator
}

func (g *Generator) NextSeq(ctx context.Context, topic string) (uint64, error) {
	raw, loaded := g.ranges.LoadOrStore(topic, &seqRange{})
	if !loaded {
		if g.rangeN.Add(1) > maxCachedRanges {
			g.pruneRanges()
		}
	}
	sr := raw.(*seqRange)
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.next > 0 && sr.next <= sr.max {
		value := sr.next
		sr.next++
		return value, nil
	}
	if g.allocator != nil {
		var floor uint64
		if g.floor != nil {
			value, ferr := g.floor(ctx, topic)
			if ferr != nil {
				return 0, fmt.Errorf("load durable seq floor for topic %s: %w", topic, ferr)
			}
			floor = value
		}
		first, last, aerr := g.allocator.AllocateRange(ctx, topic, g.batchSize, floor)
		if aerr != nil {
			return 0, fmt.Errorf("allocate durable seq range for topic %s: %w", topic, aerr)
		}
		if last < first || first == 0 {
			return 0, fmt.Errorf("durable seq allocator returned invalid range [%d,%d] for topic %s", first, last, topic)
		}
		sr.max, sr.next = last, first
		value := sr.next
		sr.next++
		return value, nil
	}
	key := fmt.Sprintf("im:seq:%s", topic)
	var end int64
	if g.floor != nil {
		// Read the durable floor on every range refill. This is one indexed
		// Mongo read per batch (100 messages) and removes the race where two
		// workers create a lost Redis key with different floors.
		floor, ferr := g.floor(ctx, topic)
		if ferr != nil {
			return 0, fmt.Errorf("load durable seq floor for topic %s: %w", topic, ferr)
		}
		value, rerr := g.redis.Eval(ctx, resetRangeScript, []string{key}, floor, g.batchSize).Int64()
		if rerr != nil {
			return 0, fmt.Errorf("allocating seq range for topic %s: %w", topic, rerr)
		}
		end = value
	} else {
		result, rerr := g.redis.Eval(ctx, allocateRangeScript, []string{key}, g.batchSize).Slice()
		if rerr != nil {
			return 0, fmt.Errorf("redis allocate seq range for topic %s: %w", topic, rerr)
		}
		end, _ = result[1].(int64)
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
	out := make([]uint64, 0, count)
	for i := 0; i < count; i++ {
		value, err := g.NextSeq(ctx, topic)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

// pruneRanges drops roughly a quarter of cached ranges when the cap is hit.
// Range state is only an optimization: the durable allocator guarantees that
// the next allocation cannot reuse a sequence.
func (g *Generator) pruneRanges() {
	g.pruneMu.Lock()
	defer g.pruneMu.Unlock()
	target := int64(maxCachedRanges * 3 / 4)
	removed := int64(0)
	g.ranges.Range(func(key, _ any) bool {
		if g.rangeN.Load() <= target {
			return false
		}
		g.ranges.Delete(key)
		g.rangeN.Add(-1)
		removed++
		return true
	})
	if removed > 0 {
		g.log.Debugf("pruned %d cached topic seq ranges", removed)
	}
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
