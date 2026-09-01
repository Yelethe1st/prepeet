package oidc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/oidc"
)

// A provider is offered when signing in through it would work, not merely when
// somebody typed its credentials into the configuration.
//
// The distinction matters on the one screen where a person has no way to
// recover: a button that fails at the provider looks like the product is
// broken, and the candidate has no idea whether their account exists.

func TestAReachableProviderIsHealthy(t *testing.T) {
	t.Parallel()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer provider.Close()

	if !oidc.NewHealth(time.Minute).Healthy(context.Background(), provider.URL) {
		t.Fatal("a provider that answers was reported unhealthy")
	}
}

func TestAnUnreachableProviderIsNotOffered(t *testing.T) {
	t.Parallel()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	address := provider.URL
	provider.Close() // nothing is listening now

	if oidc.NewHealth(time.Minute).Healthy(context.Background(), address) {
		t.Fatal("a provider nothing is listening on was offered")
	}
}

func TestAProviderAnsweringAServerErrorIsNotOffered(t *testing.T) {
	t.Parallel()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer provider.Close()

	if oidc.NewHealth(time.Minute).Healthy(context.Background(), provider.URL) {
		t.Fatal("a provider answering 500 was offered")
	}
}

func TestAnAuthorizeEndpointRefusingABareProbeIsStillHealthy(t *testing.T) {
	t.Parallel()
	// An authorize endpoint asked without parameters answers 400 or a redirect,
	// and both mean it is there. Treating a 4xx as unhealthy would hide every
	// provider, which is the opposite of the failure this guards.
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer provider.Close()

	if !oidc.NewHealth(time.Minute).Healthy(context.Background(), provider.URL) {
		t.Fatal("a provider that answered at all was reported unhealthy")
	}
}

func TestTheAnswerIsCachedRatherThanProbedPerRequest(t *testing.T) {
	t.Parallel()
	var probes atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		probes.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer provider.Close()

	health := oidc.NewHealth(time.Minute)
	for range 5 {
		health.Healthy(context.Background(), provider.URL)
	}

	// The sign-in screen is the busiest unauthenticated page in the product.
	// Probing Google on every load would add their latency to ours and would
	// look like traffic they are entitled to rate limit.
	if probes.Load() != 1 {
		t.Fatalf("the provider was probed %d times for five questions", probes.Load())
	}
}

func TestAStaleAnswerIsProbedAgain(t *testing.T) {
	t.Parallel()
	var probes atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		probes.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer provider.Close()

	// A cache with no expiry would keep offering a provider that went down an
	// hour ago, which is the same failure the check was added to prevent.
	health := oidc.NewHealth(time.Nanosecond)
	health.Healthy(context.Background(), provider.URL)
	time.Sleep(time.Millisecond)
	health.Healthy(context.Background(), provider.URL)

	if probes.Load() < 2 {
		t.Fatalf("an expired answer was reused: %d probes", probes.Load())
	}
}
