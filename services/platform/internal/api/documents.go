package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// The document surface: PRO-02 at the HTTP boundary. Owner scoping is the
// session, as everywhere under /me; the browser carries the bytes itself
// against presigned URLs and the server never proxies a file.

// CandidateDocuments is what the API needs from the document flows,
// declared here per ADR-0005 and wired in cmd.
type CandidateDocuments interface {
	StartUpload(ctx context.Context, userID, mediaType string, sizeBytes int64, partCount int) (StartedUpload, error)
	CompleteUpload(ctx context.Context, userID, documentID, uploadID, sha256 string, parts []UploadPart, sizeBytes int64) (Document, error)
	AbortUpload(ctx context.Context, userID, documentID string) error
	DeleteDocument(ctx context.Context, userID, documentID string) error
	ListDocuments(ctx context.Context, userID string) ([]Document, error)
	ListFacts(ctx context.Context, userID, documentID string) ([]Fact, error)
	ReviewFact(ctx context.Context, userID, factID, status string, corrected json.RawMessage) (Fact, error)
}

// Document mirrors the contract's Document at the port.
type Document struct {
	ID              string
	Kind            string
	Version         int
	MediaType       string
	SizeBytes       int64
	State           string
	SHA256          string
	ExtractionState string
	CreatedAt       time.Time
	StoredAt        *time.Time
	DeletedAt       *time.Time
}

// Fact mirrors the contract's Fact at the port: the extraction, its
// provenance, and the candidate's review of it.
type Fact struct {
	ID               string
	DocumentID       string
	Kind             string
	Value            json.RawMessage
	CorrectedValue   json.RawMessage
	SpanStart        int
	SpanEnd          int
	Confidence       float64
	ExtractorVersion string
	Status           string
	CreatedAt        time.Time
	ReviewedAt       *time.Time
}

// StartedUpload is what the browser needs to carry the bytes.
type StartedUpload struct {
	Document  Document
	UploadID  string
	PartURLs  []string
	ExpiresAt time.Time
}

// UploadPart identifies one uploaded part on completion.
type UploadPart struct {
	Number int
	ETag   string
}

// Document refusals the port maps onto responses.
var (
	// ErrDocumentMissing covers absence and somebody else's document alike.
	ErrDocumentMissing = errors.New("api: no such document")
	// ErrDocumentConflict means the document is not in a state the operation
	// applies to; nothing changed.
	ErrDocumentConflict = errors.New("api: that document is not in a state this operation applies to")
	// ErrFactMissing covers absence and somebody else's fact alike.
	ErrFactMissing = errors.New("api: no such fact")
)

// documents handles the /me/documents operations.
type documents struct {
	authentication *authentication
	flows          CandidateDocuments
}

