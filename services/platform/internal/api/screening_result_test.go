package api_test

// SCR-07 at the interface: whatever DEC-11's determination decided, the API
// enforces it, and hiding a link is not a control. Every test here reads the
// same rich stored outcome through a different disclosure level and asserts the
// response contains exactly what that level allows, by allowlisting the keys:
// a field the level does not grant failing to appear is the point, and a new
// field leaking in fails the sweep the day it is added.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

const (
	screeningCandidate = "00000000-0000-7000-8000-00000000cd01"
	resultPath         = "/api/v1/screening/sessions/00000000-0000-7000-8000-00000000ab01/result"
)

// richOutcome is a full evaluation as the port would answer it: everything a
// permissive jurisdiction may show, so the filter has something to withhold.
func richOutcome(disclosure string) api.ScreeningOutcome {
	return api.ScreeningOutcome{
		Disclosure: disclosure,
		State:      "review_ready",
		Evaluated:  true,
		Competencies: []api.ScreeningCompetency{
			{CompetencyID: "go", Status: "assessed", Band: "strong", EvidenceCount: 3},
			{CompetencyID: "sql", Status: "unassessed", EvidenceCount: 0},
		},
		Evidence: []api.ScreeningEvidence{
			{CompetencyID: "go", Quote: "I built the scheduler", Disposition: "supporting"},
		},
		Covered: 1, Total: 2,
	}
}

