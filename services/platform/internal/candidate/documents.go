package candidate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/candidate/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// The CV: uploaded browser-direct against presigned URLs, versioned rather
// than replaced, recorded in the row that outlives its object.
//
// Implements PRO-02. The properties that matter: every upload is a new
// version so nothing rewrites history; the row is the authoritative record
// per data-architecture.md, so a bundle that pinned version 2's digest can
// answer for itself after version 2's object is deleted; and an upload that
// stalls or fails has its own state a person can see and recover from.

// The bounds. A CV is a document, not a dataset.
const (
	// MaxDocumentBytes bounds one CV at 10 MiB, which is generous for the
	// densest PDF and small enough that abuse is boring.
	MaxDocumentBytes = 10 << 20
	// maxUploadParts bounds the multipart split. 10 MiB never needs more.
	maxUploadParts = 4
	// uploadTTL is how long the presigned URLs live: long enough for a slow
	// connection, short enough that a leaked URL goes stale the same hour.
	uploadTTL = 30 * time.Minute
)

// documentMediaTypes is the allowlist. A CV arrives in one of three shapes;
// anything else is refused by name rather than sniffed, because sniffing is
// how an SVG with a script becomes "an image".
var documentMediaTypes = map[string]string{
	"application/pdf": ".pdf",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
	"text/plain": ".txt",
}

// Stable refusals for the document flows.
var (
	ErrDocumentTooLarge = errors.New("candidate: DOCUMENT_TOO_LARGE: a document is at most 10 MiB")
	ErrDocumentType     = errors.New("candidate: DOCUMENT_TYPE_UNSUPPORTED: upload a PDF, Word document or plain text file")
	ErrDocumentParts    = errors.New("candidate: DOCUMENT_PARTS_INVALID: between 1 and 4 upload parts")
	ErrDocumentNotFound = errors.New("candidate: DOCUMENT_NOT_FOUND: no such document")
	ErrDocumentState    = errors.New("candidate: DOCUMENT_STATE_INVALID: that document is not in a state this operation applies to")
)

// Document is one version of one person's document.
type Document struct {
	ID        string
	Kind      string
	Version   int
	MediaType string
	SizeBytes int64
	State     string
	SHA256    string
	// ExtractionState is where PRO-03's reading of this version stands: none
	// until stored, pending while the workflow runs, then extracted,
	// unsupported or failed. Informational always - no extraction outcome
	// blocks the document or the profile.
	ExtractionState string
	CreatedAt       time.Time
	StoredAt        *time.Time
	DeletedAt       *time.Time
}

// StartedUpload is what the browser needs to carry the bytes itself.
type StartedUpload struct {
	Document Document
	UploadID string
	PartURLs []string
	// ExpiresAt is when the URLs stop working; the client shows it rather
	// than discovering it as a mid-upload failure.
	ExpiresAt time.Time
}

// Uploads is what the document flows need from object storage, declared here
// so tests can fake the store while the database half stays real.
type Uploads interface {
	InitiateUpload(ctx context.Context, req objectstore.InitiateRequest) (objectstore.Upload, error)
	CompleteUpload(ctx context.Context, req objectstore.CompleteRequest) (objectstore.Object, error)
	AbortUpload(ctx context.Context, key objectstore.Key, uploadID string) error
	Delete(ctx context.Context, key objectstore.Key) error
}

// Documents is the CV service.
type Documents struct {
	pool    *Store
	uploads Uploads
	events  *outbox.Store
}

// NewDocuments wires the document flows.
//
// The outbox is a direct dependency rather than a port because publication is
// not optional behaviour to substitute: candidate.document_uploaded.v1 leaves
// in the same transaction that marks the document stored, or the document is
// not stored. Tests get the real outbox against the test database.
func NewDocuments(store *Store, uploads Uploads, events *outbox.Store) *Documents {
	return &Documents{pool: store, uploads: uploads, events: events}
}

