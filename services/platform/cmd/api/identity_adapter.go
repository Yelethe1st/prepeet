package main

import (
	"context"
	"errors"

	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/oidc"
)

// identityAdapter presents the identity context as the port the API layer
// declared.
//
// This translation is what ADR-0005 costs, and it lives here because cmd is the
// one place allowed to see both contexts. The module boundary test enforces
// that: internal/api importing internal/identity does not compile.
//
// It is worth the cost for a specific reason rather than on principle. The two
// vocabularies differ where it matters: identity distinguishes ErrNotFound from
// ErrCredentialsInvalid because its own logic needs to, and the API must not,
// since a response that could tell them apart is an account-existence oracle.
// The collapse happens here, once, rather than being a rule every handler
// remembers.
type identityAdapter struct {
	service *identity.Service
}

func (a identityAdapter) Register(ctx context.Context, input api.Registration) error {
	_, err := a.service.Register(ctx, identity.RegisterInput{
		Email:            input.Email,
		Password:         input.Password,
		AccountType:      identity.AccountType(input.AccountType),
		OrganisationName: input.OrganisationName,
	})
	// The outcome is deliberately discarded. It reports whether an account was
	// created, and the HTTP layer must not know: a handler holding that fact is
	// one refactor away from responding differently.
	return a.translate(err)
}

func (a identityAdapter) Authenticate(ctx context.Context, email, password string) (api.Session, error) {
	session, err := a.service.Authenticate(ctx, email, password)
	if err != nil {
		return api.Session{}, a.translate(err)
	}
	return sessionFrom(session), nil
}

func (a identityAdapter) Refresh(ctx context.Context, refreshToken string) (api.Session, error) {
	session, err := a.service.Refresh(ctx, refreshToken)
	if err != nil {
		return api.Session{}, a.translate(err)
	}
	return sessionFrom(session), nil
}

func (a identityAdapter) Lookup(ctx context.Context, sessionToken string) (api.Principal, error) {
	row, err := a.service.Lookup(ctx, sessionToken)
	if err != nil {
		return api.Principal{}, a.translate(err)
	}
	return api.Principal{
		UserID:          row.UserID,
		SessionID:       row.ID,
		AuthenticatedAt: row.AuthenticatedAt,
		ActiveTenantID:  row.ActiveTenantID,
	}, nil
}

// Authorize resolves the session and decides one capability through the
// single policy path. A denial arrives as api.ErrForbidden whatever the
// reason was: the reason is the audit record's, not the client's.
func (a identityAdapter) Authorize(ctx context.Context, sessionToken, capability string) (api.Principal, error) {
	row, err := a.service.Authorize(ctx, sessionToken, authz.Capability(capability))
	if err != nil {
		var forbidden *identity.ForbiddenError
		if errors.As(err, &forbidden) {
			return api.Principal{}, api.ErrForbidden
		}
		return api.Principal{}, a.translate(err)
	}
	return api.Principal{
		UserID:          row.UserID,
		SessionID:       row.ID,
		AuthenticatedAt: row.AuthenticatedAt,
		ActiveTenantID:  row.ActiveTenantID,
	}, nil
}

func (a identityAdapter) SelectTenant(ctx context.Context, sessionToken, tenantID string) (api.Principal, error) {
	if err := a.service.SelectTenant(ctx, sessionToken, tenantID); err != nil {
		return api.Principal{}, a.translate(err)
	}
	// Read back rather than assume, so the response describes what was stored
	// rather than what was asked for. They agree today; the day they do not is
	// the day a caller would be told its request succeeded as sent.
	return a.Lookup(ctx, sessionToken)
}

func (a identityAdapter) Revoke(ctx context.Context, sessionToken, reason string) error {
	return a.translate(a.service.Revoke(ctx, sessionToken, reason))
}

// Describe reports the person. Capabilities are not here, because they belong
// to the session rather than to the person: the same person holds different
// authority in each workspace. See DescribeSession.
func (a identityAdapter) Describe(ctx context.Context, userID string) (api.User, error) {
	user, err := a.service.Describe(ctx, userID)
	if err != nil {
		return api.User{}, a.translate(err)
	}
	memberships := make([]api.Membership, 0, len(user.Memberships))
	for _, membership := range user.Memberships {
		memberships = append(memberships, api.Membership{
			TenantID:   membership.TenantID,
			TenantName: membership.TenantName,
			Status:     membership.Status,
		})
	}

	// Role is deliberately not carried across. The contract's Membership says
	// which tenants a person belongs to and whether each is active; what they
	// may do there is an authorization answer, and putting a role in the
	// response invites a client to decide from it. IAM-04 already owns that
	// decision and answers it server side.
	return api.User{
		ID:            user.ID,
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		Memberships:   memberships,
	}, nil
}

