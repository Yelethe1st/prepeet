package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
	verdicts      []api.InsightFeedbackInput
	givenFeedback []api.InsightVerdictView
	created       api.InterviewSession
	started       api.StartedInterview
	ack           api.ControlAck
	ingested      []api.ControlEventIn
	epoch         int
	replayed      []api.ControlEventOut
	transcript    api.TranscriptView
	result        api.EvaluationResultView
	review        api.ReviewView
	receipt       api.CompletionReceiptView
	consent       api.PracticeConsent
	err           error
	selection     *api.InterviewSelection
	users         []string
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

func (f *fakeInterviews) Transcript(_ context.Context, userID, sessionID string) (api.TranscriptView, error) {
	f.users = append(f.users, "transcript:"+userID+":"+sessionID)
	return f.transcript, f.err
}

func (f *fakeInterviews) Results(_ context.Context, userID, sessionID string) (api.EvaluationResultView, error) {
	f.users = append(f.users, "results:"+userID+":"+sessionID)
	return f.result, f.err
}

func (f *fakeInterviews) Brief(_ context.Context, sessionID, candidateID, mode string) (api.BriefView, error) {
	f.users = append(f.users, "brief:"+candidateID+":"+sessionID+":"+mode)
	if f.err != nil {
		return api.BriefView{}, f.err
	}
	return api.BriefView{
		SessionID: sessionID, Minutes: 25,
		PersonaName: "Ama", PersonaStyle: "Warm and structured", PersonaAbout: "Gentle.",
		RoleTitle: "Senior Backend Engineer", Competencies: []string{"Systems design"},
		Plan: json.RawMessage(`{"stages":["intro","core","close"]}`),
	}, nil
}

func (f *fakeInterviews) IngestServiceEvents(_ context.Context, sessionID, candidateID, mode string, events []api.ControlEventIn) (api.ControlAck, error) {
	f.users = append(f.users, "service:"+candidateID+":"+sessionID+":"+mode)
	f.ingested = events
	return f.ack, f.err
}

func (f *fakeInterviews) MySessions(_ context.Context, userID string) ([]api.InterviewSession, error) {
	f.users = append(f.users, "list:"+userID)
	if f.err != nil {
		return nil, f.err
	}
	return []api.InterviewSession{f.created}, nil
}

func (f *fakeInterviews) CreateRedo(_ context.Context, userID, sessionID string, sequence int) (api.InterviewSession, error) {
	f.users = append(f.users, fmt.Sprintf("redo:%s:%s:%d", userID, sessionID, sequence))
	if f.err != nil {
		return api.InterviewSession{}, f.err
	}
	created := f.created
	created.RedoOf = &api.RedoOfView{SessionID: sessionID, Sequence: sequence, Question: "Tell me about it"}
	return created, nil
}

func (f *fakeInterviews) DeliveryBaseline(_ context.Context, userID string) (api.BaselineView, error) {
	f.users = append(f.users, "baseline:"+userID)
	return api.BaselineView{
		BaselineVersion: "baseline-1", SessionsMeasured: 6, MinimumSessions: 5, Ready: true,
		Ranges: map[string][2]float64{"words_per_minute": {130, 170}},
		Note:   "These ranges are guidance about you, not a target: there is no correct speaking rate.",
	}, nil
}

