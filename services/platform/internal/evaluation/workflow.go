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
	Extract(ctx context.Context, ref SessionRef) (Extraction, error)
}

// Extraction is what the intelligence plane answered, with what it cost.
type Extraction struct {
	Sealed         SealedInput
	Spans          []Span
	Contradictions []Contradiction
	CostUnits      int
}

// RubricSource answers the rubric exactly as the session's bundle pinned
// it - reference, version, digest and the digest-addressed body - wired in
// cmd from the bundle and the registry. Never the currently published
// version: the pin is the whole point.
type RubricSource interface {
	PinnedRubric(ctx context.Context, ref SessionRef) (RubricPin, error)
	// PinnedPolicy answers the model policy the same bundle pinned, so
	// what a stage may spend is the session's own answer and not the
	// registry's current one (ADR-0019, EVL-07).
	PinnedPolicy(ctx context.Context, ref SessionRef) (PolicyPin, error)
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
	rubrics   RubricSource
}

// NewActivities wires the store, the outbox, the extractor and the rubric
// source.
func NewActivities(store *Store, events *outbox.Store, extractor Extractor, rubrics RubricSource) *Activities {
	return &Activities{store: store, events: events, extractor: extractor, rubrics: rubrics}
}

// ExtractAndStore runs the whole stage: fetch, extract, validate, replace.
//
// Safe to re-run after a worker death: the extractor is deterministic over
// the sealed input, validation is pure, and Replace converges by wholesale
// replacement per extraction version.
// ExtractOutcome carries what aggregation needs from extraction. The
// sealed input's competencies ride the workflow (identifiers and short
// names, not transcript content), so aggregation never refetches.
type ExtractOutcome struct {
	ExtractionVersion string
	Sealed            SealedInput
	// CostUnits is what the capability reported spending, recorded
	// against the stage's budget.
	CostUnits int
}

func (a *Activities) ExtractAndStore(ctx context.Context, input EvidenceInput) (ExtractOutcome, error) {
	ref := SessionRef{
		SessionID: input.SessionID, Mode: input.Mode,
		CandidateID: input.CandidateID, TenantID: input.TenantID,
	}

	extracted, err := a.extractor.Extract(ctx, ref)
	if err != nil {
		// The stage's own record, so an operator can see which stage
		// failed and whether it is worth retrying (EVL-07).
		var failure *ExtractFailure
		if errors.As(err, &failure) {
			_ = a.store.RecordStage(ctx, ref, StageOutcome{
				Stage: StageEvidence, Status: "failed", Reason: failure.Code,
				Retryable: failure.Retryable, Required: true,
			})
			if !failure.Retryable {
				return ExtractOutcome{}, temporal.NewNonRetryableApplicationError(failure.Message, failure.Code, failure)
			}
		}
		return ExtractOutcome{}, err
	}
	sealed, spans, pairs := extracted.Sealed, extracted.Spans, extracted.Contradictions

	// The honesty gate: whoever extracted, a span that does not resolve to
	// the sealed input never lands, and the whole batch is refused with it.
	if err := Validate(sealed, spans); err != nil {
		return ExtractOutcome{}, temporal.NewNonRetryableApplicationError(
			err.Error(), "FAILURE_CODE_SCHEMA_VALIDATION_FAILED", err)
	}
	// Contradiction pairs pass the same gate: each side must be an exact
	// slice of a real candidate turn, or the whole batch refuses.
	if err := ValidateContradictions(sealed, pairs); err != nil {
		return ExtractOutcome{}, temporal.NewNonRetryableApplicationError(
			err.Error(), "FAILURE_CODE_SCHEMA_VALIDATION_FAILED", err)
	}

	version := "evidence-unknown"
	if len(spans) > 0 {
		version = spans[0].ExtractionVersion
	}
	if len(spans) == 0 && len(pairs) > 0 {
		version = pairs[0].ExtractionVersion
	}
	if err := a.store.Replace(ctx, ref, version, spans, pairs); err != nil {
		return ExtractOutcome{}, err
	}
	if err := a.store.RecordStage(ctx, ref, StageOutcome{
		Stage: StageEvidence, Status: "completed", Required: true,
		CostUnits: extracted.CostUnits,
	}); err != nil {
		return ExtractOutcome{}, err
	}

	// Turn text stays out of the workflow history: only the competency
	// list travels, which aggregation needs and ADR-0007's payload rule
	// permits.
	return ExtractOutcome{
		ExtractionVersion: version,
		Sealed:            SealedInput{SessionID: sealed.SessionID, Competencies: sealed.Competencies},
		CostUnits:         extracted.CostUnits,
	}, nil
}

// Aggregate runs aggregate-1 over the stored evidence against the PINNED
// rubric and persists the result with its notification, exactly once.
//
// Safe to re-run: the aggregation is a pure function, the rubric arrives
// by digest, and StoreResult converges on the session's unique result.
func (a *Activities) Aggregate(ctx context.Context, input EvidenceInput, extractionVersion string, sealed SealedInput) error {
	ref := SessionRef{
		SessionID: input.SessionID, Mode: input.Mode,
		CandidateID: input.CandidateID, TenantID: input.TenantID,
	}

	pin, err := a.rubrics.PinnedRubric(ctx, ref)
	if err != nil {
		var failure *ExtractFailure
		if errors.As(err, &failure) && !failure.Retryable {
			return temporal.NewNonRetryableApplicationError(failure.Message, failure.Code, failure)
		}
		return err
	}
	rubric, err := ParseRubric(pin.Body)
	if err != nil {
		// A pinned rubric that does not parse is a publication bug the
		// validating state should have refused; surface, never guess.
		return temporal.NewNonRetryableApplicationError(
			err.Error(), "FAILURE_CODE_SCHEMA_VALIDATION_FAILED", err)
	}

	spans, err := a.store.List(ctx, ref)
	if err != nil {
		return err
	}

	aggregation := Aggregate(rubric, sealed.Competencies, spans)

	// EVL-05's publication gate: recompute from a FRESH read of the store
	// and refuse on any difference. Deliberately not the same slice the
	// aggregation consumed, so a dangling reference or a store mutation
	// between the two is caught, and so the gate stays meaningful when a
	// model replaces aggregate-1 behind this contract.
	reread, err := a.store.List(ctx, ref)
	if err != nil {
		return err
	}
	if err := ValidatePublication(rubric, sealed.Competencies, reread, aggregation); err != nil {
		return temporal.NewNonRetryableApplicationError(
			err.Error(), "FAILURE_CODE_SCHEMA_VALIDATION_FAILED", err)
	}

	warnings := []string{}
	if aggregation.CoveredCompetencies == 0 && aggregation.TotalCompetencies > 0 {
		warnings = append(warnings, "NO_COMPETENCY_EVIDENCED")
	}
	if _, err := a.store.StoreResult(ctx, a.events, ref, pin, extractionVersion, aggregation, warnings); err != nil {
		return err
	}
	// Aggregation is pure Go and spends nothing; the record still stands
	// so the stage's standing is answerable rather than inferred from the
	// result's existence.
	return a.store.RecordStage(ctx, ref, StageOutcome{
		Stage: StageAggregation, Status: "completed", Required: true,
	})
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
	var extracted ExtractOutcome
	err := workflow.ExecuteActivity(ctx, activities.ExtractAndStore, input).Get(ctx, &extracted)
	if err == nil {
		err = workflow.ExecuteActivity(ctx, activities.Aggregate, input,
			extracted.ExtractionVersion, extracted.Sealed).Get(ctx, nil)
	}
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
