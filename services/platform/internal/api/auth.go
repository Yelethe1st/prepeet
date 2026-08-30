package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"go.opentelemetry.io/otel/trace"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/httpserver"
	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// authentication serves the authentication half of the contract.
//
// It holds no rules of its own. Every decision about whether a credential is
// good, whether a session is live, or what a refused token means belongs to the
// identity context; this translates between that and HTTP. When a handler here
// starts making a decision, the decision is in the wrong place.
type authentication struct {
	identity    Identity
	environment config.Environment
	// limits counts attempts at the endpoints an attacker gets unlimited
	// tries at (SEC-10).
	limits limits
}

// Register creates an account, and says the same thing either way.
func (a *authentication) Register(ctx context.Context, request prepeetapi.RegisterRequestObject) (prepeetapi.RegisterResponseObject, error) {
	body := request.Body
	if err := a.limits.check(ctx, "register", string(body.Email), networkFromContext(ctx)); err != nil {
		return a.failed(ctx, err), nil
	}

	organisation := ""
	if body.OrganisationName != nil {
		organisation = *body.OrganisationName
	}

	err := a.identity.Register(ctx, Registration{
		Email:            string(body.Email),
		Password:         body.Password,
		AccountType:      string(body.AccountType),
		OrganisationName: organisation,
	})
	if err != nil {
		return a.failed(ctx, err), nil
	}

	// The same 202 whether an account was created or the address was already
	// known. The identity service does not report which, so this handler could
	// not distinguish them even if it wanted to, which is the point: the
	// property is structural rather than a discipline kept here.
	return prepeetapi.Register202JSONResponse{
		Body:    prepeetapi.RegistrationAccepted{Status: prepeetapi.VerificationSent},
		Headers: prepeetapi.Register202ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// Login exchanges credentials for a session.
func (a *authentication) Login(ctx context.Context, request prepeetapi.LoginRequestObject) (prepeetapi.LoginResponseObject, error) {
	// Counted before the work: a refused attempt must not cost the
	// argon2id hash that makes this endpoint worth attacking.
	if err := a.limits.check(ctx, "login", string(request.Body.Email), networkFromContext(ctx)); err != nil {
		return a.failed(ctx, err), nil
	}

	session, err := a.identity.Authenticate(ctx, string(request.Body.Email), request.Body.Password)
	if err != nil {
		return a.failed(ctx, err), nil
	}

	issued, err := a.issued(session)
	if err != nil {
		return a.failed(ctx, err), nil
	}
	return issued, nil
}

// Refresh rotates the session.
func (a *authentication) Refresh(ctx context.Context, request prepeetapi.RefreshRequestObject) (prepeetapi.RefreshResponseObject, error) {
	presented := refreshTokenFromContext(ctx)
	if presented == "" {
		// Refused without reaching the service. A request carrying no refresh
		// token cannot be refreshed whatever the service would say, and asking
		// it would mean one database round trip per request from any client
		// that has lost its cookie.
		return a.rejectedSession(ctx), nil
	}

	session, err := a.identity.Refresh(ctx, presented)
	if err != nil {
		return a.failed(ctx, err), nil
	}

	issued, err := a.issued(session)
	if err != nil {
		return a.failed(ctx, err), nil
	}
	return issued, nil
}

// Logout ends the session and clears the cookies.
//
// Idempotent, including when the session is already gone. A browser will send
// this twice, and a second logout reporting a failure would be a failure of
// nothing.
func (a *authentication) Logout(ctx context.Context, request prepeetapi.LogoutRequestObject) (prepeetapi.LogoutResponseObject, error) {
	if presented := sessionTokenFromContext(ctx); presented != "" {
		// The outcome is deliberately not checked against ErrSessionRejected:
		// a session that is already revoked is a successful logout. An
		// unexpected failure is logged by the service and still answered with
		// 204, because the cookies are cleared either way and telling the
		// person their logout failed would invite them to try again with a
		// token that no longer works.
		if err := a.identity.Revoke(ctx, presented, "logout"); err != nil && !errors.Is(err, ErrSessionRejected) {
			a.record(ctx, err)
		}
	}
	return sessionCleared{environment: a.environment}, nil
}

// GetCurrentUser describes who is acting.
func (a *authentication) GetCurrentUser(ctx context.Context, _ prepeetapi.GetCurrentUserRequestObject) (prepeetapi.GetCurrentUserResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		// Refused without reaching the service, for the same reason refresh is:
		// a request with no session cookie cannot be authenticated whatever the
		// service would say, and asking anyway is a database round trip per
		// unauthenticated request, which is a denial of service anyone can run.
		return a.rejectedSession(ctx), nil
	}

	principal, err := a.identity.Lookup(ctx, presented)
	if err != nil {
		return a.failed(ctx, err), nil
	}

	user, err := a.identity.DescribeSession(ctx, presented, principal.UserID)
	if err != nil {
		return a.failed(ctx, err), nil
	}

	// From the session, not from the person's record, which never carries one.
	// Reading it from the record left this null after a successful selection
	// while the capabilities in the same response were the workspace ones:
	// authority without any indication of which workspace it applied to.
	user.ActiveTenantID = principal.ActiveTenantID

	body, err := currentUserBody(user)
	if err != nil {
		return a.failed(ctx, err), nil
	}
	return prepeetapi.GetCurrentUser200JSONResponse{
		Body:    body,
		Headers: prepeetapi.GetCurrentUser200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ListMemberships answers which workspaces this person belongs to.
func (a *authentication) ListMemberships(ctx context.Context, _ prepeetapi.ListMembershipsRequestObject) (prepeetapi.ListMembershipsResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return a.rejectedSession(ctx), nil
	}

	principal, err := a.identity.Lookup(ctx, presented)
	if err != nil {
		return a.failed(ctx, err), nil
	}

	user, err := a.identity.Describe(ctx, principal.UserID)
	if err != nil {
		return a.failed(ctx, err), nil
	}

	body, err := currentUserBody(user)
	if err != nil {
		return a.failed(ctx, err), nil
	}

	return prepeetapi.ListMemberships200JSONResponse{
		Body:    prepeetapi.MembershipList{Memberships: body.Memberships},
		Headers: prepeetapi.ListMemberships200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// SetActiveTenant chooses which workspace the session acts under.
func (a *authentication) SetActiveTenant(ctx context.Context, request prepeetapi.SetActiveTenantRequestObject) (prepeetapi.SetActiveTenantResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return a.rejectedSession(ctx), nil
	}

	// A null tenant clears the selection, which is how somebody leaves a
	// workspace without signing out.
	tenantID := ""
	if request.Body.TenantID != nil {
		tenantID = request.Body.TenantID.String()
	}

	principal, err := a.identity.SelectTenant(ctx, presented, tenantID)
	if err != nil {
		return a.failed(ctx, err), nil
	}

	body := prepeetapi.Session{
		UserID:          mustUUID(principal.UserID),
		AuthenticatedAt: principal.AuthenticatedAt,
	}
	if principal.ActiveTenantID != "" {
		active, err := parseUUID(principal.ActiveTenantID, "active tenant")
		if err != nil {
			return a.failed(ctx, err), nil
		}
		body.ActiveTenantID = &active
	}

	return prepeetapi.SetActiveTenant200JSONResponse{
		Body:    body,
		Headers: prepeetapi.SetActiveTenant200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// currentUserBody converts what identity reports into the contract shape.
//
// Separate from the handler so the identifier parsing, which is the only part
// that can fail, is not interleaved with the HTTP decisions.
func currentUserBody(user User) (prepeetapi.CurrentUser, error) {
	id, err := parseUUID(user.ID, "user")
	if err != nil {
		return prepeetapi.CurrentUser{}, err
	}

	// An empty slice rather than nil, so the field serialises as [] and no
	// client has to handle two shapes for "belongs to no tenant".
	memberships := make([]prepeetapi.Membership, 0, len(user.Memberships))
	for _, membership := range user.Memberships {
		tenantID, err := parseUUID(membership.TenantID, "tenant")
		if err != nil {
			return prepeetapi.CurrentUser{}, err
		}
		memberships = append(memberships, prepeetapi.Membership{
			TenantID:   tenantID,
			TenantName: membership.TenantName,
			Status:     prepeetapi.MembershipStatus(membership.Status),
		})
	}

	// Empty rather than nil, so the field serialises as [] and no client has to
	// handle two shapes for "may do nothing here".
	capabilities := user.Capabilities
	if capabilities == nil {
		capabilities = []string{}
	}

	body := prepeetapi.CurrentUser{
		UserID:        id,
		EmailVerified: user.EmailVerified,
		Memberships:   memberships,
		Capabilities:  capabilities,
	}
	if user.Email != "" {
		email := openapi_types.Email(user.Email)
		body.Email = &email
	}
	if user.ActiveTenantID != "" {
		active, err := parseUUID(user.ActiveTenantID, "active tenant")
		if err != nil {
			return prepeetapi.CurrentUser{}, err
		}
		body.ActiveTenantID = &active
	}
	return body, nil
}

// mustUUID is parseUUID where a failure has already been ruled out.
//
// Used only where the identifier came from a row this process just read, so a
// parse failure is corrupt data rather than input. It returns the zero value
// rather than panicking: one malformed row should spoil one response, not the
// process.
func mustUUID(value string) openapi_types.UUID {
	parsed, _ := parseUUID(value, "identifier")
	return parsed
}

// parseUUID converts an identifier this system produced.
//
// A failure here is a bug or corrupt data rather than bad input, since these
// values come from our own database. It is returned as an error rather than
// panicking: a malformed row should fail one request, not the process.
func parseUUID(value, what string) (openapi_types.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return openapi_types.UUID{}, fmt.Errorf("api: %s identifier is not a uuid: %w", what, err)
	}
	return parsed, nil
}

// issued builds the shared 200 for login and refresh.
func (a *authentication) issued(session Session) (sessionIssued, error) {
	id, err := parseUUID(session.UserID, "user")
	if err != nil {
		return sessionIssued{}, err
	}

	body := prepeetapi.Session{
		UserID:          id,
		ExpiresAt:       session.ExpiresAt,
		AuthenticatedAt: session.AuthenticatedAt,
	}
	if session.ActiveTenantID != "" {
		active, err := parseUUID(session.ActiveTenantID, "active tenant")
		if err != nil {
			return sessionIssued{}, err
		}
		body.ActiveTenantID = &active
	}

	return sessionIssued{body: body, environment: a.environment, session: session}, nil
}

// failed turns a service error into the right response.
//
// One place, so every operation answers the same way for the same cause. The
// mapping is the security-relevant part of this file: a validation failure says
// which field, a refused credential says nothing at all, and anything
// unrecognised becomes a 500 whose message is ours rather than the error's.
func (a *authentication) failed(ctx context.Context, err error) failure {
	base := failure{environment: a.environment, requestID: httpserver.RequestIDFrom(ctx)}

	if status, code, message, ok := tokenFailure(err); ok {
		base.status = status
		base.code = code
		base.message = message
		return base
	}

	// Start's refusals each carry their own stable code; the person's next
	// action differs per code, which is the whole reason they are distinct.
	var startRefused *StartRefusedError
	if errors.As(err, &startRefused) {
		base.status = http.StatusConflict
		base.code = startRefused.Code
		base.message = startRefused.Message
		return base
	}

	switch {
	case errors.Is(err, ErrDocumentMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no document at that identifier."
		return base
	case errors.Is(err, ErrFactMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no fact at that identifier."
		return base
	case errors.Is(err, ErrOAuthProviderUnknown):
		base.status = http.StatusNotFound
		base.code = "OAUTH_PROVIDER_UNKNOWN"
		base.message = "That sign-in provider is not available here."
		return base
	case errors.Is(err, ErrOAuthStateRejected):
		base.status = http.StatusConflict
		base.code = "OAUTH_STATE_INVALID"
		base.message = "That sign-in could not be completed. Start again from the sign-in page."
		return base
	case errors.Is(err, ErrOAuthStateExpired):
		base.status = http.StatusConflict
		base.code = "OAUTH_STATE_EXPIRED"
		base.message = "That sign-in took too long. Start again and it will work."
		return base
	case errors.Is(err, ErrOAuthAddressUnverified):
		base.status = http.StatusConflict
		base.code = "OAUTH_EMAIL_UNVERIFIED"
		// Names what to do, and does not confirm that an account exists: it
		// says what would be true either way.
		base.message = "That provider has not verified the address, so it cannot be used to sign in here. " +
			"Sign in with your email and password."
		return base
	case errors.Is(err, ErrFeedbackUnknownInsight):
		base.status = http.StatusBadRequest
		base.code = "FEEDBACK_INSIGHT_UNKNOWN"
		base.message = "That is not one of the insights in this session's analysis."
		return base
	case errors.Is(err, ErrFeedbackPracticeOnly):
		base.status = http.StatusConflict
		base.code = "FEEDBACK_PRACTICE_ONLY"
		base.message = "Feedback on coaching is part of practice. A screening interview does not carry it."
		return base
	case errors.Is(err, ErrFeedbackMissingBody):
		base.status = http.StatusBadRequest
		base.code = string(prepeetapi.VALIDATIONFAILED)
		base.message = "Say which insight this is about and whether it described you."
		return base
	case errors.Is(err, ErrDeliveryOmitted):
		base.status = http.StatusConflict
		base.code = "DELIVERY_OMITTED"
		base.message = "Delivery measurement was not run for this session. " +
			"That is a decision about the session, not a result about you."
		return base
	case errors.Is(err, ErrDeliveryFailed):
		base.status = http.StatusConflict
		base.code = "DELIVERY_FAILED"
		base.message = "Delivery measurement could not be produced for this session. " +
			"Nothing about your evaluation is affected."
		return base
	case errors.Is(err, ErrDeliveryNotReady):
		base.status = http.StatusConflict
		base.code = "DELIVERY_NOT_READY"
		base.message = "The delivery analysis has not finished for this session. Ask again shortly."
		return base
	case errors.Is(err, ErrResultNotReady):
		base.status = http.StatusConflict
		base.code = "RESULT_NOT_READY"
		base.message = "Evaluation has not finished for this session. Ask again shortly."
		return base
	case errors.Is(err, ErrSessionMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no session at that identifier."
		return base
	case errors.Is(err, ErrMemberMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no membership at that identifier."
		return base
	case errors.Is(err, ErrMemberConflict):
		base.status = http.StatusConflict
		base.code = string(prepeetapi.FORBIDDEN)
		base.message = "That membership is not in a state this operation applies to, or changed since it was read."
		return base
	case errors.Is(err, ErrDocumentConflict):
		base.status = http.StatusConflict
		base.code = string(prepeetapi.FORBIDDEN)
		base.message = "That document is not in a state this operation applies to."
		return base
	}

	var limited *RateLimitedError
	if errors.As(err, &limited) {
		base.status = http.StatusTooManyRequests
		base.code = string(prepeetapi.RATELIMITED)
		base.message = fmt.Sprintf(
			"Too many attempts. Try again in %d seconds.", int(math.Ceil(limited.RetryAfter.Seconds())))
		base.retryAfter = limited.RetryAfter
		return base
	}

	var cooldown *CooldownError
	if errors.As(err, &cooldown) {
		base.status = http.StatusTooManyRequests
		base.code = string(prepeetapi.RESENDCOOLINGDOWN)
		base.message = fmt.Sprintf("Another email was sent moments ago. You can request a new one in %d seconds.",
			int(cooldown.RetryAfter.Seconds()))
		base.retryAfter = cooldown.RetryAfter
		return base
	}

	var invalid *ValidationError
	switch {
	case errors.As(err, &invalid):
		base.status = http.StatusBadRequest
		base.code = string(prepeetapi.VALIDATIONFAILED)
		base.message = "Some of the details were not accepted."
		base.fields = invalid.Fields

	case errors.Is(err, ErrCredentialsRejected):
		// No detail, deliberately. A message distinguishing an unknown address
		// from a wrong password would undo the dummy verification that makes
		// the two take the same time.
		base.status = http.StatusUnauthorized
		base.code = string(prepeetapi.UNAUTHENTICATED)
		base.message = "Those details did not sign you in."

	case errors.Is(err, ErrForbidden):
		// Not 404. The workspace may well exist, and answering "no such thing"
		// to somebody who cannot use it would be a way to test which
		// identifiers are real. Not 401 either: the session is fine, and
		// answering that would sign somebody out for clicking a workspace that
		// is not theirs.
		base.status = http.StatusForbidden
		base.code = string(prepeetapi.FORBIDDEN)
		base.message = "You do not have access to that workspace."

	case errors.Is(err, ErrSessionRejected):
		base.status = http.StatusUnauthorized
		base.code = string(prepeetapi.UNAUTHENTICATED)
		base.message = "Please sign in again."
		base.clearCookies = true

	default:
		// The error's own text never reaches the client. A driver error carries
		// a connection string and a provider error can carry a prompt, and this
		// is the boundary where either would leave the system.
		a.record(ctx, err)
		base.status = http.StatusInternalServerError
		base.code = string(prepeetapi.INTERNAL)
		base.message = "Something went wrong on our side. Please try again."
		base.retryable = true
	}
	return base
}

// rejectedSession is the answer to a request whose session is missing or dead.
func (a *authentication) rejectedSession(ctx context.Context) failure {
	return a.failed(ctx, ErrSessionRejected)
}

// record attaches an unexpected failure to the active span.
//
// Scrubbed, because this is the same error text that was just kept out of the
// response, and a span is read by anyone with dashboard access.
func (a *authentication) record(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(errors.New(telemetry.Scrub(err.Error())))
	span.SetAttributes(telemetry.MustAttr(telemetry.KeyOutcome, "internal_error"))
}

// oauthLabels are what the sign-in screen calls each provider.
//
// Here rather than in identity, because identity knows a provider by the key
// it was configured under and the label is a presentation concern. A provider
// configured without a label here is shown by its key, which is ugly and
// working rather than absent.
var oauthLabels = map[string]string{
	"google":    "Google",
	"microsoft": "Microsoft",
}

// ListOAuthProviders answers which providers this deployment offers.
func (a *authentication) ListOAuthProviders(_ context.Context, _ prepeetapi.ListOAuthProvidersRequestObject) (prepeetapi.ListOAuthProvidersResponseObject, error) {
	configured := a.identity.ConfiguredOAuthProviders()
	providers := make([]struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}, 0, len(configured))
	for _, id := range configured {
		label, known := oauthLabels[id]
		if !known {
			label = id
		}
		providers = append(providers, struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		}{ID: id, Label: label})
	}
	return prepeetapi.ListOAuthProviders200JSONResponse{
		Body:    prepeetapi.OAuthProviders{Providers: providers},
		Headers: prepeetapi.ListOAuthProviders200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// StartOAuth begins the round trip and answers where to send the browser.
func (a *authentication) StartOAuth(ctx context.Context, request prepeetapi.StartOAuthRequestObject) (prepeetapi.StartOAuthResponseObject, error) {
	// Counted by network alone. Minting state is cheap, but an uncounted
	// endpoint that writes a row per call is a way to fill a table.
	//
	// The subject is empty on purpose. There is no address yet: nobody has
	// said who they are, and keying on the provider would put every person
	// signing in with Google into one bucket, so one attacker could lock
	// everybody out of it.
	if err := a.limits.check(ctx, "oauth_start", "", networkFromContext(ctx)); err != nil {
		return a.failed(ctx, err), nil
	}

	redirectTo := ""
	if request.Body != nil && request.Body.RedirectTo != nil {
		redirectTo = *request.Body.RedirectTo
	}

	start, err := a.identity.BeginOAuth(ctx, request.Provider, redirectTo)
	if err != nil {
		return a.failed(ctx, err), nil
	}
	return prepeetapi.StartOAuth200JSONResponse{
		Body: prepeetapi.OAuthStart{
			AuthorizationURL: start.AuthorizationURL,
			State:            start.State,
		},
		Headers: prepeetapi.StartOAuth200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// CompleteOAuth finishes the round trip and issues the session.
//
// It ends at a.issued, which is where Login and Refresh end: one place writes
// the cookies, so an OAuth session is indistinguishable from a password one
// to everything downstream, including logout and revocation.
func (a *authentication) CompleteOAuth(ctx context.Context, request prepeetapi.CompleteOAuthRequestObject) (prepeetapi.CompleteOAuthResponseObject, error) {
	// By network, for the reason StartOAuth gives.
	if err := a.limits.check(ctx, "oauth_callback", "", networkFromContext(ctx)); err != nil {
		return a.failed(ctx, err), nil
	}
	if request.Body == nil {
		return a.failed(ctx, ErrOAuthStateRejected), nil
	}

	session, redirectTo, err := a.identity.CompleteOAuth(ctx, request.Provider,
		request.Body.State, request.Body.Code)
	if err != nil {
		return a.failed(ctx, err), nil
	}

	issued, err := a.issued(session)
	if err != nil {
		return a.failed(ctx, err), nil
	}
	// The destination is carried back rather than discarded. It was validated
	// when it was stored, at the start of the round trip, because an open
	// redirect stored is an open redirect; here it is only handed on.
	return oauthSessionIssued{sessionIssued: issued, redirectTo: redirectTo}, nil
}