// StartUpload allocates the next version and presigns the upload.
//
// The row is written first, in uploading state: a crash after presigning
// leaves a visible uploading row a person can abort or supersede, never an
// orphan object nothing accounts for.
func (d *Documents) StartUpload(ctx context.Context, userID, mediaType string, sizeBytes int64, partCount int) (StartedUpload, error) {
	extension, known := documentMediaTypes[mediaType]
	if !known {
		return StartedUpload{}, ErrDocumentType
	}
	if sizeBytes <= 0 || sizeBytes > MaxDocumentBytes {
		return StartedUpload{}, ErrDocumentTooLarge
	}
	if partCount < 1 || partCount > maxUploadParts {
		return StartedUpload{}, ErrDocumentParts
	}

	tx, err := d.pool.pool.Begin(ctx)
	if err != nil {
		return StartedUpload{}, fmt.Errorf("candidate: beginning upload: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return StartedUpload{}, err
	}
	q := db.New(tx)

	version, err := q.NextDocumentVersion(ctx, db.NextDocumentVersionParams{
		UserID: userID, Kind: "cv",
	})
	if err != nil {
		return StartedUpload{}, fmt.Errorf("candidate: numbering the version: %w", err)
	}

	key, err := objectstore.NewCandidateKey(userID, fmt.Sprintf("cv-v%d%s", version, extension))
	if err != nil {
		return StartedUpload{}, err
	}

	upload, err := d.uploads.InitiateUpload(ctx, objectstore.InitiateRequest{
		Key: key, PartCount: partCount, TTL: uploadTTL, ContentType: mediaType,
	})
	if err != nil {
		return StartedUpload{}, fmt.Errorf("candidate: initiating the upload: %w", err)
	}

	documentID := id.New().String()
	if err := q.InsertDocument(ctx, db.InsertDocumentParams{
		ID: documentID, UserID: userID, Kind: "cv", Version: version,
		StorageKey: key.String(), MediaType: mediaType, SizeBytes: sizeBytes,
		UploadID: upload.UploadID,
	}); err != nil {
		// The multipart upload is left for abort-or-expiry; the row it would
		// have described never existed, which is the safe direction.
		return StartedUpload{}, fmt.Errorf("candidate: recording the upload: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return StartedUpload{}, err
	}

	document, err := d.get(ctx, documentID, userID)
	if err != nil {
		return StartedUpload{}, err
	}
	return StartedUpload{
		Document: document, UploadID: upload.UploadID,
		PartURLs: upload.PartURLs, ExpiresAt: upload.ExpiresAt,
	}, nil
}

// CompleteUpload finalises the object and marks the version stored.
//
// The store verifies the size against what actually landed; the digest is
// recorded as the client computed it, because it is the identity extraction
// and composition will pin and a lying client only corrupts their own CV.
func (d *Documents) CompleteUpload(ctx context.Context, userID, documentID, uploadID, sha256 string, parts []objectstore.CompletedPart, sizeBytes int64) (Document, error) {
	document, row, err := d.getRow(ctx, documentID, userID)
	if err != nil {
		return Document{}, err
	}
	if document.State != "uploading" || row.UploadID != uploadID {
		return Document{}, ErrDocumentState
	}

	key, err := objectstore.NewCandidateKey(userID, keyName(row.StorageKey))
	if err != nil {
		return Document{}, err
	}
	object, err := d.uploads.CompleteUpload(ctx, objectstore.CompleteRequest{
		Key: key, UploadID: uploadID, Parts: parts,
		SizeBytes: sizeBytes, SHA256: sha256,
	})
	if err != nil {
		return Document{}, fmt.Errorf("candidate: completing the upload: %w", err)
	}

	if err := d.markStored(ctx, userID, row, object.SHA256, object.SizeBytes); err != nil {
		return Document{}, err
	}
	return d.get(ctx, documentID, userID)
}

// AbortUpload gives a stalled upload its recoverable ending: the multipart
// upload is discarded and the row says failed, visibly, beside whatever
// version supersedes it.
func (d *Documents) AbortUpload(ctx context.Context, userID, documentID string) error {
	document, row, err := d.getRow(ctx, documentID, userID)
	if err != nil {
		return err
	}
	if document.State != "uploading" {
		return ErrDocumentState
	}

	key, err := objectstore.NewCandidateKey(userID, keyName(row.StorageKey))
	if err != nil {
		return err
	}
	if err := d.uploads.AbortUpload(ctx, key, row.UploadID); err != nil {
		return fmt.Errorf("candidate: aborting the upload: %w", err)
	}
	return d.transition(ctx, userID, func(q *db.Queries) (int64, error) {
		return q.MarkDocumentFailed(ctx, documentID)
	})
}

// Delete removes the object and stamps the row deleted.
//
// The row stays. It is the record of what existed - the digest a session
// bundle may have pinned - and PRO-02's rule is precisely that deletion never
// rewrites a session composed from an earlier version. What is destroyed is
// the bytes; what is kept is the account of them.
func (d *Documents) Delete(ctx context.Context, userID, documentID string) error {
	document, row, err := d.getRow(ctx, documentID, userID)
	if err != nil {
		return err
	}
	if document.State != "stored" {
		return ErrDocumentState
	}

	key, err := objectstore.NewCandidateKey(userID, keyName(row.StorageKey))
	if err != nil {
		return err
	}
	if err := d.uploads.Delete(ctx, key); err != nil {
		return err
	}
	return d.transition(ctx, userID, func(q *db.Queries) (int64, error) {
		return q.MarkDocumentDeleted(ctx, documentID)
	})
}

// List returns every version, newest first, states included.
func (d *Documents) List(ctx context.Context, userID string) ([]Document, error) {
	tx, err := d.pool.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("candidate: beginning list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return nil, err
	}

	rows, err := db.New(tx).ListDocuments(ctx, db.ListDocumentsParams{
		UserID: userID, Kind: "cv",
	})
	if err != nil {
		return nil, fmt.Errorf("candidate: listing documents: %w", err)
	}
	documents := make([]Document, 0, len(rows))
	for _, row := range rows {
		documents = append(documents, documentFrom(db.GetDocumentRow(row)))
	}
	return documents, nil
}

// ── the plumbing

func (d *Documents) get(ctx context.Context, documentID, userID string) (Document, error) {
	document, _, err := d.getRow(ctx, documentID, userID)
	return document, err
}

func (d *Documents) getRow(ctx context.Context, documentID, userID string) (Document, db.GetDocumentRow, error) {
	tx, err := d.pool.pool.Begin(ctx)
	if err != nil {
		return Document{}, db.GetDocumentRow{}, fmt.Errorf("candidate: beginning read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return Document{}, db.GetDocumentRow{}, err
	}

	row, err := db.New(tx).GetDocument(ctx, documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, db.GetDocumentRow{}, ErrDocumentNotFound
	}
	if err != nil {
		return Document{}, db.GetDocumentRow{}, fmt.Errorf("candidate: reading document: %w", err)
	}
	return documentFrom(row), row, nil
}

// markStored records the digest and publishes the fact, atomically.
//
// The event rides the same transaction as the state change: if the outbox
// refuses the event, the document is not stored, because a stored document
// whose upload was never announced is a CV extraction silently never reads.
func (d *Documents) markStored(ctx context.Context, userID string, row db.GetDocumentRow, sha256 string, sizeBytes int64) error {
	tx, err := d.pool.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("candidate: beginning store: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return err
	}

	moved, err := db.New(tx).MarkDocumentStored(ctx, db.MarkDocumentStoredParams{
		ID: row.ID, Sha256: sha256, SizeBytes: sizeBytes,
	})
	if err != nil {
		return fmt.Errorf("candidate: marking stored: %w", err)
	}
	if moved == 0 {
		return ErrDocumentState
	}

	payload, err := json.Marshal(map[string]any{
		"document_id":  row.ID,
		"candidate_id": userID,
		"storage_key":  row.StorageKey,
		"media_type":   row.MediaType,
		"byte_size":    sizeBytes,
	})
	if err != nil {
		return fmt.Errorf("candidate: encoding the uploaded event: %w", err)
	}
	if _, err := d.events.Publish(ctx, tx, outbox.Event{
		Type:          "candidate.document_uploaded.v1",
		SchemaVersion: "1.0",
		// No tenant: the CV belongs to the person, per ADR-0002.
		Producer: "candidate",
		Actor:    outbox.Actor{Type: "user", ID: userID},
		Purpose:  "profile",
		Payload:  payload,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *Documents) transition(ctx context.Context, userID string, change func(*db.Queries) (int64, error)) error {
	tx, err := d.pool.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("candidate: beginning transition: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return err
	}

	moved, err := change(db.New(tx))
	if err != nil {
		return fmt.Errorf("candidate: transitioning document: %w", err)
	}
	if moved == 0 {
		return ErrDocumentState
	}
	return tx.Commit(ctx)
}

// keyName strips the key back to its object name for rebuilding a typed Key.
func keyName(storageKey string) string {
	for i := len(storageKey) - 1; i >= 0; i-- {
		if storageKey[i] == '/' {
			return storageKey[i+1:]
		}
	}
	return storageKey
}

func documentFrom(row db.GetDocumentRow) Document {
	return Document{
		ID: row.ID, Kind: row.Kind, Version: int(row.Version),
		MediaType: row.MediaType, SizeBytes: row.SizeBytes, State: row.State,
		SHA256: row.Sha256, ExtractionState: row.ExtractionState,
		CreatedAt: row.CreatedAt,
		StoredAt:  row.StoredAt, DeletedAt: row.DeletedAt,
	}
}
