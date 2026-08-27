package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// The upload surface: the three-step path the browser takes (start,
// complete, abort), the delete, and the listing.
//
// What is worth holding here is that every one of them is owner-scoped
// through the session and that a refusal from the port keeps its own
// status: an upload that silently succeeded for the wrong person, or a
// stalled upload the person cannot abort, are both worse than a refusal.

func serveDocuments(t *testing.T, documents *fakeDocuments) http.Handler {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		Identity:    &fakeIdentity{principal: api.Principal{UserID: "00000000-0000-7000-8000-0000000000f9"}},
		Candidates:  &fakeCandidates{},
		Documents:   documents,
		Catalog:     &fakeCatalog{},
		Interviews:  &fakeInterviews{},
		Members:     &fakeMembers{},
		Billing:     &fakeBilling{},
		Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

func storedDocument() api.Document {
	stored := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	return api.Document{
		ID: "00000000-0000-7000-8000-0000000000d1", Kind: "cv", Version: 2,
		MediaType: "application/pdf", SizeBytes: 2048, State: "stored",
		SHA256:          strings.Repeat("a", 64),
		ExtractionState: "extracted",
		CreatedAt:       stored, StoredAt: &stored,
	}
}

const startUpload = `{"media_type":"application/pdf","size_bytes":2048,"part_count":1}`

func TestTheUploadPathNeedsASessionAtEveryStep(t *testing.T) {
	handler := serveDocuments(t, &fakeDocuments{})
	document := "00000000-0000-7000-8000-0000000000d1"

	for _, step := range []struct {
		method, path, body string
	}{
		{http.MethodPost, "/api/v1/me/documents", startUpload},
		{http.MethodPost, "/api/v1/me/documents/" + document + "/complete", `{}`},
		{http.MethodPost, "/api/v1/me/documents/" + document + "/abort", ""},
		{http.MethodDelete, "/api/v1/me/documents/" + document, ""},
		{http.MethodGet, "/api/v1/me/documents", ""},
	} {
		response := doJSON(t, handler, step.method, step.path, step.body)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without a session = %d, want 401", step.method, step.path, response.Code)
		}
	}
}

func TestStartingAnUploadAnswersThePresignedPartsAndTheVersion(t *testing.T) {
	documents := &fakeDocuments{started: api.StartedUpload{
		Document: storedDocument(), UploadID: "u1",
		PartURLs:  []string{"https://bucket.example/part-1"},
		ExpiresAt: time.Date(2026, 8, 27, 12, 15, 0, 0, time.UTC),
	}}
	handler := serveDocuments(t, documents)

	response := post(t, handler, "/api/v1/me/documents", startUpload, sessionCookie())
	if response.Code != http.StatusCreated && response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		UploadID string   `json:"upload_id"`
		PartURLs []string `json:"part_urls"`
		Document struct {
			Version int    `json:"version"`
			State   string `json:"state"`
		} `json:"document"`
	}
	decodeInto(t, response, &body)
	if body.UploadID != "u1" || len(body.PartURLs) != 1 {
		t.Fatalf("body = %+v", body)
	}
	if body.Document.Version != 2 {
		t.Fatalf("the version the upload allocated is missing: %+v", body.Document)
	}
}

func TestCompletingAnUploadRecordsTheDigest(t *testing.T) {
	documents := &fakeDocuments{stored: storedDocument()}
	handler := serveDocuments(t, documents)

	body := `{"upload_id":"u1","sha256":"` + strings.Repeat("a", 64) + `","size_bytes":2048,` +
		`"parts":[{"number":1,"etag":"e1"}]}`
	response := post(t, handler,
		"/api/v1/me/documents/00000000-0000-7000-8000-0000000000d1/complete", body, sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var stored struct {
		State  string `json:"state"`
		SHA256 string `json:"sha256"`
	}
	decodeInto(t, response, &stored)
	if stored.State != "stored" || len(stored.SHA256) != 64 {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestAbortAndDeleteAnswerWithoutABody(t *testing.T) {
	handler := serveDocuments(t, &fakeDocuments{})
	document := "00000000-0000-7000-8000-0000000000d1"

	abort := post(t, handler, "/api/v1/me/documents/"+document+"/abort", "", sessionCookie())
	if abort.Code != http.StatusNoContent && abort.Code != http.StatusOK {
		t.Fatalf("abort = %d: %s", abort.Code, abort.Body)
	}
	removed := doJSON(t, handler, http.MethodDelete, "/api/v1/me/documents/"+document, "", sessionCookie())
	if removed.Code != http.StatusNoContent && removed.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", removed.Code, removed.Body)
	}
}

func TestTheListingCarriesEveryVersionAndItsState(t *testing.T) {
	deleted := storedDocument()
	deleted.State = "deleted"
	deleted.Version = 1
	handler := serveDocuments(t, &fakeDocuments{listed: []api.Document{deleted, storedDocument()}})

	response := get(t, handler, "/api/v1/me/documents", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		Documents []struct {
			Version int    `json:"version"`
			State   string `json:"state"`
		} `json:"documents"`
	}
	decodeInto(t, response, &body)
	// History survives deletion: the deleted version is still listed.
	if len(body.Documents) != 2 || body.Documents[0].State != "deleted" {
		t.Fatalf("documents = %+v", body.Documents)
	}
}

func TestAPortRefusalKeepsItsOwnStatus(t *testing.T) {
	handler := serveDocuments(t, &fakeDocuments{err: api.ErrDocumentMissing})

	response := doJSON(t, handler, http.MethodDelete,
		"/api/v1/me/documents/00000000-0000-7000-8000-0000000000d1", "", sessionCookie())
	if response.Code != http.StatusNotFound {
		t.Fatalf("a missing document = %d, want 404", response.Code)
	}

	conflicted := serveDocuments(t, &fakeDocuments{err: api.ErrDocumentConflict})
	response = post(t, conflicted,
		"/api/v1/me/documents/00000000-0000-7000-8000-0000000000d1/abort", "", sessionCookie())
	if response.Code != http.StatusConflict {
		t.Fatalf("a state conflict = %d, want 409", response.Code)
	}
}