func (f *fakeInterviews) Delivery(_ context.Context, userID, sessionID string) (api.DeliveryAnalysisView, error) {
	f.users = append(f.users, "delivery:"+userID+":"+sessionID)
	if f.err != nil {
		return api.DeliveryAnalysisView{}, f.err
	}
	return api.DeliveryAnalysisView{
		Feedback:  f.givenFeedback,
		SessionID: sessionID, Status: "not_assessable", Warnings: []string{"AUDIO_CLIPPED"},
		CalculationVersion: "articulation-features-v1", PolicyVersion: "articulation-practice-v1",
		Analysis:  json.RawMessage(`{"profile":{"dimensions":{"pace":{"level":"solid"}}}}`),
		CreatedAt: time.Date(2026, 8, 27, 16, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakeInterviews) Review(_ context.Context, userID, sessionID string) (api.ReviewView, error) {
	f.users = append(f.users, "review:"+userID+":"+sessionID)
	return f.review, f.err
}

func (f *fakeInterviews) RecordInsightFeedback(_ context.Context, userID, sessionID string, verdict api.InsightFeedbackInput) error {
	f.users = append(f.users, fmt.Sprintf("feedback:%s:%s:%s:%s:%t",
		userID, sessionID, verdict.Kind, verdict.Key, verdict.Helpful))
	f.verdicts = append(f.verdicts, verdict)
	return f.err
}

func (f *fakeInterviews) CompleteInterview(_ context.Context, userID, sessionID string, epoch, finalSequence int) (api.CompletionReceiptView, error) {
	f.users = append(f.users, fmt.Sprintf("complete:%s:%s:%d:%d", userID, sessionID, epoch, finalSequence))
	return f.receipt, f.err
}

func serveInterviews(t *testing.T, interviews *fakeInterviews) http.Handler {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		AgentToken:  "agent-secret-agent-secret",
		Identity:    &fakeIdentity{principal: api.Principal{UserID: "00000000-0000-7000-8000-0000000000f9"}},
		Candidates:  &fakeCandidates{},
		Documents:   &fakeDocuments{},
		Catalog:     &fakeCatalog{},
		Interviews:  interviews,
		Members:     &fakeMembers{},
		Billing:     &fakeBilling{},
		Settings:    &stubSettings{},
		Invitations: defaultStubInvitations(), ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(), RecruiterAccommodations: defaultStubInvitations(), Recruiting: &stubRecruiting{},
		SensitiveReads: &recordingAuditor{},
		Progression:    &stubProgression{},
		Environment:    config.EnvironmentLocal,
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
		Timing: api.TimingPolicyView{
			PolicyVersion: 1, ReconnectGraceSeconds: 120, MaxOverrunSeconds: 300,
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
		Timing struct {
			PolicyVersion         int `json:"policy_version"`
			ReconnectGraceSeconds int `json:"reconnect_grace_seconds"`
			MaxOverrunSeconds     int `json:"max_overrun_seconds"`
		} `json:"timing"`
	}
	decodeInto(t, response, &body)
	if body.Session.State != "connecting" || body.Realtime.Token != "tok-1" ||
		body.Realtime.Room != "00000000-0000-7000-8000-0000000000e1" {
		t.Fatalf("body = %+v", body)
	}
	// SES-05: the grace window arrives from the server's versioned policy,
	// never from a constant compiled into the client.
	if body.Timing.PolicyVersion != 1 || body.Timing.ReconnectGraceSeconds != 120 || body.Timing.MaxOverrunSeconds != 300 {
		t.Fatalf("timing = %+v", body.Timing)
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

func TestTheTranscriptCarriesProvenanceAcrossTheWire(t *testing.T) {
	interviews := &fakeInterviews{transcript: api.TranscriptView{
		Segments: []api.TranscriptSegmentView{
			{
				Epoch: 1, Sequence: 2, Type: "transcript.segment.final",
				Speaker: "candidate", Text: "I lead the migration",
				StartMs: 5000, EndMs: 8000, Confidence: 0.81,
				Words:      []api.TranscriptWordView{{Word: "lead", StartMs: 5500, EndMs: 5900, Confidence: 0.7}},
				Superseded: true, CorrectedBySequence: 3,
			},
			{
				Epoch: 1, Sequence: 3, Type: "transcript.segment.corrected",
				Speaker: "candidate", Text: "I led the migration",
				StartMs: 5000, EndMs: 8000, Confidence: 0.95, Supersedes: 2,
			},
		},
		OrphanCorrections: []int{9},
	}}
	handler := serveInterviews(t, interviews)

	response := doJSON(t, handler, http.MethodGet,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/transcript", "", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	var body struct {
		Segments []struct {
			Text                string `json:"text"`
			Superseded          bool   `json:"superseded"`
			CorrectedBySequence *int   `json:"corrected_by_sequence"`
			Supersedes          *int   `json:"supersedes"`
			Words               []struct {
				W       string `json:"w"`
				StartMs int    `json:"start_ms"`
			} `json:"words"`
		} `json:"segments"`
		OrphanCorrections []int `json:"orphan_corrections"`
	}
	decodeInto(t, response, &body)
	if len(body.Segments) != 2 {
		t.Fatalf("segments = %+v", body.Segments)
	}
	original, corrected := body.Segments[0], body.Segments[1]
	if !original.Superseded || original.CorrectedBySequence == nil || *original.CorrectedBySequence != 3 {
		t.Fatalf("original = %+v", original)
	}
	if corrected.Supersedes == nil || *corrected.Supersedes != 2 {
		t.Fatalf("corrected = %+v", corrected)
	}
	if len(original.Words) != 1 || original.Words[0].StartMs != 5500 {
		t.Fatalf("word timing = %+v", original.Words)
	}
	if len(body.OrphanCorrections) != 1 || body.OrphanCorrections[0] != 9 {
		t.Fatalf("orphans = %v", body.OrphanCorrections)
	}
}

func TestCompletionAnswersTheReceipt(t *testing.T) {
	interviews := &fakeInterviews{receipt: api.CompletionReceiptView{
		SessionID: "00000000-0000-7000-8000-0000000000e1", State: "evaluating",
		SealedEpoch: 1, SealedSequence: 5,
		Gaps:             [][2]int{{4, 4}},
		TranscriptDigest: "sha256:abc", BundleDigest: "sha256:def",
		MediaStatus: "missing", Warnings: []string{"MEDIA_MISSING", "SEQUENCE_GAPS_RECORDED"},
		SealedAt: time.Date(2026, 8, 26, 14, 0, 0, 0, time.UTC),
	}}
	handler := serveInterviews(t, interviews)

	response := doJSON(t, handler, http.MethodPost,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/complete",
		`{"connection_epoch":1,"final_sequence":5}`, sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if interviews.users[0] != "complete:00000000-0000-7000-8000-0000000000f9:00000000-0000-7000-8000-0000000000e1:1:5" {
		t.Fatalf("the port saw %v", interviews.users)
	}

	var body struct {
		State    string   `json:"state"`
		Warnings []string `json:"warnings"`
		Gaps     []struct {
			From int `json:"from"`
		} `json:"gaps"`
		MediaStatus string `json:"media_status"`
	}
	decodeInto(t, response, &body)
	if body.State != "evaluating" || body.MediaStatus != "missing" ||
		len(body.Warnings) != 2 || len(body.Gaps) != 1 || body.Gaps[0].From != 4 {
		t.Fatalf("receipt = %+v", body)
	}
}

// EVL-03's API surface: the insufficient-evidence outcome distinct on the
// wire, coverage named both ways, and nothing anywhere for silence to
// drag toward zero.

func resultFixture() api.EvaluationResultView {
	return api.EvaluationResultView{
		SessionID: "00000000-0000-7000-8000-0000000000e1",
		Rubric: api.RubricPinView{
			Reference: "rubric/practice-default", Version: "1.0.0", Digest: "sha256:abc",
		},
		AggregationVersion: "aggregate-1", ExtractionVersion: "evidence-1",
		ModelVersion: "none", PolicyVersion: "none",
		Competencies: []api.CompetencyResultView{
			{
				CompetencyID: "clinical-reasoning", Status: "unassessed", Confidence: "not_assessable",
				EvidenceCount: 1, Supporting: 1,
				EvidenceIDs: []string{"sp-1"},
				ReasonCodes: []string{"INSUFFICIENT_EVIDENCE"},
			},
			{
				CompetencyID: "never-raised", Status: "unassessed", Confidence: "not_assessable",
				ReasonCodes: []string{"NOT_DISCUSSED"},
			},
			{
				CompetencyID: "systems-design", Status: "assessed", Confidence: "high", Band: "strong",
				EvidenceCount: 4, Supporting: 4,
				EvidenceIDs: []string{"sp-2", "sp-3", "sp-4", "sp-5"},
				ReasonCodes: []string{},
			},
		},
		Evidence: []api.EvidenceSpanView{{
			ID: "sp-2", CompetencyID: "systems-design", Kind: "supporting",
			Quote: "we cut latency by 40 percent", SegmentSequence: 3,
			StartMs: 61_000, EndMs: 64_000,
		}},
		Contradictions: []api.ContradictionView{{
			Topic: []string{"migration", "payments", "team"},
			SideA: api.ContradictionSideView{
				SegmentSequence: 3, Quote: "I led the payments migration team of 5 engineers.",
				StartMs: 5000, EndMs: 9000,
			},
			SideB: api.ContradictionSideView{
				SegmentSequence: 5, Quote: "The payments migration team I led was 12 people.",
				StartMs: 15000, EndMs: 19000,
			},
		}},
		Delivery: api.DeliveryView{Status: "not_assessable", Warnings: []string{"AUDIO_CLIPPED"}},
		Omissions: []api.OmissionView{
			{Stage: "articulation", Reason: "BUDGET_EXHAUSTED", Retryable: false},
			{Stage: "coaching", Reason: "FAILURE_CODE_PROVIDER_TIMEOUT", Retryable: true},
		},
		CoverageReached:     []string{"clinical-reasoning", "systems-design"},
		CoverageNotReached:  []string{"never-raised"},
		CoveredCompetencies: 2, TotalCompetencies: 3,
		ResultDigest: "sha256:def", Warnings: []string{},
		CreatedAt: time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC),
	}
}

func TestResultsNeedASession(t *testing.T) {
	handler := serveInterviews(t, &fakeInterviews{})

	response := get(t, handler, "/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/results")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("no session got %d, want 401", response.Code)
	}
}

func TestResultsKeepUnassessedApartFromPoorOnTheWire(t *testing.T) {
	interviews := &fakeInterviews{result: resultFixture()}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/results", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	var body struct {
		Rubric struct {
			Reference string `json:"reference"`
			Version   string `json:"version"`
			Digest    string `json:"digest"`
		} `json:"rubric"`
		ModelVersion string `json:"model_version"`
		Competencies []struct {
			CompetencyID string   `json:"competency_id"`
			Status       string   `json:"status"`
			Confidence   string   `json:"confidence"`
			Band         *string  `json:"band"`
			EvidenceIDs  []string `json:"evidence_ids"`
			ReasonCodes  []string `json:"reason_codes"`
		} `json:"competencies"`
		Coverage struct {
			Reached    []string `json:"reached"`
			NotReached []string `json:"not_reached"`
		} `json:"coverage"`
		Evidence []struct {
			Quote   string `json:"quote"`
			StartMs int    `json:"start_ms"`
		} `json:"evidence"`
	}
	decodeInto(t, response, &body)

	if body.Rubric.Reference != "rubric/practice-default" || body.Rubric.Digest != "sha256:abc" {
		t.Fatalf("the pin is not echoed: %+v", body.Rubric)
	}
	if body.ModelVersion != "none" {
		t.Fatalf("model_version = %q, want the honest none", body.ModelVersion)
	}
	byID := map[string]struct {
		CompetencyID string   `json:"competency_id"`
		Status       string   `json:"status"`
		Confidence   string   `json:"confidence"`
		Band         *string  `json:"band"`
		EvidenceIDs  []string `json:"evidence_ids"`
		ReasonCodes  []string `json:"reason_codes"`
	}{}
	for _, c := range body.Competencies {
		byID[c.CompetencyID] = c
	}
	thin := byID["clinical-reasoning"]
	if thin.Status != "unassessed" || thin.Band != nil {
		t.Fatalf("insufficient evidence must be its own state with no band: %+v", thin)
	}
	if thin.Confidence != "not_assessable" {
		t.Fatalf("unassessed confidence = %q, want not_assessable, never low", thin.Confidence)
	}
	if len(thin.ReasonCodes) != 1 || thin.ReasonCodes[0] != "INSUFFICIENT_EVIDENCE" {
		t.Fatalf("reasons = %v", thin.ReasonCodes)
	}
	silent := byID["never-raised"]
	if len(silent.ReasonCodes) != 1 || silent.ReasonCodes[0] != "NOT_DISCUSSED" {
		t.Fatalf("an unreached competency must say NOT_DISCUSSED: %+v", silent)
	}
	assessed := byID["systems-design"]
	if assessed.Status != "assessed" || assessed.Band == nil || *assessed.Band != "strong" {
		t.Fatalf("assessed = %+v", assessed)
	}
	if assessed.Confidence != "high" || len(assessed.EvidenceIDs) != 4 {
		t.Fatalf("confidence/evidence = %q/%v", assessed.Confidence, assessed.EvidenceIDs)
	}
	if !reflect.DeepEqual(body.Coverage.Reached, []string{"clinical-reasoning", "systems-design"}) ||
		!reflect.DeepEqual(body.Coverage.NotReached, []string{"never-raised"}) {
		t.Fatalf("coverage = %+v", body.Coverage)
	}
	if len(body.Evidence) != 1 || body.Evidence[0].Quote != "we cut latency by 40 percent" ||
		body.Evidence[0].StartMs != 61_000 {
		t.Fatalf("evidence = %+v", body.Evidence)
	}

	// The third box on the wire: no overall or averaged number exists for
	// unassessed to be zero in.
	var raw map[string]any
	decodeInto(t, response, &raw)
	for _, forbidden := range []string{"overall", "overall_band", "score", "overall_score"} {
		if _, present := raw[forbidden]; present {
			t.Fatalf("the result carries %q; any overall number would need a rule for unassessed", forbidden)
		}
	}
}

func TestResultsBeforeEvaluationSayNotReadyByName(t *testing.T) {
	interviews := &fakeInterviews{err: api.ErrResultNotReady}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/results", sessionCookie())
	if response.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", response.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeInto(t, response, &body)
	if body.Error.Code != "RESULT_NOT_READY" {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestSomeoneElsesResultsDoNotExist(t *testing.T) {
	interviews := &fakeInterviews{err: api.ErrSessionMissing}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/results", sessionCookie())
	if response.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", response.Code)
	}
}

func TestContradictionsArriveWithBothSidesAndTheFraming(t *testing.T) {
	// EVL-04's surface: both sides quoted with timestamps, and the neutral
	// copy shipped by the server so no consumer can drop it.
	interviews := &fakeInterviews{result: resultFixture()}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/results", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	var body struct {
		Contradictions []struct {
			Topic []string `json:"topic"`
			SideA struct {
				SegmentSequence int    `json:"segment_sequence"`
				Quote           string `json:"quote"`
				StartMs         int    `json:"start_ms"`
				EndMs           int    `json:"end_ms"`
			} `json:"side_a"`
			SideB struct {
				SegmentSequence int    `json:"segment_sequence"`
				Quote           string `json:"quote"`
				StartMs         int    `json:"start_ms"`
				EndMs           int    `json:"end_ms"`
			} `json:"side_b"`
		} `json:"contradictions"`
		Framing struct {
			Unverified     string `json:"unverified"`
			Contradictions string `json:"contradictions"`
		} `json:"framing"`
	}
	decodeInto(t, response, &body)

	if len(body.Contradictions) != 1 {
		t.Fatalf("contradictions = %+v", body.Contradictions)
	}
	pair := body.Contradictions[0]
	if pair.SideA.Quote == "" || pair.SideB.Quote == "" {
		t.Fatalf("a side arrived unquoted: %+v", pair)
	}
	if pair.SideA.StartMs != 5000 || pair.SideB.EndMs != 19000 {
		t.Fatalf("timestamps did not survive: %+v", pair)
	}
	if !strings.Contains(body.Framing.Unverified, "does not mean") {
		t.Fatalf("the unverified framing does not disclaim: %q", body.Framing.Unverified)
	}
	if body.Framing.Contradictions == "" {
		t.Fatal("the contradiction framing is missing")
	}
	// Neutral on the wire: the whole response carries no judgment words.
	serialized := strings.ToLower(response.Body.String())
	for _, forbidden := range []string{"honest", "integrity", "credib", "lying", "deceit", "decept"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("the response contains %q", forbidden)
		}
	}
}

// PRC-02's surface: the rewrite arrives as typed parts so a placeholder
// can never render as a fact, and a coaching failure is a stated absence
// with the evaluation intact, not an error.

func TestTheReviewArrivesWithTypedRewriteParts(t *testing.T) {
	interviews := &fakeInterviews{review: api.ReviewView{
		SessionID:         "00000000-0000-7000-8000-0000000000e1",
		CoachingVersion:   "coaching-1",
		CoachingAvailable: true,
		Answers: []api.AnswerCoachingView{{
			Sequence:  5,
			Strengths: []api.CoachingPointView{},
			Gaps: []api.CoachingPointView{{
				Statement: "This is a claim about yourself with nothing a listener could check.",
				Quote:     "I am usually good at tradeoffs.",
			}},
			Rewrite: []api.RewritePartView{
				{Kind: "quote", Text: "I am usually good at tradeoffs."},
				{Kind: "placeholder", Text: "[Which project or moment shows this? Name it, and what happened.]"},
			},
		}},
	}}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/review", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	var body struct {
		CoachingAvailable bool `json:"coaching_available"`
		Answers           []struct {
			Sequence int `json:"sequence"`
			Gaps     []struct {
				Statement string `json:"statement"`
				Quote     string `json:"quote"`
			} `json:"gaps"`
			Rewrite []struct {
				Kind string `json:"kind"`
				Text string `json:"text"`
			} `json:"rewrite"`
		} `json:"answers"`
	}
	decodeInto(t, response, &body)
	if !body.CoachingAvailable || len(body.Answers) != 1 {
		t.Fatalf("body = %+v", body)
	}
	answer := body.Answers[0]
	if answer.Gaps[0].Quote != "I am usually good at tradeoffs." {
		t.Fatalf("the gap lost its quote: %+v", answer.Gaps)
	}
	if answer.Rewrite[0].Kind != "quote" || answer.Rewrite[1].Kind != "placeholder" {
		t.Fatalf("rewrite kinds = %+v", answer.Rewrite)
	}
}

