package group

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

const botNegCacheTTL = 30 * time.Second

// BotQueryFunc reports whether a single user is a bot.
type BotQueryFunc func(ctx context.Context, id int64) (bool, error)

// BotQueryManyFunc reports which IDs in a batch are bots. It keeps group
// fan-out from issuing one PostgreSQL query per recipient on a negative-cache
// miss.
type BotQueryManyFunc func(ctx context.Context, ids []int64) ([]int64, error)

// BotIndex is a periodically refreshed cache of bot user IDs with query-on-miss
// fallback so a newly created bot triggers immediately.
type BotIndex struct {
	load      func(ctx context.Context) ([]int64, error)
	query     BotQueryFunc
	queryMany BotQueryManyFunc
	log       *log.Helper

	mu    sync.RWMutex
	ids   map[int64]struct{}
	neg   map[int64]time.Time
	ready atomic.Bool
}

func NewBotIndex(load func(ctx context.Context) ([]int64, error), logger log.Logger) *BotIndex {
	return NewBotIndexWithQuery(load, nil, logger)
}

func NewBotIndexWithQuery(load func(ctx context.Context) ([]int64, error), query BotQueryFunc, logger log.Logger) *BotIndex {
	return &BotIndex{load: load, query: query, log: log.NewHelper(logger), neg: make(map[int64]time.Time)}
}

// SetQueryMany installs the batch query used by FilterHumans. It must be
// called before serving traffic.
func (b *BotIndex) SetQueryMany(query BotQueryManyFunc) {
	b.mu.Lock()
	b.queryMany = query
	b.mu.Unlock()
}

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
	now := time.Now()
	b.mu.Lock()
	b.ids = set
	// Do not reset the negative cache on every refresh: a busy group would
	// otherwise re-query every non-bot member after each refresh. Expired
	// entries are pruned; users that became bots are removed below.
	for id, expires := range b.neg {
		if now.After(expires) {
			delete(b.neg, id)
			continue
		}
		if _, isBot := set[id]; isBot {
			delete(b.neg, id)
		}
	}
	b.mu.Unlock()
	b.ready.Store(true)
	return nil
}

func (b *BotIndex) Ready() bool { return b.ready.Load() }

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

// FilterHumans returns the subset of ids that are not bots, preserving order
// and dropping duplicates. It uses one batched query for cache misses when a
// batch query is configured, then falls back to IsBot.
func (b *BotIndex) FilterHumans(ctx context.Context, userIDs []int64) ([]int64, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	b.mu.RLock()
	queryMany := b.queryMany
	if queryMany == nil {
		b.mu.RUnlock()
		out := make([]int64, 0, len(userIDs))
		seen := make(map[int64]struct{}, len(userIDs))
		for _, id := range userIDs {
			if id <= 0 {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			bot, err := b.IsBot(ctx, id)
			if err != nil {
				return nil, err
			}
			if !bot {
				out = append(out, id)
			}
		}
		return out, nil
	}

	now := time.Now()
	botStatus := make(map[int64]bool, len(userIDs))
	missing := make([]int64, 0, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, seen := botStatus[id]; seen {
			continue
		}
		if _, known := b.ids[id]; known {
			botStatus[id] = true
			continue
		}
		if exp, ok := b.neg[id]; ok && now.Before(exp) {
			botStatus[id] = false
			continue
		}
		missing = append(missing, id)
	}
	b.mu.RUnlock()

	if len(missing) > 0 {
		bots, err := queryMany(ctx, missing)
		if err != nil {
			return nil, err
		}
		botSet := make(map[int64]struct{}, len(bots))
		for _, id := range bots {
			botSet[id] = struct{}{}
		}
		b.mu.Lock()
		if b.ids == nil {
			b.ids = make(map[int64]struct{})
		}
		for _, id := range missing {
			if _, isBot := botSet[id]; isBot {
				b.ids[id] = struct{}{}
				delete(b.neg, id)
			} else {
				b.neg[id] = now.Add(botNegCacheTTL)
			}
		}
		b.mu.Unlock()
		for _, id := range missing {
			_, botStatus[id] = botSet[id]
		}
	}

	out := make([]int64, 0, len(userIDs))
	seen := make(map[int64]struct{}, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if !botStatus[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

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
