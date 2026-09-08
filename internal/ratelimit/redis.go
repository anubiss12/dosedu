// Package ratelimit provides a minimal Redis-backed fixed-window rate
// limiter — just enough to deter abuse of public, unauthenticated
// endpoints (the placement test). Not a sliding-window/token-bucket
// implementation; a simple INCR+EXPIRE window is enough for this.
package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const OneHour = time.Hour

type Limiter struct {
	redis *redis.Client
}

func New(redisClient *redis.Client) *Limiter {
	return &Limiter{redis: redisClient}
}

// Allow reports whether `key` may perform one more action within the
// current window, incrementing its counter as a side effect. The
// first call for a key sets the window's expiry; later calls within
// the same window don't reset it.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	count, err := l.redis.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		if err := l.redis.Expire(ctx, key, window).Err(); err != nil {
			return false, err
		}
	}
	return count <= int64(limit), nil
}