func TestACoachingFailureIsAStatedAbsenceNotAnError(t *testing.T) {
	interviews := &fakeInterviews{review: api.ReviewView{
		SessionID:         "00000000-0000-7000-8000-0000000000e1",
		CoachingVersion:   "coaching-1",
		CoachingAvailable: false,
		Note:              "Coaching could not be derived for this session. Your evaluation is complete and unaffected.",
		Answers:           []api.AnswerCoachingView{},
	}}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/review", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("a coaching failure must not fail the request: %d", response.Code)
	}
	var body struct {
		CoachingAvailable bool    `json:"coaching_available"`
		Note              *string `json:"note"`
	}
	decodeInto(t, response, &body)
	if body.CoachingAvailable || body.Note == nil || *body.Note == "" {
		t.Fatalf("the absence is not stated: %+v", body)
	}
}

func TestReviewBeforeEvaluationSaysNotReady(t *testing.T) {
	interviews := &fakeInterviews{err: api.ErrResultNotReady}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/review", sessionCookie())
	if response.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", response.Code)
	}
}

func TestTheSessionListAnswersUnderTheCallersOwnUser(t *testing.T) {
	interviews := &fakeInterviews{created: api.InterviewSession{
		ID: "00000000-0000-7000-8000-0000000000e1", Mode: "practice", State: "expired",
		Config:              api.InterviewSelection{Discipline: "d", Role: "r", Shape: "s", Minutes: 40, Persona: "p"},
		RecordingPreference: "transcript_only", ConsentVersion: "1.0.0",
		CreatedAt: time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC),
	}}
	handler := serveInterviews(t, interviews)

	response := get(t, handler, "/api/v1/me/sessions", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		Sessions []struct {
			State string `json:"state"`
		} `json:"sessions"`
	}
	decodeInto(t, response, &body)
	// An expired session is history, not a hidden row.
	if len(body.Sessions) != 1 || body.Sessions[0].State != "expired" {
		t.Fatalf("sessions = %+v", body.Sessions)
	}
	if interviews.users[0] != "list:00000000-0000-7000-8000-0000000000f9" {
		t.Fatalf("the port saw %v", interviews.users)
	}
}

