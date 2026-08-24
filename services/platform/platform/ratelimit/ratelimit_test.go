package ratelimit_test

import (
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/ratelimit"
)

var start = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

func newLimiter(rule ratelimit.Rule) (*ratelimit.Limiter, *time.Time) {
	now := start
	limiter := ratelimit.New(rule, func() time.Time { return now })
	return limiter, &now
}

func TestRequestsUnderTheLimitAreAllowed(t *testing.T) {
	t.Parallel()

	limiter, _ := newLimiter(ratelimit.Rule{Limit: 5, Window: time.Minute})

	for i := range 5 {
		if decision := limiter.Allow("daniel@example.com"); !decision.Allowed {
			t.Errorf("request %d was refused while under the limit", i+1)
		}
	}
}

func TestTheRequestOverTheLimitIsRefused(t *testing.T) {
	t.Parallel()

	limiter, _ := newLimiter(ratelimit.Rule{Limit: 3, Window: time.Minute})

	for range 3 {
		limiter.Allow("daniel@example.com")
	}

	decision := limiter.Allow("daniel@example.com")
	if decision.Allowed {
		t.Error("the fourth request was allowed under a limit of three")
	}
	if decision.RetryAfter <= 0 {
		t.Error("RetryAfter is not positive, and a client needs to know when to try again")
	}
}

// The window has to move, or an attacker who exhausts the limit is locked out
// forever and a legitimate person who mistyped their password twice is locked
// out for the rest of the day.
func TestTheLimitRecoversAsTheWindowPasses(t *testing.T) {
	t.Parallel()

	limiter, now := newLimiter(ratelimit.Rule{Limit: 2, Window: time.Minute})

	limiter.Allow("daniel@example.com")
	limiter.Allow("daniel@example.com")
	if limiter.Allow("daniel@example.com").Allowed {
		t.Fatal("the third request was allowed under a limit of two")
	}

	*now = now.Add(time.Minute + time.Second)

	if !limiter.Allow("daniel@example.com").Allowed {
		t.Error("the limit did not recover after the window passed")
	}
}

// One person exhausting their attempts must not lock out everyone else, which
// is what a single global counter would do.
func TestKeysAreCountedSeparately(t *testing.T) {
	t.Parallel()

	limiter, _ := newLimiter(ratelimit.Rule{Limit: 2, Window: time.Minute})

	limiter.Allow("daniel@example.com")
	limiter.Allow("daniel@example.com")

	if !limiter.Allow("amara@example.com").Allowed {
		t.Error("one key exhausting its limit refused a different key")
	}
}

// Limiting per address alone is close to useless: an attacker with a list of
// addresses gets the full allowance against each one. The network has to be
// counted too, and the stricter of the two decides.
func TestTheStricterOfTwoLimitsDecides(t *testing.T) {
	t.Parallel()

	byAddress, _ := newLimiter(ratelimit.Rule{Limit: 5, Window: time.Minute})
	byNetwork, _ := newLimiter(ratelimit.Rule{Limit: 3, Window: time.Minute})

	allowed := 0
	for i := range 5 {
		address := string(rune('a'+i)) + "@example.com"
		// Every attempt is a different address from the same network, which is
		// exactly what credential stuffing looks like.
		a := byAddress.Allow(address)
		n := byNetwork.Allow("203.0.113.7")
		if a.Allowed && n.Allowed {
			allowed++
		}
	}

	if allowed != 3 {
		t.Errorf("allowed %d attempts, want 3: the network limit must bind when the address limit does not", allowed)
	}
}

// Rate limiting is applied before the credential check, so it must not tell an
// attacker which addresses exist. A registered address and an unknown one are
// the same key to this package: it never looks either up.
func TestTheLimiterCannotDistinguishAKnownAddress(t *testing.T) {
	t.Parallel()

	limiter, _ := newLimiter(ratelimit.Rule{Limit: 2, Window: time.Minute})

	known := limiter.Allow("registered@example.com")
	unknown := limiter.Allow("never-seen@example.com")

	if known.Allowed != unknown.Allowed || known.RetryAfter != unknown.RetryAfter {
		t.Error("the limiter treated two keys differently, and it has no way to know which is registered")
	}
}

// A limiter that never forgets is a memory leak with a long fuse: one entry per
// address ever tried, held forever.
func TestExpiredEntriesAreForgotten(t *testing.T) {
	t.Parallel()

	limiter, now := newLimiter(ratelimit.Rule{Limit: 5, Window: time.Minute})

	for i := range 100 {
		limiter.Allow(string(rune('a'+i%26)) + "@example.com")
	}
	if size := limiter.Size(); size == 0 {
		t.Fatal("the limiter recorded nothing")
	}

	*now = now.Add(2 * time.Minute)
	limiter.Sweep()

	if size := limiter.Size(); size != 0 {
		t.Errorf("Size = %d after the window passed, want 0", size)
	}
}

// An empty key would collapse every anonymous caller into one bucket, so the
// first of them would exhaust the limit for all the rest.
func TestAnEmptyKeyIsRefused(t *testing.T) {
	t.Parallel()

	limiter, _ := newLimiter(ratelimit.Rule{Limit: 5, Window: time.Minute})

	if limiter.Allow("").Allowed {
		t.Error("an empty key was allowed, which would share one bucket between everyone")
	}
}

// A rule that allows nothing, or counts over no time, is a configuration
// mistake that would lock out every user. Failing loudly at construction is
// better than at three in the morning.
func TestAnUnusableRuleIsRejected(t *testing.T) {
	t.Parallel()

	for name, rule := range map[string]ratelimit.Rule{
		"no limit":  {Limit: 0, Window: time.Minute},
		"no window": {Limit: 5, Window: 0},
		"negative":  {Limit: -1, Window: time.Minute},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Errorf("a rule with %s was accepted", name)
				}
			}()
			ratelimit.New(rule, time.Now)
		})
	}
}

// The limiter is consulted from every request handler at once.
func TestConcurrentUseIsSafe(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.New(ratelimit.Rule{Limit: 1000, Window: time.Minute}, time.Now)

	done := make(chan struct{})
	for range 8 {
		go func() {
			for i := range 100 {
				limiter.Allow(string(rune('a' + i%26)))
			}
			done <- struct{}{}
		}()
	}
	for range 8 {
		<-done
	}
}
