package msgworker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// botNegCacheTTL bounds how long a negative query-on-miss answer is reused.
// Human-to-human chats dominate message volume; without a negative cache
// every message in such a chat would hit the DB once per side. 10s caps a hot
// peer at a handful of queries per minute, while a freshly created bot waits
// at most this long (far below the 30s refresh interval) to be recognized.
const botNegCacheTTL = 10 * time.Second

// BotQueryFunc reports whether a single user is a bot. It is used to answer
// cache misses on demand so newly created bot users trigger agents
// immediately instead of waiting for the next periodic refresh.
type BotQueryFunc func(ctx context.Context, id int64) (bool, error)

// BotIndex is a periodically refreshed cache of bot user IDs. It lets the
// message worker skip Agent outbox writes for messages that cannot trigger
// any agent (human-to-human P2P chats, group messages without bot mentions),
// instead of paying an outbox row plus a Kafka event for every message and
// filtering on the agent worker side. Cache misses are resolved on demand via
// the query func (query-on-miss), which closes the window between a bot user
// being created in PostgreSQL and the next periodic refresh.
type BotIndex struct {
	load  func(ctx context.Context) ([]int64, error)
	query BotQueryFunc // nil disables query-on-miss
	log   *log.Helper

	mu  sync.RWMutex
	ids map[int64]struct{}
	// neg holds short-lived negative answers for regular users so hot
	// human-to-human conversations do not query the DB on every message.
	// It is dropped on every successful refresh to bound memory.
	neg map[int64]time.Time
	// ready is set after the first successful load; until then the index
	// must not filter anything.
	ready atomic.Bool
}

func NewBotIndex(load func(ctx context.Context) ([]int64, error), logger log.Logger) *BotIndex {
	return NewBotIndexWithQuery(load, nil, logger)
}

// NewBotIndexWithQuery creates an index that additionally resolves cache
// misses through query. A nil query keeps memory-only lookup behavior.
func NewBotIndexWithQuery(load func(ctx context.Context) ([]int64, error), query BotQueryFunc, logger log.Logger) *BotIndex {
	return &BotIndex{
		load:  load,
		query: query,
		log:   log.NewHelper(logger),
		neg:   make(map[int64]time.Time),
	}
}

// Start loads the index once and then refreshes it on the given interval
// until ctx is canceled. A failed refresh keeps the last good set.
func (b *BotIndex) Start(ctx context.Context, interval time.Duration) {
	if err := b.Refresh(ctx); err != nil {
		b.log.Warnf("initial bot index refresh failed: %v", err)
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := b.Refresh(ctx); err != nil {
					b.log.Warnf("bot index refresh failed: %v", err)
				}
			}
		}
	}()
}

func (b *BotIndex) Refresh(ctx context.Context) error {
	ids, err := b.load(ctx)
	if err != nil {
		return err
	}
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	b.mu.Lock()
	b.ids = set
	// The periodic load returns every bot user, so query-on-miss positives
	// survive in the new set; negatives are cheap to rebuild and dropping
	// them bounds the map to one interval of active users.
	b.neg = make(map[int64]time.Time)
	b.mu.Unlock()
	b.ready.Store(true)
	return nil
}

// Ready reports whether at least one successful load happened.
func (b *BotIndex) Ready() bool { return b.ready.Load() }

// IsBot reports whether userID is a bot. The in-memory set answers first; on
// a miss (and with a query func configured) the DB is asked once: positives
// join the set permanently, negatives are remembered for botNegCacheTTL only.
func (b *BotIndex) IsBot(ctx context.Context, userID int64) (bool, error) {
	b.mu.RLock()
	if _, ok := b.ids[userID]; ok {
		b.mu.RUnlock()
		return true, nil
	}
	if exp, ok := b.neg[userID]; ok && time.Now().Before(exp) {
		b.mu.RUnlock()
		return false, nil
	}
	b.mu.RUnlock()

	if b.query == nil {
		return false, nil
	}
	isBot, err := b.query(ctx, userID)
	if err != nil {
		return false, err
	}
	b.mu.Lock()
	if isBot {
		if b.ids == nil {
			b.ids = make(map[int64]struct{})
		}
		b.ids[userID] = struct{}{}
		delete(b.neg, userID)
	} else {
		b.neg[userID] = time.Now().Add(botNegCacheTTL)
	}
	b.mu.Unlock()
	return isBot, nil
}

// AnyBot reports whether at least one of the given user IDs is a bot.
func (b *BotIndex) AnyBot(ctx context.Context, userIDs []int64) (bool, error) {
	for _, id := range userIDs {
		bot, err := b.IsBot(ctx, id)
		if err != nil {
			return false, err
		}
		if bot {
			return true, nil
		}
	}
	return false, nil
}