// The agent's internal ingest (ADR-0019): a service credential, never a
// person's session; sequences are the server's to assign.

const serviceBatch = `{"candidate_id":"00000000-0000-7000-8000-0000000000f9","mode":"practice",` +
	`"events":[{"event_id":"00000000-0000-7000-8000-0000000000a1","type":"turn.boundary",` +
	`"occurred_at":"2026-08-27T12:00:00Z"}]}`

func serviceRequest(t *testing.T, handler http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/internal/interviews/00000000-0000-7000-8000-0000000000e1/events",
		strings.NewReader(serviceBatch))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestTheInternalIngestRefusesWithoutTheServiceToken(t *testing.T) {
	interviews := &fakeInterviews{ack: api.ControlAck{Epoch: 1, Accepted: 1}}
	handler := serveInterviews(t, interviews)

	if response := serviceRequest(t, handler, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("no token got %d, want 401", response.Code)
	}
	if response := serviceRequest(t, handler, "wrong-secret-wrong-secret"); response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token got %d, want 401", response.Code)
	}
	if len(interviews.users) != 0 {
		t.Fatalf("the port was reached without a credential: %v", interviews.users)
	}
}

func TestASessionCookieIsNotAServiceCredential(t *testing.T) {
	// A person's session must never open the internal surface, however
	// valid: the two credentials are different kinds of authority.
	interviews := &fakeInterviews{ack: api.ControlAck{Epoch: 1, Accepted: 1}}
	handler := serveInterviews(t, interviews)

	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/internal/interviews/00000000-0000-7000-8000-0000000000e1/events",
		strings.NewReader(serviceBatch))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(sessionCookie())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("a session cookie opened the internal surface: %d", recorder.Code)
	}
}

