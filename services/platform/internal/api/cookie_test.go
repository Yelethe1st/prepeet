package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// Session tokens live in cookies rather than in a response body, so no script
// on the page can read them. That only holds if the flags are right.
func TestSessionCookiesAreNotReadableByScript(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	api.SetSessionCookies(rec, config.EnvironmentProduction, "ses_token", "ref_token",
		time.Now().Add(time.Hour), time.Now().Add(24*time.Hour))

	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("set %d cookies, want 2", len(cookies))
	}

	for _, cookie := range cookies {
		if !cookie.HttpOnly {
			t.Errorf("%s is not HttpOnly, so a script on the page can read the token", cookie.Name)
		}
		if !cookie.Secure {
			t.Errorf("%s is not Secure, so it would travel over plain HTTP", cookie.Name)
		}
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("%s has SameSite %v, want Lax", cookie.Name, cookie.SameSite)
		}
	}

	// Path is asserted per cookie rather than uniformly: the refresh cookie is
	// deliberately narrower, which TestRefreshCookieIsScopedToTheRefreshEndpoint
	// covers.
	for _, cookie := range cookies {
		if cookie.Name == api.SessionCookieName && cookie.Path != "/" {
			t.Errorf("the session cookie has path %q, want / so it travels with every request", cookie.Path)
		}
	}
}

// Local development runs over plain HTTP. A Secure cookie would simply not be
// stored, so nobody could log in, and the usual fix is someone disabling the
// flag everywhere.
func TestSecureFlagIsRelaxedOnlyForLocalDevelopment(t *testing.T) {
	t.Parallel()

	for environment, wantSecure := range map[config.Environment]bool{
		config.EnvironmentLocal:      false,
		config.EnvironmentPreview:    true,
		config.EnvironmentStaging:    true,
		config.EnvironmentProduction: true,
	} {
		rec := httptest.NewRecorder()
		api.SetSessionCookies(rec, environment, "ses_token", "ref_token",
			time.Now().Add(time.Hour), time.Now().Add(24*time.Hour))

		for _, cookie := range rec.Result().Cookies() {
			if cookie.Secure != wantSecure {
				t.Errorf("%s: %s Secure = %v, want %v", environment, cookie.Name, cookie.Secure, wantSecure)
			}
		}
	}
}

// The refresh cookie is scoped to the refresh endpoint, so an ordinary request
// never carries it. A token that is not sent cannot be stolen from a request
// that had no reason to include it.
func TestRefreshCookieIsScopedToTheRefreshEndpoint(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	api.SetSessionCookies(rec, config.EnvironmentProduction, "ses_token", "ref_token",
		time.Now().Add(time.Hour), time.Now().Add(24*time.Hour))

	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name != api.RefreshCookieName {
			continue
		}
		if !strings.Contains(cookie.Path, "refresh") {
			t.Errorf("the refresh cookie path is %q, want it scoped to the refresh endpoint", cookie.Path)
		}
	}
}

// Logging out has to remove both cookies from the browser, or the next request
// presents a token the server has already revoked and the person sees a
// confusing failure rather than a clean logout.
func TestClearingCookiesExpiresBoth(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	api.ClearSessionCookies(rec, config.EnvironmentProduction)

	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cleared %d cookies, want 2", len(cookies))
	}
	for _, cookie := range cookies {
		if cookie.MaxAge >= 0 {
			t.Errorf("%s has MaxAge %d, want a negative value to delete it", cookie.Name, cookie.MaxAge)
		}
		if cookie.Value != "" {
			t.Errorf("%s still carries a value: %q", cookie.Name, cookie.Value)
		}
	}
}

// A token in a Set-Cookie header is unavoidable; a token anywhere else is not.
func TestTokensNeverAppearOutsideTheCookieHeader(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	api.SetSessionCookies(rec, config.EnvironmentProduction, "ses_secret_value", "ref_secret_value",
		time.Now().Add(time.Hour), time.Now().Add(24*time.Hour))

	for name, values := range rec.Header() {
		if name == "Set-Cookie" {
			continue
		}
		for _, value := range values {
			if strings.Contains(value, "secret_value") {
				t.Errorf("header %s carries a token: %q", name, value)
			}
		}
	}
	if strings.Contains(rec.Body.String(), "secret_value") {
		t.Error("a token appears in the response body")
	}
}

func TestReadingTokensFromARequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: "ses_value"})
	req.AddCookie(&http.Cookie{Name: api.RefreshCookieName, Value: "ref_value"})

	if got := api.SessionTokenFrom(req); got != "ses_value" {
		t.Errorf("SessionTokenFrom = %q, want %q", got, "ses_value")
	}
	if got := api.RefreshTokenFrom(req); got != "ref_value" {
		t.Errorf("RefreshTokenFrom = %q, want %q", got, "ref_value")
	}
}

// A request with no cookie is the ordinary case for an anonymous visitor, not
// an error. Returning empty lets the caller decide, which is the only place
// that knows whether anonymous is allowed.
func TestReadingTokensWhenThereAreNone(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if got := api.SessionTokenFrom(req); got != "" {
		t.Errorf("SessionTokenFrom = %q, want empty", got)
	}
	if got := api.RefreshTokenFrom(req); got != "" {
		t.Errorf("RefreshTokenFrom = %q, want empty", got)
	}
}

// The refresh cookie is scoped to one path, so an ordinary request carries only
// the session cookie. Reading the refresh token there must come back empty
// rather than falling back to anything else.
func TestTheSessionCookieIsNotMistakenForTheRefreshCookie(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: "ses_value"})

	if got := api.RefreshTokenFrom(req); got != "" {
		t.Errorf("RefreshTokenFrom = %q, want empty when only a session cookie is present", got)
	}
}