func serveScreeningResult(t *testing.T, stub *stubScreening) http.Handler {
	t.Helper()
	identity := &fakeIdentity{
		principal: api.Principal{UserID: screeningCandidate},
		allowed:   []authz.Capability{authz.SessionReadScreenConfirmation},
	}
	handler, err := api.NewServer(api.ServerConfig{
		Identity: identity, Candidates: &fakeCandidates{}, Documents: &fakeDocuments{},
		Catalog: &fakeCatalog{}, Interviews: &fakeInterviews{}, Members: &fakeMembers{},
		Billing: &fakeBilling{}, Progression: &stubProgression{},
		SensitiveReads: &recordingAuditor{}, Settings: &stubSettings{},
		Recruiting: &stubRecruiting{}, Invitations: defaultStubInvitations(),
		ScreeningInvitations: stub, CandidateAccommodations: stub, Requirements: defaultStubRequirements(), RecruiterAccommodations: defaultStubInvitations(), Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

func resultRequest(t *testing.T, handler http.Handler, withCookie bool) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, resultPath, strings.NewReader(""))
	if withCookie {
		request.AddCookie(sessionCookie())
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder.Code, decoded
}

// bodyKeys names what the response actually carried, sorted for the failure
// message.
func bodyKeys(body map[string]any) []string {
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Each level's response carries exactly the allowed keys: nothing missing,
// nothing extra. The equality in both directions is the ticket's third
// criterion made mechanical.
func TestEachLevelDisclosesExactlyItsKeys(t *testing.T) {
	cases := []struct {
		level string
		keys  []string
	}{
		{"submission_only", []string{"disclosure", "status"}},
		{"completion_status", []string{"disclosure", "status"}},
		{"evidence_without_band", []string{"competencies", "coverage", "disclosure", "evidence", "status"}},
		{"full_evaluation", []string{"competencies", "coverage", "disclosure", "evidence", "status"}},
	}
	for _, tc := range cases {
		t.Run(tc.level, func(t *testing.T) {
			stub := defaultStubScreening()
			stub.outcome = richOutcome(tc.level)
			status, body := resultRequest(t, serveScreeningResult(t, stub), true)
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200", status)
			}
			got := bodyKeys(body)
			want := tc.keys
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("%s served keys %v, the policy allows exactly %v", tc.level, got, want)
			}
		})
	}
}

// submission_only does not even disclose completion: the status is submitted
// however far the interview actually got.
func TestSubmissionOnlyHidesCompletion(t *testing.T) {
	stub := defaultStubScreening()
	stub.outcome = richOutcome("submission_only")
	_, body := resultRequest(t, serveScreeningResult(t, stub), true)
	if body["status"] != "submitted" {
		t.Fatalf("status = %v; submission_only may only say submitted", body["status"])
	}
}

// completion_status discloses how far it got and nothing about the content.
func TestCompletionStatusDisclosesProgressOnly(t *testing.T) {
	stub := defaultStubScreening()
	stub.outcome = richOutcome("completion_status")
	_, body := resultRequest(t, serveScreeningResult(t, stub), true)
	if body["status"] != "evaluated" {
		t.Fatalf("status = %v, want evaluated", body["status"])
	}
}

// evidence_without_band shows the assessment's shape and the candidate's own
// words with every band stripped, including inside the competency entries.
func TestEvidenceWithoutBandStripsEveryBand(t *testing.T) {
	stub := defaultStubScreening()
	stub.outcome = richOutcome("evidence_without_band")
	_, body := resultRequest(t, serveScreeningResult(t, stub), true)

	competencies, _ := body["competencies"].([]any)
	if len(competencies) != 2 {
		t.Fatalf("competencies = %v", body["competencies"])
	}
	for _, entry := range competencies {
		if _, present := entry.(map[string]any)["band"]; present {
			t.Fatalf("a band leaked through evidence_without_band: %v", entry)
		}
	}
}

// full_evaluation is the widest level: bands appear, for assessed competencies.
func TestFullEvaluationCarriesBands(t *testing.T) {
	stub := defaultStubScreening()
	stub.outcome = richOutcome("full_evaluation")
	_, body := resultRequest(t, serveScreeningResult(t, stub), true)

	competencies, _ := body["competencies"].([]any)
	found := false
	for _, entry := range competencies {
		if band, present := entry.(map[string]any)["band"]; present {
			found = true
			if band != "strong" {
				t.Fatalf("band = %v", band)
			}
		}
	}
	if !found {
		t.Fatal("full_evaluation carried no band")
	}
}

// A disclosure level this build does not recognise fails closed to
// submission_only: a decision the server cannot read is not guessed upward.
func TestAnUnknownLevelFailsClosed(t *testing.T) {
	stub := defaultStubScreening()
	stub.outcome = richOutcome("everything_plus_notes")
	_, body := resultRequest(t, serveScreeningResult(t, stub), true)

	if body["disclosure"] != "submission_only" || body["status"] != "submitted" {
		t.Fatalf("an unknown level served %v", body)
	}
	if got := bodyKeys(body); strings.Join(got, ",") != "disclosure,status" {
		t.Fatalf("an unknown level leaked keys: %v", got)
	}
}

// Coaching, notes, comparison and decisions never appear at any level. The
// filter cannot leak them because the schema has no fields for them; this
// pins that fact at the interface so a schema change that adds one fails here.
func TestNoLevelCarriesCoachingNotesComparisonOrDecisions(t *testing.T) {
	for _, level := range []string{"submission_only", "completion_status", "evidence_without_band", "full_evaluation"} {
		stub := defaultStubScreening()
		stub.outcome = richOutcome(level)
		_, body := resultRequest(t, serveScreeningResult(t, stub), true)
		for _, forbidden := range []string{"coaching", "notes", "comparison", "decision", "decisions", "contradictions", "reason_codes"} {
			if _, present := body[forbidden]; present {
				t.Fatalf("%s carried %q", level, forbidden)
			}
		}
	}
}

// A session that is not the caller's own screening session is a 404, exactly
// like one that does not exist.
func TestSomebodyElsesScreeningResultIsNotFound(t *testing.T) {
	stub := defaultStubScreening()
	stub.resultErr = api.ErrSessionMissing
	status, _ := resultRequest(t, serveScreeningResult(t, stub), true)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// No session cookie is a 401: the result read is authenticated, unlike the
// token-bearing acceptance endpoints beside it.
func TestTheResultReadRequiresASession(t *testing.T) {
	stub := defaultStubScreening()
	stub.outcome = richOutcome("full_evaluation")
	status, _ := resultRequest(t, serveScreeningResult(t, stub), false)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

// A caller without session.read_screen_confirmation is refused before the port
// is asked anything.
func TestTheResultReadRequiresTheCapability(t *testing.T) {
	stub := defaultStubScreening()
	stub.outcome = richOutcome("full_evaluation")
	identity := &fakeIdentity{
		principal: api.Principal{UserID: screeningCandidate},
		allowed:   []authz.Capability{authz.CampaignRead}, // the wrong authority
	}
	handler, err := api.NewServer(api.ServerConfig{
		Identity: identity, Candidates: &fakeCandidates{}, Documents: &fakeDocuments{},
		Catalog: &fakeCatalog{}, Interviews: &fakeInterviews{}, Members: &fakeMembers{},
		Billing: &fakeBilling{}, Progression: &stubProgression{},
		SensitiveReads: &recordingAuditor{}, Settings: &stubSettings{},
		Recruiting: &stubRecruiting{}, Invitations: defaultStubInvitations(),
		ScreeningInvitations: stub, CandidateAccommodations: stub, Requirements: defaultStubRequirements(), RecruiterAccommodations: defaultStubInvitations(), Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	status, _ := resultRequest(t, handler, true)
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
}
