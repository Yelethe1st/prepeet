package objectstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// maxParts bounds a multipart upload. S3's own limit is 10,000, and a request
// for more than that is a bug or an attempt to make the API do work on the
// caller's behalf.
const maxParts = 10_000

// ErrNotFound is returned when an object does not exist.
var ErrNotFound = errors.New("objectstore: object not found")

// IsNotFound reports whether err means the object is absent, as opposed to the
// store being unreachable. A caller needs the difference: a missing recording
// is a product state that the completion screen already handles, while an
// unreachable store is an incident.
func IsNotFound(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		return api.ErrorCode() == "NotFound" || api.ErrorCode() == "NoSuchKey"
	}
	return false
}

// S3Config configures the adapter.
//
// The same struct serves MinIO locally and S3 in eu-west-2, per ADR-0001. Only
// Endpoint, credentials and UsePathStyle differ between them, which is what
// keeps this package free of a local versus deployed branch.
type S3Config struct {
	// Endpoint overrides the AWS endpoint. Empty means real S3.
	Endpoint string
	Region   string
	Bucket   string
	// AccessKey and SecretKey are for local development only. Deployed
	// environments leave them empty and the SDK resolves a workload identity,
	// which is what PLT-07 provides.
	AccessKey string
	SecretKey string
	// UsePathStyle addresses buckets as a path rather than a subdomain. MinIO
	// needs it; S3 does not.
	UsePathStyle bool
}

// S3Store stores objects in any S3 compatible service.
type S3Store struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
	region  string
}

// NewS3Store builds an adapter from config.
func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("objectstore: bucket is required")
	}

	options := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
		// The SDK defaults to adding a checksum header to every request and
		// asking for checksum validation on every response. Both are signed
		// into a presigned URL, so a browser that does not send the matching
		// header gets a signature mismatch rather than the object. Requesting
		// checksums only where the operation requires them keeps presigned URLs
		// usable by an ordinary HTTP client, which is the entire point of them.
		awsconfig.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
		awsconfig.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
	}
	if cfg.AccessKey != "" {
		options = append(options, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("objectstore: loading AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	})

	return &S3Store{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  cfg.Bucket,
		region:  cfg.Region,
	}, nil
}

// EnsureBucket creates the bucket if it is absent.
//
// This exists for local development and tests. Deployed buckets are created by
// Terraform with versioning, encryption and lifecycle rules that application
// code has no business setting, and the deployed workload identity is not
// permitted to create a bucket at all.
func (s *S3Store) EnsureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}
	if !IsNotFound(err) {
		return fmt.Errorf("objectstore: checking bucket: %w", err)
	}
	input := &s3.CreateBucketInput{Bucket: aws.String(s.bucket)}
	// Every region except us-east-1 requires an explicit location constraint.
	// MinIO tolerates its absence and real S3 does not, which is precisely the
	// kind of divergence that makes an emulator-only test suite misleading.
	if s.region != "" && s.region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(s.region),
		}
	}
	if _, err := s.client.CreateBucket(ctx, input); err != nil {
		return fmt.Errorf("objectstore: creating bucket: %w", err)
	}
	return nil
}

// InitiateRequest starts an upload.
type InitiateRequest struct {
	Key         Key
	PartCount   int
	TTL         time.Duration
	ContentType string
}

// Upload is an in-progress multipart upload.
type Upload struct {
	UploadID string
	// PartURLs are presigned PUT URLs, one per part, in part order. The browser
	// uploads directly to these and never holds a durable credential.
	PartURLs  []string
	ExpiresAt time.Time
}

// CompletedPart identifies one uploaded part.
type CompletedPart struct {
	Number int
	ETag   string
}

// CompleteRequest finalises an upload.
type CompleteRequest struct {
	Key      Key
	UploadID string
	Parts    []CompletedPart
	// SizeBytes is what the client says it sent. It is verified against what
	// was actually stored, because an evaluation run over a truncated recording
	// would be presented to a candidate as their answer.
	SizeBytes int64
	// SHA256 is the client's digest of the whole object, recorded for the
	// evidence trail. It is stored rather than recomputed here: recomputing
	// would mean downloading the object again on the request path.
	SHA256 string
}

