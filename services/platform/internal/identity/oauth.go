package identity

// Configured OAuth sign-in: IAM-08.
//
// DEC-02 required OAuth for the first release and ADR-0003 put it in Go
// beside the password flow. It arrives at the same entry point everything
// else does: an OAuth sign-in ends in issue(), so it gets the same cookies,
// the same rotation, the same revocation and the same audit as a password.
// A second way to mint a session would be a second way to be signed in that
// only one half of the system knows how to end.
//
// What the provider is trusted for is deliberately narrow. It tells us a
// stable subject and, sometimes, that it has verified an email. It is never
// trusted to tell us who somebody is by email alone: see linking, below.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/token"
)

// How long an authorisation may stay in flight. Long enough to read a consent
// screen and pick an account, short enough that an abandoned state is not a
// standing invitation.
const oauthStateTTL = 10 * time.Minute

var (
	// ErrOAuthStateInvalid covers absent, unknown and already-used state.
	//
	// One error for all three on purpose. A caller cannot tell a forged state
	// from a replayed one, and answering differently would say which.
	ErrOAuthStateInvalid = errors.New("identity: the sign-in could not be completed")

	// ErrOAuthStateExpired is a state that timed out rather than one that was
	// wrong. Distinct because the person deserves "start again" rather than
	// "something is wrong with you".
	ErrOAuthStateExpired = errors.New("identity: the sign-in took too long")

	// ErrOAuthEmailUnverified is the linking refusal.
	//
	// The provider gave an address it has not verified, and an account with
	// that address already exists. Linking on it would hand over that account
	// to anybody who can set an unverified email at the provider.
	ErrOAuthEmailUnverified = errors.New("identity: the provider has not verified that address")

	// ErrOAuthProviderUnknown is a provider that is not configured.
	ErrOAuthProviderUnknown = errors.New("identity: unknown sign-in provider")
)

// ProviderIdentity is everything a provider tells us about an account.
type ProviderIdentity struct {
	// Subject is the provider's own stable identifier. The link is keyed on
	// this and never on the email, because an email can be reassigned by a
	// domain owner and changed by its holder, and either would silently move
	// the link to a different person.
	Subject string
	Email   string
	// EmailVerified is the provider asserting it has proved control of the
	// address. Everything about linking turns on it.
	EmailVerified bool
}

// Provider is what identity asks of one OAuth provider.
//
// Consumer-defined, per ADR-0005: the HTTP round-trip, the client secret and
// the provider's own quirks live in the composition root, and this package
// stays testable without a network.
type Provider interface {
	// AuthorizationURL is where to send the browser.
	AuthorizationURL(state, codeChallenge string) string
	// Exchange turns the authorisation code into what the provider knows,
	// presenting the PKCE verifier that matches the challenge sent above.
	Exchange(ctx context.Context, code, codeVerifier string) (ProviderIdentity, error)
}

// OAuthRepository is the storage half, kept separate from Repository so a
// deployment without OAuth configured needs none of it.
type OAuthRepository interface {
	CreateOAuthState(ctx context.Context, state OAuthState) error
	// ConsumeOAuthState takes the state exactly once. The single-use check is
	// the UPDATE's own condition rather than a read followed by a write, so
	// two callbacks arriving together cannot both succeed.
	ConsumeOAuthState(ctx context.Context, stateHash string) (OAuthState, error)
	FindOAuthIdentity(ctx context.Context, provider, subject string) (string, error)
	LinkOAuthIdentity(ctx context.Context, userID, provider, subject, email string) error
}

// OAuthState is one in-flight authorisation.
type OAuthState struct {
	ID           string
	Provider     string
	StateHash    string
	CodeVerifier string
	RedirectTo   string
	ExpiresAt    time.Time
}

// Authorization is what the caller needs to send the browser onward.
type Authorization struct {
	URL string
	// State in plaintext, which exists only in this return value and in the
	// query string. Storage holds its hash.
	State string
}

// BeginOAuth mints one authorisation and answers where to send the browser.
func (s *Service) BeginOAuth(ctx context.Context, providerName, redirectTo string) (Authorization, error) {
	provider, ok := s.providers[providerName]
	if !ok {
		return Authorization{}, fmt.Errorf("%w: %q", ErrOAuthProviderUnknown, providerName)
	}

	state, err := token.New(token.PurposeOAuthState)
	if err != nil {
		return Authorization{}, err
	}
	verifier, err := token.New(token.PurposeOAuthVerifier)
	if err != nil {
		return Authorization{}, err
	}

	err = s.oauth.CreateOAuthState(ctx, OAuthState{
		ID: id.New().String(), Provider: providerName,
		StateHash: state.Hash, CodeVerifier: verifier.Plaintext,
		RedirectTo: redirectTo,
		ExpiresAt:  s.clock().Add(oauthStateTTL),
	})
	if err != nil {
		return Authorization{}, fmt.Errorf("identity: recording the sign-in attempt: %w", err)
	}

	return Authorization{
		URL:   provider.AuthorizationURL(state.Plaintext, token.ChallengeFor(verifier.Plaintext)),
		State: state.Plaintext,
	}, nil
}

