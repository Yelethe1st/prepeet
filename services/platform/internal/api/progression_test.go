package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

const progressionUser = "00000000-0000-7000-8000-0000000000f7"

// PRG-04's screens need three things the contract now carries: the evidence
// behind a competency with its date, staleness that is visible rather than
// inferred, and readiness that names what judged it.
//
// These test the boundary rather than the calculation, which internal/progression
// already covers: that the reading reaches the browser intact, that a
// never-observed competency does not arrive looking like a poor one, and that
// nothing here can produce a figure spanning two roles.

type stubProgression struct {
	skills    api.SkillHistory
	readiness []api.RoleReadiness
	err       error
	sawUser   string
}

func (s *stubProgression) Skills(_ context.Context, userID string) (api.SkillHistory, error) {
	s.sawUser = userID
	return s.skills, s.err
}

func (s *stubProgression) Readiness(_ context.Context, userID string) ([]api.RoleReadiness, error) {
	s.sawUser = userID
	return s.readiness, s.err
}

func progressionFixture() *stubProgression {
	observed := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	return &stubProgression{
		skills: api.SkillHistory{Competencies: []api.SkillStanding{
			{
				CompetencyID: "systems-design", Name: "Systems design",
				Discipline: "engineering", Role: "backend",
				Standing: "stale", Band: "solid",
				Evidence: []api.SkillEvidence{{
					ObservedAt: observed, AgeDays: 91, Standing: "stale", Band: "solid",
					RubricReference: "rubric/backend", RubricVersion: "1.2.0",
				}},
			},
			{
				// Never observed. The one that must not arrive as a zero.
				CompetencyID: "incident-response", Name: "Incident response",
				Discipline: "engineering", Role: "backend",
				Standing: "none",
			},
		}},
		readiness: []api.RoleReadiness{{
			Role: "backend", Discipline: "engineering",
			StandardReference: "role-standard/backend", StandardVersion: "2.0.0",
			StandardDigest: "sha256:abc", ComputedAt: observed,
			Assessed:   []api.ReadinessCompetency{{CompetencyID: "systems-design", Name: "Systems design", Band: "solid", Standing: "stale"}},
			Unassessed: []api.ReadinessCompetency{{CompetencyID: "incident-response", Name: "Incident response"}},
		}},
	}
}

func TestSkillsCarryTheEvidenceDateAndItsAge(t *testing.T) {
	t.Parallel()

	body := getJSON(t, "/api/v1/me/progression/skills", progressionFixture())

	competencies := body["competencies"].([]any)
	first := competencies[0].(map[string]any)
	evidence := first["evidence"].([]any)
	if len(evidence) == 0 {
		t.Fatal("a competency arrived with no evidence behind it, so it cannot be expanded")
	}
	reading := evidence[0].(map[string]any)
	for _, field := range []string{"observed_at", "age_days", "rubric_version"} {
		if reading[field] == nil {
			t.Fatalf("evidence is missing %s, which the screen needs to show when it was measured", field)
		}
	}
}

func TestStaleEvidenceSaysSoRatherThanBeingCountedAsCurrent(t *testing.T) {
	t.Parallel()

	body := getJSON(t, "/api/v1/me/progression/skills", progressionFixture())

	first := body["competencies"].([]any)[0].(map[string]any)
	if first["standing"] != "stale" {
		t.Fatalf("a 91 day old reading is presented as %v", first["standing"])
	}
}

func TestANeverObservedCompetencyIsNotABandOfZero(t *testing.T) {
	t.Parallel()

	body := getJSON(t, "/api/v1/me/progression/skills", progressionFixture())

	second := body["competencies"].([]any)[1].(map[string]any)
	if second["standing"] != "none" {
		t.Fatalf("an unobserved competency has standing %v", second["standing"])
	}
	if _, present := second["band"]; present {
		t.Fatalf("an unobserved competency carries a band: %v", second["band"])
	}
}

func TestReadinessNamesTheStandardThatJudgedIt(t *testing.T) {
	t.Parallel()

	body := getJSON(t, "/api/v1/me/progression/readiness", progressionFixture())

	role := body["roles"].([]any)[0].(map[string]any)
	for _, field := range []string{"standard_reference", "standard_version", "standard_digest"} {
		if role[field] == nil || role[field] == "" {
			t.Fatalf("readiness does not name %s, so it cannot be reproduced", field)
		}
	}
}

func TestReadinessKeepsAssessedAndUnassessedApart(t *testing.T) {
	t.Parallel()

	body := getJSON(t, "/api/v1/me/progression/readiness", progressionFixture())

	role := body["roles"].([]any)[0].(map[string]any)
	if len(role["unassessed"].([]any)) == 0 {
		t.Fatal("the unassessed competency was dropped, so an unasked question vanishes")
	}
	for _, entry := range role["unassessed"].([]any) {
		if _, present := entry.(map[string]any)["band"]; present {
			t.Fatal("an unassessed competency carries a band")
		}
	}
}

func TestProgressionRefusesWithoutASession(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/api/v1/me/progression/skills", "/api/v1/me/progression/readiness",
	} {
		recorder := progressionRequest(t, path, progressionFixture(), "")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s answered %d without a session", path, recorder.Code)
		}
	}
}

func TestProgressionReadsOnlyTheCallersOwnHistory(t *testing.T) {
	t.Parallel()

	// The capability is owner-scoped, so the only user id the handler may pass
	// down is the one the session resolved to. A handler taking it from the
	// request would let one candidate read another's practice history.
	stub := progressionFixture()
	getJSON(t, "/api/v1/me/progression/skills", stub)
	if stub.sawUser != progressionUser {
		t.Fatalf("the store was asked for %q rather than the session's own user", stub.sawUser)
	}
}

func TestAProgressionFailureIsNotAnEmptyHistory(t *testing.T) {
	t.Parallel()

	// An error rendered as an empty list would tell a candidate they have
	// practised nothing, which is worse than an error.
	stub := progressionFixture()
	stub.err = errors.New("the database is unreachable")

	recorder := progressionRequest(t, "/api/v1/me/progression/skills", stub, "ses_valid")

	if recorder.Code == http.StatusOK {
		t.Fatalf("a store failure answered 200 with body %s", recorder.Body.String())
	}
}

// getJSON runs an authenticated request and decodes a successful body.
func getJSON(t *testing.T, path string, stub *stubProgression) map[string]any {
	t.Helper()
	recorder := progressionRequest(t, path, stub, "ses_valid")
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s answered %d: %s", path, recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v", path, err)
	}
	return body
}

func progressionRequest(t *testing.T, path string, stub *stubProgression, token string) *httptest.ResponseRecorder {
	t.Helper()
	identity := &fakeIdentity{principal: api.Principal{UserID: progressionUser}}
	handler, err := api.NewServer(api.ServerConfig{
		Identity: identity, Candidates: &fakeCandidates{}, Documents: &fakeDocuments{},
		Catalog: &fakeCatalog{}, Interviews: &fakeInterviews{}, Members: &fakeMembers{},
		Billing: &fakeBilling{}, Progression: stub, Settings: &stubSettings{}, Invitations: defaultStubInvitations(), ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(), Requirements: defaultStubRequirements(), RecruiterAccommodations: defaultStubInvitations(), Recruiting: &stubRecruiting{},
		SensitiveReads: &recordingAuditor{}, Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		request.AddCookie(&http.Cookie{Name: "prepeet_session", Value: token})
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
