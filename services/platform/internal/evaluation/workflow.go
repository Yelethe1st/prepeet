package evaluation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// The evidence workflow: EVL-01's durable stage.
//
// One activity does the whole read - fetch through the sealed input's own
// digest, extract, validate against the input, store by replacement - so
// no transcript content ever crosses the workflow boundary (ADR-0007's
// payload rule) and a worker death retries into identical rows. Fabricated
// evidence is a non-retryable refusal: retrying a liar produces the same
// lie, and the failure event is what surfaces it.

// TaskQueue is where the evaluation context's workflows run.
const TaskQueue = "prepeet-evaluation"

// Extractor is what the workflow asks of the intelligence plane, declared
// here per ADR-0005 and wired in cmd: the sealed input document and the
// spans the capability read from it.
type Extractor interface {
	Extract(ctx context.Context, ref SessionRef) (SealedInput, []Span, error)
}

// ExtractFailure is a typed refusal with the contract's own retry decision.
type ExtractFailure struct {
	Code      string
	Retryable bool
	Message   string
}

func (f *ExtractFailure) Error() string {
	return fmt.Sprintf("evaluation: evidence extraction failed: %s: %s", f.Code, f.Message)
}

// EvidenceInput is the workflow's argument: identifiers only.
type EvidenceInput struct {
	SessionID   string
	Mode        string
	CandidateID string
	TenantID    string
}

// Activities is the workflow's side of the world.
type Activities struct {
	store     *Store
	events    *outbox.Store
	extractor Extractor
}

// NewActivities wires the store, the outbox and the extractor.
func NewActivities(store *Store, events *outbox.Store, extractor Extractor) *Activities {
	return &Activities{store: store, events: events, extractor: extractor}
}

// ExtractAndStore runs the whole stage: fetch, extract, validate, replace.
//
// Safe to re-run after a worker death: the extractor is deterministic over
// the sealed input, validation is pure, and Replace converges by wholesale
// replacement per extraction version.
func (a *Activities) ExtractAndStore(ctx context.Context, input EvidenceInput) (int, error) {
	ref := SessionRef{
		SessionID: input.SessionID, Mode: input.Mode,
		CandidateID: input.CandidateID, TenantID: input.TenantID,
	}

	sealed, spans, err := a.extractor.Extract(ctx, ref)
	if err != nil {
		var failure *ExtractFailure
		if errors.As(err, &failure) && !failure.Retryable {
			return 0, temporal.NewNonRetryableApplicationError(failure.Message, failure.Code, failure)
		}
		return 0, err
	}

	// The honesty gate: whoever extracted, a span that does not resolve to
	// the sealed input never lands, and the whole batch is refused with it.
	if err := Validate(sealed, spans); err != nil {
		return 0, temporal.NewNonRetryableApplicationError(
			err.Error(), "FAILURE_CODE_SCHEMA_VALIDATION_FAILED", err)
	}

	version := "evidence-unknown"
	if len(spans) > 0 {
		version = spans[0].ExtractionVersion
	}
	if err := a.store.Replace(ctx, ref, version, spans); err != nil {
		return 0, err
	}
	return len(spans), nil
}

// PublishFailed records the stage's failure as the catalogue's event, so
// the session's owner context can move the state machine without this one
// importing it.
func (a *Activities) PublishFailed(ctx context.Context, input EvidenceInput, code string) error {
	payload := []byte(fmt.Sprintf(
		`{"evaluation_id":%q,"session_id":%q,"failure":%q,"retryable":false}`,
		id.New().String(), input.SessionID, code))

	tx, err := a.store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("evaluation: beginning failure publish: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := a.events.Publish(ctx, tx, outbox.Event{
		Type:          "evaluation.failed.v1",
		SchemaVersion: "1.0",
		TenantID:      input.TenantID,
		Producer:      "evaluation",
		Actor:         outbox.Actor{Type: "service", ID: input.CandidateID},
		Purpose:       input.Mode,
		Payload:       payload,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// EvidenceWorkflow drives one sealed session through evidence extraction.
func EvidenceWorkflow(ctx workflow.Context, input EvidenceInput) error {
	options := workflow.ActivityOptions{
		StartToCloseTimeout: 90 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    5,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, options)

	var activities *Activities
	var stored int
	err := workflow.ExecuteActivity(ctx, activities.ExtractAndStore, input).Get(ctx, &stored)
	if err == nil {
		return nil
	}

	code := "EVIDENCE_EXTRACTION_FAILED"
	var applicationErr *temporal.ApplicationError
	if errors.As(err, &applicationErr) && applicationErr.Type() != "" {
		code = applicationErr.Type()
	}
	if publishErr := workflow.ExecuteActivity(ctx, activities.PublishFailed, input, code).Get(ctx, nil); publishErr != nil {
		return publishErr
	}
	return err
}
