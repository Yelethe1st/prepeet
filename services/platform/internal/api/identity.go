package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Identity is what this package needs in order to serve the authentication
// endpoints.
//
// It is declared here, by the consumer, rather than imported from the context
// that implements it. ADR-0005 forbids one bounded context importing another,
// and the module boundary test enforces it, so the alternative is not "import
// identity and be careful" but "do not compile".
//
// The cost is visible and worth naming: the types below duplicate shapes that
// exist in the identity context, and cmd/api translates between them. That
// translation is the price of the two being separable, and it is small because
// this interface is deliberately narrow. It describes what the HTTP layer does,
// not what identity can do.
type Identity interface {
	// Register creates an account, or does nothing, and reports neither. The
	// caller cannot tell a new address from a known one, which is the whole
	// point: confirming it would let anyone enumerate who practises for
	// interviews.
	Register(ctx context.Context, input Registration) error

	// Authenticate exchanges credentials for a session. It returns
	// ErrCredentialsRejected for both a wrong password and an unknown address,
	// and takes comparable time for each.
	Authenticate(ctx context.Context, email, password string) (Session, error)

	// Refresh rotates a session. Presenting a retired token revokes the whole
	// family and returns ErrSessionRejected.
	Refresh(ctx context.Context, refreshToken string) (Session, error)

	// Lookup resolves a session token to who is acting. It returns
	// ErrSessionRejected for a token that is missing, expired, retired or
	// revoked, since none of those is a distinction the caller may act on.
	Lookup(ctx context.Context, sessionToken string) (Principal, error)

	// Revoke ends a session and its family. It is idempotent, because logging
	// out twice is not an error and a browser will do it.
	Revoke(ctx context.Context, sessionToken, reason string) error

	// Describe returns what /me reports about a user.
	Describe(ctx context.Context, userID string) (User, error)

	// DescribeSession returns the same, plus what this session may do.
	//
	// Separate from Describe because the subjects differ: a person has
	// memberships, and a session has authority under the one workspace it is
	// acting in. An endpoint that needs only the person should not pay for a
	// capability lookup, and one that needs authority must not derive it from
	// the person.
	DescribeSession(ctx context.Context, sessionToken, userID string) (User, error)

	// RequestTokenEmail sends one of the four token emails. It reports
	// success for an unknown address, because the response must not say which
	// addresses hold accounts, and a CooldownError when a recent email for
	// the same flow is still fresh.
	RequestTokenEmail(ctx context.Context, kind, email string) error

	// ConfirmEmailVerification consumes a verification token. Replay-safe:
	// a consumed token earns ErrTokenUsed and repeats nothing.
	ConfirmEmailVerification(ctx context.Context, token string) error

	// ConfirmPasswordReset consumes a recovery token, replaces the password
	// and revokes every session in one transaction.
	ConfirmPasswordReset(ctx context.Context, token, password string) error

	// ConsumeMagicLink exchanges a sign-in token for a session.
	ConsumeMagicLink(ctx context.Context, token string) (Session, error)

	// ConsumeOTP exchanges an emailed code for a session. A wrong code and an
	// unknown address answer identically with ErrCodeIncorrect.
	ConsumeOTP(ctx context.Context, email, code string) (Session, error)

	// SelectTenant sets which tenant the session acts under, after verifying
	// the membership. An empty tenantID clears the selection.
	//
	// It returns ErrNoMembership when the person does not belong to that
	// tenant, which is distinct from a rejected session: one means sign in
	// again and the other means that workspace is not yours, and answering the
	// first for the second would sign somebody out for clicking the wrong name.
	SelectTenant(ctx context.Context, sessionToken, tenantID string) (Principal, error)
}

// Registration is a request to create an account.
type Registration struct {
	Email            string
	Password         string
	AccountType      string
	OrganisationName string
}

// String redacts the password. A struct printed with %v is the ordinary way a
// live credential reaches a log, and a registration body is a plausible thing
// to log while debugging a validation failure.
func (r Registration) String() string {
	return fmt.Sprintf("api.Registration{Email:%s Password:[redacted] AccountType:%s OrganisationName:%s}",
		r.Email, r.AccountType, r.OrganisationName)
}

// Session is a newly issued session, with the tokens that go into cookies.
type Session struct {
	UserID          string
	SessionToken    string
	RefreshToken    string
	ExpiresAt       time.Time
	RefreshExpires  time.Time
	AuthenticatedAt time.Time
	// ActiveTenantID is empty for a candidate who belongs to no tenant. Tenant
	// selection lands with IAM-03; until then a session is always untenanted.
	ActiveTenantID string
}

// String redacts both tokens, for the same reason Registration redacts its
// password.
func (s Session) String() string {
	return fmt.Sprintf("api.Session{UserID:%s SessionToken:[redacted] RefreshToken:[redacted] ExpiresAt:%s}",
		s.UserID, s.ExpiresAt.Format(time.RFC3339))
}

// Principal is who a request is acting as.
type Principal struct {
	UserID          string
	SessionID       string
	AuthenticatedAt time.Time
	ActiveTenantID  string
}

// User is what /me reports.
type User struct {
	ID             string
	Email          string
	EmailVerified  bool
	Memberships    []Membership
	ActiveTenantID string
	// Capabilities is what this session may do under its active tenant.
	//
	// A property of the session rather than of the person, because it depends
	// on which workspace they are acting under and changes when they switch.
	Capabilities []string
}

// Membership is one tenant a user belongs to.
type Membership struct {
	TenantID   string
	TenantName string
	Status     string
}

// The failures this layer distinguishes.
//
// Deliberately few. Every reason a credential or session might be refused
// collapses into one error, because the response must not distinguish them
// either: an expired session and a revoked one are the same 401 to the caller,
// and an error type that could tell them apart is an invitation to leak the
// difference in a log line or a message.
var (
	// ErrCredentialsRejected means the credentials did not authenticate, for
	// any reason.
	ErrCredentialsRejected = errors.New("api: those credentials did not authenticate")

	// ErrTokenInvalid through ErrCodeExhausted are the token outcomes. Each
	// is distinct because the prototype gives each its own screen, and they
	// are safe to distinguish: everyone the difference is visible to is
	// already holding the link.
	ErrTokenInvalid    = errors.New("api: that link is not valid")
	ErrTokenExpired    = errors.New("api: that link has expired")
	ErrTokenUsed       = errors.New("api: that link has already been used")
	ErrTokenSuperseded = errors.New("api: a newer link has replaced that one")
	ErrCodeIncorrect   = errors.New("api: that code is not right")
	ErrCodeExhausted   = errors.New("api: too many wrong codes")

	// ErrSessionRejected means the session or refresh token is not usable, for
	// any reason.
	ErrSessionRejected = errors.New("api: that session is not valid")

	// ErrForbidden means the session is fine and the act is not permitted.
	ErrForbidden = errors.New("api: that is not permitted for this session")
)

// FieldError names a request field that failed validation.
//
// Field-level errors exist so a form can put the message next to the input that
// caused it. Code is what the interface branches on; Message is for a person and
// is never parsed.
type FieldError struct {
	Field   string
	Code    string
	Message string
}

// ValidationError carries one or more field errors.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	names := make([]string, 0, len(e.Fields))
	for _, field := range e.Fields {
		names = append(names, field.Field)
	}
	return "api: validation failed on " + strings.Join(names, ", ")
}

// Invalid builds a validation error for one field.
func Invalid(field, code, message string) *ValidationError {
	return &ValidationError{Fields: []FieldError{{Field: field, Code: code, Message: message}}}
}