func TestTheInternalIngestLandsUnderTheCandidateTheAgentHeard(t *testing.T) {
	interviews := &fakeInterviews{ack: api.ControlAck{
		Epoch: 2, Accepted: 4,
		Outcomes: []api.ControlOutcome{{EventID: "00000000-0000-7000-8000-0000000000a1", Status: "accepted"}},
	}}
	handler := serveInterviews(t, interviews)

	response := serviceRequest(t, handler, "agent-secret-agent-secret")
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		ConnectionEpoch  int `json:"connection_epoch"`
		AcceptedSequence int `json:"accepted_sequence"`
	}
	decodeInto(t, response, &body)
	if body.ConnectionEpoch != 2 || body.AcceptedSequence != 4 {
		t.Fatalf("ack = %+v", body)
	}
	if interviews.users[0] != "service:00000000-0000-7000-8000-0000000000f9:00000000-0000-7000-8000-0000000000e1:practice" {
		t.Fatalf("the port saw %v", interviews.users)
	}
	// No sequence travels from the agent: the server's to assign.
	if len(interviews.ingested) != 1 || interviews.ingested[0].Sequence != 0 {
		t.Fatalf("ingested = %+v", interviews.ingested)
	}
}

func TestTheBriefIsServedOnlyToTheServiceToken(t *testing.T) {
	interviews := &fakeInterviews{}
	handler := serveInterviews(t, interviews)
	path := "/api/v1/internal/interviews/00000000-0000-7000-8000-0000000000e1/brief" +
		"?candidate_id=00000000-0000-7000-8000-0000000000f9&mode=practice"

	unauthenticated := get(t, handler, path)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("no token got %d", unauthenticated.Code)
	}

	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer agent-secret-agent-secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	var body struct {
		Minutes int `json:"minutes"`
		Persona struct {
			Name string `json:"name"`
		} `json:"persona"`
		Role struct {
			Competencies []string `json:"competencies"`
		} `json:"role"`
		Plan map[string]any `json:"plan"`
	}
	decodeInto(t, recorder, &body)
	if body.Minutes != 25 || body.Persona.Name != "Ama" || len(body.Role.Competencies) != 1 || body.Plan["stages"] == nil {
		t.Fatalf("brief = %+v", body)
	}
	if interviews.users[len(interviews.users)-1] != "brief:00000000-0000-7000-8000-0000000000f9:00000000-0000-7000-8000-0000000000e1:practice" {
		t.Fatalf("the port saw %v", interviews.users)
	}
}

func TestANotAssessableDeliverySaysItIsNotALowResult(t *testing.T) {
	interviews := &fakeInterviews{result: resultFixture()}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/results", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		Delivery struct {
			Status   string   `json:"status"`
			Warnings []string `json:"warnings"`
			Note     string   `json:"note"`
		} `json:"delivery"`
		Competencies []struct {
			Band *string `json:"band"`
		} `json:"competencies"`
	}
	decodeInto(t, response, &body)
	if body.Delivery.Status != "not_assessable" || body.Delivery.Warnings[0] != "AUDIO_CLIPPED" {
		t.Fatalf("delivery = %+v", body.Delivery)
	}
	if !strings.Contains(body.Delivery.Note, "not a low result") ||
		!strings.Contains(body.Delivery.Note, "not affected any score") {
		t.Fatalf("the note does not say the one thing it must: %q", body.Delivery.Note)
	}
	// And the content evaluation on the same response is untouched by it.
	assessed := 0
	for _, competency := range body.Competencies {
		if competency.Band != nil {
			assessed++
		}
	}
	if assessed != 1 {
		t.Fatalf("delivery status altered the content bands: %d assessed", assessed)
	}
}

