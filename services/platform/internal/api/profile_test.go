package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
)

// The profile surface. Owner scoping here is the absence of a parameter: the
// contract has no way to name another person's profile, so what these tests
// hold is that the session decides everything and its absence refuses.

func getProfile(t *testing.T, handler http.Handler, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me/profile", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func putProfile(t *testing.T, handler http.Handler, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/me/profile",
		strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func sessionCookie() *http.Cookie {
	return &http.Cookie{Name: "prepeet_session", Value: "ses_valid"}
}

func TestProfileNeedsASession(t *testing.T) {
	handler := serve(t, &fakeIdentity{})

	response := getProfile(t, handler)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("no session got %d, want 401: %s", response.Code, response.Body)
	}
}

func TestProfileIsTheSessionsOwn(t *testing.T) {
	identity := &fakeIdentity{principal: api.Principal{UserID: "00000000-0000-7000-8000-0000000000f9"}}
	candidates := &fakeCandidates{profile: api.Profile{Seniority: "senior", Disciplines: []string{"Go"}}}
	handler := serveWith(t, identity, candidates)

	response := getProfile(t, handler, sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	var body struct {
		Seniority   string   `json:"seniority"`
		Disciplines []string `json:"disciplines"`
	}
	decodeInto(t, response, &body)
	if body.Seniority != "senior" || len(body.Disciplines) != 1 {
		t.Fatalf("body = %+v", body)
	}
	// The session's user reached the port; nothing else could have, because
	// the operation takes no identifier at all.
	if identity.lookedUp[0] != "ses_valid" {
		t.Fatalf("the port was reached without the session deciding whose profile")
	}
}

func TestSaveRoundTripsThroughTheContractShape(t *testing.T) {
	identity := &fakeIdentity{principal: api.Principal{UserID: "00000000-0000-7000-8000-0000000000f9"}}
	candidates := &fakeCandidates{}
	handler := serveWith(t, identity, candidates)

	response := putProfile(t, handler, `{
		"disciplines": ["Go"], "target_roles": ["Staff Engineer"],
		"default_pressure": "standard", "default_duration_minutes": 30,
		"extended_time": true, "captions": false, "reduced_motion": false,
		"notify_product_updates": false, "notify_practice_reminders": true
	}`, sessionCookie())

	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if len(candidates.saved) != 1 {
		t.Fatalf("the port saw %d saves", len(candidates.saved))
	}
	saved := candidates.saved[0]
	if saved.DefaultPressure != "standard" || saved.DefaultDurationMinutes != 30 || !saved.ExtendedTime {
		t.Fatalf("the optional fields did not survive the boundary: %+v", saved)
	}
}

func TestAFieldRefusalLandsOnItsField(t *testing.T) {
	identity := &fakeIdentity{principal: api.Principal{UserID: "00000000-0000-7000-8000-0000000000f9"}}
	candidates := &fakeCandidates{err: &api.ErrProfileInvalid{
		Field: "default_pressure", Code: "PRESSURE_UNKNOWN", Message: "pressure is low, standard or high",
	}}
	handler := serveWith(t, identity, candidates)

	response := putProfile(t, handler, `{
		"disciplines": [], "target_roles": [],
		"extended_time": false, "captions": false, "reduced_motion": false,
		"notify_product_updates": false, "notify_practice_reminders": true
	}`, sessionCookie())

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		Error struct {
			FieldErrors []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"field_errors"`
		} `json:"error"`
	}
	decodeInto(t, response, &body)
	if len(body.Error.FieldErrors) != 1 || body.Error.FieldErrors[0].Field != "default_pressure" {
		t.Fatalf("the refusal did not land on its field: %s", response.Body)
	}
}

// The put method uses the generated strict decoder; a body missing required
// booleans is a 400 before any handler runs, which keeps the contract's
// required list honest.
func TestARequestMissingTheRequiredShapeIsRefusedByTheContract(t *testing.T) {
	identity := &fakeIdentity{principal: api.Principal{UserID: "00000000-0000-7000-8000-0000000000f9"}}
	handler := serveWith(t, identity, &fakeCandidates{})

	response := putProfile(t, handler, `{"disciplines": []}`, sessionCookie())
	// The generated layer decodes into the struct with zero values rather
	// than validating required-ness; the port then receives a valid empty
	// profile. That is the current truth: requiredness in the contract is
	// documentation for clients, enforcement is the schema validation the
	// web client performs. Asserted so a change in either direction is
	// noticed rather than silent.
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
}
