//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tclocalstack "github.com/testcontainers/testcontainers-go/modules/localstack"

	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
	"github.com/Yelethe1st/prepeet/services/platform/platform/grpcdial"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
)

// PRO-03 across the whole seam: Go's adapter presigns a real object in a real
// bucket, Python fetches it through that grant, verifies the digest itself,
// and the claims come back as span-linked facts. This is the acceptance path
// - a text CV in, exact spans out - with nothing faked between the planes.

// e2eCV is the document; the spans asserted below index into these bytes.
const e2eCV = `Amara Osei

Senior Backend Engineer, Northwind Health
Mar 2020 - Present

Skills:
Go, PostgreSQL
`

const e2eCandidate = "00000000-0000-7000-8000-0000000000e1"

// startObjectStore boots LocalStack and stores the CV where the storage key
// says it lives, through the same multipart path the browser uses.
func startObjectStore(t *testing.T) (*objectstore.S3Store, string, string) {
	t.Helper()
	ctx := context.Background()

	container, err := tclocalstack.Run(ctx, "localstack/localstack:3.8",
		testcontainers.WithEnv(map[string]string{"S3_SKIP_SIGNATURE_VALIDATION": "0"}))
	if err != nil {
		t.Fatalf("starting LocalStack: %v", err)
	}
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

	key, err := objectstore.NewCandidateKey(e2eCandidate, "cv-v1.txt")
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	body := []byte(e2eCV)
	upload, err := store.InitiateUpload(ctx, objectstore.InitiateRequest{
		Key: key, PartCount: 1, TTL: 5 * time.Minute, ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, upload.PartURLs[0], bytes.NewReader(body))
	if err != nil {
		t.Fatalf("building PUT: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("uploading: %v", err)
	}
	defer response.Body.Close()
	digest := sha256.Sum256(body)
	if _, err := store.CompleteUpload(ctx, objectstore.CompleteRequest{
		Key: key, UploadID: upload.UploadID,
		Parts:     []objectstore.CompletedPart{{Number: 1, ETag: response.Header.Get("ETag")}},
		SizeBytes: int64(len(body)), SHA256: hex.EncodeToString(digest[:]),
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	return store, key.String(), hex.EncodeToString(digest[:])
}

func TestGoExtractsThroughPython(t *testing.T) {
	address := startIntelligence(t)
	store, storageKey, digest := startObjectStore(t)

	// The composer's dial is reused: one channel to the plane, two
	// capabilities on it, exactly as the worker wires it.
	_, conn, err := newComposer(address, grpcdial.Config{Insecure: true}, registry)
	if err != nil {
		t.Fatalf("dialling: %v", err)
	}
	defer conn.Close()
	adapter := newExtractor(conn, store)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	facts, err := adapter.Extract(ctx, candidate.ExtractRequest{
		DocumentID:  "doc_e2e_1",
		CandidateID: e2eCandidate,
		StorageKey:  storageKey,
		MediaType:   "text/plain",
		SHA256:      digest, // bare hex, exactly as the document row records it
	})
	if err != nil {
		t.Fatalf("Extract across the wire: %v", err)
	}

	// The acceptance criterion, end to end: every span indexes back into the
	// uploaded bytes and yields the text the fact claims to come from.
	kinds := map[string]bool{}
	for _, fact := range facts {
		kinds[fact.Kind] = true
		if fact.SpanStart < 0 || fact.SpanEnd > len(e2eCV) || fact.SpanEnd <= fact.SpanStart {
			t.Fatalf("fact %q carries span %d-%d outside the document", fact.Kind, fact.SpanStart, fact.SpanEnd)
		}
		if fact.ExtractorVersion != "extract-1" {
			t.Fatalf("extractor version = %q", fact.ExtractorVersion)
		}
		if fact.Kind == "skill" {
			var value struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(fact.Value, &value); err != nil {
				t.Fatalf("decoding: %v", err)
			}
			if got := e2eCV[fact.SpanStart:fact.SpanEnd]; got != value.Name {
				t.Fatalf("skill span reads %q, value says %q; the provenance lies", got, value.Name)
			}
		}
	}
	for _, required := range []string{"role", "date_range", "skill"} {
		if !kinds[required] {
			t.Fatalf("no %s fact came back; kinds = %v", required, kinds)
		}
	}

	// The digest pin, across languages: lie about which bytes were wanted and
	// Python's own verification refuses with the contract's code.
	_, err = adapter.Extract(ctx, candidate.ExtractRequest{
		DocumentID: "doc_e2e_2", CandidateID: e2eCandidate,
		StorageKey: storageKey, MediaType: "text/plain",
		SHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	var failure *candidate.ExtractFailure
	if !errors.As(err, &failure) || failure.Code != "FAILURE_CODE_ARTIFACT_NOT_FOUND" {
		t.Fatalf("a digest mismatch = %v, want ARTIFACT_NOT_FOUND", err)
	}

	// And the degradation path: a format extract-1 cannot read refuses with
	// the code the workflow maps to unsupported, declared non-retryable by
	// the contract itself.
	_, err = adapter.Extract(ctx, candidate.ExtractRequest{
		DocumentID: "doc_e2e_3", CandidateID: e2eCandidate,
		StorageKey: storageKey, MediaType: "application/pdf", SHA256: digest,
	})
	if !errors.As(err, &failure) || failure.Code != candidate.UnsupportedDocumentCode {
		t.Fatalf("a pdf = %v, want %s", err, candidate.UnsupportedDocumentCode)
	}
	if failure.Retryable {
		t.Fatal("UNASSESSABLE_INPUT must be non-retryable by the descriptor")
	}
}
