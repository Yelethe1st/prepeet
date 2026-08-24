// Package ratelimit counts requests against a key and refuses the ones over a
// limit.
//
// ADR-0003 chose to build authentication rather than buy it, and named
// credential stuffing as the obligation that comes with that choice. A vendor
// would be doing this work continuously; now it is ours. Rate limiting is also
// the only defence that works before a breached-password check exists, because
// it does not care whether the password is any good, only how many times it has
// been guessed.
//
// The counting is in memory. That is correct while one instance runs and wrong
// the moment two do: each would enforce its own share of the limit, so an
// attacker gets the limit multiplied by the instance count. Moving the counter
// to Redis is SEC-10's last item, and the ticket that adds a second instance is
// what should trigger it.
//
// This package never looks a key up anywhere. It cannot distinguish a
// registered address from an unknown one, which is deliberate: it runs before
// the credential check, and a limiter that behaved differently for a known
// address would enumerate accounts on its own.
//
// Implements part of SEC-10.
package ratelimit

import (
	"fmt"
	"sync"
	"time"
)

// Rule is how many requests are allowed in how long.
type Rule struct {
	Limit  int
	Window time.Duration
}

// Decision is the outcome for one request.
type Decision struct {
	Allowed bool
	// RetryAfter is how long until the caller could succeed, rounded up to a
	// second because that is the resolution the Retry-After header carries.
	RetryAfter time.Duration
	// Remaining is how many requests are left in this window. Zero when
	// refused.
	Remaining int
}

// Limiter counts requests per key using a fixed window.
//
// A fixed window rather than a sliding one or a token bucket. It permits a
// burst at a window boundary, up to twice the limit across two adjacent
// windows, and that is an acceptable trade here: the limits protect against
// thousands of attempts, not against six instead of five. A sliding window
// costs a timestamp list per key, and this runs on every login.
type Limiter struct {
	rule Rule
	now  func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	count     int
	expiresAt time.Time
}

// New builds a limiter.
//
// It panics on an unusable rule. A limit of zero would refuse every request and
// a window of zero would divide time into nothing, and both are configuration
// mistakes that lock out every user. Failing at construction is better than
// discovering it at three in the morning.
func New(rule Rule, now func() time.Time) *Limiter {
	if rule.Limit <= 0 {
		panic(fmt.Sprintf("ratelimit: limit is %d, which would refuse every request", rule.Limit))
	}
	if rule.Window <= 0 {
		panic(fmt.Sprintf("ratelimit: window is %s, which counts over no time at all", rule.Window))
	}
	return &Limiter{rule: rule, now: now, buckets: make(map[string]*bucket)}
}

// Allow counts one request against key and reports whether it may proceed.
//
// An empty key is refused rather than counted. Counting it would collapse every
// caller without one into a single bucket, so the first of them would exhaust
// the limit for all the rest.
func (l *Limiter) Allow(key string) Decision {
	if key == "" {
		return Decision{Allowed: false, RetryAfter: l.rule.Window}
	}

	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, found := l.buckets[key]
	if !found || !now.Before(b.expiresAt) {
		b = &bucket{expiresAt: now.Add(l.rule.Window)}
		l.buckets[key] = b
	}

	if b.count >= l.rule.Limit {
		return Decision{
			Allowed:    false,
			RetryAfter: roundUpToSecond(b.expiresAt.Sub(now)),
			Remaining:  0,
		}
	}

	b.count++
	return Decision{
		Allowed:   true,
		Remaining: l.rule.Limit - b.count,
	}
}

// Sweep removes expired buckets.
//
// Without it the map holds one entry per key ever seen, which is a memory leak
// with a long fuse: every address anyone has ever tried to log in with, held
// until the process restarts. The caller runs this periodically.
func (l *Limiter) Sweep() {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	for key, b := range l.buckets {
		if !now.Before(b.expiresAt) {
			delete(l.buckets, key)
		}
	}
}

// Size reports how many keys are currently tracked, for tests and for a gauge.
func (l *Limiter) Size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// roundUpToSecond rounds a wait up, because Retry-After carries whole seconds
// and rounding down would tell a client to retry while it is still refused.
func roundUpToSecond(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Second
	}
	if d%time.Second == 0 {
		return d
	}
	return d.Truncate(time.Second) + time.Second
}
