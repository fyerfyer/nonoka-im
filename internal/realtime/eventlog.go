package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	runEventTTL       = 15 * time.Minute
	runEventMaxPerRun = 500
	runEventKeyPrefix = "agent:run:"
)

// RedisRunEventStore assigns sequence numbers with INCR and keeps a bounded
// per-run replay buffer. Delta events are never persisted to PostgreSQL.
type RedisRunEventStore struct {
	client redis.UniversalClient
}

func NewRedisRunEventStore(client redis.UniversalClient) *RedisRunEventStore {
	if client == nil {
		return nil
	}
	return &RedisRunEventStore{client: client}
}

func (s *RedisRunEventStore) seqKey(runID uint64) string {
	return fmt.Sprintf("%s%d:seq", runEventKeyPrefix, runID)
}

func (s *RedisRunEventStore) listKey(runID uint64) string {
	return fmt.Sprintf("%s%d:events", runEventKeyPrefix, runID)
}

func (s *RedisRunEventStore) Append(ctx context.Context, event RunEvent) (RunEvent, error) {
	if s == nil || s.client == nil {
		return event, fmt.Errorf("redis run event store unavailable")
	}
	seq, err := s.client.Incr(ctx, s.seqKey(event.RunID)).Result()
	if err != nil {
		return event, err
	}
	event.Seq = uint64(seq)
	if event.TimestampMs == 0 {
		event.TimestampMs = time.Now().UnixMilli()
	}
	body, err := json.Marshal(event)
	if err != nil {
		return event, err
	}
	pipe := s.client.TxPipeline()
	pipe.RPush(ctx, s.listKey(event.RunID), body)
	pipe.LTrim(ctx, s.listKey(event.RunID), -runEventMaxPerRun, -1)
	pipe.Expire(ctx, s.listKey(event.RunID), runEventTTL)
	pipe.Expire(ctx, s.seqKey(event.RunID), runEventTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return event, err
	}
	return event, nil
}

func (s *RedisRunEventStore) Since(ctx context.Context, runID, afterSeq uint64) ([]RunEvent, error) {
	if s == nil || s.client == nil {
		return nil, nil
	}
	values, err := s.client.LRange(ctx, s.listKey(runID), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	out := make([]RunEvent, 0, len(values))
	for _, value := range values {
		var event RunEvent
		if err := json.Unmarshal([]byte(value), &event); err != nil {
			continue
		}
		if event.Seq > afterSeq {
			out = append(out, event)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

// MemoryRunEventStore is a bounded in-process store for unit tests and
// single-process fallbacks.
type MemoryRunEventStore struct {
	mu     sync.Mutex
	seq    map[uint64]uint64
	events map[uint64][]RunEvent
}

func NewMemoryRunEventStore() *MemoryRunEventStore {
	return &MemoryRunEventStore{seq: map[uint64]uint64{}, events: map[uint64][]RunEvent{}}
}

func (s *MemoryRunEventStore) Append(_ context.Context, event RunEvent) (RunEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq[event.RunID]++
	event.Seq = s.seq[event.RunID]
	if event.TimestampMs == 0 {
		event.TimestampMs = time.Now().UnixMilli()
	}
	events := append(s.events[event.RunID], event)
	if len(events) > runEventMaxPerRun {
		events = events[len(events)-runEventMaxPerRun:]
	}
	s.events[event.RunID] = events
	return event, nil
}

func (s *MemoryRunEventStore) Since(_ context.Context, runID, afterSeq uint64) ([]RunEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.events[runID]
	out := make([]RunEvent, 0, len(events))
	for _, event := range events {
		if event.Seq > afterSeq {
			out = append(out, event)
		}
	}
	return out, nil
}
