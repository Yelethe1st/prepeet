package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	started   api.StartedInterview
	ack       api.ControlAck
	ingested  []api.ControlEventIn
	epoch     int
	replayed  []api.ControlEventOut
	consent   api.PracticeConsent
	err       error
	selection *api.InterviewSelection
	users     []string
}

func (f *fakeInterviews) CreatePractice(_ context.Context, userID string, selection api.InterviewSelection) (api.InterviewSession, error) {
	f.users = append(f.users, userID)
	f.selection = &selection
	return f.created, f.err
}

func (f *fakeInterviews) PracticeConsent(_ context.Context) (api.PracticeConsent, error) {
	return f.consent, f.err
}

func (f *fakeInterviews) GetPractice(_ context.Context, userID, sessionID string) (api.InterviewSession, error) {
	f.users = append(f.users, userID+":"+sessionID)
	return f.created, f.err
}

func (f *fakeInterviews) StartPractice(_ context.Context, userID, sessionID string) (api.StartedInterview, error) {
	f.users = append(f.users, "start:"+userID+":"+sessionID)
	return f.started, f.err
}

func (f *fakeInterviews) IngestEvents(_ context.Context, userID, sessionID string, epoch int, events []api.ControlEventIn) (api.ControlAck, error) {
	f.users = append(f.users, "ingest:"+userID+":"+sessionID)
	f.ingested = events
	f.epoch = epoch
	return f.ack, f.err
}

func (f *fakeInterviews) ReplayEvents(_ context.Context, userID, sessionID string, afterEpoch, afterSequence int) ([]api.ControlEventOut, error) {
	f.users = append(f.users, "replay:"+userID+":"+sessionID)
	return f.replayed, f.err
}

func serveInterviews(t *testing.T, interviews *fakeInterviews) http.Handler {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		Identity:    &fakeIdentity{principal: api.Principal{UserID: "00000000-0000-7000-8000-0000000000f9"}},
		Candidates:  &fakeCandidates{},
		Documents:   &fakeDocuments{},
		Catalog:     &fakeCatalog{},
		Interviews:  interviews,
		Members:     &fakeMembers{},
		Billing:     &fakeBilling{},
		Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

const validCreate = `{"mode":"practice","discipline":"software-engineering","role":"rl_swe","shape":"shape_technical","minutes":40,"persona":"per_ravi",` +
	`"recording":{"preference":"transcript_only","consent_version":"1.0.0"}}`

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
			Recording: api.RecordingConsent{Preference: "transcript_only", ConsentVersion: "1.0.0"},
		},
		RecordingPreference: "transcript_only", ConsentVersion: "1.0.0",
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
	if interviews.selection.Recording.Preference != "transcript_only" ||
		interviews.selection.Recording.ConsentVersion != "1.0.0" {
		t.Fatalf("the recording choice arrived as %+v", interviews.selection.Recording)
	}
	if interviews.users[0] != "00000000-0000-7000-8000-0000000000f9" {
		t.Fatalf("the port saw user %s", interviews.users[0])
	}

	var body struct {
		State  string `json:"state"`
		Config struct {
			Shape string `json:"shape"`
		} `json:"config"`
		RecordingPreference string `json:"recording_preference"`
		ConsentVersion      string `json:"consent_version"`
	}
	decodeInto(t, response, &body)
	if body.State != "composing" || body.Config.Shape != "shape_technical" {
		t.Fatalf("body = %+v", body)
	}
	if body.RecordingPreference != "transcript_only" || body.ConsentVersion != "1.0.0" {
		t.Fatalf("the session does not answer what it keeps: %+v", body)
	}
}

