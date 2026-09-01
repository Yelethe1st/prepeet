//go:build integration

package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/token"
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

// ---------------------------------------------------------------------------
// Regressions from the IAM-08 review. Each names the finding it holds down.

// Finding: account creation committed before the link, so a failure between
// them left a user with an empty password hash and no provider identity. The
// address was taken, password sign-in could not succeed, and the next provider
// attempt refused to link an unverified address to the account that existed.
func TestANewAccountAndItsProviderLinkAreOneTransaction(t *testing.T) {
	ctx := context.Background()
	repo := identity.NewRepository(pool)
	address := newAddress()
	subject := "google-" + id.New().String()

	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: subject, Email: address, EmailVerified: true,
	}}
	service := identity.NewService(repo, time.Now).
		WithOAuth(repo, map[string]identity.Provider{"google": provider})

	begun, err := service.BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	session, _, err := service.CompleteOAuth(ctx, "google", begun.State, "auth-code")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// The link exists for the account that was created, which is the half that
	// used to be able to go missing.
	linked, err := repo.FindOAuthIdentity(ctx, "google", subject)
	if err != nil {
		t.Fatalf("the account was created without its provider link: %v", err)
	}
	if linked != session.UserID {
		t.Fatalf("the link points at %s and the session is %s", linked, session.UserID)
	}
}

// The same account, signed into twice concurrently. One transaction wins the
// unique index and the loser must read the winner back rather than error,
// which is the race the fix has to survive.
func TestTwoSimultaneousFirstSignInsResolveToOneAccount(t *testing.T) {
	ctx := context.Background()
	repo := identity.NewRepository(pool)
	address := newAddress()
	subject := "google-" + id.New().String()

	service := func() *identity.Service {
		return identity.NewService(repo, time.Now).
			WithOAuth(repo, map[string]identity.Provider{"google": &stubProvider{
				identity: identity.ProviderIdentity{
					Subject: subject, Email: address, EmailVerified: true,
				},
			}})
	}

	first, err := service().BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	second, err := service().BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin again: %v", err)
	}

	results := make(chan string, 2)
	failures := make(chan error, 2)
	for _, state := range []string{first.State, second.State} {
		go func(state string) {
			session, _, err := service().CompleteOAuth(ctx, "google", state, "auth-code")
			if err != nil {
				failures <- err
				return
			}
			results <- session.UserID
		}(state)
	}

	seen := []string{}
	for range 2 {
		select {
		case userID := <-results:
			seen = append(seen, userID)
		case err := <-failures:
			t.Fatalf("a concurrent first sign-in failed rather than resolving: %v", err)
		}
	}
	if seen[0] != seen[1] {
		t.Fatalf("one address became two accounts: %s and %s", seen[0], seen[1])
	}
}

// Finding: the destination recorded at the start was returned by the service
// and thrown away by the caller. It has to survive the round trip.
func TestTheDestinationSurvivesTheRoundTrip(t *testing.T) {
	ctx := context.Background()
	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: "google-" + id.New().String(), Email: newAddress(), EmailVerified: true,
	}}
	service := oauthService(t, provider)

	begun, err := service.BeginOAuth(ctx, "google", "/session/abc/results")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	_, redirect, err := service.CompleteOAuth(ctx, "google", begun.State, "auth-code")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if redirect != "/session/abc/results" {
		t.Fatalf("redirect = %q, want the destination the sign-in was started with", redirect)
	}
}

// Finding: DeleteExpiredOAuthStates was generated and never called, so every
// completed, expired or abandoned attempt left a permanent row holding a PKCE
// verifier and a destination.
func TestStartingASignInSweepsTheExpiredOnes(t *testing.T) {
	ctx := context.Background()
	repo := identity.NewRepository(pool)
	provider := &stubProvider{}

	// A state minted well in the past, which is what an abandoned sign-in
	// leaves behind.
	stale := identity.NewService(repo, func() time.Time {
		return time.Now().Add(-2 * time.Hour)
	}).WithOAuth(repo, map[string]identity.Provider{"google": provider})
	abandoned, err := stale.BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin in the past: %v", err)
	}

	// Any sign-in started now sweeps it.
	if _, err := oauthService(t, provider).BeginOAuth(ctx, "google", ""); err != nil {
		t.Fatalf("begin now: %v", err)
	}

	// Gone rather than merely expired: consuming it finds nothing at all.
	_, err = repo.ConsumeOAuthState(ctx, token.HashOf(abandoned.State))
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("the abandoned state is still there: %v", err)
	}
}

