//go:build integration

// Integration tests for the S3 adapter, run against LocalStack.
//
// These exist because an emulator is where this product is developed but not
// where it runs. Multipart upload, presigned URLs and their expiry are exactly
// where an emulator and real S3 diverge, so they are exercised against a real
// S3 API implementation rather than a hand written fake, which would pass
// whatever the adapter happened to do.
//
// LocalStack rather than MinIO, measured rather than assumed. Running this same
// suite against both showed MinIO returning nothing from ListMultipartUploads,
// which makes the orphan reconciliation PLT-05 requires untestable, and MinIO
// accepting a CreateBucket without a location constraint that real S3 rejects.
// LocalStack caught both. See ADR-0001.
//
// Testcontainers gives each run its own instance with empty buckets, rather
// than a shared stack accumulating state across weeks of development.
//
// Run with: make test-integration
package objectstore_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tclocalstack "github.com/testcontainers/testcontainers-go/modules/localstack"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
)

const (
	// Pinned, so a release changing presigned URL behaviour is a deliberate
	// upgrade rather than a mystery failure one morning.
	localstackImage = "localstack/localstack:3.8"

	testBucket  = "prepeet-media"
	testSession = "ses_7Kq2XA"
)

// LocalStack is started once for the package rather than once per test.
// A container per test is affordable with a small image and is not with this
// one: ten starting at once exhausted the machine and every test timed out
// waiting for readiness. Tests stay isolated by using their own tenant prefix,
// which is a truer reflection of production anyway, where one bucket holds
// every tenant's objects and the prefix is what separates them.
var sharedStore *objectstore.S3Store

func TestMain(m *testing.M) {
	ctx := context.Background()

	// Signature validation is off by default in LocalStack, which would let an
	// expired presigned URL keep working and make the expiry test pass without
	// proving anything.
	//
	// The startup deadline is raised from the module's own sixty seconds.
	// LocalStack takes about fifty on a warm machine and more on a cold one,
	// so the default sits close enough to the real time that the suite fails
	// intermittently with "context deadline exceeded" and a container that was
	// about to be ready.
	//
	// A second cause is worth writing down because the symptom is identical
	// and the conclusion is not. Under a full run this failed three times with
	// the deadline coming from the Docker socket rather than from LocalStack:
	// "get state: ... context deadline exceeded", with the daemon saturated.
	// It looked like a property of the suite. It was not: each of those runs
	// had a web test suite or a second Go run alongside it on the same
	// machine, and the full suite on an idle machine passes every package. CI
	// gives the Go job a runner to itself, so this is a local-development
	// characteristic rather than a pipeline risk, and the remedy is to run one
	// suite at a time rather than to change the suite.
	container, err := tclocalstack.Run(ctx, localstackImage,
		testcontainers.WithEnv(map[string]string{"S3_SKIP_SIGNATURE_VALIDATION": "0"}),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/_localstack/health").
				WithPort("4566/tcp").
				WithStartupTimeout(4*time.Minute)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting LocalStack: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "terminating LocalStack: %v\n", err)
		}
	}()

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LocalStack host: %v\n", err)
		os.Exit(1)
	}
	port, err := container.MappedPort(ctx, "4566/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "LocalStack port: %v\n", err)
		os.Exit(1)
	}

	sharedStore, err = objectstore.NewS3Store(ctx, objectstore.S3Config{
		Endpoint:     "http://" + host + ":" + port.Port(),
		Region:       "eu-west-2",
		Bucket:       testBucket,
		AccessKey:    "test",
		SecretKey:    "test",
		UsePathStyle: true, // an emulator addresses buckets by path, not by subdomain
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "building store: %v\n", err)
		os.Exit(1)
	}
	if err := sharedStore.EnsureBucket(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "creating bucket: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	// defer does not run after os.Exit, so terminate explicitly.
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating LocalStack: %v\n", err)
	}
	os.Exit(code)
}

// newStore returns the shared store. Each test scopes itself by tenant.
func newStore(t *testing.T) *objectstore.S3Store {
	t.Helper()
	return sharedStore
}

// tenantFor gives each test its own tenant, so tests sharing one bucket cannot
// see each other's objects any more than two real tenants can.
func tenantFor(t *testing.T) string {
	t.Helper()
	return "tn_" + strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, t.Name())
}

func testKey(t *testing.T, name string) objectstore.Key {
	t.Helper()
	key, err := objectstore.NewKey(objectstore.KeyParts{
		TenantID:  tenantFor(t),
		SessionID: testSession,
		Purpose:   objectstore.PurposeMedia,
		Name:      name,
	})
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}
	return key
}

