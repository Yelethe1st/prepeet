package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// SEC-10's remaining boxes, from the outside: the endpoints an attacker
// gets unlimited attempts at are counted per address and per network, a
// refusal is 429 with Retry-After, and the counts are separate per
// endpoint so exhausting one does not lock somebody out of another.

// countingLimiter allows a fixed number of attempts per key.
type countingLimiter struct {
	allowance int
	wait      time.Duration
	mu        sync.Mutex
	seen      map[string]int
	broken    error
}

func newCountingLimiter(allowance int) *countingLimiter {
	return &countingLimiter{allowance: allowance, wait: 90 * time.Second, seen: map[string]int{}}
}

func (l *countingLimiter) Allow(_ context.Context, key string) (api.LimitDecision, error) {
	if l.broken != nil {
		return api.LimitDecision{}, l.broken
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen[key]++
	if l.seen[key] > l.allowance {
		return api.LimitDecision{Allowed: false, RetryAfter: l.wait}, nil
	}
	return api.LimitDecision{Allowed: true}, nil
}

func (l *countingLimiter) keys() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.seen))
	for key := range l.seen {
		out = append(out, key)
	}
	return out
}

func serveLimited(t *testing.T, perAddress, perNetwork api.Limiter) http.Handler {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		Identity:           &fakeIdentity{},
		Candidates:         &fakeCandidates{},
		Documents:          &fakeDocuments{},
		Catalog:            &fakeCatalog{},
		Interviews:         &fakeInterviews{},
		Members:            &fakeMembers{},
		Billing:            &fakeBilling{},
		AttemptsPerAddress: perAddress,
		AttemptsPerNetwork: perNetwork,
		Environment:        config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

// attempt posts one login from the given address and network.
func attempt(t *testing.T, handler http.Handler, email, remote string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"email":"` + email + `","password":"correct horse battery staple"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = remote
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestAnAddressIsLimitedHoweverManyNetworksItComesFrom(t *testing.T) {
	handler := serveLimited(t, newCountingLimiter(3), newCountingLimiter(1000))

	for i := 0; i < 3; i++ {
		if code := attempt(t, handler, "ama@example.com", "203.0.113.9:1").Code; code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d was limited early", i+1)
		}
	}
	// A fresh network does not buy the same address a fresh allowance.
	response := attempt(t, handler, "ama@example.com", "198.51.100.7:1")
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("a new network reset the address's count: %d", response.Code)
	}
}

func TestANetworkIsLimitedHoweverManyAddressesItTries(t *testing.T) {
	// The case the ticket names: one attacker with many addresses. A
	// per-address count alone would never see them.
	handler := serveLimited(t, newCountingLimiter(1000), newCountingLimiter(3))

	for i := 0; i < 3; i++ {
		attempt(t, handler, "victim"+string(rune('a'+i))+"@example.com", "203.0.113.9:1")
	}
	response := attempt(t, handler, "someone-else@example.com", "203.0.113.9:1")
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("an unlimited spread of addresses was never counted: %d", response.Code)
	}
}

func TestALimitedResponseIs429WithRetryAfter(t *testing.T) {
	handler := serveLimited(t, newCountingLimiter(0), newCountingLimiter(1000))

	response := attempt(t, handler, "ama@example.com", "203.0.113.9:1")
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", response.Code)
	}
	if retryAfter := response.Header().Get("Retry-After"); retryAfter != "90" {
		t.Fatalf("Retry-After = %q, want the wait in seconds", retryAfter)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeInto(t, response, &body)
	if body.Error.Code != "RATE_LIMITED" {
		t.Fatalf("code = %q", body.Error.Code)
	}
	// The body says the same number the header does, so an interface can
	// show a countdown without parsing headers.
	if !strings.Contains(body.Error.Message, "90 seconds") {
		t.Fatalf("message = %q", body.Error.Message)
	}
}

func TestEachEndpointHasItsOwnAllowance(t *testing.T) {
	// Exhausting one endpoint must not lock somebody out of another:
	// otherwise an attacker locks a person out of signing in by spending
	// their password-reset allowance.
	perAddress := newCountingLimiter(1)
	handler := serveLimited(t, perAddress, newCountingLimiter(1000))

	attempt(t, handler, "ama@example.com", "203.0.113.9:1")
	attempt(t, handler, "ama@example.com", "203.0.113.9:1")

	reset := `{"kind":"password_reset","email":"ama@example.com"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/email/request", strings.NewReader(reset))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "203.0.113.9:1"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusTooManyRequests {
		t.Fatal("spending the login allowance also refused a password reset")
	}

	// The keys say which endpoint each count belongs to.
	var login, token bool
	for _, key := range perAddress.keys() {
		if strings.HasPrefix(key, "login:") {
			login = true
		}
		if strings.HasPrefix(key, "token-email:") {
			token = true
		}
	}
	if !login || !token {
		t.Fatalf("keys = %v", perAddress.keys())
	}
}

func TestTheLimiterFailsOpenWhenItCannotBeRead(t *testing.T) {
	// The counter shares a database with the credentials it protects, so
	// a counter that cannot be read is a database that cannot
	// authenticate anyway: refusing here would turn a degraded dependency
	// into an outage.
	broken := newCountingLimiter(0)
	broken.broken = context.DeadlineExceeded
	handler := serveLimited(t, broken, broken)

	if code := attempt(t, handler, "ama@example.com", "203.0.113.9:1").Code; code == http.StatusTooManyRequests {
		t.Fatal("an unreadable counter locked everybody out")
	}
}

func TestWithoutLimitersConfiguredNothingIsRefused(t *testing.T) {
	handler := serveLimited(t, nil, nil)
	for i := 0; i < 5; i++ {
		if code := attempt(t, handler, "ama@example.com", "203.0.113.9:1").Code; code == http.StatusTooManyRequests {
			t.Fatal("an unconfigured limiter refused an attempt")
		}
	}
}