func TestScreeningCannotBeAskedForAtAll(t *testing.T) {
	// The third box, at its strongest: the contract has no screening value to
	// send. The refusal is the enum, before any handler runs.
	interviews := &fakeInterviews{}
	handler := serveInterviews(t, interviews)

	response := post(t, handler, "/api/v1/interviews",
		`{"mode":"screening","discipline":"software-engineering","role":"rl_swe","shape":"shape_technical","minutes":40,"persona":"per_ravi",`+
			`"recording":{"preference":"transcript_only","consent_version":"1.0.0"}}`,
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

func TestTheConsentTextIsServedWithItsVersion(t *testing.T) {
	interviews := &fakeInterviews{consent: api.PracticeConsent{
		Version: "1.0.0", Title: "What we keep from this session",
		Statements: []string{"practice only"},
		AudioAndTranscript: api.ConsentChoiceView{
			Label: "The audio and the transcript", Explanation: "replay and delivery",
		},
		TranscriptOnly: api.ConsentChoiceView{
			Label: "The transcript only", Explanation: "audio discarded",
			Forfeits: []string{"Replay of this session", "Delivery measurement for this session"},
		},
	}}
	handler := serveInterviews(t, interviews)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/interviews/practice-consent", nil)
	request.AddCookie(sessionCookie())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	var body struct {
		Version string `json:"version"`
		Choices struct {
			TranscriptOnly struct {
				Forfeits []string `json:"forfeits"`
			} `json:"transcript_only"`
		} `json:"choices"`
	}
	decodeInto(t, response, &body)
	if body.Version != "1.0.0" {
		t.Fatalf("version = %q", body.Version)
	}
	// The second criterion at the wire: the forfeits travel by name.
	if len(body.Choices.TranscriptOnly.Forfeits) != 2 {
		t.Fatalf("forfeits = %v", body.Choices.TranscriptOnly.Forfeits)
	}
}

func TestARecordingPreferenceOutsideTheEnumIsRefused(t *testing.T) {
	interviews := &fakeInterviews{}
	handler := serveInterviews(t, interviews)

	response := post(t, handler, "/api/v1/interviews",
		`{"mode":"practice","discipline":"software-engineering","role":"rl_swe","shape":"shape_technical","minutes":40,"persona":"per_ravi",`+
			`"recording":{"preference":"everything_forever","consent_version":"1.0.0"}}`,
		sessionCookie())

	if response.Code != http.StatusBadRequest {
		t.Fatalf("an unknown preference got %d, want 400", response.Code)
	}
	if interviews.selection != nil {
		t.Fatal("the unknown preference reached the port")
	}
}

func TestASessionIsReadableByItsOwnerAlone(t *testing.T) {
	interviews := &fakeInterviews{created: api.InterviewSession{
		ID: "00000000-0000-7000-8000-0000000000e1", Mode: "practice", State: "ready",
		Config: api.InterviewSelection{
			Discipline: "software-engineering", Role: "rl_swe",
			Shape: "shape_technical", Minutes: 40, Persona: "per_ravi",
		},
		RecordingPreference: "audio_and_transcript", ConsentVersion: "1.0.0",
		CreatedAt: time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
	}}
	handler := serveInterviews(t, interviews)

	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1", nil)
	request.AddCookie(sessionCookie())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	// The session's owner reached the port beside the id: the port decides
	// visibility from both, so somebody else's session is a 404, not a leak.
	if interviews.users[0] != "00000000-0000-7000-8000-0000000000f9:00000000-0000-7000-8000-0000000000e1" {
		t.Fatalf("the port saw %s", interviews.users[0])
	}

	var body struct {
		State  string `json:"state"`
		Config struct {
			Persona string `json:"persona"`
		} `json:"config"`
		RecordingPreference string `json:"recording_preference"`
	}
	decodeInto(t, response, &body)
	if body.State != "ready" || body.Config.Persona != "per_ravi" || body.RecordingPreference != "audio_and_transcript" {
		t.Fatalf("body = %+v", body)
	}
}

func TestSomebodyElsesSessionIsNotFound(t *testing.T) {
	handler := serveInterviews(t, &fakeInterviews{err: api.ErrSessionMissing})

	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1", nil)
	request.AddCookie(sessionCookie())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404: %s", response.Code, response.Body)
	}
}

func TestStartAnswersWithTheScopedGrant(t *testing.T) {
	expires := time.Date(2026, 8, 26, 12, 2, 0, 0, time.UTC)
	interviews := &fakeInterviews{started: api.StartedInterview{
		Session: api.InterviewSession{
			ID: "00000000-0000-7000-8000-0000000000e1", Mode: "practice", State: "connecting",
			Config:              api.InterviewSelection{Discipline: "d", Role: "r", Shape: "s", Minutes: 40, Persona: "p"},
			RecordingPreference: "audio_and_transcript", ConsentVersion: "1.0.0",
			CreatedAt: time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
		},
		Realtime: api.RoomGrantView{
			URL: "wss://rtc.local", Room: "00000000-0000-7000-8000-0000000000e1",
			Token: "tok-1", ExpiresAt: expires,
		},
	}}
	handler := serveInterviews(t, interviews)

	response := doJSON(t, handler, http.MethodPost,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/start", "", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	var body struct {
		Session struct {
			State string `json:"state"`
		} `json:"session"`
		Realtime struct {
			URL   string `json:"url"`
			Room  string `json:"room"`
			Token string `json:"token"`
		} `json:"realtime"`
	}
	decodeInto(t, response, &body)
	if body.Session.State != "connecting" || body.Realtime.Token != "tok-1" ||
		body.Realtime.Room != "00000000-0000-7000-8000-0000000000e1" {
		t.Fatalf("body = %+v", body)
	}
	if interviews.users[0] != "start:00000000-0000-7000-8000-0000000000f9:00000000-0000-7000-8000-0000000000e1" {
		t.Fatalf("the port saw %v", interviews.users)
	}
}

func TestEachStartRefusalKeepsItsCodeOnTheWire(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&api.StartRefusedError{Code: "SESSION_EXPIRED", Message: "m"}, "SESSION_EXPIRED"},
		{&api.StartRefusedError{Code: "SESSION_ALREADY_STARTED", Message: "m"}, "SESSION_ALREADY_STARTED"},
		{&api.StartRefusedError{Code: "SESSION_NOT_READY", Message: "m"}, "SESSION_NOT_READY"},
		{&api.StartRefusedError{Code: "QUOTA_EXHAUSTED", Message: "m"}, "QUOTA_EXHAUSTED"},
	}
	for _, test := range cases {
		handler := serveInterviews(t, &fakeInterviews{err: test.err})

		response := doJSON(t, handler, http.MethodPost,
			"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/start", "", sessionCookie())
		if response.Code != http.StatusConflict {
			t.Errorf("%s answered %d, want 409", test.want, response.Code)
			continue
		}
		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		decodeInto(t, response, &body)
		if body.Error.Code != test.want {
			t.Errorf("code = %q, want %q", body.Error.Code, test.want)
		}
	}
}

