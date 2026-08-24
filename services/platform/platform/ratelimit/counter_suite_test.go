package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/ratelimit"
)

// The behaviour every Counter must have, written once.
//
// This file has no build tag, so it compiles for both the in-memory tests and
// the integration ones. Each implementation runs it and then adds only the
// tests that are specific to it.
//
// It exists because the assertions had already been duplicated once. Five of
// them lived in both the memory and the PostgreSQL test files, and a third
// implementation would have copied them a third time. Copied tests drift: the
// copy that is not updated keeps passing while describing behaviour nobody has
// any more.

// newCounter builds a counter with a given rule and clock. The clock is a
// parameter because the window has to be advanced without waiting, and the rule
// because several assertions need a different limit.
type newCounter func(ratelimit.Rule, func() time.Time) ratelimit.Counter

// runCounterContract runs every assertion that holds for any Counter.
//
// Keys are derived from the subtest name so implementations that share state,
// which PostgreSQL does, do not bleed between assertions.
func runCounterContract(t *testing.T, build newCounter) {
	t.Helper()

	t.Run("allows requests under the limit", func(t *testing.T) {
		counter := build(ratelimit.Rule{Limit: 5, Window: time.Minute}, time.Now)

		for i := range 5 {
			decision := allow(t, counter, key(t))
			if !decision.Allowed {
				t.Errorf("request %d was refused while under the limit", i+1)
			}
		}
	})

	t.Run("refuses the request over the limit", func(t *testing.T) {
		counter := build(ratelimit.Rule{Limit: 3, Window: time.Minute}, time.Now)

		for range 3 {
			allow(t, counter, key(t))
		}

		decision := allow(t, counter, key(t))
		if decision.Allowed {
			t.Error("the fourth request was allowed under a limit of three")
		}
		if decision.RetryAfter <= 0 {
			t.Error("RetryAfter is not positive, and a client needs to know when to try again")
		}
	})

	// The window has to move, or an attacker who exhausts the limit is locked
	// out forever and a person who mistyped their password twice is locked out
	// for the rest of the day.
	t.Run("recovers as the window passes", func(t *testing.T) {
		now := time.Now()
		counter := build(ratelimit.Rule{Limit: 2, Window: time.Minute}, func() time.Time { return now })

		allow(t, counter, key(t))
		allow(t, counter, key(t))
		if allow(t, counter, key(t)).Allowed {
			t.Fatal("the third request was allowed under a limit of two")
		}

		now = now.Add(time.Minute + time.Second)

		if !allow(t, counter, key(t)).Allowed {
			t.Error("the limit did not recover after the window passed")
		}
	})

	// One person exhausting their attempts must not lock out everyone else,
	// which is what a single global counter would do.
	t.Run("counts keys separately", func(t *testing.T) {
		counter := build(ratelimit.Rule{Limit: 2, Window: time.Minute}, time.Now)

		allow(t, counter, key(t)+"-exhausted")
		allow(t, counter, key(t)+"-exhausted")

		if !allow(t, counter, key(t)+"-untouched").Allowed {
			t.Error("one key exhausting its limit refused a different key")
		}
	})

	// Rate limiting runs before the credential check, so a counter that behaved
	// differently for a registered address would enumerate accounts on its own.
	// It never looks a key up anywhere, which is what makes that impossible
	// rather than merely unintended.
	t.Run("cannot distinguish a registered address", func(t *testing.T) {
		counter := build(ratelimit.Rule{Limit: 2, Window: time.Minute}, time.Now)

		known := allow(t, counter, key(t)+"-registered")
		unknown := allow(t, counter, key(t)+"-never-seen")

		if known.Allowed != unknown.Allowed || known.Remaining != unknown.Remaining {
			t.Error("the counter treated two keys differently, and it has no way to know which is registered")
		}
	})

	// An empty key would collapse every caller without one into a single
	// bucket, so the first of them would exhaust the limit for all the rest.
	t.Run("refuses an empty key", func(t *testing.T) {
		counter := build(ratelimit.Rule{Limit: 5, Window: time.Minute}, time.Now)

		if allow(t, counter, "").Allowed {
			t.Error("an empty key was allowed, which would share one bucket between everyone")
		}
	})

	t.Run("reports how many requests remain", func(t *testing.T) {
		counter := build(ratelimit.Rule{Limit: 3, Window: time.Minute}, time.Now)

		first := allow(t, counter, key(t))
		second := allow(t, counter, key(t))

		if first.Remaining != 2 {
			t.Errorf("Remaining after one request = %d, want 2", first.Remaining)
		}
		if second.Remaining != 1 {
			t.Errorf("Remaining after two requests = %d, want 1", second.Remaining)
		}
	})

	// A limit of zero refuses every request and a window of zero counts over no
	// time. Both are configuration mistakes that lock out every user, and
	// failing at construction is better than discovering it at three in the
	// morning.
	t.Run("refuses an unusable rule", func(t *testing.T) {
		for name, rule := range map[string]ratelimit.Rule{
			"no limit":       {Limit: 0, Window: time.Minute},
			"no window":      {Limit: 5, Window: 0},
			"negative limit": {Limit: -1, Window: time.Minute},
		} {
			t.Run(name, func(t *testing.T) {
				defer func() {
					if recover() == nil {
						t.Errorf("a rule with %s was accepted", name)
					}
				}()
				build(rule, time.Now)
			})
		}
	})
}

// allow reads a decision, failing the test on an error the caller did not
// expect, so assertions read as decisions rather than as error handling.
func allow(t *testing.T, counter ratelimit.Counter, k string) ratelimit.Decision {
	t.Helper()

	decision, err := counter.Allow(context.Background(), k)
	if err != nil {
		t.Fatalf("Allow(%q): %v", k, err)
	}
	return decision
}

// key gives each subtest its own key, so an implementation that shares state
// between tests does not carry a count from one assertion into the next.
func key(t *testing.T) string {
	t.Helper()
	return t.Name() + "@example.com"
}