// put uploads bytes to a presigned URL exactly as a browser would, using no
// credentials of its own. If this works with a plain HTTP client, it works from
// a browser that never holds a durable credential.
func put(t *testing.T, url string, body []byte) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("building PUT: %v", err)
	}
	req.ContentLength = int64(len(body))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT to presigned URL: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(res.Body)
		t.Fatalf("PUT returned %d: %s", res.StatusCode, detail)
	}
	return strings.Trim(res.Header.Get("ETag"), `"`)
}

// A session's audio arrives in parts during the interview and is finalised at
// the end. The browser uploads directly with short lived authorization and
// never holds a durable credential.
func TestMultipartUploadRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	key := testKey(t, "candidate-audio.opus")

	// S3 requires every part except the last to be at least 5 MiB.
	partOne := bytes.Repeat([]byte("a"), 5*1024*1024)
	partTwo := []byte("the final, shorter part")
	whole := append(append([]byte{}, partOne...), partTwo...)
	digest := sha256.Sum256(whole)

	upload, err := store.InitiateUpload(ctx, objectstore.InitiateRequest{
		Key:         key,
		PartCount:   2,
		TTL:         5 * time.Minute,
		ContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("InitiateUpload: %v", err)
	}
	if len(upload.PartURLs) != 2 {
		t.Fatalf("PartURLs = %d, want 2", len(upload.PartURLs))
	}

	parts := []objectstore.CompletedPart{
		{Number: 1, ETag: put(t, upload.PartURLs[0], partOne)},
		{Number: 2, ETag: put(t, upload.PartURLs[1], partTwo)},
	}

	object, err := store.CompleteUpload(ctx, objectstore.CompleteRequest{
		Key:       key,
		UploadID:  upload.UploadID,
		Parts:     parts,
		SHA256:    hex.EncodeToString(digest[:]),
		SizeBytes: int64(len(whole)),
	})
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	if object.SizeBytes != int64(len(whole)) {
		t.Errorf("SizeBytes = %d, want %d", object.SizeBytes, len(whole))
	}
}

// A finalised object whose bytes do not match what the client said it sent is
// not evidence. Accepting it would let a corrupted recording be evaluated as if
// it were the candidate's answer.
func TestCompleteRejectsASizeMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	key := testKey(t, "truncated.opus")

	body := bytes.Repeat([]byte("b"), 5*1024*1024)
	upload, err := store.InitiateUpload(ctx, objectstore.InitiateRequest{
		Key: key, PartCount: 1, TTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("InitiateUpload: %v", err)
	}
	etag := put(t, upload.PartURLs[0], body)

	_, err = store.CompleteUpload(ctx, objectstore.CompleteRequest{
		Key:       key,
		UploadID:  upload.UploadID,
		Parts:     []objectstore.CompletedPart{{Number: 1, ETag: etag}},
		SizeBytes: int64(len(body)) + 1024, // the client claims more than it sent
	})
	if err == nil {
		t.Fatal("CompleteUpload accepted a size mismatch, want it rejected")
	}
}

func TestPresignedPlaybackReturnsTheStoredBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	key := testKey(t, "playback.opus")

	body := bytes.Repeat([]byte("c"), 5*1024*1024)
	upload, err := store.InitiateUpload(ctx, objectstore.InitiateRequest{Key: key, PartCount: 1, TTL: time.Minute})
	if err != nil {
		t.Fatalf("InitiateUpload: %v", err)
	}
	etag := put(t, upload.PartURLs[0], body)
	if _, err := store.CompleteUpload(ctx, objectstore.CompleteRequest{
		Key: key, UploadID: upload.UploadID,
		Parts:     []objectstore.CompletedPart{{Number: 1, ETag: etag}},
		SizeBytes: int64(len(body)),
	}); err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}

	url, err := store.PresignPlayback(ctx, key, 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignPlayback: %v", err)
	}

	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET presigned URL: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET returned %d, want 200", res.StatusCode)
	}
	got, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("body length = %d, want %d", len(got), len(body))
	}
}