func sessionFrom(session identity.Session) api.Session {
	return api.Session{
		UserID:          session.UserID,
		SessionToken:    session.SessionToken,
		RefreshToken:    session.RefreshToken,
		ExpiresAt:       session.ExpiresAt,
		RefreshExpires:  session.RefreshExpires,
		AuthenticatedAt: session.AuthenticatedAt,
	}
}

// translate maps identity's vocabulary onto the API's.
//
// The default case passes the error through unchanged, so an unrecognised
// failure becomes a 500 rather than being silently reclassified as a client
// error. Mapping the other way, defaulting to a 4xx, would hide outages as
// validation failures.
// RequestTokenEmail fans one endpoint out to the four request flows.
//
// The switch lives here rather than in the service so that each flow keeps
// its own named method and its own tests, and here rather than in the handler
// so the HTTP layer never learns the flows differ.
func (a identityAdapter) RequestTokenEmail(ctx context.Context, kind, email string) error {
	switch kind {
	case "verify_email":
		return a.translate(a.service.RequestEmailVerification(ctx, email))
	case "password_reset":
		return a.translate(a.service.RequestPasswordReset(ctx, email))
	case "magic_link":
		return a.translate(a.service.RequestMagicLink(ctx, email))
	case "otp":
		return a.translate(a.service.RequestOTP(ctx, email))
	default:
		// The contract's enum makes this unreachable from outside; reaching it
		// means the enum and this switch have drifted.
		return api.Invalid("kind", "KIND_INVALID", "that is not a token email kind")
	}
}

func (a identityAdapter) ConfirmEmailVerification(ctx context.Context, token string) error {
	return a.translate(a.service.ConfirmEmailVerification(ctx, token))
}

func (a identityAdapter) ConfirmPasswordReset(ctx context.Context, token, password string) error {
	return a.translate(a.service.ConfirmPasswordReset(ctx, token, password))
}

func (a identityAdapter) ConsumeMagicLink(ctx context.Context, token string) (api.Session, error) {
	session, err := a.service.ConsumeMagicLink(ctx, token)
	if err != nil {
		return api.Session{}, a.translate(err)
	}
	return sessionFrom(session), nil
}

func (a identityAdapter) ConsumeOTP(ctx context.Context, email, code string) (api.Session, error) {
	session, err := a.service.ConfirmOTP(ctx, email, code)
	if err != nil {
		return api.Session{}, a.translate(err)
	}
	return sessionFrom(session), nil
}

func (a identityAdapter) translate(err error) error {
	if err == nil {
		return nil
	}

	var cooldown *identity.CooldownError
	if errors.As(err, &cooldown) {
		return &api.CooldownError{RetryAfter: cooldown.RetryAfter}
	}

	switch {
	case errors.Is(err, identity.ErrCredentialsInvalid):
		return api.ErrCredentialsRejected

	case errors.Is(err, identity.ErrTokenInvalid):
		return api.ErrTokenInvalid
	case errors.Is(err, identity.ErrTokenExpired):
		return api.ErrTokenExpired
	case errors.Is(err, identity.ErrTokenUsed):
		return api.ErrTokenUsed
	case errors.Is(err, identity.ErrTokenSuperseded):
		return api.ErrTokenSuperseded
	case errors.Is(err, identity.ErrCodeIncorrect):
		return api.ErrCodeIncorrect
	case errors.Is(err, identity.ErrTooManyAttempts):
		return api.ErrCodeExhausted

	case errors.Is(err, identity.ErrNoMembership):
		return api.ErrForbidden

	case errors.Is(err, identity.ErrSessionInvalid), errors.Is(err, identity.ErrNotFound):
		// ErrNotFound collapses into a rejected session on purpose. It reaches
		// here from a lookup for a token that does not exist, and "no such
		// session" and "that session is over" must be one answer.
		return api.ErrSessionRejected

	case errors.Is(err, identity.ErrEmailInvalid):
		return api.Invalid("email", "EMAIL_INVALID", err.Error())

	case errors.Is(err, identity.ErrPasswordTooShort), errors.Is(err, identity.ErrPasswordTooLong):
		return api.Invalid("password", "PASSWORD_INVALID", err.Error())

	case errors.Is(err, identity.ErrAccountType):
		return api.Invalid("account_type", "ACCOUNT_TYPE_INVALID", err.Error())

	case errors.Is(err, identity.ErrOrganisationName):
		return api.Invalid("organisation_name", "ORGANISATION_NAME_REQUIRED", err.Error())

	default:
		return err
	}
}

