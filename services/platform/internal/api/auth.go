package api

import (
	"context"
	"errors"
	"fmt"
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
}

// Register creates an account, and says the same thing either way.
func (a *authentication) Register(ctx context.Context, request prepeetapi.RegisterRequestObject) (prepeetapi.RegisterResponseObject, error) {
	body := request.Body

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

	switch {
	case errors.Is(err, ErrDocumentMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no document at that identifier."
		return base
	case errors.Is(err, ErrDocumentConflict):
		base.status = http.StatusConflict
		base.code = string(prepeetapi.FORBIDDEN)
		base.message = "That document is not in a state this operation applies to."
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
