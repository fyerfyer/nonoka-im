package group

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

const botNegCacheTTL = 10 * time.Second

// BotQueryFunc reports whether a single user is a bot.
type BotQueryFunc func(ctx context.Context, id int64) (bool, error)

// BotIndex is a periodically refreshed cache of bot user IDs with query-on-miss
// fallback so a newly created bot triggers immediately.
type BotIndex struct {
	load  func(ctx context.Context) ([]int64, error)
	query BotQueryFunc
	log   *log.Helper

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
	b.neg = make(map[int64]time.Time)
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