// The whole point of a short lifetime is that the URL stops working. If expiry
// were not enforced by the storage layer, a leaked URL would grant permanent
// access to a candidate's recording.
func TestPresignedPlaybackStopsWorkingAfterItExpires(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	key := testKey(t, "expiring.opus")

	body := bytes.Repeat([]byte("d"), 5*1024*1024)
	upload, err := store.InitiateUpload(ctx, objectstore.InitiateRequest{Key: key, PartCount: 1, TTL: time.Minute})
	if err != nil {
		t.Fatalf("InitiateUpload: %v", err)
	}
	etag := put(t, upload.PartURLs[0], body)
	if _, err := store.CompleteUpload(ctx, objectstore.CompleteRequest{
		Key: key, UploadID: upload.UploadID,
		Parts:     []objectstore.CompletedPart{{Number: 1, ETag: etag}},
		SizeBytes: int64(len(body)),
	}); err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}

	// ClampTTL floors this at MinPresignTTL, so the wait is bounded and short.
	url, err := store.PresignPlayback(ctx, key, time.Nanosecond)
	if err != nil {
		t.Fatalf("PresignPlayback: %v", err)
	}

	time.Sleep(objectstore.MinPresignTTL + 2*time.Second)

	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET presigned URL: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		t.Errorf("GET returned 200 after expiry, want it refused")
	}
}

// An interview that ends badly leaves a multipart upload that was never
// completed. Those parts are billed and are invisible in a normal listing, so
// reconciliation has to find them.
func TestIncompleteUploadsAreDiscoverableAndAbortable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	key := testKey(t, "abandoned.opus")

	upload, err := store.InitiateUpload(ctx, objectstore.InitiateRequest{Key: key, PartCount: 1, TTL: time.Minute})
	if err != nil {
		t.Fatalf("InitiateUpload: %v", err)
	}
	put(t, upload.PartURLs[0], bytes.Repeat([]byte("e"), 5*1024*1024))

	prefix, err := objectstore.Prefix(tenantFor(t), testSession, objectstore.PurposeMedia)
	if err != nil {
		t.Fatalf("Prefix: %v", err)
	}

	incomplete, err := store.ListIncompleteUploads(ctx, prefix)
	if err != nil {
		t.Fatalf("ListIncompleteUploads: %v", err)
	}
	if len(incomplete) != 1 {
		t.Fatalf("ListIncompleteUploads = %d, want 1", len(incomplete))
	}
	if incomplete[0].UploadID != upload.UploadID {
		t.Errorf("UploadID = %q, want %q", incomplete[0].UploadID, upload.UploadID)
	}

	if err := store.AbortUpload(ctx, key, upload.UploadID); err != nil {
		t.Fatalf("AbortUpload: %v", err)
	}

	remaining, err := store.ListIncompleteUploads(ctx, prefix)
	if err != nil {
		t.Fatalf("ListIncompleteUploads after abort: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("ListIncompleteUploads after abort = %d, want 0", len(remaining))
	}
}

// One tenant's prefix must not be reachable through another tenant's
// reconciliation listing, even though both live in the same bucket.
func TestListingIsScopedToOneTenantPrefix(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)

	mine, other := tenantFor(t)+"-mine", tenantFor(t)+"-other"
	for _, tenant := range []string{mine, other} {
		key, err := objectstore.NewKey(objectstore.KeyParts{
			TenantID: tenant, SessionID: testSession,
			Purpose: objectstore.PurposeMedia, Name: "audio.opus",
		})
		if err != nil {
			t.Fatalf("NewKey: %v", err)
		}
		if _, err := store.InitiateUpload(ctx, objectstore.InitiateRequest{Key: key, PartCount: 1, TTL: time.Minute}); err != nil {
			t.Fatalf("InitiateUpload for %s: %v", tenant, err)
		}
	}

	prefix, err := objectstore.Prefix(mine, testSession, objectstore.PurposeMedia)
	if err != nil {
		t.Fatalf("Prefix: %v", err)
	}

	incomplete, err := store.ListIncompleteUploads(ctx, prefix)
	if err != nil {
		t.Fatalf("ListIncompleteUploads: %v", err)
	}

	if len(incomplete) != 1 {
		t.Fatalf("ListIncompleteUploads = %d, want only this tenant's upload", len(incomplete))
	}
	if !strings.Contains(incomplete[0].Key, mine) {
		t.Errorf("listing returned key %q, want only %s", incomplete[0].Key, mine)
	}
	if strings.Contains(incomplete[0].Key, other) {
		t.Error("listing crossed into another tenant's prefix")
	}
}

func TestHeadReportsWhatWasStored(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	key := testKey(t, "headed.opus")

	body := bytes.Repeat([]byte("f"), 5*1024*1024)
	upload, err := store.InitiateUpload(ctx, objectstore.InitiateRequest{
		Key: key, PartCount: 1, TTL: time.Minute, ContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("InitiateUpload: %v", err)
	}
	etag := put(t, upload.PartURLs[0], body)
	if _, err := store.CompleteUpload(ctx, objectstore.CompleteRequest{
		Key: key, UploadID: upload.UploadID,
		Parts:     []objectstore.CompletedPart{{Number: 1, ETag: etag}},
		SizeBytes: int64(len(body)),
	}); err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}

	object, err := store.Head(ctx, key)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if object.SizeBytes != int64(len(body)) {
		t.Errorf("SizeBytes = %d, want %d", object.SizeBytes, len(body))
	}
}