func TestTheDeliveryBlockCarriesNoAggregateScore(t *testing.T) {
	// ART-03's second box on the wire: the delivery block is status,
	// warnings and the note - no field a total could live in.
	interviews := &fakeInterviews{result: resultFixture()}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/results", sessionCookie())
	var body struct {
		Delivery map[string]any `json:"delivery"`
	}
	decodeInto(t, response, &body)
	for key := range body.Delivery {
		switch key {
		case "status", "warnings", "note":
		default:
			t.Fatalf("delivery carries %q; a delivery score is forbidden anywhere", key)
		}
	}
}

func TestTheDeliveryEndpointServesTheAnalysisAndTheStatement(t *testing.T) {
	handler := serveInterviews(t, &fakeInterviews{})

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/delivery", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		Status   string         `json:"status"`
		Note     string         `json:"note"`
		Analysis map[string]any `json:"analysis"`
	}
	decodeInto(t, response, &body)
	if body.Status != "not_assessable" || !strings.Contains(body.Note, "not a low result") {
		t.Fatalf("body = %+v", body)
	}
	if body.Analysis["profile"] == nil {
		t.Fatal("the analysis document did not arrive")
	}

	notReady := serveInterviews(t, &fakeInterviews{err: api.ErrDeliveryNotReady})
	if response := get(t, notReady,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/delivery", sessionCookie()); response.Code != http.StatusConflict {
		t.Fatalf("not ready = %d, want 409", response.Code)
	}
}

func TestTheBaselineIsTheCallersOwnWithItsGuidanceNote(t *testing.T) {
	interviews := &fakeInterviews{}
	handler := serveInterviews(t, interviews)

	response := get(t, handler, "/api/v1/me/delivery-baseline", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		Ready  bool `json:"ready"`
		Ranges map[string]struct {
			Low  float64 `json:"low"`
			High float64 `json:"high"`
		} `json:"ranges"`
		Note string `json:"note"`
	}
	decodeInto(t, response, &body)
	if !body.Ready || body.Ranges["words_per_minute"].Low != 130 || !strings.Contains(body.Note, "no correct speaking rate") {
		t.Fatalf("body = %+v", body)
	}
	if interviews.users[0] != "baseline:00000000-0000-7000-8000-0000000000f9" {
		t.Fatalf("the port saw %v", interviews.users)
	}
}

func TestARedoIsANewSessionNamingItsOrigin(t *testing.T) {
	interviews := &fakeInterviews{created: api.InterviewSession{
		ID: "00000000-0000-7000-8000-0000000000e2", Mode: "practice", State: "draft",
		Config:              api.InterviewSelection{Discipline: "d", Role: "r", Shape: "s", Minutes: 5, Persona: "p"},
		RecordingPreference: "transcript_only", ConsentVersion: "1.0.0",
		CreatedAt: time.Date(2026, 8, 27, 18, 0, 0, 0, time.UTC),
	}}
	handler := serveInterviews(t, interviews)

	response := post(t, handler, "/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/turns/3/redos", "", sessionCookie())
	if response.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		ID     string `json:"id"`
		RedoOf struct {
			SessionID string `json:"session_id"`
			Sequence  int    `json:"sequence"`
			Question  string `json:"question"`
		} `json:"redo_of"`
	}
	decodeInto(t, response, &body)
	if body.ID != "00000000-0000-7000-8000-0000000000e2" || body.RedoOf.SessionID != "00000000-0000-7000-8000-0000000000e1" || body.RedoOf.Sequence != 3 {
		t.Fatalf("body = %+v", body)
	}
	if interviews.users[0] != "redo:00000000-0000-7000-8000-0000000000f9:00000000-0000-7000-8000-0000000000e1:3" {
		t.Fatalf("the port saw %v", interviews.users)
	}

	refused := serveInterviews(t, &fakeInterviews{err: &api.StartRefusedError{Code: "REDO_EXISTS", Message: "once"}})
	response = post(t, refused, "/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/turns/3/redos", "", sessionCookie())
	if response.Code != http.StatusConflict {
		t.Fatalf("a second redo answered %d, want 409", response.Code)
	}
}

func TestReplayAnswersTheTimelineAfterACursor(t *testing.T) {
	// The recovery read: a client that lost its place asks for everything
	// after the cursor it holds, and the cursor it asks with reaches the
	// port unchanged. Replaying from the same cursor must answer the same
	// events, which is what a client rebuilds itself on.
	interviews := &fakeInterviews{replayed: []api.ControlEventOut{
		{
			EventID: "00000000-0000-7000-8000-0000000000b1", Epoch: 1, Sequence: 4,
			Type: "turn.boundary", Payload: json.RawMessage(`{"speaker":"candidate"}`),
			OccurredAt: time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC),
		},
	}}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/events?after_epoch=1&after_sequence=3",
		sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		Events []struct {
			EventID  string         `json:"event_id"`
			Sequence int            `json:"sequence"`
			Type     string         `json:"type"`
			Payload  map[string]any `json:"payload"`
		} `json:"events"`
	}
	decodeInto(t, response, &body)
	if len(body.Events) != 1 || body.Events[0].Sequence != 4 || body.Events[0].Type != "turn.boundary" {
		t.Fatalf("events = %+v", body.Events)
	}
	if body.Events[0].Payload["speaker"] != "candidate" {
		t.Fatalf("the payload did not survive: %+v", body.Events[0].Payload)
	}
	if interviews.users[0] != "replay:00000000-0000-7000-8000-0000000000f9:00000000-0000-7000-8000-0000000000e1" {
		t.Fatalf("the port saw %v", interviews.users)
	}
}

