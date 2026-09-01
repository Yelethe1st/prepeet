package oidc

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Whether a provider would actually work, rather than whether somebody typed
// its credentials into the configuration.
//
// The sign-in screen is the one place a person has no way to recover. A button
// that fails at the provider looks like the product is broken, and a candidate
// who clicks it has no idea whether their account exists, whether they are
// signed in, or what to try instead. Offering only providers that answer is
// worth a probe.

// probeTimeout bounds one check.
//
// Short on purpose: this runs while somebody is waiting to see whether they can
// sign in at all. A provider that has not answered in two seconds is not one to
// put a button in front of them for, whatever it is doing.
const probeTimeout = 2 * time.Second

// Health answers whether a provider's endpoint is reachable, cached.
//
// Cached because the sign-in screen is the busiest unauthenticated page in the
// product. Probing Google on every load would add their latency to every render
// of ours, and would look like exactly the traffic they are entitled to rate
// limit.
type Health struct {
	ttl    time.Duration
	client *http.Client

	mu      sync.Mutex
	answers map[string]answer
}

type answer struct {
	healthy bool
	at      time.Time
}

// NewHealth builds a checker whose answers expire after ttl.
func NewHealth(ttl time.Duration) *Health {
	return &Health{
		ttl:     ttl,
		client:  &http.Client{Timeout: probeTimeout},
		answers: map[string]answer{},
	}
}

// Healthy reports whether the endpoint answered recently enough to offer.
//
// Any answer counts, including a refusal. An authorize endpoint asked without
// parameters replies 400 or redirects, and both mean it is there; treating a
// 4xx as unhealthy would hide every provider, which is the opposite of what
// this is for. Only a server error or no answer at all is a failure.
func (h *Health) Healthy(ctx context.Context, endpoint string) bool {
	if endpoint == "" {
		return false
	}

	h.mu.Lock()
	cached, found := h.answers[endpoint]
	h.mu.Unlock()
	if found && time.Since(cached.at) < h.ttl {
		return cached.healthy
	}

	healthy := h.probe(ctx, endpoint)

	h.mu.Lock()
	h.answers[endpoint] = answer{healthy: healthy, at: time.Now()}
	h.mu.Unlock()
	return healthy
}

// probe asks the endpoint whether it is there.
//
// GET rather than HEAD: authorization endpoints are not required to implement
// HEAD, and a 405 from one that does not would read as an outage. The response
// body is discarded unread, so the cost is a round trip rather than a download.
//
// Redirects are not followed. A provider answering 302 has answered, and
// following it would mean probing wherever it points, which is not the endpoint
// whose health was asked about.
func (h *Health) probe(ctx context.Context, endpoint string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}

	client := *h.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()

	return response.StatusCode < http.StatusInternalServerError
}