// CompleteOAuth finishes the round trip and issues a session.
//
// The order matters. The state is consumed before the provider is called, so
// a replayed callback is refused whether or not the provider would have
// answered, and a slow provider cannot leave a state usable in the meantime.
func (s *Service) CompleteOAuth(ctx context.Context, providerName, state, code string) (Session, string, error) {
	provider, ok := s.providers[providerName]
	if !ok {
		return Session{}, "", fmt.Errorf("%w: %q", ErrOAuthProviderUnknown, providerName)
	}
	if state == "" || code == "" {
		return Session{}, "", ErrOAuthStateInvalid
	}

	stored, err := s.oauth.ConsumeOAuthState(ctx, token.HashOf(state))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Session{}, "", ErrOAuthStateInvalid
		}
		return Session{}, "", fmt.Errorf("identity: consuming the sign-in state: %w", err)
	}
	// A state minted for Google must not complete a Microsoft callback, or
	// one provider's redirect could be replayed at another.
	if stored.Provider != providerName {
		return Session{}, "", ErrOAuthStateInvalid
	}
	if s.clock().After(stored.ExpiresAt) {
		return Session{}, "", ErrOAuthStateExpired
	}

	asserted, err := provider.Exchange(ctx, code, stored.CodeVerifier)
	if err != nil {
		return Session{}, "", fmt.Errorf("identity: exchanging the authorisation code: %w", err)
	}
	if asserted.Subject == "" {
		return Session{}, "", fmt.Errorf("identity: %s returned no subject", providerName)
	}

	userID, err := s.resolveOAuthUser(ctx, providerName, asserted)
	if err != nil {
		return Session{}, "", err
	}

	now := s.clock()
	session, err := s.issue(ctx, userID, id.New().String(), now, now)
	if err != nil {
		return Session{}, "", err
	}
	return session, stored.RedirectTo, nil
}

// resolveOAuthUser answers which person this provider account is, creating
// one if the provider has verified an address nobody has registered.
//
// The three branches are the whole security of the feature.
func (s *Service) resolveOAuthUser(ctx context.Context, providerName string, asserted ProviderIdentity) (string, error) {
	// Already linked: the subject is what we key on, so a changed email at
	// the provider follows the person rather than moving the account.
	linked, err := s.oauth.FindOAuthIdentity(ctx, providerName, asserted.Subject)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return "", fmt.Errorf("identity: looking up the provider account: %w", err)
	}
	if linked != "" {
		if err := s.oauth.LinkOAuthIdentity(ctx, linked, providerName, asserted.Subject, asserted.Email); err != nil {
			return "", fmt.Errorf("identity: refreshing the provider account: %w", err)
		}
		return linked, nil
	}

	email := NormaliseEmail(asserted.Email)
	if email == "" {
		return "", fmt.Errorf("identity: %s returned no address", providerName)
	}
	existing, _, err := s.repo.FindCredentialsByEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return "", fmt.Errorf("identity: looking up address: %w", err)
	}

	// The refusal that matters. An unverified address must never reach an
	// account that already exists: whoever can set an unverified email at the
	// provider would otherwise be handed that account.
	if existing != "" && !asserted.EmailVerified {
		return "", ErrOAuthEmailUnverified
	}

	if existing != "" {
		if err := s.oauth.LinkOAuthIdentity(ctx, existing, providerName, asserted.Subject, email); err != nil {
			return "", fmt.Errorf("identity: linking the provider account: %w", err)
		}
		return existing, nil
	}

	// Nobody has this address. Register them, as a candidate and only as a
	// candidate: an organisation registration creates a tenant and an owning
	// membership, and IAM-08 is explicit that signing in with a provider
	// never silently creates one.
	userID := id.New().String()
	if err := s.repo.CreateUserWithCredentials(ctx, userID, email, ""); err != nil {
		return "", fmt.Errorf("identity: creating the account: %w", err)
	}
	if err := s.oauth.LinkOAuthIdentity(ctx, userID, providerName, asserted.Subject, email); err != nil {
		return "", fmt.Errorf("identity: linking the new account: %w", err)
	}
	return userID, nil
}

// ConfiguredProviders names what is available, for the sign-in screen.
//
// Configuration rather than compilation, per IAM-08: adding a provider is a
// deployment change and a test, not a release.
func (s *Service) ConfiguredProviders() []string {
	names := make([]string, 0, len(s.providers))
	for name := range s.providers {
		names = append(names, name)
	}
	// Sorted, so the sign-in screen does not reorder itself between requests
	// purely because Go randomises map iteration.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && strings.Compare(names[j-1], names[j]) > 0; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}
	return names
}
