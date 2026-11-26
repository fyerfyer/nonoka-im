package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter implements a shared fixed-window counter for bot credentials.
// Redis errors fail open so a limiter outage does not take bots offline.
type Limiter struct{ client redis.UniversalClient }

func New(client redis.UniversalClient) *Limiter { return &Limiter{client: client} }

func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if l == nil || l.client == nil || limit <= 0 {
		return true, nil
	}
	if window <= 0 {
		window = time.Minute
	}
	n, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return true, err
	}
	if n == 1 {
		_ = l.client.Expire(ctx, key, window).Err()
	}
	return n <= int64(limit), nil
}