// ListDocuments answers the version history, states included.
func (d *documents) ListDocuments(ctx context.Context, _ prepeetapi.ListDocumentsRequestObject) (prepeetapi.ListDocumentsResponseObject, error) {
	principal, refused := d.authenticated(ctx)
	if refused != nil {
		return *refused, nil
	}

	stored, err := d.flows.ListDocuments(ctx, principal.UserID)
	if err != nil {
		return d.authentication.failed(ctx, err), nil
	}
	body := prepeetapi.DocumentList{Documents: make([]prepeetapi.Document, 0, len(stored))}
	for _, document := range stored {
		encoded, err := documentBody(document)
		if err != nil {
			return d.authentication.failed(ctx, err), nil
		}
		body.Documents = append(body.Documents, encoded)
	}
	return prepeetapi.ListDocuments200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ListDocuments200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// StartDocumentUpload allocates the version and presigns the upload.
func (d *documents) StartDocumentUpload(ctx context.Context, request prepeetapi.StartDocumentUploadRequestObject) (prepeetapi.StartDocumentUploadResponseObject, error) {
	principal, refused := d.authenticated(ctx)
	if refused != nil {
		return *refused, nil
	}

	started, err := d.flows.StartUpload(ctx, principal.UserID,
		string(request.Body.MediaType), request.Body.SizeBytes, request.Body.PartCount)
	if err != nil {
		return d.authentication.failed(ctx, err), nil
	}

	document, err := documentBody(started.Document)
	if err != nil {
		return d.authentication.failed(ctx, err), nil
	}
	return prepeetapi.StartDocumentUpload201JSONResponse{
		Body: prepeetapi.StartedUpload{
			Document:  document,
			UploadID:  started.UploadID,
			PartUrls:  started.PartURLs,
			ExpiresAt: started.ExpiresAt,
		},
		Headers: prepeetapi.StartDocumentUpload201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// CompleteDocumentUpload finalises the object and records the digest.
func (d *documents) CompleteDocumentUpload(ctx context.Context, request prepeetapi.CompleteDocumentUploadRequestObject) (prepeetapi.CompleteDocumentUploadResponseObject, error) {
	principal, refused := d.authenticated(ctx)
	if refused != nil {
		return *refused, nil
	}

	parts := make([]UploadPart, 0, len(request.Body.Parts))
	for _, part := range request.Body.Parts {
		parts = append(parts, UploadPart{Number: part.Number, ETag: part.Etag})
	}

	stored, err := d.flows.CompleteUpload(ctx, principal.UserID,
		request.DocumentID.String(), request.Body.UploadID, request.Body.Sha256,
		parts, request.Body.SizeBytes)
	if err != nil {
		return d.authentication.failed(ctx, err), nil
	}
	document, err := documentBody(stored)
	if err != nil {
		return d.authentication.failed(ctx, err), nil
	}
	return prepeetapi.CompleteDocumentUpload200JSONResponse{
		Body:    document,
		Headers: prepeetapi.CompleteDocumentUpload200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// AbortDocumentUpload abandons a stalled upload into its failed state.
func (d *documents) AbortDocumentUpload(ctx context.Context, request prepeetapi.AbortDocumentUploadRequestObject) (prepeetapi.AbortDocumentUploadResponseObject, error) {
	principal, refused := d.authenticated(ctx)
	if refused != nil {
		return *refused, nil
	}

	if err := d.flows.AbortUpload(ctx, principal.UserID, request.DocumentID.String()); err != nil {
		return d.authentication.failed(ctx, err), nil
	}
	return prepeetapi.AbortDocumentUpload204Response{
		Headers: prepeetapi.AbortDocumentUpload204ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// DeleteDocument destroys the bytes and keeps the record.
func (d *documents) DeleteDocument(ctx context.Context, request prepeetapi.DeleteDocumentRequestObject) (prepeetapi.DeleteDocumentResponseObject, error) {
	principal, refused := d.authenticated(ctx)
	if refused != nil {
		return *refused, nil
	}

	if err := d.flows.DeleteDocument(ctx, principal.UserID, request.DocumentID.String()); err != nil {
		return d.authentication.failed(ctx, err), nil
	}
	return prepeetapi.DeleteDocument204Response{
		Headers: prepeetapi.DeleteDocument204ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ListDocumentFacts answers one document's extraction, spans included.
func (d *documents) ListDocumentFacts(ctx context.Context, request prepeetapi.ListDocumentFactsRequestObject) (prepeetapi.ListDocumentFactsResponseObject, error) {
	principal, refused := d.authenticated(ctx)
	if refused != nil {
		return *refused, nil
	}

	stored, err := d.flows.ListFacts(ctx, principal.UserID, request.DocumentID.String())
	if err != nil {
		return d.authentication.failed(ctx, err), nil
	}
	body := prepeetapi.FactList{Facts: make([]prepeetapi.Fact, 0, len(stored))}
	for _, fact := range stored {
		encoded, err := factBody(fact)
		if err != nil {
			return d.authentication.failed(ctx, err), nil
		}
		body.Facts = append(body.Facts, encoded)
	}
	return prepeetapi.ListDocumentFacts200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ListDocumentFacts200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ReviewFact records the candidate's confirm, correct or reject.
func (d *documents) ReviewFact(ctx context.Context, request prepeetapi.ReviewFactRequestObject) (prepeetapi.ReviewFactResponseObject, error) {
	principal, refused := d.authenticated(ctx)
	if refused != nil {
		return *refused, nil
	}

	var corrected json.RawMessage
	if request.Body.CorrectedValue != nil {
		encoded, err := json.Marshal(request.Body.CorrectedValue)
		if err != nil {
			return d.authentication.failed(ctx, err), nil
		}
		corrected = encoded
	}

	fact, err := d.flows.ReviewFact(ctx, principal.UserID, request.FactID.String(),
		string(request.Body.Status), corrected)
	if err != nil {
		return d.authentication.failed(ctx, err), nil
	}
	encoded, err := factBody(fact)
	if err != nil {
		return d.authentication.failed(ctx, err), nil
	}
	return prepeetapi.ReviewFact200JSONResponse{
		Body:    encoded,
		Headers: prepeetapi.ReviewFact200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

func (d *documents) authenticated(ctx context.Context) (Principal, *failure) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		refused := d.authentication.rejectedSession(ctx)
		return Principal{}, &refused
	}
	principal, err := d.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		refused := d.authentication.failed(ctx, err)
		return Principal{}, &refused
	}
	return principal, nil
}

func documentBody(document Document) (prepeetapi.Document, error) {
	id, err := uuid.Parse(document.ID)
	if err != nil {
		return prepeetapi.Document{}, err
	}
	body := prepeetapi.Document{
		ID:              id,
		Kind:            prepeetapi.DocumentKind(document.Kind),
		Version:         document.Version,
		MediaType:       document.MediaType,
		SizeBytes:       document.SizeBytes,
		State:           prepeetapi.DocumentState(document.State),
		ExtractionState: prepeetapi.DocumentExtractionState(document.ExtractionState),
		CreatedAt:       document.CreatedAt,
		StoredAt:        document.StoredAt,
		DeletedAt:       document.DeletedAt,
	}
	if document.SHA256 != "" {
		digest := document.SHA256
		body.Sha256 = &digest
	}
	return body, nil
}

// factBody encodes one fact for the wire.
func factBody(fact Fact) (prepeetapi.Fact, error) {
	id, err := uuid.Parse(fact.ID)
	if err != nil {
		return prepeetapi.Fact{}, err
	}
	documentID, err := uuid.Parse(fact.DocumentID)
	if err != nil {
		return prepeetapi.Fact{}, err
	}

	var value map[string]interface{}
	if err := json.Unmarshal(fact.Value, &value); err != nil {
		return prepeetapi.Fact{}, err
	}
	body := prepeetapi.Fact{
		ID: id, DocumentID: documentID,
		Kind:             prepeetapi.FactKind(fact.Kind),
		Value:            value,
		SpanStart:        fact.SpanStart,
		SpanEnd:          fact.SpanEnd,
		Confidence:       float32(fact.Confidence),
		ExtractorVersion: fact.ExtractorVersion,
		Status:           prepeetapi.FactStatus(fact.Status),
		CreatedAt:        fact.CreatedAt,
		ReviewedAt:       fact.ReviewedAt,
	}
	if fact.CorrectedValue != nil {
		var corrected map[string]interface{}
		if err := json.Unmarshal(fact.CorrectedValue, &corrected); err != nil {
			return prepeetapi.Fact{}, err
		}
		body.CorrectedValue = &corrected
	}
	return body, nil
}

// The failure type must speak these operations' responses.
var (
	_ prepeetapi.ListDocumentFactsResponseObject      = failure{}
	_ prepeetapi.ReviewFactResponseObject             = failure{}
	_ prepeetapi.ListDocumentsResponseObject          = failure{}
	_ prepeetapi.StartDocumentUploadResponseObject    = failure{}
	_ prepeetapi.CompleteDocumentUploadResponseObject = failure{}
	_ prepeetapi.AbortDocumentUploadResponseObject    = failure{}
	_ prepeetapi.DeleteDocumentResponseObject         = failure{}
)

func (f failure) VisitListDocumentsResponse(w http.ResponseWriter) error       { return f.write(w) }
func (f failure) VisitStartDocumentUploadResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitCompleteDocumentUploadResponse(w http.ResponseWriter) error {
	return f.write(w)
}
func (f failure) VisitAbortDocumentUploadResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitDeleteDocumentResponse(w http.ResponseWriter) error      { return f.write(w) }
func (f failure) VisitListDocumentFactsResponse(w http.ResponseWriter) error   { return f.write(w) }
func (f failure) VisitReviewFactResponse(w http.ResponseWriter) error          { return f.write(w) }
