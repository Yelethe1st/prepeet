package candidate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/candidate/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// The extraction workflow: from an uploaded CV to span-linked facts.
//
// Implements the pipeline half of PRO-03. The document was announced by
// candidate.document_uploaded.v1; this workflow reads it through the
// intelligence plane and stores what was read as proposals the candidate
// confirms or corrects in PRO-04. Three properties carry the ticket:
// every fact stores the exact source span it came from, unparseable text is
// stored rather than dropped, and no outcome here blocks the profile - a
// format extraction cannot read leaves extraction_state saying so and the
// journey continues manually.
//
// The exactly-once shape matches PLT-06's: activities are idempotent against
// the document's own extraction_state guard and the wholesale replacement of
// proposed facts, never against delivery promises.

// ExtractionTaskQueue is where the candidate context's workflows run,
// following the queue-per-bounded-context naming the first workflow set.
const ExtractionTaskQueue = "prepeet-candidate"

// UnsupportedDocumentCode is the taxonomy code the contract uses for a format
// the extractor cannot honestly read. It selects the unsupported outcome
// rather than the failed one, because "we do not read PDFs yet" is a fact
// about the product, not a fault in the pipeline.
const UnsupportedDocumentCode = "FAILURE_CODE_UNASSESSABLE_INPUT"

// Extractor is what the workflow asks of the intelligence plane, declared
// here per ADR-0005 and wired to CTR-02's stubs in cmd. Identifiers and a
// digest in, facts out; the document's bytes never cross this port.
type Extractor interface {
	Extract(ctx context.Context, request ExtractRequest) ([]ExtractedFact, error)
}

// ExtractRequest names the pinned document extraction reads.
type ExtractRequest struct {
	DocumentID  string
	CandidateID string
	StorageKey  string
	MediaType   string
	// SHA256 is the digest recorded at upload completion. It pins which
	// bytes this extraction is of: the capability verifies what it fetched
	// against it, so a re-upload racing an extraction cannot produce facts
	// of neither version.
	SHA256 string
}

// ExtractedFact is one thing the document asserted, with its provenance.
type ExtractedFact struct {
	Kind       string
	Value      json.RawMessage
	SpanStart  int
	SpanEnd    int
	Confidence float64
	// ExtractorVersion names the reading that produced the fact, so a later
	// extractor can supersede it knowingly.
	ExtractorVersion string
}

// ExtractFailure is a typed refusal from the extractor, with the retry
// decision read from the RPC contract's own descriptor by the adapter.
type ExtractFailure struct {
	Code      string
	Retryable bool
	Message   string
}

func (f *ExtractFailure) Error() string {
	return fmt.Sprintf("candidate: extraction failed: %s: %s", f.Code, f.Message)
}

// ExtractionInput is the workflow's argument: the event's identifiers,
// nothing read ahead. The digest is deliberately absent - the activity reads
// it from the document row at execution time, because the row is the
// authoritative record and an event cannot go stale against it.
type ExtractionInput struct {
	DocumentID  string
	CandidateID string
}

// ExtractionActivities is the workflow's side of the world.
type ExtractionActivities struct {
	store     *Store
	extractor Extractor
}

// NewExtractionActivities wires the store and the extractor for a worker.
func NewExtractionActivities(store *Store, extractor Extractor) *ExtractionActivities {
	return &ExtractionActivities{store: store, extractor: extractor}
}

// Extract reads the document row and asks the intelligence plane to read the
// document. Safe to re-run: extraction is pinned to the row's digest, and
// extract-1 is deterministic over the same bytes.
func (a *ExtractionActivities) Extract(ctx context.Context, input ExtractionInput) ([]ExtractedFact, error) {
	row, err := a.document(ctx, input)
	if err != nil {
		return nil, err
	}
	if row.ExtractionState != "pending" && row.ExtractionState != "extracted" {
		// Somebody already decided - or the upload never completed. Refusing
		// non-retryably keeps a stale replay from resurrecting an outcome.
		return nil, temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("document %s extraction is %s, not pending", input.DocumentID, row.ExtractionState),
			"EXTRACTION_NOT_PENDING", nil)
	}

	facts, err := a.extractor.Extract(ctx, ExtractRequest{
		DocumentID:  input.DocumentID,
		CandidateID: input.CandidateID,
		StorageKey:  row.StorageKey,
		MediaType:   row.MediaType,
		SHA256:      row.Sha256,
	})
	if err != nil {
		var failure *ExtractFailure
		if errors.As(err, &failure) && !failure.Retryable {
			// Non-retryable by the contract's own declaration; the code
			// rides the typed error to the outcome handler.
			return nil, temporal.NewNonRetryableApplicationError(
				failure.Message, failure.Code, failure)
		}
		return nil, err
	}
	return facts, nil
}