func TestReplayNeedsASessionAndSurfacesARefusal(t *testing.T) {
	path := "/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/events"
	if response := get(t, serveInterviews(t, &fakeInterviews{}), path); response.Code != http.StatusUnauthorized {
		t.Fatalf("no session = %d, want 401", response.Code)
	}
	missing := serveInterviews(t, &fakeInterviews{err: api.ErrSessionMissing})
	if response := get(t, missing, path, sessionCookie()); response.Code != http.StatusNotFound {
		t.Fatalf("someone else's session = %d, want 404", response.Code)
	}
}

func TestTheSessionResponseCarriesItsCursorSealAndOrigin(t *testing.T) {
	// One read answers everything a screen needs about where a session
	// stands: the cursor completion would seal at, the durable receipt
	// once it has, and the answer it retakes when it is a redo. Each is
	// absent rather than zeroed when it does not apply.
	sealed := time.Date(2026, 8, 27, 14, 0, 0, 0, time.UTC)
	interviews := &fakeInterviews{created: api.InterviewSession{
		ID: "00000000-0000-7000-8000-0000000000e1", Mode: "practice", State: "review_ready",
		Config:              api.InterviewSelection{Discipline: "d", Role: "r", Shape: "s", Minutes: 5, Persona: "p"},
		RecordingPreference: "audio_and_transcript", ConsentVersion: "1.0.0",
		ConnectionEpoch:  2,
		AcceptedSequence: 9,
		Seal: &api.SealView{
			SealedAt: sealed, MediaStatus: "finalized", Warnings: []string{},
		},
		RedoOf: &api.RedoOfView{
			SessionID: "00000000-0000-7000-8000-0000000000e0", Sequence: 3, Question: "Again please.",
		},
		CreatedAt: sealed,
	}}
	handler := serveInterviews(t, interviews)

	response := get(t, handler, "/api/v1/interviews/00000000-0000-7000-8000-0000000000e1", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		Cursor *struct {
			ConnectionEpoch  int `json:"connection_epoch"`
			AcceptedSequence int `json:"accepted_sequence"`
		} `json:"cursor"`
		Seal *struct {
			MediaStatus string   `json:"media_status"`
			Warnings    []string `json:"warnings"`
		} `json:"seal"`
		RedoOf *struct {
			SessionID string `json:"session_id"`
			Sequence  int    `json:"sequence"`
		} `json:"redo_of"`
	}
	decodeInto(t, response, &body)
	if body.Cursor == nil || body.Cursor.ConnectionEpoch != 2 || body.Cursor.AcceptedSequence != 9 {
		t.Fatalf("cursor = %+v", body.Cursor)
	}
	if body.Seal == nil || body.Seal.MediaStatus != "finalized" || body.Seal.Warnings == nil {
		t.Fatalf("seal = %+v", body.Seal)
	}
	if body.RedoOf == nil || body.RedoOf.Sequence != 3 {
		t.Fatalf("redo_of = %+v", body.RedoOf)
	}
}

func TestASessionThatHasNotStartedCarriesNoCursorOrSeal(t *testing.T) {
	interviews := &fakeInterviews{created: api.InterviewSession{
		ID: "00000000-0000-7000-8000-0000000000e1", Mode: "practice", State: "ready",
		Config:              api.InterviewSelection{Discipline: "d", Role: "r", Shape: "s", Minutes: 40, Persona: "p"},
		RecordingPreference: "transcript_only", ConsentVersion: "1.0.0",
		CreatedAt: time.Date(2026, 8, 27, 14, 0, 0, 0, time.UTC),
	}}
	handler := serveInterviews(t, interviews)

	response := get(t, handler, "/api/v1/interviews/00000000-0000-7000-8000-0000000000e1", sessionCookie())
	var body map[string]any
	decodeInto(t, response, &body)
	for _, absent := range []string{"cursor", "seal", "redo_of", "failure_code"} {
		if _, present := body[absent]; present {
			t.Fatalf("a session that has not started carries %q", absent)
		}
	}
}

func TestAnOmissionIsNamedWithWordsThatFitItsCause(t *testing.T) {
	// EVL-07 on the wire: a candidate is told what is missing, and the
	// two causes are told apart, because one is worth waiting for and the
	// other never will be.
	interviews := &fakeInterviews{result: resultFixture()}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/results", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		Omissions []struct {
			Stage     string `json:"stage"`
			Reason    string `json:"reason"`
			Retryable bool   `json:"retryable"`
			Note      string `json:"note"`
		} `json:"omissions"`
		Competencies []struct {
			Band *string `json:"band"`
		} `json:"competencies"`
	}
	decodeInto(t, response, &body)

	if len(body.Omissions) != 2 {
		t.Fatalf("omissions = %+v", body.Omissions)
	}
	byStage := map[string]string{}
	retryable := map[string]bool{}
	for _, omission := range body.Omissions {
		byStage[omission.Stage] = omission.Note
		retryable[omission.Stage] = omission.Retryable
	}
	// The exhausted budget is final, and says so without promising a retry.
	if retryable["articulation"] || strings.Contains(byStage["articulation"], "retried") {
		t.Fatalf("an exhausted budget promised a retry: %q", byStage["articulation"])
	}
	if !strings.Contains(byStage["articulation"], "Delivery measurement") {
		t.Fatalf("the omission does not name what is missing: %q", byStage["articulation"])
	}
	// The retryable failure says to expect it, and neither blames the person.
	if !retryable["coaching"] || !strings.Contains(byStage["coaching"], "being retried") {
		t.Fatalf("a retryable omission did not say so: %q", byStage["coaching"])
	}
	for stage, note := range byStage {
		if !strings.Contains(note, "complete and unaffected") {
			t.Fatalf("%s does not say the results stand: %q", stage, note)
		}
	}

	// And the result above it is untouched by either.
	assessed := 0
	for _, competency := range body.Competencies {
		if competency.Band != nil {
			assessed++
		}
	}
	if assessed != 1 {
		t.Fatalf("an omission changed the competency results: %d assessed", assessed)
	}
}

