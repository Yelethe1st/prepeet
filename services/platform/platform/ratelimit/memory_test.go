package ratelimit_test

import (
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/ratelimit"
)

// Memory satisfies the same contract as every other counter.
func TestMemorySatisfiesTheCounterContract(t *testing.T) {
	t.Parallel()

	runCounterContract(t, func(rule ratelimit.Rule, now func() time.Time) ratelimit.Counter {
		return ratelimit.NewMemory(rule, now)
	})
}

// What follows is specific to counting in this process, and has no equivalent
// in an implementation that counts elsewhere.

// A limiter that never forgets is a memory leak with a long fuse: one entry per
// address anyone has ever tried to log in with, held until the process
// restarts.
func TestMemoryForgetsExpiredEntries(t *testing.T) {
	t.Parallel()

	now := time.Now()
	limiter := ratelimit.NewMemory(
		ratelimit.Rule{Limit: 5, Window: time.Minute},
		func() time.Time { return now })

	for i := range 100 {
		allow(t, limiter, string(rune('a'+i%26))+"@example.com")
	}
	if limiter.Size() == 0 {
		t.Fatal("the limiter recorded nothing")
	}

	now = now.Add(2 * time.Minute)
	limiter.Sweep()

	if size := limiter.Size(); size != 0 {
		t.Errorf("Size = %d after the window passed, want 0", size)
	}
}

// The limiter is consulted from every request handler at once.
func TestMemoryIsSafeUnderConcurrentUse(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.NewMemory(ratelimit.Rule{Limit: 1000, Window: time.Minute}, time.Now)

	done := make(chan struct{})
	for range 8 {
		go func() {
			for i := range 100 {
				allow(t, limiter, string(rune('a'+i%26)))
			}
			done <- struct{}{}
		}()
	}
	for range 8 {
		<-done
	}
}

// Limiting per address alone is close to useless: an attacker with a list of
// addresses gets the full allowance against each one. The network has to be
// counted too, and the stricter of the two decides. This is a property of how
// the caller composes two counters rather than of either one, so it is asserted
// once here rather than in the shared contract.
func TestTheStricterOfTwoLimitsDecides(t *testing.T) {
	t.Parallel()

	byAddress := ratelimit.NewMemory(ratelimit.Rule{Limit: 5, Window: time.Minute}, time.Now)
	byNetwork := ratelimit.NewMemory(ratelimit.Rule{Limit: 3, Window: time.Minute}, time.Now)

	allowed := 0
	for i := range 5 {
		// A different address each time, from one network, which is exactly
		// what credential stuffing looks like.
		address := string(rune('a'+i)) + "@example.com"
		if allow(t, byAddress, address).Allowed && allow(t, byNetwork, "203.0.113.7").Allowed {
			allowed++
		}
	}

	if allowed != 3 {
		t.Errorf("allowed %d attempts, want 3: the network limit must bind when the address limit does not", allowed)
	}
}