// StoreFacts records the reading and marks the document extracted, in one
// transaction under the candidate's own row scope.
//
// Idempotent by replacement: proposals for the document are replaced
// wholesale, so a replay after a worker death converges on the same rows.
// Facts the candidate has already confirmed or corrected are left alone.
func (a *ExtractionActivities) StoreFacts(ctx context.Context, input ExtractionInput, facts []ExtractedFact) error {
	tx, err := a.store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("candidate: beginning fact store: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, input.CandidateID); err != nil {
		return err
	}
	q := db.New(tx)

	if err := q.DeleteProposedFacts(ctx, input.DocumentID); err != nil {
		return fmt.Errorf("candidate: clearing proposals: %w", err)
	}
	for _, fact := range facts {
		if err := q.InsertFact(ctx, db.InsertFactParams{
			ID: id.New().String(), UserID: input.CandidateID, DocumentID: input.DocumentID,
			Kind: fact.Kind, Value: []byte(fact.Value),
			SpanStart: int32(fact.SpanStart), SpanEnd: int32(fact.SpanEnd),
			Confidence: fact.Confidence, ExtractorVersion: fact.ExtractorVersion,
		}); err != nil {
			return fmt.Errorf("candidate: storing a fact: %w", err)
		}
	}

	moved, err := q.SetDocumentExtractionState(ctx, db.SetDocumentExtractionStateParams{
		ID: input.DocumentID, State: "extracted",
	})
	if err != nil {
		return fmt.Errorf("candidate: marking extracted: %w", err)
	}
	if moved == 0 {
		return ErrDocumentState
	}
	return tx.Commit(ctx)
}

// MarkExtractionOutcome records unsupported or failed. Its own retries in the
// workflow, because a document stuck saying pending forever is the invisible
// kind of failure PRO-03 forbids.
func (a *ExtractionActivities) MarkExtractionOutcome(ctx context.Context, input ExtractionInput, state string) error {
	tx, err := a.store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("candidate: beginning outcome: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, input.CandidateID); err != nil {
		return err
	}

	moved, err := db.New(tx).SetDocumentExtractionState(ctx, db.SetDocumentExtractionStateParams{
		ID: input.DocumentID, State: state,
	})
	if err != nil {
		return fmt.Errorf("candidate: marking %s: %w", state, err)
	}
	if moved == 0 {
		return ErrDocumentState
	}
	return tx.Commit(ctx)
}

// document reads the row under the candidate's own scope.
func (a *ExtractionActivities) document(ctx context.Context, input ExtractionInput) (db.GetDocumentRow, error) {
	tx, err := a.store.pool.Begin(ctx)
	if err != nil {
		return db.GetDocumentRow{}, fmt.Errorf("candidate: beginning read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, input.CandidateID); err != nil {
		return db.GetDocumentRow{}, err
	}
	row, err := db.New(tx).GetDocument(ctx, input.DocumentID)
	if err != nil {
		return db.GetDocumentRow{}, fmt.Errorf("candidate: reading document: %w", err)
	}
	return row, nil
}

// ExtractionWorkflow drives one document from pending to extracted,
// unsupported or failed.
//
// Started with "extract-" plus the document id as its workflow identity, so a
// redelivered document_uploaded event joins the running extraction instead of
// starting a second one - the outbox promises at-least-once, and this is
// where the surplus goes to die.
func ExtractionWorkflow(ctx workflow.Context, input ExtractionInput) error {
	options := workflow.ActivityOptions{
		// Extraction fetches the document and reads it; the ceiling matches
		// the RPC contract's 60s with room for the fetch.
		StartToCloseTimeout: 90 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    5,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, options)

	var activities *ExtractionActivities

	var facts []ExtractedFact
	extractErr := workflow.ExecuteActivity(ctx, activities.Extract, input).Get(ctx, &facts)
	if extractErr == nil {
		return workflow.ExecuteActivity(ctx, activities.StoreFacts, input, facts).Get(ctx, nil)
	}

	code := "EXTRACTION_FAILED"
	var applicationErr *temporal.ApplicationError
	if errors.As(extractErr, &applicationErr) && applicationErr.Type() != "" {
		code = applicationErr.Type()
	}

	if code == "EXTRACTION_NOT_PENDING" {
		// A redelivered event reached a document whose extraction is already
		// decided. The decision stands; the surplus delivery ends quietly.
		return nil
	}

	if code == UnsupportedDocumentCode {
		// Not a failure: the honest answer to a format extract-1 cannot
		// read. The state says unsupported, the workflow completes, and the
		// candidate's journey never noticed.
		return workflow.ExecuteActivity(ctx, activities.MarkExtractionOutcome, input, "unsupported").Get(ctx, nil)
	}

	// Out of attempts or refused outright. Recording the failure gets its own
	// retries: a document stuck in pending is a failure nobody can see.
	if err := workflow.ExecuteActivity(ctx, activities.MarkExtractionOutcome, input, "failed").Get(ctx, nil); err != nil {
		return err
	}
	return extractErr
}