// DescribeSession reports the person together with what this session may do.
//
// Two calls rather than one, because the two answers have different subjects: a
// person has memberships, and a session has authority under whichever workspace
// it is acting in. Deriving capabilities from the person would produce the union
// of everywhere they belong.
func (a identityAdapter) DescribeSession(ctx context.Context, sessionToken, userID string) (api.User, error) {
	user, err := a.Describe(ctx, userID)
	if err != nil {
		return api.User{}, err
	}

	granted, err := a.service.Capabilities(ctx, sessionToken)
	if err != nil {
		return api.User{}, a.translate(err)
	}

	capabilities := make([]string, 0, len(granted))
	for _, capability := range granted {
		capabilities = append(capabilities, string(capability))
	}
	user.Capabilities = capabilities

	return user, nil
}

var _ api.Identity = identityAdapter{}

// ConfiguredOAuthProviders names what this deployment offers.
func (a identityAdapter) ConfiguredOAuthProviders() []string {
	return a.service.ConfiguredProviders()
}

// BeginOAuth mints the state and answers where to send the browser.
func (a identityAdapter) BeginOAuth(ctx context.Context, provider, redirectTo string) (api.OAuthStart, error) {
	// Validated here rather than trusted from the request. An open redirect
	// stored is an open redirect, and this is the moment it would be stored.
	// An unsafe destination becomes no destination rather than a refusal: the
	// sign-in is what the person asked for, and the redirect is a convenience
	// they will not miss if it silently becomes the default landing page.
	safe, ok := api.SafeRedirect(redirectTo)
	if !ok {
		safe = ""
	}
	start, err := a.service.BeginOAuth(ctx, provider, safe)
	if err != nil {
		return api.OAuthStart{}, a.translateOAuth(err)
	}
	return api.OAuthStart{AuthorizationURL: start.URL, State: start.State}, nil
}

// CompleteOAuth finishes the round trip and issues the session.
func (a identityAdapter) CompleteOAuth(ctx context.Context, provider, state, code string) (api.Session, string, error) {
	session, redirectTo, err := a.service.CompleteOAuth(ctx, provider, state, code)
	if err != nil {
		return api.Session{}, "", a.translateOAuth(err)
	}
	return sessionFrom(session), redirectTo, nil
}

// translateOAuth maps identity's refusals onto the API's.
//
// The mapping exists because the two vocabularies are answerable to different
// things: identity says what happened, and the API says what it means to
// somebody looking at a screen.
func (a identityAdapter) translateOAuth(err error) error {
	switch {
	case errors.Is(err, identity.ErrOAuthStateInvalid):
		return api.ErrOAuthStateRejected
	case errors.Is(err, identity.ErrOAuthStateExpired):
		return api.ErrOAuthStateExpired
	case errors.Is(err, identity.ErrOAuthEmailUnverified):
		return api.ErrOAuthAddressUnverified
	case errors.Is(err, identity.ErrOAuthProviderUnknown):
		return api.ErrOAuthProviderUnknown
	}
	return a.translate(err)
}

// oauthProviders builds the providers this deployment offers.
//
// A provider missing its credentials is left out rather than added broken, so
// the sign-in screen never draws a button that fails at the token endpoint.
// A deployment that configures none gets an empty map, which is what the
// endpoints and the screen are already written to expect.
//
// Adding a third is a map entry and its configuration. There is no Google
// type and no Microsoft type: platform/oidc is one client and the endpoints
// are config, which is IAM-08's sixth criterion.
func oauthProviders(cfg config.Config) map[string]identity.Provider {
	providers := map[string]identity.Provider{}
	for name, settings := range map[string]config.OAuthProvider{
		"google":    cfg.OAuthGoogle,
		"microsoft": cfg.OAuthMicrosoft,
	} {
		client := oidc.Config{
			AuthorizeURL: settings.AuthorizeURL, TokenURL: settings.TokenURL,
			UserInfoURL: settings.UserInfoURL, ClientID: settings.ClientID,
			ClientSecret: settings.ClientSecret, RedirectURI: settings.RedirectURI,
		}
		if !client.Configured() {
			continue
		}
		providers[name] = oauthProvider{client: oidc.New(client)}
	}
	return providers
}

// oauthProvider adapts the OIDC client to what identity asks for.
//
// The translation is only the identity type: identity must not import a
// platform package's shape into its own vocabulary, or the provider client
// becomes something identity has an opinion about.
type oauthProvider struct{ client *oidc.Client }

func (p oauthProvider) AuthorizationURL(state, codeChallenge string) string {
	return p.client.AuthorizationURL(state, codeChallenge)
}

func (p oauthProvider) Exchange(ctx context.Context, code, codeVerifier string) (identity.ProviderIdentity, error) {
	got, err := p.client.Exchange(ctx, code, codeVerifier)
	if err != nil {
		return identity.ProviderIdentity{}, err
	}
	return identity.ProviderIdentity{
		Subject: got.Subject, Email: got.Email, EmailVerified: got.EmailVerified,
	}, nil
}
