package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// REV-02 from the wire's side. Every material conclusion resolves to the
// evidence behind it, uncertainty travels in the same object as every
// band, no field anywhere reads as a recommendation, and the read itself
// is a recorded event that refuses when the record cannot be written.

type stubReviews struct {
	review api.ScreeningReviewView
	err    error
}

func (s *stubReviews) ScreeningReview(_ context.Context, tenantID, campaignID, sessionID string) (api.ScreeningReviewView, error) {
	if s.err != nil {
		return api.ScreeningReviewView{}, s.err
	}
	return s.review, nil
}

func sampleReview() api.ScreeningReviewView {
	return api.ScreeningReviewView{
		SessionID: "00000000-0000-7000-8000-00000000a001",
		Pinned: api.PinnedConfigurationView{
			BundleDigest: "sha256:bundle",
			Rubric: api.RubricPinView{
				Reference: "rubric/icu", Version: "2.0.0", Digest: "sha256:rubric",
			},
			AggregationVersion: "aggregate-1", ExtractionVersion: "evidence-1",
			ModelVersion: "claude-sonnet-5", PolicyVersion: "runtime-policy-v1",
		},
		Competencies: []api.CompetencyResultView{{
			CompetencyID: "sd", Status: "assessed", Band: "solid",
			Confidence: "medium", EvidenceCount: 3, Supporting: 3,
			EvidenceIDs: []string{"sp-1"}, ReasonCodes: []string{},
		}},
		Evidence: []api.EvidenceSpanView{{
			ID: "sp-1", CompetencyID: "sd", Kind: "supporting",
			Quote: "sharded by clinic", SegmentSequence: 4, StartMs: 100, EndMs: 900,
		}},
		Coverage: api.ReviewCoverageView{
			Reached: []string{"sd"}, NotReached: []string{"comm"}, Covered: 1, Total: 2,
		},
		Requirements: api.RequirementsReportView{
			MapVersion: "requirement-map-1",
			Requirements: []api.RequirementFindingView{{
				RequirementID: "00000000-0000-7000-8000-00000000f001",
				Text:          "Communication with stakeholders",
				Status:        "not_discussed",
				Competencies:  []string{"comm"},
				EvidenceIDs:   []string{},
				FollowUp:      "The interview never reached \"Communication with stakeholders\". Ask about it in a follow-up conversation.",
			}},
		},
	}
}

