package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// The interview creation surface: CAT-04's server half. The wizard's
// filtering is a courtesy; what these tests hold is that the session's user
// is the only possible owner, the selection reaches the port whole, a
// catalogue refusal comes back field by field, and screening cannot be
// asked for at all - the contract's enum is the refusal.

type fakeInterviews struct {
	created   api.InterviewSession
	err       error
	selection *api.InterviewSelection
	users     []string
}

func (f *fakeInterviews) CreatePractice(_ context.Context, userID string, selection api.InterviewSelection) (api.InterviewSession, error) {
	f.users = append(f.users, userID)
	f.selection = &selection
	return f.created, f.err
}

func serveInterviews(t *testing.T, interviews *fakeInterviews) http.Handler {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		Identity:    &fakeIdentity{principal: api.Principal{UserID: "00000000-0000-7000-8000-0000000000f9"}},
		Candidates:  &fakeCandidates{},
		Documents:   &fakeDocuments{},
		Catalog:     &fakeCatalog{},
		Interviews:  interviews,
		Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

const validCreate = `{"mode":"practice","discipline":"software-engineering","role":"rl_swe","shape":"shape_technical","minutes":40,"persona":"per_ravi"}`

func TestCreatingAnInterviewNeedsASession(t *testing.T) {
	handler := serveInterviews(t, &fakeInterviews{})

	response := post(t, handler, "/api/v1/interviews", validCreate)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("no session got %d, want 401", response.Code)
	}
}

func TestTheSelectionReachesThePortUnderTheSessionsOwnUser(t *testing.T) {
	interviews := &fakeInterviews{created: api.InterviewSession{
		ID: "00000000-0000-7000-8000-0000000000e1", Mode: "practice", State: "composing",
		Config: api.InterviewSelection{
			Discipline: "software-engineering", Role: "rl_swe",
			Shape: "shape_technical", Minutes: 40, Persona: "per_ravi",
		},
		CreatedAt: time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC),
	}}
	handler := serveInterviews(t, interviews)

	response := post(t, handler, "/api/v1/interviews", validCreate, sessionCookie())
	if response.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	if interviews.selection == nil || interviews.selection.Persona != "per_ravi" || interviews.selection.Minutes != 40 {
		t.Fatalf("the selection arrived as %+v", interviews.selection)
	}
	if interviews.users[0] != "00000000-0000-7000-8000-0000000000f9" {
		t.Fatalf("the port saw user %s", interviews.users[0])
	}

	var body struct {
		State  string `json:"state"`
		Config struct {
			Shape string `json:"shape"`
		} `json:"config"`
	}
	decodeInto(t, response, &body)
	if body.State != "composing" || body.Config.Shape != "shape_technical" {
		t.Fatalf("body = %+v", body)
	}
}

func TestScreeningCannotBeAskedForAtAll(t *testing.T) {
	// The third box, at its strongest: the contract has no screening value to
	// send. The refusal is the enum, before any handler runs.
	interviews := &fakeInterviews{}
	handler := serveInterviews(t, interviews)

	response := post(t, handler, "/api/v1/interviews",
		`{"mode":"screening","discipline":"software-engineering","role":"rl_swe","shape":"shape_technical","minutes":40,"persona":"per_ravi"}`,
		sessionCookie())

	if response.Code != http.StatusBadRequest {
		t.Fatalf("screening got %d, want 400", response.Code)
	}
	if interviews.selection != nil {
		t.Fatal("the screening request reached the port")
	}
}

func TestACatalogueRefusalComesBackFieldByField(t *testing.T) {
	handler := serveInterviews(t, &fakeInterviews{err: &api.ValidationError{Fields: []api.FieldError{
		{Field: "shape", Code: "SHAPE_NOT_OFFERED", Message: "The chosen role does not offer that interview shape."},
		{Field: "minutes", Code: "DURATION_NOT_OFFERED", Message: "That interview shape does not run at that length."},
	}}})

	response := post(t, handler, "/api/v1/interviews", validCreate, sessionCookie())
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
	if len(body.Error.FieldErrors) != 2 || body.Error.FieldErrors[0].Field != "shape" || body.Error.FieldErrors[1].Code != "DURATION_NOT_OFFERED" {
		t.Fatalf("field errors = %+v", body.Error.FieldErrors)
	}
}