func TestHeadReportsAMissingObjectDistinctly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)

	_, err := store.Head(ctx, testKey(t, "never-uploaded.opus"))

	if err == nil {
		t.Fatal("Head returned no error for a missing object, want one")
	}
	if !objectstore.IsNotFound(err) {
		t.Errorf("IsNotFound(%v) = false, want true so a caller can tell missing from broken", err)
	}
}

func TestPresigningAMissingObjectStillProducesAURLThatFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)

	// Presigning does not contact S3, so it cannot know the object is missing.
	// What matters is that the resulting URL does not succeed.
	url, err := store.PresignPlayback(ctx, testKey(t, "absent.opus"), time.Minute)
	if err != nil {
		t.Fatalf("PresignPlayback: %v", err)
	}

	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET presigned URL: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		t.Error("GET returned 200 for an object that was never uploaded")
	}
}

func TestInitiateRejectsAnUnreasonablePartCount(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	key := testKey(t, "silly.opus")

	for _, count := range []int{0, -1, 100_000} {
		t.Run(fmt.Sprintf("count %d", count), func(t *testing.T) {
			if _, err := store.InitiateUpload(ctx, objectstore.InitiateRequest{
				Key: key, PartCount: count, TTL: time.Minute,
			}); err == nil {
				t.Errorf("InitiateUpload accepted PartCount %d, want it rejected", count)
			}
		})
	}
}

// The server-side object path: what the platform itself writes and reads
// back, as distinct from the presigned path the browser uses. RTC-05 and
// the sealed evaluation input both depend on it, and the reconciliation
// it supports is only as good as the digest it computes from the stored
// bytes rather than from anyone's claim about them.

func TestPutFetchAndStatReadTheBytesBackExactly(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	key, err := objectstore.NewKey(objectstore.KeyParts{
		TenantID: tenantFor(t), SessionID: "ses-1",
		Purpose: objectstore.PurposeTranscript, Name: "evaluation-input.json",
	})
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	body := []byte(`{"session_id":"ses-1","turns":[]}`)

	if err := store.Put(ctx, key, body, "application/json"); err != nil {
		t.Fatalf("put: %v", err)
	}
	fetched, err := store.Fetch(ctx, key)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !bytes.Equal(fetched, body) {
		t.Fatalf("fetched %q", fetched)
	}

	size, digest, err := store.StatDigest(ctx, key)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	sum := sha256.Sum256(body)
	if digest != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatalf("digest = %q", digest)
	}
	if size != int64(len(body)) {
		t.Fatalf("size = %d, want %d", size, len(body))
	}

	// Writing the same key again is the idempotent retry completion
	// depends on: same bytes, same digest, no second object.
	if err := store.Put(ctx, key, body, "application/json"); err != nil {
		t.Fatalf("put again: %v", err)
	}
	_, again, err := store.StatDigest(ctx, key)
	if err != nil {
		t.Fatalf("stat again: %v", err)
	}
	if again != digest {
		t.Fatalf("the retry changed the digest: %q then %q", digest, again)
	}
}

func TestAnAbsentObjectIsNotFoundRatherThanEmpty(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	key, err := objectstore.NewKey(objectstore.KeyParts{
		TenantID: tenantFor(t), SessionID: "ses-2",
		Purpose: objectstore.PurposeMedia, Name: "candidate.webm",
	})
	if err != nil {
		t.Fatalf("key: %v", err)
	}

	// Reconciliation asks this question of every track: an absent
	// artifact must fail, because zero bytes read as a silent recording.
	if _, _, err := store.StatDigest(ctx, key); err == nil {
		t.Fatal("statting an absent object reported success")
	}
	if _, err := store.Fetch(ctx, key); err == nil {
		t.Fatal("fetching an absent object reported success")
	}
}

func TestDeleteRemovesTheObjectAndIsSafeToRepeat(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	key, err := objectstore.NewKey(objectstore.KeyParts{
		TenantID: tenantFor(t), SessionID: "ses-3",
		Purpose: objectstore.PurposeDocument, Name: "cv.pdf",
	})
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	if err := store.Put(ctx, key, []byte("%PDF-1.7"), "application/pdf"); err != nil {
		t.Fatalf("put: %v", err)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Fetch(ctx, key); err == nil {
		t.Fatal("the object survived deletion")
	}
	// The record outlives the object, so deleting twice must not error.
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}
