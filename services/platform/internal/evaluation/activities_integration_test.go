//go:build integration

package evaluation_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.temporal.io/sdk/temporal"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// The activities themselves, against real PostgreSQL: the workflow suite
// mocks them to assert control flow, so what runs here is the bodies -
// what they store, what they refuse, and what a retry does.

type fakeExtractor struct {
	sealed evaluation.SealedInput
	spans  []evaluation.Span
	pairs  []evaluation.Contradiction
	cost   int
	err    error
	calls  int
}

func (f *fakeExtractor) Extract(context.Context, evaluation.SessionRef) (evaluation.Extraction, error) {
	f.calls++
	return evaluation.Extraction{
		Sealed: f.sealed, Spans: f.spans, Contradictions: f.pairs, CostUnits: f.cost,
	}, f.err
}

type fakeRubrics struct {
	pin       evaluation.RubricPin
	policy    evaluation.PolicyPin
	err       error
	policyErr error
}

func (f fakeRubrics) PinnedRubric(context.Context, evaluation.SessionRef) (evaluation.RubricPin, error) {
	return f.pin, f.err
}

func (f fakeRubrics) PinnedPolicy(context.Context, evaluation.SessionRef) (evaluation.PolicyPin, error) {
	return f.policy, f.policyErr
}

type fakeAnalyzer struct {
	analysis evaluation.Analysis
	err      error
}

func (f fakeAnalyzer) Analyze(context.Context, evaluation.SessionRef) (evaluation.Analysis, error) {
	return f.analysis, f.err
}

// honestSession is a sealed input whose spans quote it exactly.
func honestSession(sessionID string) (evaluation.SealedInput, []evaluation.Span) {
	text := "We sharded the checkout and latency fell 40 percent."
	sealed := evaluation.SealedInput{
		SessionID:    sessionID,
		Competencies: []evaluation.Competency{{ID: "systems-design", Name: "Systems design"}},
		Turns: []evaluation.Turn{{
			Sequence: 3, Speaker: "candidate", Text: text, StartMs: 1000, EndMs: 9000,
		}},
	}
	span := evaluation.Span{
		CompetencyID: "systems-design", Kind: "supporting", SegmentSequence: 3,
		Quote: text, CharStart: 0, CharEnd: len(text), StartMs: 1000, EndMs: 9000,
		ExtractionVersion: "evidence-1",
	}
	return sealed, []evaluation.Span{span, span}
}

func activityInput(sessionID string) evaluation.EvidenceInput {
	return evaluation.EvidenceInput{SessionID: sessionID, Mode: "practice", CandidateID: evidenceCandidate}
}

func TestExtractAndStoreValidatesBeforeItStores(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	sessionID := id.New().String()
	sealed, spans := honestSession(sessionID)

	extractor := &fakeExtractor{sealed: sealed, spans: spans}
	activities := evaluation.NewActivities(store, outbox.New(pool), extractor, fakeRubrics{pin: practicePin()})

	outcome, err := activities.ExtractAndStore(ctx, activityInput(sessionID))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if outcome.ExtractionVersion != "evidence-1" || len(outcome.Sealed.Competencies) != 1 {
		t.Fatalf("outcome = %+v", outcome)
	}
	// The turns do not ride the workflow: only what aggregation needs.
	if len(outcome.Sealed.Turns) != 0 {
		t.Fatal("transcript text travelled through the workflow history")
	}

	ref := evaluation.SessionRef{SessionID: sessionID, Mode: "practice", CandidateID: evidenceCandidate}
	stored, err := store.List(ctx, ref)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("%d spans stored", len(stored))
	}

	// Running it again replaces wholesale rather than doubling.
	if _, err := activities.ExtractAndStore(ctx, activityInput(sessionID)); err != nil {
		t.Fatalf("retry: %v", err)
	}
	again, _ := store.List(ctx, ref)
	if len(again) != 2 {
		t.Fatalf("the retry left %d spans", len(again))
	}
}