// ART-09 at the edge: a verdict goes in, nothing comes back, and a screening
// session cannot carry one.

const feedbackPath = "/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/delivery/feedback"

func TestAVerdictIsRecordedAndNothingIsReturned(t *testing.T) {
	interviews := &fakeInterviews{}
	handler := serveInterviews(t, interviews)

	response := put(t, handler, feedbackPath,
		`{"insight_kind":"strength","insight_key":"precision","dimension":"precision","helpful":false}`,
		sessionCookie())

	// 204: the verdict is a report about the coaching, not a way to edit it,
	// and returning the coaching again would suggest otherwise.
	if response.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if body := response.Body.String(); body != "" {
		t.Fatalf("want an empty body, got %q", body)
	}
	if len(interviews.verdicts) != 1 {
		t.Fatalf("want one verdict recorded, got %d", len(interviews.verdicts))
	}
	got := interviews.verdicts[0]
	if got.Kind != "strength" || got.Key != "precision" || got.Helpful {
		t.Fatalf("the verdict did not arrive intact: %+v", got)
	}
}

func TestAScreeningSessionRefusesAVerdict(t *testing.T) {
	handler := serveInterviews(t, &fakeInterviews{err: api.ErrFeedbackPracticeOnly})

	response := put(t, handler, feedbackPath,
		`{"insight_kind":"strength","insight_key":"precision","helpful":true}`, sessionCookie())

	if response.Code != http.StatusConflict {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), "FEEDBACK_PRACTICE_ONLY") {
		t.Fatalf("want the refusal named, got %s", response.Body)
	}
}

func TestAVerdictNeedsASession(t *testing.T) {
	interviews := &fakeInterviews{}
	handler := serveInterviews(t, interviews)

	response := put(t, handler, feedbackPath,
		`{"insight_kind":"strength","insight_key":"precision","helpful":true}`)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if len(interviews.verdicts) != 0 {
		t.Fatal("an unauthenticated request reached the flows")
	}
}

func TestTheDeliveryViewCarriesWhatTheCandidateAlreadySaid(t *testing.T) {
	interviews := &fakeInterviews{}
	interviews.givenFeedback = []api.InsightVerdictView{
		{Kind: "strength", Key: "precision", Dimension: "precision", Helpful: false},
	}
	handler := serveInterviews(t, interviews)

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/delivery", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		InsightFeedback []struct {
			InsightKind string `json:"insight_kind"`
			InsightKey  string `json:"insight_key"`
			Helpful     bool   `json:"helpful"`
		} `json:"insight_feedback"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.InsightFeedback) != 1 || body.InsightFeedback[0].InsightKey != "precision" {
		t.Fatalf("the screen cannot tell which thumb is pressed: %+v", body.InsightFeedback)
	}
}

// A candidate who has answered nothing must read as an empty list rather than
// null, or the screen crashes on the person who has never pressed anything,
// which is most of them.
func TestNoVerdictsSerialiseAsAnEmptyList(t *testing.T) {
	handler := serveInterviews(t, &fakeInterviews{})

	response := get(t, handler,
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/delivery", sessionCookie())

	if !strings.Contains(response.Body.String(), `"insight_feedback":[]`) {
		t.Fatalf("want an empty list, got %s", response.Body)
	}
}

// Regression from the ART review: a delivery that will never arrive was
// answered as DELIVERY_NOT_READY, so the screen said measurement was still
// running forever and polled every five seconds for as long as it was open.
// The reason had been recorded and the candidate was never shown it.
func TestATerminalDeliveryOutcomeIsNotReportedAsPending(t *testing.T) {
	for _, outcome := range []struct {
		name string
		err  error
		code string
	}{
		{"omitted", api.ErrDeliveryOmitted, "DELIVERY_OMITTED"},
		{"failed", api.ErrDeliveryFailed, "DELIVERY_FAILED"},
	} {
		t.Run(outcome.name, func(t *testing.T) {
			handler := serveInterviews(t, &fakeInterviews{err: outcome.err})

			response := get(t, handler,
				"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/delivery", sessionCookie())

			if response.Code != http.StatusConflict {
				t.Fatalf("status %d: %s", response.Code, response.Body)
			}
			body := response.Body.String()
			if !strings.Contains(body, outcome.code) {
				t.Fatalf("want %s, got %s", outcome.code, body)
			}
			// Distinct from pending, or the screen cannot tell them apart and
			// polls a state that can never change its own answer.
			if strings.Contains(body, "DELIVERY_NOT_READY") {
				t.Fatalf("a terminal outcome answered as pending: %s", body)
			}
		})
	}
}

// The two terminal outcomes stay distinct, because they mean different things
// to a person: one is a decision about the session and the other is something
// that went wrong.
func TestAnOmissionAndAFailureAreDifferentAnswers(t *testing.T) {
	omitted := get(t, serveInterviews(t, &fakeInterviews{err: api.ErrDeliveryOmitted}),
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/delivery", sessionCookie())
	failed := get(t, serveInterviews(t, &fakeInterviews{err: api.ErrDeliveryFailed}),
		"/api/v1/interviews/00000000-0000-7000-8000-0000000000e1/delivery", sessionCookie())

	if omitted.Body.String() == failed.Body.String() {
		t.Fatal("an omission and a failure read the same to a candidate")
	}
	// Neither may read as a result about the candidate.
	for _, body := range []string{omitted.Body.String(), failed.Body.String()} {
		for _, forbidden := range []string{"score", "low", "poor"} {
			if strings.Contains(strings.ToLower(body), forbidden) {
				t.Fatalf("the message says %q, which reads as a result: %s", forbidden, body)
			}
		}
	}
}