// The sweep must not take a live state with it, which is the way a cleanup
// turns into an outage: everybody mid-sign-in is refused.
func TestTheSweepLeavesLiveStatesAlone(t *testing.T) {
	ctx := context.Background()
	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: "google-" + id.New().String(), Email: newAddress(), EmailVerified: true,
	}}
	service := oauthService(t, provider)

	inFlight, err := service.BeginOAuth(ctx, "google", "")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// Somebody else starts one, which is what runs the sweep.
	if _, err := service.BeginOAuth(ctx, "google", ""); err != nil {
		t.Fatalf("begin again: %v", err)
	}

	if _, _, err := service.CompleteOAuth(ctx, "google", inFlight.State, "auth-code"); err != nil {
		t.Fatalf("a sign-in in flight was swept away: %v", err)
	}
}

// IAM-08's last criterion, in two halves.
//
// "Creates the same account a form does, including account type, and never
// silently creates a tenant." Account type is not a stored property: there is
// no account_type column, and nothing reads one. It is a fork in the
// registration path, where an organisation registration additionally creates a
// workspace and an owning membership and a candidate registration does not. So
// the criterion is really about which fork a provider sign-up takes, and the
// answer has to be the narrow one.
//
// It has to be the narrow one for a reason more concrete than caution: an
// organisation registration requires an organisation name, and no provider
// returns one. Asking for it afterwards would turn "Continue with Google" into
// a form and leave a half-registered account in between, which is exactly what
// CreateOrganisationAccount's single transaction exists to prevent.

func TestAProviderSignUpCreatesNoTenant(t *testing.T) {
	ctx := context.Background()
	address := newAddress()
	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: "google-" + id.New().String(), Email: address, EmailVerified: true,
	}}
	service := oauthService(t, provider)

	begun, err := service.BeginOAuth(ctx, "google", "/practice")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	session, _, err := service.CompleteOAuth(ctx, "google", begun.State, "auth-code")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// No workspace, and no membership pointing at one. A person practising
	// alone must not own an employer tenant they never asked for: it would sit
	// in the billing and retention inventories for no reason, and it would be a
	// tenant nobody named.
	// Counted as the migrator, not through the application pool.
	//
	// The first version of this asked the pool, which has no tenant context, so
	// row-level security hid every membership and the count was zero whatever
	// had been created. It would have passed if a provider sign-up created a
	// whole workspace. A probe that planted a real membership through the
	// invitation path is what exposed it: the probe succeeded and the assertion
	// still saw nothing.
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()

	var memberships int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM tenancy.memberships WHERE user_id = $1`,
		session.UserID).Scan(&memberships); err != nil {
		t.Fatalf("counting memberships: %v", err)
	}
	if memberships != 0 {
		t.Fatalf("a provider sign-up created %d membership(s)", memberships)
	}

	// The membership count is the whole check, and it is enough. A tenant is
	// only reachable through a membership, and CreateOrganisationAccount makes
	// the workspace and the owning membership in one transaction, so no
	// membership means no workspace was created for this person. tenancy.tenants
	// records no creator, which is why this asks the question that has an
	// answer rather than the one that reads more directly.
}

// The other half, and the reason the first is not a restriction.
//
// A recruiter is somebody holding a membership, not somebody with a flag. An
// account created through a provider is an ordinary account, so the invitation
// path admits it to a workspace exactly as it admits one created with a
// password. Nobody is shut out of recruiting by signing in with Google; what
// they cannot do is conjure a workspace nobody named.
func TestAProviderAccountCanBeInvitedIntoAWorkspace(t *testing.T) {
	ctx := context.Background()
	address := newAddress()
	provider := &stubProvider{identity: identity.ProviderIdentity{
		Subject: "google-" + id.New().String(), Email: address, EmailVerified: true,
	}}
	service := oauthService(t, provider)

	begun, err := service.BeginOAuth(ctx, "google", "/practice")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	session, _, err := service.CompleteOAuth(ctx, "google", begun.State, "auth-code")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// A workspace that already exists, registered the way the form registers
	// one, with a name somebody supplied. That name is the field no provider
	// returns, which is why this path stays a form.
	ownerEmail := newAddress()
	outcome, err := service.Register(ctx, identity.RegisterInput{
		Email: ownerEmail, Password: goodPassword,
		AccountType: identity.AccountOrganisation, OrganisationName: "Northwind Health",
	})
	if err != nil {
		t.Fatalf("registering the organisation: %v", err)
	}
	ownerSession, err := service.Authenticate(ctx, ownerEmail, goodPassword)
	if err != nil {
		t.Fatalf("authenticating the owner: %v", err)
	}

	members := identity.NewMembers(identity.NewRepository(pool))
	invited, err := members.Invite(ctx, outcome.TenantID, ownerSession.UserID, address, "recruiter")
	if err != nil {
		t.Fatalf("inviting the provider account: %v", err)
	}
	if invited.UserID != session.UserID {
		t.Fatalf("the invitation reached %s rather than the provider account %s",
			invited.UserID, session.UserID)
	}
}
