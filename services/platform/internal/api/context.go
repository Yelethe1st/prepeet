package api

import (
	"context"
	"net/http"
)

// What travels from the transport into a handler.
//
// oapi-codegen's strict request objects carry the parsed body and parameters
// and nothing else, so a handler cannot reach the raw request. That is usually
// a feature: a handler working from typed input cannot quietly depend on a
// header the contract does not mention.
//
// Cookies are the exception it forces. Session tokens travel in cookies rather
// than in the body precisely so no script can read them, which means the one
// thing every authenticated operation needs is the one thing the generated
// input omits. A strict middleware reads them once and puts them here.

type contextKey int

const (
	credentialsKey contextKey = iota
	principalKey
)

// credentials are the tokens a request presented.
type credentials struct {
	session string
	refresh string
}

// withCredentials reads the session and refresh cookies into ctx.
func withCredentials(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, credentialsKey, credentials{
		session: SessionTokenFrom(r),
		refresh: RefreshTokenFrom(r),
	})
}

// sessionTokenFromContext returns the session token the request presented.
func sessionTokenFromContext(ctx context.Context) string {
	presented, _ := ctx.Value(credentialsKey).(credentials)
	return presented.session
}

// refreshTokenFromContext returns the refresh token the request presented.
func refreshTokenFromContext(ctx context.Context) string {
	presented, _ := ctx.Value(credentialsKey).(credentials)
	return presented.refresh
}

// WithPrincipal records who a request is acting as.
//
// Exported so cmd can wire an alternative resolution later, such as a service
// principal for internal callers, without this package growing a second way to
// establish one.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey, principal)
}

// PrincipalFrom returns who a request is acting as, and whether anyone is.
//
// The second result is false rather than the zero Principal being meaningful,
// because a zero Principal has an empty user id and an empty tenant, and code
// that forgot to check would then act as a user who does not exist under no
// tenant. Under row-level security that reads as "no rows" rather than as an
// error, which is a failure that looks like an empty page.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey).(Principal)
	return principal, ok
}