// Object is a stored object.
type Object struct {
	Key       string
	SizeBytes int64
	ETag      string
	SHA256    string
	StoredAt  time.Time
}

// IncompleteUpload is a multipart upload that was started and never finished.
type IncompleteUpload struct {
	Key       string
	UploadID  string
	StartedAt time.Time
}

// InitiateUpload starts a multipart upload and presigns a PUT URL per part.
func (s *S3Store) InitiateUpload(ctx context.Context, req InitiateRequest) (Upload, error) {
	if req.PartCount < 1 || req.PartCount > maxParts {
		return Upload{}, fmt.Errorf("objectstore: part count %d is outside 1..%d", req.PartCount, maxParts)
	}

	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(req.Key.String()),
	}
	if req.ContentType != "" {
		input.ContentType = aws.String(req.ContentType)
	}

	created, err := s.client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return Upload{}, fmt.Errorf("objectstore: creating multipart upload: %w", err)
	}

	ttl := ClampTTL(req.TTL)
	urls := make([]string, 0, req.PartCount)
	for part := 1; part <= req.PartCount; part++ {
		signed, err := s.presign.PresignUploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(s.bucket),
			Key:        aws.String(req.Key.String()),
			UploadId:   created.UploadId,
			PartNumber: aws.Int32(int32(part)),
		}, s3.WithPresignExpires(ttl))
		if err != nil {
			// Leaving a created upload behind would bill the tenant for parts
			// nobody can finish, so it is abandoned before returning.
			_ = s.AbortUpload(ctx, req.Key, aws.ToString(created.UploadId))
			return Upload{}, fmt.Errorf("objectstore: presigning part %d: %w", part, err)
		}
		urls = append(urls, signed.URL)
	}

	return Upload{
		UploadID:  aws.ToString(created.UploadId),
		PartURLs:  urls,
		ExpiresAt: time.Now().Add(ttl),
	}, nil
}

// CompleteUpload finalises a multipart upload and verifies what was stored.
//
// Verification is the point of this method. S3 will happily assemble whatever
// parts arrived, and an interview that lost its connection mid-upload produces
// a shorter object than the client intended to send. Evaluating that as if it
// were the candidate's full answer would misrepresent them, so a mismatch fails
// here and the session records missing media instead.
func (s *S3Store) CompleteUpload(ctx context.Context, req CompleteRequest) (Object, error) {
	if len(req.Parts) == 0 {
		return Object{}, errors.New("objectstore: at least one part is required")
	}

	parts := make([]types.CompletedPart, 0, len(req.Parts))
	for _, part := range req.Parts {
		parts = append(parts, types.CompletedPart{
			PartNumber: aws.Int32(int32(part.Number)),
			ETag:       aws.String(part.ETag),
		})
	}

	if _, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(s.bucket),
		Key:             aws.String(req.Key.String()),
		UploadId:        aws.String(req.UploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	}); err != nil {
		return Object{}, fmt.Errorf("objectstore: completing upload: %w", err)
	}

	stored, err := s.Head(ctx, req.Key)
	if err != nil {
		return Object{}, fmt.Errorf("objectstore: verifying completed upload: %w", err)
	}

	if req.SizeBytes > 0 && stored.SizeBytes != req.SizeBytes {
		return Object{}, fmt.Errorf(
			"objectstore: stored %d bytes but the client reported %d: the upload is incomplete",
			stored.SizeBytes, req.SizeBytes)
	}

	stored.SHA256 = req.SHA256
	return stored, nil
}

// AbortUpload discards an incomplete multipart upload and its parts.
func (s *S3Store) AbortUpload(ctx context.Context, key Key, uploadID string) error {
	if _, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key.String()),
		UploadId: aws.String(uploadID),
	}); err != nil {
		return fmt.Errorf("objectstore: aborting upload: %w", err)
	}
	return nil
}

