package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/ratelimit"
)

// attemptCounter presents the platform's counter as the port the API
// declared, so internal/api states what it needs of a limiter rather than
// taking the implementation's shape (ADR-0005).
type attemptCounter struct {
	counter ratelimit.Counter
}

func (a attemptCounter) Allow(ctx context.Context, key string) (api.LimitDecision, error) {
	decision, err := a.counter.Allow(ctx, key)
	if err != nil {
		return api.LimitDecision{}, err
	}
	return api.LimitDecision{Allowed: decision.Allowed, RetryAfter: decision.RetryAfter}, nil
}

// authLimiter builds one counter, or nothing when the limit is zero.
//
// Zero disables rather than refusing everything: NewPostgres panics on a
// limit it could never allow, and a local run with no limit configured
// must start rather than lock everybody out.
func authLimiter(pool *pgxpool.Pool, limit int, window time.Duration) api.Limiter {
	if limit <= 0 || window <= 0 {
		return nil
	}
	return attemptCounter{counter: ratelimit.NewPostgres(pool, ratelimit.Rule{
		Limit: limit, Window: window,
	}, time.Now)}
}
