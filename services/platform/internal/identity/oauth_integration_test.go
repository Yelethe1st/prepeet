//go:build integration

package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// IAM-08 against real PostgreSQL. Every property here is one an attacker
// tests for us if we do not: a replayed state, a state borrowed from another
// provider, an expired one, and an unverified address pointed at somebody
// else's account.

type stubProvider struct {
	identity  identity.ProviderIdentity
	err       error
	verifiers []string
	challenge string
}

func (p *stubProvider) AuthorizationURL(state, codeChallenge string) string {
	p.challenge = codeChallenge
	return "https://provider.example/authorize?state=" + state
}

func (p *stubProvider) Exchange(_ context.Context, _, codeVerifier string) (identity.ProviderIdentity, error) {
	p.verifiers = append(p.verifiers, codeVerifier)
	return p.identity, p.err
}

func oauthService(t *testing.T, provider *stubProvider) *identity.Service {
	t.Helper()
	repo := identity.NewRepository(pool)
	return identity.NewService(repo, time.Now).
		WithOAuth(repo, map[string]identity.Provider{"google": provider})
}

func newAddress() string {
	return "oauth-" + strings.ToLower(id.New().String()) + "@example.com"
}

func TestAVerifiedProviderAccountSignsInAndStaysLinked(t *testing.T) {
	ctx := context.Background()
	subject := "google-" + id.New().String()
	address := newAddress()
	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: subject, Email: address, EmailVerified: true,
	}}
	service := oauthService(t, provider)

	begun, err := service.BeginOAuth(ctx, "google", "/practice")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if begun.State == "" || !strings.Contains(begun.URL, begun.State) {
		t.Fatalf("the state did not reach the browser: %+v", begun)
	}
	if provider.challenge == "" {
		t.Fatal("no PKCE challenge was sent to the provider")
	}

	session, redirect, err := service.CompleteOAuth(ctx, "google", begun.State, "auth-code")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if session.SessionToken == "" {
		t.Fatal("no session was issued")
	}
	if redirect != "/practice" {
		t.Fatalf("redirect = %q, want the one begun with", redirect)
	}
	// The verifier reached the provider, which is the whole point of PKCE:
	// the challenge went out in the open and the verifier did not.
	if len(provider.verifiers) != 1 || provider.verifiers[0] == "" {
		t.Fatalf("the PKCE verifier was not presented: %v", provider.verifiers)
	}

	// Signing in again finds the existing link rather than making a second
	// account, and does so on the subject even though nothing else matched.
	again, err := service.BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin again: %v", err)
	}
	second, _, err := service.CompleteOAuth(ctx, "google", again.State, "auth-code")
	if err != nil {
		t.Fatalf("complete again: %v", err)
	}
	if second.UserID != session.UserID {
		t.Fatalf("the same provider account signed in as two people: %s then %s",
			session.UserID, second.UserID)
	}
}

func TestAReplayedStateIsRefused(t *testing.T) {
	ctx := context.Background()
	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: "google-" + id.New().String(), Email: newAddress(), EmailVerified: true,
	}}
	service := oauthService(t, provider)

	begun, err := service.BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, _, err := service.CompleteOAuth(ctx, "google", begun.State, "auth-code"); err != nil {
		t.Fatalf("first completion: %v", err)
	}

	_, _, err = service.CompleteOAuth(ctx, "google", begun.State, "auth-code")
	if !errors.Is(err, identity.ErrOAuthStateInvalid) {
		t.Fatalf("a replayed state was not refused: %v", err)
	}
	// And the provider was not called a second time: the state is consumed
	// before the exchange, so a replay costs nothing downstream.
	if len(provider.verifiers) != 1 {
		t.Fatalf("the provider was called %d times for one usable state", len(provider.verifiers))
	}
}

func TestAnUnknownStateIsRefused(t *testing.T) {
	ctx := context.Background()
	service := oauthService(t, &stubProvider{})

	_, _, err := service.CompleteOAuth(ctx, "google", "not-a-state-we-minted", "auth-code")

	if !errors.Is(err, identity.ErrOAuthStateInvalid) {
		t.Fatalf("a forged state was not refused: %v", err)
	}
}

func TestAnAbsentStateIsRefused(t *testing.T) {
	ctx := context.Background()
	service := oauthService(t, &stubProvider{})

	_, _, err := service.CompleteOAuth(ctx, "google", "", "auth-code")

	if !errors.Is(err, identity.ErrOAuthStateInvalid) {
		t.Fatalf("a missing state was tolerated: %v", err)
	}
}

// A state minted for one provider must not complete another's callback, or a
// redirect from the weaker provider is replayable at the stronger one.
func TestAStateCannotCrossProviders(t *testing.T) {
	ctx := context.Background()
	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: "s", Email: newAddress(), EmailVerified: true,
	}}
	repo := identity.NewRepository(pool)
	service := identity.NewService(repo, time.Now).WithOAuth(repo, map[string]identity.Provider{
		"google": provider, "microsoft": provider,
	})

	begun, err := service.BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	_, _, err = service.CompleteOAuth(ctx, "microsoft", begun.State, "auth-code")
	if !errors.Is(err, identity.ErrOAuthStateInvalid) {
		t.Fatalf("a google state completed a microsoft callback: %v", err)
	}
}

