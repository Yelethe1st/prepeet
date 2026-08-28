package api

// Applying the rate limiter to the endpoints an attacker gets unlimited
// attempts at: SEC-10, on the counter PLT built.
//
// Two counts per attempt, not one. Per email address, because credential
// stuffing works through a list of addresses one at a time; and per
// network, because one attacker with many addresses is the ordinary case
// and a per-address count alone would never see them. Either refusal
// refuses, and the tighter of the two waits is what the caller is told.
//
// The limiter is consulted before the work, so a refused attempt costs an
// argon2id hash it never performs, and it fails open: authentication
// cannot happen without the database the counter lives in, so a counter
// that cannot be read must not also take the service down.

import (
	"context"
	"fmt"
	"time"
)

// Limiter counts attempts against a key. Consumer-defined here and
// satisfied by platform/ratelimit in cmd.
type Limiter interface {
	Allow(ctx context.Context, key string) (LimitDecision, error)
}

// LimitDecision is one counter's answer.
type LimitDecision struct {
	Allowed    bool
	RetryAfter time.Duration
}

// RateLimitedError refuses an attempt at the API boundary, carrying the
// wait so the caller can be told when to come back.
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("api: too many attempts; wait %s", e.RetryAfter)
}

// limits holds the counters the authentication endpoints consult.
//
// Both are optional: a nil limiter allows everything, which is what a
// test harness and a single-process local run want, and never what a
// deployment gets.
type limits struct {
	// perAddress counts attempts against one email address.
	perAddress Limiter
	// perNetwork counts attempts from one network prefix.
	perNetwork Limiter
}

// check counts one attempt and refuses when either counter says so.
//
// The key is the operation plus the subject, so the allowance for logging
// in is not spent by asking for a password reset: an attacker must not be
// able to lock somebody out of one endpoint by exhausting another.
func (l limits) check(ctx context.Context, operation, address, network string) error {
	var worst time.Duration
	refused := false

	for _, attempt := range []struct {
		limiter Limiter
		subject string
	}{
		{l.perAddress, address},
		{l.perNetwork, network},
	} {
		if attempt.limiter == nil || attempt.subject == "" {
			continue
		}
		decision, err := attempt.limiter.Allow(ctx, operation+":"+attempt.subject)
		if err != nil {
			// Fails open, deliberately: the counter shares a database with
			// the credentials, so a counter that cannot be read is a
			// database that cannot authenticate anyway. Refusing here
			// would turn a degraded dependency into an outage.
			continue
		}
		if !decision.Allowed {
			refused = true
			if decision.RetryAfter > worst {
				worst = decision.RetryAfter
			}
		}
	}

	if refused {
		return &RateLimitedError{RetryAfter: worst}
	}
	return nil
}