func TestControlEventsRoundTripThroughTheWire(t *testing.T) {
	interviews := &fakeInterviews{ack: api.ControlAck{
		Epoch: 1, Accepted: 2,
		Missing:  [][2]int{{3, 3}},
		Outcomes: []api.ControlOutcome{{EventID: "00000000-0000-7000-8000-0000000000ee", Status: "accepted"}},
	}}
	handler := serveInterviews(t, interviews)

	response := doJSON(t, handler, http.MethodPost,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/events",
		`{"connection_epoch":1,"events":[{"event_id":"00000000-0000-7000-8000-0000000000ee","sequence":4,"type":"transcript.segment.final","payload":{"text":"hi"},"occurred_at":"2026-08-26T12:00:00Z"}]}`,
		sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if interviews.epoch != 1 || len(interviews.ingested) != 1 || interviews.ingested[0].Sequence != 4 {
		t.Fatalf("the port saw epoch %d events %+v", interviews.epoch, interviews.ingested)
	}

	var body struct {
		AcceptedSequence int `json:"accepted_sequence"`
		Missing          []struct {
			From int `json:"from"`
			To   int `json:"to"`
		} `json:"missing"`
	}
	decodeInto(t, response, &body)
	if body.AcceptedSequence != 2 || len(body.Missing) != 1 || body.Missing[0].From != 3 {
		t.Fatalf("ack = %+v", body)
	}
}

func TestAStaleEpochAnswers409WithItsName(t *testing.T) {
	handler := serveInterviews(t, &fakeInterviews{
		err: &api.StartRefusedError{Code: "EPOCH_STALE", Message: "superseded"},
	})

	response := doJSON(t, handler, http.MethodPost,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/events",
		`{"connection_epoch":1,"events":[]}`, sessionCookie())
	if response.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", response.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeInto(t, response, &body)
	if body.Error.Code != "EPOCH_STALE" {
		t.Fatalf("code = %q", body.Error.Code)
	}
}