func TestAnExpiredStateIsToldItExpired(t *testing.T) {
	ctx := context.Background()
	repo := identity.NewRepository(pool)
	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: "s-" + id.New().String(), Email: newAddress(), EmailVerified: true,
	}}

	// Begin in the past, complete now: eleven minutes against a ten-minute
	// window, without eleven minutes passing.
	past := time.Now().Add(-11 * time.Minute)
	begun, err := identity.NewService(repo, func() time.Time { return past }).
		WithOAuth(repo, map[string]identity.Provider{"google": provider}).
		BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	_, _, err = oauthService(t, provider).CompleteOAuth(ctx, "google", begun.State, "auth-code")

	// Distinct from invalid: "start again" is a different sentence from
	// "something is wrong", and the person deserves the right one.
	if !errors.Is(err, identity.ErrOAuthStateExpired) {
		t.Fatalf("an expired state was not named as expired: %v", err)
	}
}

// The linking rule, attacked. This is the account-takeover path: a provider
// that asserts an address it has not verified, pointed at an account that
// already exists.
func TestAnUnverifiedAddressNeverReachesAnExistingAccount(t *testing.T) {
	ctx := context.Background()
	repo := identity.NewRepository(pool)
	service := identity.NewService(repo, time.Now)

	address := newAddress()
	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: address, Password: "a-long-enough-password", AccountType: "candidate",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: "attacker-" + id.New().String(), Email: address, EmailVerified: false,
	}}
	oauth := identity.NewService(repo, time.Now).
		WithOAuth(repo, map[string]identity.Provider{"google": provider})

	begun, err := oauth.BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	_, _, err = oauth.CompleteOAuth(ctx, "google", begun.State, "auth-code")

	if !errors.Is(err, identity.ErrOAuthEmailUnverified) {
		t.Fatalf("an unverified address reached an existing account: %v", err)
	}
}

func TestAVerifiedAddressLinksToTheAccountThatOwnsIt(t *testing.T) {
	ctx := context.Background()
	repo := identity.NewRepository(pool)
	address := newAddress()

	registered, err := identity.NewService(repo, time.Now).Register(ctx, identity.RegisterInput{
		Email: address, Password: "a-long-enough-password", AccountType: "candidate",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: "google-" + id.New().String(), Email: address, EmailVerified: true,
	}}
	oauth := identity.NewService(repo, time.Now).
		WithOAuth(repo, map[string]identity.Provider{"google": provider})

	begun, err := oauth.BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	session, _, err := oauth.CompleteOAuth(ctx, "google", begun.State, "auth-code")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// The same person, not a second account with the same address.
	if session.UserID != registered.UserID {
		t.Fatalf("signed in as %s, want the registered %s", session.UserID, registered.UserID)
	}
}

func TestAnUnknownProviderIsRefused(t *testing.T) {
	ctx := context.Background()
	service := oauthService(t, &stubProvider{})

	if _, err := service.BeginOAuth(ctx, "myspace", ""); !errors.Is(err, identity.ErrOAuthProviderUnknown) {
		t.Fatalf("an unconfigured provider was accepted: %v", err)
	}
}

func TestConfiguredProvidersAreStablyOrdered(t *testing.T) {
	repo := identity.NewRepository(pool)
	service := identity.NewService(repo, time.Now).WithOAuth(repo, map[string]identity.Provider{
		"microsoft": &stubProvider{}, "google": &stubProvider{},
	})

	// Twice, because Go randomises map iteration and a sign-in screen that
	// reorders its buttons between requests is one people misclick.
	first := strings.Join(service.ConfiguredProviders(), ",")
	second := strings.Join(service.ConfiguredProviders(), ",")

	if first != "google,microsoft" || second != first {
		t.Fatalf("providers came back as %q then %q", first, second)
	}
}

// An account created by signing in with a provider has no password, and
// trying one must fail exactly as a wrong password does. Anything else is an
// oracle: a different error tells an attacker which addresses are
// provider-only, which is the set worth trying a provider attack against.
func TestAProviderOnlyAccountRefusesAPasswordLikeAnyWrongOne(t *testing.T) {
	ctx := context.Background()
	repo := identity.NewRepository(pool)
	address := newAddress()
	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: "google-" + id.New().String(), Email: address, EmailVerified: true,
	}}
	service := identity.NewService(repo, time.Now).
		WithOAuth(repo, map[string]identity.Provider{"google": provider})

	begun, err := service.BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, _, err := service.CompleteOAuth(ctx, "google", begun.State, "auth-code"); err != nil {
		t.Fatalf("complete: %v", err)
	}

	_, viaPassword := service.Authenticate(ctx, address, "any-password-at-all")
	_, viaUnknown := service.Authenticate(ctx, newAddress(), "any-password-at-all")

	if !errors.Is(viaPassword, identity.ErrCredentialsInvalid) {
		t.Fatalf("a provider-only account answered %v, want the ordinary refusal", viaPassword)
	}
	if viaPassword.Error() != viaUnknown.Error() {
		t.Fatalf("provider-only says %q and unknown says %q: the difference is an oracle",
			viaPassword, viaUnknown)
	}
}
