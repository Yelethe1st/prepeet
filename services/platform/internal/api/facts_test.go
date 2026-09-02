package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// The facts surface: PRO-04 at the HTTP boundary. The contract's promise is
// the ticket's first box - every fact arrives with its span, confidence and
// review state - and the review endpoint carries the candidate's move to the
// port without inventing any authority of its own.

const factsDocument = "00000000-0000-7000-8000-0000000000aa"
const reviewedFact = "00000000-0000-7000-8000-0000000000ab"

func aProposedFact() api.Fact {
	return api.Fact{
		ID: reviewedFact, DocumentID: factsDocument, Kind: "skill",
		Value:     json.RawMessage(`{"name":"Go","confidence":0.8}`),
		SpanStart: 80, SpanEnd: 82, Confidence: 0.8,
		ExtractorVersion: "extract-1", Status: "proposed",
		CreatedAt: time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC),
	}
}

func serveFacts(t *testing.T, documents *fakeDocuments) http.Handler {
	t.Helper()
	identity := &fakeIdentity{principal: api.Principal{UserID: "00000000-0000-7000-8000-0000000000f9"}}
	handler, err := api.NewServer(api.ServerConfig{
		Identity:       identity,
		Candidates:     &fakeCandidates{},
		Documents:      documents,
		Catalog:        &fakeCatalog{},
		Interviews:     &fakeInterviews{},
		Members:        &fakeMembers{},
		Billing:        &fakeBilling{},
		Settings:       &stubSettings{},
		SensitiveReads: &recordingAuditor{},
		Progression:    &stubProgression{},
		Environment:    config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

func getFacts(t *testing.T, handler http.Handler, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me/documents/"+factsDocument+"/facts", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestFactsNeedASession(t *testing.T) {
	handler := serveFacts(t, &fakeDocuments{})

	if response := getFacts(t, handler); response.Code != http.StatusUnauthorized {
		t.Fatalf("no session got %d, want 401", response.Code)
	}
	response := post(t, handler, "/api/v1/me/facts/"+reviewedFact+"/review", `{"status":"confirmed"}`)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("no session got %d, want 401", response.Code)
	}
}

func TestFactsArriveWithTheirProvenance(t *testing.T) {
	documents := &fakeDocuments{facts: []api.Fact{aProposedFact()}}
	handler := serveFacts(t, documents)

	response := getFacts(t, handler, sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	var body struct {
		Facts []map[string]any `json:"facts"`
	}
	decodeInto(t, response, &body)
	if len(body.Facts) != 1 {
		t.Fatalf("facts = %v", body.Facts)
	}
	fact := body.Facts[0]
	// The first box, at the wire: span, confidence and review state all
	// present, and the value decoded as the object it is.
	if fact["span_start"] != float64(80) || fact["span_end"] != float64(82) {
		t.Fatalf("span = %v-%v", fact["span_start"], fact["span_end"])
	}
	if fact["confidence"] != 0.8 || fact["status"] != "proposed" || fact["extractor_version"] != "extract-1" {
		t.Fatalf("fact = %v", fact)
	}
	if _, present := fact["corrected_value"]; present {
		t.Fatal("a proposed fact must not claim a correction")
	}
	if value, ok := fact["value"].(map[string]any); !ok || value["name"] != "Go" {
		t.Fatalf("value = %v", fact["value"])
	}
}

func TestReviewCarriesTheMoveToThePort(t *testing.T) {
	corrected := aProposedFact()
	corrected.Status = "corrected"
	corrected.CorrectedValue = json.RawMessage(`{"name":"Golang"}`)
	now := time.Date(2026, 8, 25, 11, 0, 0, 0, time.UTC)
	corrected.ReviewedAt = &now
	documents := &fakeDocuments{fact: corrected}
	handler := serveFacts(t, documents)

	response := post(t, handler, "/api/v1/me/facts/"+reviewedFact+"/review",
		`{"status":"corrected","corrected_value":{"name":"Golang"}}`, sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	if documents.reviewedID != reviewedFact || documents.reviewedStatus != "corrected" {
		t.Fatalf("the port saw %s -> %s", documents.reviewedID, documents.reviewedStatus)
	}
	var sent map[string]any
	if err := json.Unmarshal(documents.reviewedValue, &sent); err != nil || sent["name"] != "Golang" {
		t.Fatalf("the corrected value reached the port as %s", documents.reviewedValue)
	}

	var body map[string]any
	decodeInto(t, response, &body)
	if body["status"] != "corrected" {
		t.Fatalf("body = %v", body)
	}
	if value, ok := body["corrected_value"].(map[string]any); !ok || value["name"] != "Golang" {
		t.Fatalf("corrected_value = %v", body["corrected_value"])
	}
	if body["reviewed_at"] == nil {
		t.Fatal("a reviewed fact must say when")
	}
}

func TestAMissingFactIsNotFound(t *testing.T) {
	documents := &fakeDocuments{err: api.ErrFactMissing}
	handler := serveFacts(t, documents)

	response := post(t, handler, "/api/v1/me/facts/"+reviewedFact+"/review",
		`{"status":"confirmed"}`, sessionCookie())
	if response.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404: %s", response.Code, response.Body)
	}
}

func TestARefusedReviewIsAValidationFailure(t *testing.T) {
	documents := &fakeDocuments{err: api.Invalid("corrected_value", "FACT_REVIEW_INVALID",
		"A correction needs its corrected value.")}
	handler := serveFacts(t, documents)

	response := post(t, handler, "/api/v1/me/facts/"+reviewedFact+"/review",
		`{"status":"corrected"}`, sessionCookie())
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", response.Code, response.Body)
	}
	var body struct {
		Error struct {
			FieldErrors []struct {
				Field string `json:"field"`
			} `json:"field_errors"`
		} `json:"error"`
	}
	decodeInto(t, response, &body)
	if len(body.Error.FieldErrors) != 1 || body.Error.FieldErrors[0].Field != "corrected_value" {
		t.Fatalf("fields = %+v", body.Error)
	}
}

func TestDocumentsCarryTheirExtractionState(t *testing.T) {
	// PRO-03's degradation, visible: the list says whether the CV was read,
	// so the profile screen can show "we could not read this format" instead
	// of silence.
	documents := &fakeDocuments{listed: []api.Document{{
		ID: factsDocument, Kind: "cv", Version: 1, MediaType: "application/pdf",
		SizeBytes: 10, State: "stored", ExtractionState: "unsupported",
		CreatedAt: time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC),
	}}}
	handler := serveFacts(t, documents)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/me/documents", nil)
	request.AddCookie(sessionCookie())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	var body struct {
		Documents []map[string]any `json:"documents"`
	}
	decodeInto(t, response, &body)
	if body.Documents[0]["extraction_state"] != "unsupported" {
		t.Fatalf("extraction_state = %v", body.Documents[0]["extraction_state"])
	}
}
