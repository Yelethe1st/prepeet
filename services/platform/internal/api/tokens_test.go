package api_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
)

// The token endpoints at the HTTP boundary.
//
// The service tests prove the flows; these prove the boundary's promises: the
// request half answers 202 whatever it knows, the cooldown arrives as a 429
// with a countdown the interface can show, and every consume outcome keeps
// its own code, because the prototype gives each its own screen and a
// collapsed code would collapse the screens.

func TestRequestingATokenEmailAnswers202(t *testing.T) {
	identity := &fakeIdentity{}
	handler := serve(t, identity)

	response := post(t, handler, "/api/v1/auth/email/request",
		`{"kind":"password_reset","email":"amara.eze@example.com"}`)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", response.Code, response.Body)
	}
	if len(identity.tokenRequests) != 1 || identity.tokenRequests[0] != "password_reset:amara.eze@example.com" {
		t.Fatalf("the service saw %v", identity.tokenRequests)
	}
}

func TestTheCooldownIsVisible(t *testing.T) {
	identity := &fakeIdentity{tokenRequestErr: &api.CooldownError{RetryAfter: 42 * time.Second}}
	handler := serve(t, identity)

	response := post(t, handler, "/api/v1/auth/email/request",
		`{"kind":"verify_email","email":"amara.eze@example.com"}`)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: %s", response.Code, response.Body)
	}
	// The header is what a generic client backs off on; the body's code is
	// what the interface builds its countdown from. Both, or one audience is
	// left guessing.
	if got := response.Header().Get("Retry-After"); got != "42" {
		t.Errorf("Retry-After = %q, want 42", got)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeInto(t, response, &body)
	if body.Error.Code != "RESEND_COOLING_DOWN" {
		t.Errorf("code = %q", body.Error.Code)
	}
}

func TestEveryTokenOutcomeKeepsItsOwnCode(t *testing.T) {
	// The ticket's third criterion at the boundary: expired and already-used
	// are their own outcomes, not one generic failure.
	cases := map[string]struct {
		err  error
		code string
	}{
		"invalid":    {api.ErrTokenInvalid, "TOKEN_INVALID"},
		"expired":    {api.ErrTokenExpired, "TOKEN_EXPIRED"},
		"used":       {api.ErrTokenUsed, "TOKEN_USED"},
		"superseded": {api.ErrTokenSuperseded, "TOKEN_SUPERSEDED"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			identity := &fakeIdentity{confirmErr: tc.err}
			handler := serve(t, identity)

			response := post(t, handler, "/api/v1/auth/email/verify",
				`{"token":"vrf_sometoken"}`)

			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422: %s", response.Code, response.Body)
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			decodeInto(t, response, &body)
			if body.Error.Code != tc.code {
				t.Errorf("code = %q, want %q", body.Error.Code, tc.code)
			}
		})
	}
}

func TestAConsumedVerificationAnswers204(t *testing.T) {
	identity := &fakeIdentity{}
	handler := serve(t, identity)

	response := post(t, handler, "/api/v1/auth/email/verify", `{"token":"vrf_goodtoken"}`)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", response.Code, response.Body)
	}
}

func TestAMagicLinkSetsTheSameCookiesLoginDoes(t *testing.T) {
	identity := &fakeIdentity{session: api.Session{
		UserID:          "0199023d-2f3b-7c30-b7ac-1f00b3d6ae5e",
		SessionToken:    "ses_secret",
		RefreshToken:    "ref_secret",
		ExpiresAt:       time.Now().Add(time.Hour),
		RefreshExpires:  time.Now().Add(24 * time.Hour),
		AuthenticatedAt: time.Now(),
	}}
	handler := serve(t, identity)

	response := post(t, handler, "/api/v1/auth/magic/consume", `{"token":"mgc_goodtoken"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	cookies := cookiesOf(t, response)
	for _, name := range []string{"prepeet_session", "prepeet_refresh"} {
		cookie, ok := cookies[name]
		if !ok {
			t.Fatalf("no %s cookie; a magic-link session that lives somewhere else is a second session shape", name)
		}
		if !cookie.HttpOnly {
			t.Errorf("%s is readable from script", name)
		}
	}
}

func TestAWrongOTPKeepsItsCode(t *testing.T) {
	identity := &fakeIdentity{confirmErr: api.ErrCodeIncorrect}
	handler := serve(t, identity)

	response := post(t, handler, "/api/v1/auth/otp/consume",
		`{"email":"amara.eze@example.com","code":"123456"}`)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeInto(t, response, &body)
	if body.Error.Code != "CODE_INCORRECT" {
		t.Errorf("code = %q", body.Error.Code)
	}
}

func TestAnExhaustedOTPSaysToRequestANewOne(t *testing.T) {
	identity := &fakeIdentity{confirmErr: api.ErrCodeExhausted}
	handler := serve(t, identity)

	response := post(t, handler, "/api/v1/auth/otp/consume",
		`{"email":"amara.eze@example.com","code":"123456"}`)

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeInto(t, response, &body)
	if body.Error.Code != "CODE_ATTEMPTS_EXHAUSTED" {
		t.Errorf("code = %q", body.Error.Code)
	}
}

// The one wiring mistake the enum cannot prevent.
func TestAnUnknownKindIsAValidationFailureNotAPanic(t *testing.T) {
	identity := &fakeIdentity{}
	handler := serve(t, identity)

	response := post(t, handler, "/api/v1/auth/email/request",
		`{"kind":"carrier_pigeon","email":"amara.eze@example.com"}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s\nfake saw: %v", response.Code, response.Body, identity.tokenRequests)
	}
}

// Guard against the fake drifting from the port.
var _ api.Identity = (*fakeIdentity)(nil)

// Silence unused-import when cases shrink.
var _ = errors.Is