// PresignPlayback returns a time-bound URL for reading one object.
//
// The lifetime is clamped rather than trusted. Authorization to call this is
// decided by the policy layer against the active tenant; this method assumes
// that has already happened and only bounds the blast radius if the URL leaks.
func (s *S3Store) PresignPlayback(ctx context.Context, key Key, ttl time.Duration) (string, error) {
	signed, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key.String()),
	}, s3.WithPresignExpires(ClampTTL(ttl)))
	if err != nil {
		return "", fmt.Errorf("objectstore: presigning playback: %w", err)
	}
	return signed.URL, nil
}

// Head reports what is stored under key without fetching it.
// Delete removes one object.
//
// Deleting a missing object succeeds, because the caller's intent - this key
// holds nothing - is already true, and a retry after a half-failure must not
// be told it failed for having succeeded. The authoritative record of what
// existed is the database row, which deletion never touches.
// Put writes one server-side object. The upload paths stay browser-direct;
// this exists for artifacts the server itself produces, such as the sealed
// evaluation input, where a presigned round trip would add a hop for bytes
// already in hand. Idempotent: the same key and bytes overwrite in place.
func (s *S3Store) Put(ctx context.Context, key Key, body []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key.String()),
		Body:        bytes.NewReader(body),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("objectstore: putting %s: %w", key, err)
	}
	return nil
}

// Fetch reads one object whole, for server-side consumers that need the
// bytes rather than a grant. Bounded by the same ceiling uploads have.
func (s *S3Store) Fetch(ctx context.Context, key Key) ([]byte, error) {
	response, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key.String()),
	})
	if err != nil {
		return nil, fmt.Errorf("objectstore: fetching %s: %w", key, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxObjectBytes+1))
	if err != nil {
		return nil, fmt.Errorf("objectstore: reading %s: %w", key, err)
	}
	if len(body) > maxObjectBytes {
		return nil, fmt.Errorf("objectstore: %s exceeds the size ceiling", key)
	}
	return body, nil
}

// maxObjectBytes bounds a server-side fetch; nothing this reads is media.
const maxObjectBytes = 32 << 20

func (s *S3Store) Delete(ctx context.Context, key Key) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key.String()),
	})
	if err != nil {
		return fmt.Errorf("objectstore: deleting %s: %w", key, err)
	}
	return nil
}

func (s *S3Store) Head(ctx context.Context, key Key) (Object, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key.String()),
	})
	if err != nil {
		if IsNotFound(err) {
			return Object{}, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return Object{}, fmt.Errorf("objectstore: heading object: %w", err)
	}

	object := Object{
		Key:       key.String(),
		SizeBytes: aws.ToInt64(out.ContentLength),
		ETag:      aws.ToString(out.ETag),
	}
	if out.LastModified != nil {
		object.StoredAt = *out.LastModified
	}
	return object, nil
}

// ListIncompleteUploads returns multipart uploads under prefix that were never
// completed.
//
// These are invisible in an ordinary object listing and are billed, so
// reconciliation has to look for them explicitly. An interview that ended in a
// device failure leaves exactly this behind.
//
// The prefix is what scopes the listing to one tenant and session. Callers must
// build it with Prefix rather than concatenating strings, or the scoping is
// only as good as the caller's care.
func (s *S3Store) ListIncompleteUploads(ctx context.Context, prefix string) ([]IncompleteUpload, error) {
	if prefix == "" {
		return nil, errors.New("objectstore: prefix is required: an unscoped listing crosses tenants")
	}

	var (
		found      []IncompleteUpload
		keyMarker  *string
		uploadMark *string
	)
	for {
		out, err := s.client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{
			Bucket:         aws.String(s.bucket),
			Prefix:         aws.String(prefix),
			KeyMarker:      keyMarker,
			UploadIdMarker: uploadMark,
		})
		if err != nil {
			return nil, fmt.Errorf("objectstore: listing incomplete uploads: %w", err)
		}

		for _, upload := range out.Uploads {
			incomplete := IncompleteUpload{
				Key:      aws.ToString(upload.Key),
				UploadID: aws.ToString(upload.UploadId),
			}
			if upload.Initiated != nil {
				incomplete.StartedAt = *upload.Initiated
			}
			found = append(found, incomplete)
		}

		if !aws.ToBool(out.IsTruncated) {
			return found, nil
		}
		keyMarker, uploadMark = out.NextKeyMarker, out.NextUploadIdMarker
	}
}