func TestExtractAndStoreRefusesAFabricationWithoutStoringAnything(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	sessionID := id.New().String()
	sealed, spans := honestSession(sessionID)
	lying := spans[0]
	lying.Quote = "We saved the company two million pounds."

	activities := evaluation.NewActivities(store, outbox.New(pool),
		&fakeExtractor{sealed: sealed, spans: []evaluation.Span{lying}}, fakeRubrics{pin: practicePin()})

	_, err := activities.ExtractAndStore(ctx, activityInput(sessionID))
	if err == nil {
		t.Fatal("a fabricated span was accepted")
	}
	var applicationErr *temporal.ApplicationError
	if !errors.As(err, &applicationErr) || applicationErr.Type() != "FAILURE_CODE_SCHEMA_VALIDATION_FAILED" {
		t.Fatalf("refusal = %v", err)
	}
	if !applicationErr.NonRetryable() {
		t.Fatal("a fabrication is not worth retrying")
	}
	stored, _ := store.List(ctx, evaluation.SessionRef{SessionID: sessionID, Mode: "practice", CandidateID: evidenceCandidate})
	if len(stored) != 0 {
		t.Fatalf("%d spans stored despite the refusal", len(stored))
	}
}

func TestExtractAndStoreCarriesARetryableFailureThrough(t *testing.T) {
	ctx := context.Background()
	activities := evaluation.NewActivities(evaluation.NewStore(pool), outbox.New(pool),
		&fakeExtractor{err: &evaluation.ExtractFailure{
			Code: "FAILURE_CODE_PROVIDER_UNAVAILABLE", Retryable: true, Message: "later",
		}}, fakeRubrics{pin: practicePin()})

	_, err := activities.ExtractAndStore(ctx, activityInput(id.New().String()))
	var applicationErr *temporal.ApplicationError
	if errors.As(err, &applicationErr) && applicationErr.NonRetryable() {
		t.Fatalf("a retryable failure was marked terminal: %v", err)
	}
	if err == nil {
		t.Fatal("the failure was swallowed")
	}
}

func TestAggregateStoresTheResultAndRefusesAnIncoherentRubric(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	sessionID := id.New().String()
	sealed, spans := honestSession(sessionID)
	extractor := &fakeExtractor{sealed: sealed, spans: spans}
	activities := evaluation.NewActivities(store, outbox.New(pool), extractor, fakeRubrics{pin: practicePin()})
	input := activityInput(sessionID)

	outcome, err := activities.ExtractAndStore(ctx, input)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if err := activities.Aggregate(ctx, input, outcome.ExtractionVersion, outcome.Sealed); err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	ref := evaluation.SessionRef{SessionID: sessionID, Mode: "practice", CandidateID: evidenceCandidate}
	result, err := store.ResultOf(ctx, ref)
	if err != nil {
		t.Fatalf("result: %v", err)
	}
	if result.RubricVersion != practicePin().Version || len(result.Aggregation.Competencies) != 1 {
		t.Fatalf("result = %+v", result)
	}
	// Re-running converges on the same stored result.
	if err := activities.Aggregate(ctx, input, outcome.ExtractionVersion, outcome.Sealed); err != nil {
		t.Fatalf("aggregate retry: %v", err)
	}

	// A pinned rubric that cannot judge is a publication bug, refused
	// terminally rather than retried.
	incoherent := evaluation.NewActivities(store, outbox.New(pool), extractor,
		fakeRubrics{pin: evaluation.RubricPin{
			Reference: "rubric/broken", Version: "9", Digest: "sha256:b",
			Body: json.RawMessage(`{"bands":[]}`),
		}})
	err = incoherent.Aggregate(ctx, activityInput(id.New().String()), "evidence-1", sealed)
	var applicationErr *temporal.ApplicationError
	if !errors.As(err, &applicationErr) || !applicationErr.NonRetryable() {
		t.Fatalf("an incoherent rubric = %v", err)
	}

	// And a bundle that pins no rubric at all keeps its own code.
	missing := evaluation.NewActivities(store, outbox.New(pool), extractor,
		fakeRubrics{err: &evaluation.ExtractFailure{
			Code: "FAILURE_CODE_ARTIFACT_NOT_FOUND", Retryable: false, Message: "no rubric",
		}})
	err = missing.Aggregate(ctx, activityInput(id.New().String()), "evidence-1", sealed)
	if !errors.As(err, &applicationErr) || applicationErr.Type() != "FAILURE_CODE_ARTIFACT_NOT_FOUND" {
		t.Fatalf("a missing rubric = %v", err)
	}
}

