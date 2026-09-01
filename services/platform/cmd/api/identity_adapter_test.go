package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/platform/oidc"
)

// A provider that does not answer is not offered.
//
// The sign-in screen is the one place a person has no way to recover: a button
// that fails at the provider looks like the product is broken, and they have no
// idea whether their account exists or whether they are now signed in.
func TestAnUnhealthyProviderIsNotOffered(t *testing.T) {
	t.Parallel()

	reachable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer reachable.Close()
	unreachable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadAddress := unreachable.URL
	unreachable.Close()

	adapter := identityAdapter{
		service: identity.NewService(nil, time.Now).
			WithOAuth(nil, map[string]identity.Provider{
				"google":    stubProvider{},
				"microsoft": stubProvider{},
			}),
		health: oidc.NewHealth(time.Minute),
		authorizeEndpoints: map[string]string{
			"google":    reachable.URL,
			"microsoft": deadAddress,
		},
	}

	offered := adapter.AvailableOAuthProviders(context.Background())

	if len(offered) != 1 || offered[0] != "google" {
		t.Fatalf("want google alone, got %v", offered)
	}
}

// With no checker wired, configuration is the answer. That is the honest
// default for a deployment that has not asked for probing.
func TestWithoutAHealthCheckerEveryConfiguredProviderIsOffered(t *testing.T) {
	t.Parallel()

	adapter := identityAdapter{
		service: identity.NewService(nil, time.Now).
			WithOAuth(nil, map[string]identity.Provider{"google": stubProvider{}}),
	}

	if offered := adapter.AvailableOAuthProviders(context.Background()); len(offered) != 1 {
		t.Fatalf("want the configured provider, got %v", offered)
	}
}

type stubProvider struct{}

func (stubProvider) AuthorizationURL(string, string) string { return "https://example.test" }

func (stubProvider) Exchange(context.Context, string, string) (identity.ProviderIdentity, error) {
	return identity.ProviderIdentity{}, nil
}
