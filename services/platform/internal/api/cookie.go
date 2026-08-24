package api

import (
	"net/http"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// Cookie names. The prefix keeps them recognisable in a browser's storage
// inspector, which is where a person looks when something has gone wrong.
const (
	SessionCookieName = "prepeet_session"
	RefreshCookieName = "prepeet_refresh"

	// refreshCookiePath scopes the refresh cookie to the one endpoint that
	// needs it. An ordinary request then never carries it, and a token that is
	// not sent cannot be stolen from a request that had no reason to include
	// it.
	refreshCookiePath = "/api/v1/auth/refresh"
)

// SetSessionCookies writes the session and refresh cookies.
//
// Tokens travel in cookies rather than in a response body so that no script on
// the page can read them. That is the whole reason for the arrangement, and it
// only holds if every flag below is right, which is why each is asserted by
// test rather than trusted to review.
//
// SameSite is Lax rather than Strict. Strict would drop the session cookie on
// any inbound navigation, so a candidate following an invitation link from
// their email would arrive logged out and be asked to sign in again for no
// reason they can see. Lax keeps top-level navigation working while still
// withholding the cookie from cross-site form posts and subresource requests,
// which is where cross-site request forgery lives.
func SetSessionCookies(w http.ResponseWriter, environment config.Environment,
	sessionToken, refreshToken string, sessionExpires, refreshExpires time.Time,
) {
	secure := environment != config.EnvironmentLocal

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		Expires:  sessionExpires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    refreshToken,
		Path:     refreshCookiePath,
		Expires:  refreshExpires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookies removes both cookies from the browser.
//
// Revoking the session server side is what actually ends it. Clearing the
// cookies is so the next request does not present a token the server has
// already refused, which would show the person a confusing failure instead of
// a clean logout.
//
// The paths must match the ones the cookies were set with, or the browser
// treats these as different cookies and leaves the originals in place.
func ClearSessionCookies(w http.ResponseWriter, environment config.Environment) {
	secure := environment != config.EnvironmentLocal

	for _, cookie := range []struct{ name, path string }{
		{SessionCookieName, "/"},
		{RefreshCookieName, refreshCookiePath},
	} {
		http.SetCookie(w, &http.Cookie{
			Name:     cookie.name,
			Value:    "",
			Path:     cookie.path,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// SessionTokenFrom reads the session token a request presented, if any.
func SessionTokenFrom(r *http.Request) string {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// RefreshTokenFrom reads the refresh token a request presented, if any.
func RefreshTokenFrom(r *http.Request) string {
	cookie, err := r.Cookie(RefreshCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
