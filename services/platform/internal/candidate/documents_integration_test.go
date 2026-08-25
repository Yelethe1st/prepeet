//go:build integration

package candidate_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tclocalstack "github.com/testcontainers/testcontainers-go/modules/localstack"

	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// PRO-02 against real PostgreSQL and real S3 semantics: the browser's own
// path - presigned PUT, complete, list, delete - with every state the ticket
// names, and the property the second criterion protects: history survives
// its objects.
//
// The PostgreSQL container comes from the profile suite's TestMain; the
// LocalStack container is this file's own, started lazily so profile tests
// do not pay for S3.

var documentStore *objectstore.S3Store

// startS3 boots LocalStack once for the document tests.
func startS3(t *testing.T) *objectstore.S3Store {
	t.Helper()
	if documentStore != nil {
		return documentStore
	}
	ctx := context.Background()

	container, err := tclocalstack.Run(ctx, "localstack/localstack:3.8",
		testcontainers.WithEnv(map[string]string{"S3_SKIP_SIGNATURE_VALIDATION": "0"}))
	if err != nil {
		t.Fatalf("starting LocalStack: %v", err)
	}
	t.Cleanup(func() {
		// Terminated at process end by testcontainers' reaper; explicit
		// termination here would kill it for the next test in the package.
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	port, err := container.MappedPort(ctx, "4566/tcp")
	if err != nil {
		t.Fatalf("port: %v", err)
	}

	store, err := objectstore.NewS3Store(ctx, objectstore.S3Config{
		Endpoint: "http://" + host + ":" + port.Port(),
		Region:   "eu-west-2", Bucket: "prepeet-documents",
		AccessKey: "test", SecretKey: "test", UsePathStyle: true,
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("bucket: %v", err)
	}
	documentStore = store
	return store
}

func documents(t *testing.T) *candidate.Documents {
	t.Helper()
	return candidate.NewDocuments(candidate.NewStore(pool), startS3(t), outbox.New(pool))
}

// uploadCV walks the browser's whole path and returns the stored document.
func uploadCV(t *testing.T, service *candidate.Documents, userID string, body []byte) candidate.Document {
	t.Helper()
	ctx := context.Background()

	started, err := service.StartUpload(ctx, userID, "application/pdf", int64(len(body)), 1)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.Document.State != "uploading" {
		t.Fatalf("fresh upload state = %s", started.Document.State)
	}

	// The bytes travel exactly as the browser sends them: a PUT against the
	// presigned URL, no credential in sight.
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, started.PartURLs[0], bytes.NewReader(body))
	if err != nil {
		t.Fatalf("building PUT: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("uploading: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the presigned PUT answered %d", response.StatusCode)
	}
	etag := response.Header.Get("ETag")

	digest := sha256.Sum256(body)
	stored, err := service.CompleteUpload(ctx, userID, started.Document.ID, started.UploadID,
		hex.EncodeToString(digest[:]),
		[]objectstore.CompletedPart{{Number: 1, ETag: etag}}, int64(len(body)))
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	return stored
}

func TestTheWholeUploadPathStoresAVersionedRecordedCV(t *testing.T) {
	service := documents(t)
	body := []byte("%PDF-1.7 pretend curriculum vitae, version one")

	stored := uploadCV(t, service, amaraID, body)

	if stored.State != "stored" || stored.Version < 1 {
		t.Fatalf("stored = %+v", stored)
	}
	if stored.SHA256 == "" || stored.SizeBytes != int64(len(body)) {
		t.Fatalf("the record is missing its digest or size: %+v", stored)
	}
	if stored.StoredAt == nil {
		t.Fatal("no stored_at")
	}
}

func TestReplacementIsANewVersionAndHistorySurvivesDeletion(t *testing.T) {
	// The second criterion. Replace the CV, delete the old version's object,
	// and the old version's record - the digest a composed bundle would have
	// pinned - still answers; and the bundle table itself is untouched by
	// construction, being written only by the ready transition.
	ctx := context.Background()
	service := documents(t)

	first := uploadCV(t, service, amaraID, []byte("%PDF-1.7 the first attempt"))
	second := uploadCV(t, service, amaraID, []byte("%PDF-1.7 the improved rewrite"))

	if second.Version != first.Version+1 {
		t.Fatalf("replacement is version %d after %d; every upload is a new version", second.Version, first.Version)
	}
	if first.SHA256 == second.SHA256 {
		t.Fatal("two different bodies share a digest")
	}

	if err := service.Delete(ctx, amaraID, first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	versions, err := service.List(ctx, amaraID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var deleted *candidate.Document
	for i := range versions {
		if versions[i].ID == first.ID {
			deleted = &versions[i]
		}
	}
	if deleted == nil {
		t.Fatal("the deleted version vanished from history; a bundle that pinned it can no longer answer")
	}
	if deleted.State != "deleted" || deleted.SHA256 != first.SHA256 {
		t.Fatalf("the deleted version's record changed: %+v", deleted)
	}

	// And deleting it twice is a state refusal, not a repeat.
	if err := service.Delete(ctx, amaraID, first.ID); !errors.Is(err, candidate.ErrDocumentState) {
		t.Fatalf("second delete = %v, want ErrDocumentState", err)
	}
}

func TestAnAbortedUploadIsFailedVisiblyAndRecoverably(t *testing.T) {
	// The third criterion: the stalled upload gets its own state, the person
	// sees it in the list, and recovery is simply the next version.
	ctx := context.Background()
	service := documents(t)

	started, err := service.StartUpload(ctx, priyaID, "application/pdf", 1024, 1)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := service.AbortUpload(ctx, priyaID, started.Document.ID); err != nil {
		t.Fatalf("abort: %v", err)
	}

	versions, err := service.List(ctx, priyaID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(versions) == 0 || versions[0].State != "failed" {
		t.Fatalf("the aborted upload is not visible as failed: %+v", versions)
	}

	// Completing the corpse is refused, not honoured late.
	_, err = service.CompleteUpload(ctx, priyaID, started.Document.ID, started.UploadID,
		"deadbeef", []objectstore.CompletedPart{{Number: 1, ETag: "x"}}, 1024)
	if !errors.Is(err, candidate.ErrDocumentState) {
		t.Fatalf("completing an aborted upload = %v, want ErrDocumentState", err)
	}

	// Recovery: the next upload simply takes the next version.
	recovered := uploadCV(t, service, priyaID, []byte("%PDF-1.7 second try"))
	if recovered.Version != started.Document.Version+1 {
		t.Fatalf("recovery version = %d, want %d", recovered.Version, started.Document.Version+1)
	}
}

func TestTheBoundsRefuseByName(t *testing.T) {
	ctx := context.Background()
	service := documents(t)

	if _, err := service.StartUpload(ctx, amaraID, "image/svg+xml", 1024, 1); !errors.Is(err, candidate.ErrDocumentType) {
		t.Fatalf("svg = %v, want ErrDocumentType", err)
	}
	if _, err := service.StartUpload(ctx, amaraID, "application/pdf", candidate.MaxDocumentBytes+1, 1); !errors.Is(err, candidate.ErrDocumentTooLarge) {
		t.Fatalf("oversized = %v, want ErrDocumentTooLarge", err)
	}
	if _, err := service.StartUpload(ctx, amaraID, "application/pdf", 1024, 9); !errors.Is(err, candidate.ErrDocumentParts) {
		t.Fatalf("nine parts = %v, want ErrDocumentParts", err)
	}
}

func TestAnotherPersonsDocumentsAreInvisibleEvenByID(t *testing.T) {
	ctx := context.Background()
	service := documents(t)
	stored := uploadCV(t, service, amaraID, []byte("%PDF-1.7 private"))

	// Priya, by id: the owner scope makes the row not exist for her.
	if err := service.Delete(ctx, priyaID, stored.ID); !errors.Is(err, candidate.ErrDocumentNotFound) {
		t.Fatalf("cross-owner delete = %v, want ErrDocumentNotFound", err)
	}
	if _, err := service.CompleteUpload(ctx, priyaID, stored.ID, "u", "d", nil, 1); !errors.Is(err, candidate.ErrDocumentNotFound) {
		t.Fatalf("cross-owner complete = %v, want ErrDocumentNotFound", err)
	}
}