func serveReview(t *testing.T, recruiting *stubRecruiting, reviews *stubReviews, auditor *recordingAuditor) http.Handler {
	t.Helper()
	identity := &fakeIdentity{
		principal: api.Principal{UserID: progressionUser, ActiveTenantID: campaignTenant},
		allowed:   []authz.Capability{authz.EvaluationReadScreen},
	}
	handler, err := api.NewServer(api.ServerConfig{
		Identity: identity, Candidates: &fakeCandidates{}, Documents: &fakeDocuments{},
		Catalog: &fakeCatalog{}, Interviews: &fakeInterviews{}, Members: &fakeMembers{},
		Billing: &fakeBilling{}, Progression: &stubProgression{},
		SensitiveReads: auditor, Settings: &stubSettings{},
		ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(),
		ReInvitations: defaultStubInvitations(), Requirements: defaultStubRequirements(),
		RecruiterAccommodations: defaultStubInvitations(), Recruiting: recruiting,
		Invitations: defaultStubInvitations(), Reviews: reviews,
		Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

const reviewPath = "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123" +
	"/sessions/00000000-0000-7000-8000-00000000a001/review"

func TestTheReviewIsEvidenceFirstWithUncertaintyBesideEveryBand(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	handler := serveReview(t, rec, &stubReviews{review: sampleReview()}, &recordingAuditor{})

	status, body := campaignRequest(t, handler, http.MethodGet, reviewPath, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %v", status, body)
	}

	// The pinned block names what actually ran, by digest.
	pinned, _ := body["pinned"].(map[string]any)
	if pinned["bundle_digest"] != "sha256:bundle" {
		t.Fatalf("pinned = %v", pinned)
	}

	// Uncertainty rides in the same object as the band, never a footnote:
	// one competency entry carries band, confidence, counts and reasons
	// together, and its evidence ids resolve to spans in the same document.
	competencies, _ := body["competencies"].([]any)
	competency, _ := competencies[0].(map[string]any)
	for _, field := range []string{"band", "confidence", "supporting", "contradictory", "evidence_ids"} {
		if _, present := competency[field]; !present {
			t.Fatalf("competency lacks %q beside its band: %v", field, competency)
		}
	}
	evidence, _ := body["evidence"].([]any)
	span, _ := evidence[0].(map[string]any)
	ids, _ := competency["evidence_ids"].([]any)
	if len(ids) == 0 || ids[0] != span["id"] {
		t.Fatalf("the conclusion does not resolve to its evidence: %v vs %v", ids, span)
	}

	// The requirement finding carries its text, its named status and the
	// suggested human follow-up.
	requirements, _ := body["requirements"].(map[string]any)
	findings, _ := requirements["requirements"].([]any)
	finding, _ := findings[0].(map[string]any)
	if finding["status"] != "not_discussed" || finding["follow_up"] == nil {
		t.Fatalf("finding = %v", finding)
	}
}

func TestTheReviewCarriesNoRecommendationAnywhere(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	handler := serveReview(t, rec, &stubReviews{review: sampleReview()}, &recordingAuditor{})

	_, body := campaignRequest(t, handler, http.MethodGet, reviewPath, "")
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	// The whole response, searched by name: nothing recommends, suggests,
	// advances or summarises into a headline number. The decision belongs
	// to the reviewer, and a field hinting otherwise would be the platform
	// quietly making it.
	for _, forbidden := range []string{"recommend", "suggested_band", "advance", "decline", "overall", "percent"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("the review carries %q: %s", forbidden, encoded)
		}
	}
}

func TestReadingTheReviewIsARecordedEvent(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	auditor := &recordingAuditor{}
	handler := serveReview(t, rec, &stubReviews{review: sampleReview()}, auditor)

	status, _ := campaignRequest(t, handler, http.MethodGet, reviewPath, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if len(auditor.reads) != 1 {
		t.Fatalf("audit rows = %d, want exactly one for one read", len(auditor.reads))
	}
	read := auditor.reads[0]
	if read.Action != "GetScreeningReview" || read.Outcome != "allowed" {
		t.Fatalf("recorded = %+v", read)
	}
	if read.Subject != "00000000-0000-7000-8000-00000000a001" {
		t.Fatalf("subject = %q, want the session read", read.Subject)
	}
}

func TestAReviewThatCannotBeRecordedIsRefused(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	auditor := &recordingAuditor{err: context.DeadlineExceeded}
	handler := serveReview(t, rec, &stubReviews{review: sampleReview()}, auditor)

	status, body := campaignRequest(t, handler, http.MethodGet, reviewPath, "")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: an unrecordable read must not happen", status)
	}
	errorBody, _ := body["error"].(map[string]any)
	if errorBody["code"] != "AUDIT_UNAVAILABLE" {
		t.Fatalf("code = %v", errorBody["code"])
	}
}

func TestTheReviewRefusalsKeepTheirCodes(t *testing.T) {
	// Not on the campaign: absence, not refusal.
	handler := serveReview(t, &stubRecruiting{detailErr: api.ErrCampaignNoAccess},
		&stubReviews{}, &recordingAuditor{})
	status, _ := campaignRequest(t, handler, http.MethodGet, reviewPath, "")
	if status != http.StatusNotFound {
		t.Fatalf("no access status = %d, want 404", status)
	}

	// No such session on the campaign: the same absence.
	handler = serveReview(t, &stubRecruiting{detail: openCampaignDetail()},
		&stubReviews{err: api.ErrReviewSessionMissing}, &recordingAuditor{})
	status, _ = campaignRequest(t, handler, http.MethodGet, reviewPath, "")
	if status != http.StatusNotFound {
		t.Fatalf("missing session status = %d, want 404", status)
	}

	// Evaluation still in flight: a wait, not an absence.
	handler = serveReview(t, &stubRecruiting{detail: openCampaignDetail()},
		&stubReviews{err: api.ErrReviewNotReady}, &recordingAuditor{})
	status, body := campaignRequest(t, handler, http.MethodGet, reviewPath, "")
	if status != http.StatusConflict {
		t.Fatalf("not ready status = %d, want 409: %v", status, body)
	}
	errorBody, _ := body["error"].(map[string]any)
	if errorBody["code"] != "REVIEW_NOT_READY" {
		t.Fatalf("code = %v", errorBody["code"])
	}
}