func TestPublishFailedRecordsTheStagesOwnCode(t *testing.T) {
	ctx := context.Background()
	sessionID := id.New().String()
	activities := evaluation.NewActivities(evaluation.NewStore(pool), outbox.New(pool),
		&fakeExtractor{}, fakeRubrics{pin: practicePin()})

	if err := activities.PublishFailed(ctx, activityInput(sessionID), "FAILURE_CODE_PROVIDER_TIMEOUT"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	var code string
	if err := pool.QueryRow(ctx, `
		SELECT payload->>'failure' FROM integration.outbox
		WHERE event_type = 'evaluation.failed.v1' AND payload->>'session_id' = $1`,
		sessionID).Scan(&code); err != nil {
		t.Fatalf("reading the event: %v", err)
	}
	if code != "FAILURE_CODE_PROVIDER_TIMEOUT" {
		t.Fatalf("failure code = %q", code)
	}
}

func TestAnalyzeAndStoreKeepsDeliveryInItsOwnRow(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	sessionID := id.New().String()
	input := activityInput(sessionID)

	activities := evaluation.NewArticulationActivities(store, fakeAnalyzer{analysis: evaluation.Analysis{
		Status: "partially_assessable", Warnings: []string{"INSUFFICIENT_SPEECH"},
		Document:           json.RawMessage(`{"metrics":{"words_per_minute":120}}`),
		CalculationVersion: "articulation-features-v1", PolicyVersion: "articulation-practice-v1",
		InputDigest: "sha256:input",
	}})

	status, err := activities.AnalyzeAndStore(ctx, input)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if status != "partially_assessable" {
		t.Fatalf("status = %q", status)
	}
	stored, err := store.ArticulationOf(ctx, evaluation.SessionRef{
		SessionID: sessionID, Mode: "practice", CandidateID: evidenceCandidate,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if stored.CalculationVersion != "articulation-features-v1" || len(stored.Warnings) != 1 {
		t.Fatalf("stored = %+v", stored)
	}

	// A terminal refusal from the plane stays this workflow's own.
	refusing := evaluation.NewArticulationActivities(store, fakeAnalyzer{
		err: &evaluation.ExtractFailure{Code: "FAILURE_CODE_UNASSESSABLE_INPUT", Retryable: false, Message: "no speech"},
	})
	_, err = refusing.AnalyzeAndStore(ctx, activityInput(id.New().String()))
	var applicationErr *temporal.ApplicationError
	if !errors.As(err, &applicationErr) || !applicationErr.NonRetryable() {
		t.Fatalf("refusal = %v", err)
	}
	if !strings.Contains(err.Error(), "no speech") {
		t.Fatalf("the refusal lost its words: %v", err)
	}
}

// EVL-07 against real PostgreSQL: an optional stage failing or being
// priced out leaves the core evaluation exactly as it was, and the reason
// is recorded in a form an operator and a candidate can each act on.

func practicePolicy() evaluation.PolicyPin {
	return evaluation.PolicyPin{
		Reference: evaluation.PolicyReference, Version: "1.0.0", Digest: "sha256:policy",
		Body: json.RawMessage(`{"stages":[
			{"id":"evidence","required":true,"budget_units":100},
			{"id":"aggregation","required":true,"budget_units":20},
			{"id":"articulation","required":false,"budget_units":60}]}`),
	}
}

// exhaustedPolicy budgets delivery nothing, which is how a session that
// has spent its allowance looks to the stage that runs next.
func exhaustedPolicy() evaluation.PolicyPin {
	pin := practicePolicy()
	pin.Body = json.RawMessage(`{"stages":[
		{"id":"evidence","required":true,"budget_units":100},
		{"id":"aggregation","required":true,"budget_units":20},
		{"id":"articulation","required":false,"budget_units":0}]}`)
	return pin
}

// completeEvaluation runs the required stages and returns the stored result.
func completeEvaluation(t *testing.T, sessionID string) evaluation.Result {
	t.Helper()
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	sealed, spans := honestSession(sessionID)
	activities := evaluation.NewActivities(store, outbox.New(pool),
		&fakeExtractor{sealed: sealed, spans: spans, cost: 7},
		fakeRubrics{pin: practicePin(), policy: practicePolicy()})
	input := activityInput(sessionID)

	outcome, err := activities.ExtractAndStore(ctx, input)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if err := activities.Aggregate(ctx, input, outcome.ExtractionVersion, outcome.Sealed); err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	result, err := store.ResultOf(ctx, evaluation.SessionRef{
		SessionID: sessionID, Mode: "practice", CandidateID: evidenceCandidate,
	})
	if err != nil {
		t.Fatalf("result: %v", err)
	}
	return result
}

func TestTheRequiredStagesRecordWhatTheyDidAndSpent(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	sessionID := id.New().String()
	completeEvaluation(t, sessionID)

	ref := evaluation.SessionRef{SessionID: sessionID, Mode: "practice", CandidateID: evidenceCandidate}
	outcomes, err := store.StageOutcomes(ctx, ref)
	if err != nil {
		t.Fatalf("outcomes: %v", err)
	}
	evidence, found := evaluation.Standing(outcomes, evaluation.StageEvidence)
	if !found || evidence.Status != "completed" || !evidence.Required || evidence.CostUnits != 7 {
		t.Fatalf("evidence = %+v", evidence)
	}
	aggregation, found := evaluation.Standing(outcomes, evaluation.StageAggregation)
	if !found || aggregation.Status != "completed" || !aggregation.Required {
		t.Fatalf("aggregation = %+v", aggregation)
	}
	// Nothing is missing from a complete evaluation.
	if omissions := evaluation.Omissions(outcomes); len(omissions) != 0 {
		t.Fatalf("omissions = %+v", omissions)
	}
}

func TestAnExhaustedBudgetOmitsDeliveryAndKeepsTheResult(t *testing.T) {
	// The second box: the deterministic result and its status survive, and
	// the omission is recorded rather than the evaluation quietly thinning.
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	sessionID := id.New().String()
	before := completeEvaluation(t, sessionID)
	ref := evaluation.SessionRef{SessionID: sessionID, Mode: "practice", CandidateID: evidenceCandidate}

	delivery := evaluation.NewArticulationActivities(store, fakeAnalyzer{analysis: evaluation.Analysis{
		Status: "assessable", Document: json.RawMessage(`{}`),
	}}).WithPolicy(fakeRubrics{pin: practicePin(), policy: exhaustedPolicy()})

	status, err := delivery.AnalyzeAndStore(ctx, activityInput(sessionID))
	if err != nil {
		t.Fatalf("a priced-out optional stage failed instead of being omitted: %v", err)
	}
	if status != "omitted" {
		t.Fatalf("status = %q, want omitted", status)
	}

	// The core result is untouched, and no delivery row was written.
	after, err := store.ResultOf(ctx, ref)
	if err != nil {
		t.Fatalf("reread: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("the result changed:\nbefore %+v\nafter  %+v", before, after)
	}
	if _, err := store.ArticulationOf(ctx, ref); !errors.Is(err, evaluation.ErrNoArticulation) {
		t.Fatalf("an omitted delivery stored an analysis: %v", err)
	}

	// And the omission says which part is missing and why.
	outcomes, _ := store.StageOutcomes(ctx, ref)
	omissions := evaluation.Omissions(outcomes)
	if len(omissions) != 1 || omissions[0].Stage != evaluation.StageArticulation ||
		omissions[0].Reason != evaluation.ReasonBudgetExhausted {
		t.Fatalf("omissions = %+v", omissions)
	}
	// Budget exhaustion will not resolve on its own; the candidate is not
	// told to wait for it.
	if omissions[0].Retryable {
		t.Fatal("budget exhaustion was reported as retryable")
	}
}

func TestAFailedOptionalStageLeavesTheEvaluationIntactAndSaysWhy(t *testing.T) {
	// The first and third boxes: the core result stands, and terminal and
	// retryable failures are told apart on the record.
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	sessionID := id.New().String()
	before := completeEvaluation(t, sessionID)
	ref := evaluation.SessionRef{SessionID: sessionID, Mode: "practice", CandidateID: evidenceCandidate}

	terminal := evaluation.NewArticulationActivities(store, fakeAnalyzer{
		err: &evaluation.ExtractFailure{
			Code: "FAILURE_CODE_UNASSESSABLE_INPUT", Retryable: false, Message: "no speech",
		},
	}).WithPolicy(fakeRubrics{pin: practicePin(), policy: practicePolicy()})

	if _, err := terminal.AnalyzeAndStore(ctx, activityInput(sessionID)); err == nil {
		t.Fatal("a terminal delivery failure reported success")
	}

	after, err := store.ResultOf(ctx, ref)
	if err != nil {
		t.Fatalf("reread: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("a failed optional stage changed the core evaluation")
	}

	outcomes, _ := store.StageOutcomes(ctx, ref)
	standing, found := evaluation.Standing(outcomes, evaluation.StageArticulation)
	if !found || standing.Status != "failed" || standing.Required {
		t.Fatalf("standing = %+v", standing)
	}
	if standing.Retryable {
		t.Fatalf("a terminal failure was recorded as retryable: %+v", standing)
	}
	if standing.Reason != "FAILURE_CODE_UNASSESSABLE_INPUT" {
		t.Fatalf("the operator cannot see which failure it was: %+v", standing)
	}

	// A retryable failure on a later attempt is a new row, and the
	// standing moves: the two are told apart rather than merged.
	retryable := evaluation.NewArticulationActivities(store, fakeAnalyzer{
		err: &evaluation.ExtractFailure{
			Code: "FAILURE_CODE_PROVIDER_TIMEOUT", Retryable: true, Message: "later",
		},
	}).WithPolicy(fakeRubrics{pin: practicePin(), policy: practicePolicy()})
	if _, err := retryable.AnalyzeAndStore(ctx, activityInput(sessionID)); err == nil {
		t.Fatal("a retryable failure reported success")
	}
	outcomes, _ = store.StageOutcomes(ctx, ref)
	standing, _ = evaluation.Standing(outcomes, evaluation.StageArticulation)
	if !standing.Retryable || standing.Reason != "FAILURE_CODE_PROVIDER_TIMEOUT" {
		t.Fatalf("the retryable attempt did not become the standing: %+v", standing)
	}
	// Both attempts are kept: an operator can see it failed twice.
	attempts := 0
	for _, o := range outcomes {
		if o.Stage == evaluation.StageArticulation {
			attempts++
		}
	}
	if attempts != 2 {
		t.Fatalf("%d recorded attempts, want both", attempts)
	}
}

func TestTheStageRecordIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	sessionID := id.New().String()
	ref := evaluation.SessionRef{SessionID: sessionID, Mode: "practice", CandidateID: evidenceCandidate}
	if err := store.RecordStage(ctx, ref, evaluation.StageOutcome{
		Stage: evaluation.StageEvidence, Status: "completed", Required: true,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.user_id', $1, true), set_config('app.tenant_id', '', true)`,
		evidenceCandidate); err != nil {
		t.Fatalf("scoping: %v", err)
	}
	tag, err := tx.Exec(ctx,
		`UPDATE evaluation.stage_outcomes SET status = 'failed' WHERE session_id = $1`, sessionID)
	if err == nil && tag.RowsAffected() == 0 {
		t.Fatal("the attack matched zero rows; the trigger was never exercised")
	}
	if err == nil {
		t.Fatal("a recorded stage outcome was edited")
	}
}
