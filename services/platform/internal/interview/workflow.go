package interview

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// The composition workflow: the durable path from draft to ready.
//
// This is the first workflow in the system, and it carries PLT-06's promise:
// a worker killed mid-flight resumes without duplicating state, usage or
// notification. The promise is kept the boring way - every activity is
// idempotent against the aggregate's own guards - rather than by trusting
// exactly-once delivery that Temporal does not offer.
//
// Everything crossing the workflow boundary is identifiers and digests, which
// is ADR-0007's payload rule; the converter refuses anything larger before it
// is stored.

// TaskQueue is where this context's workflows and activities run. PLT-06 left
// task-queue naming to the first workflow; the name is the bounded context, so
// queue ownership follows module ownership.
const TaskQueue = "prepeet-interview"

// Composer is what the workflow asks of the intelligence plane.
//
// Declared here per ADR-0005: interview consumes the capability, so interview
// says how narrow it is. cmd wires the gRPC client from CTR-02's generated
// stubs; tests wire fakes. The port speaks identifiers in and a bundle
// reference out, never content.
type Composer interface {
	Compose(ctx context.Context, request ComposeRequest) (ComposeResult, error)
}

// ComposeRequest names the pinned inputs composition reads.
type ComposeRequest struct {
	SessionID   string
	Mode        string
	CandidateID string
	TenantID    string
	BlueprintID string
}

// ComposeResult is the bundle, by reference and digest.
type ComposeResult struct {
	BundleRef      string
	BundleDigest   string
	BundleRevision int
}

// ComposeFailure is a typed refusal from the composer.
//
// Retryable follows the RPC contract's own declaration: the failure codes and
// their retry decisions live as descriptor options in packages/contracts/rpc,
// and the adapter in cmd translates them here, so the workflow never parses a
// message string to decide whether trying again could help.
type ComposeFailure struct {
	Code      string
	Retryable bool
	Message   string
}

func (f *ComposeFailure) Error() string {
	return fmt.Sprintf("interview: composition failed: %s: %s", f.Code, f.Message)
}

// CompositionInput is the workflow's argument: identifiers only.
type CompositionInput struct {
	SessionID   string
	Mode        string
	CandidateID string
	TenantID    string
	BlueprintID string
	// Actor is whose authority the workflow acts under, carried into every
	// transition's audit row with Type "service" marking the automation.
	ActorID string
}

// Activities is the workflow's side of the world, one instance per worker.
type Activities struct {
	store    *Store
	composer Composer
}

// NewActivities wires the store and the composer for a worker.
func NewActivities(store *Store, composer Composer) *Activities {
	return &Activities{store: store, composer: composer}
}

// Compose asks the intelligence plane for the bundle.
//
// Safe to re-run after a worker death: composition is pinned to the session's
// inputs, so a second run produces the same bundle, and the request carries
// the session id as its idempotency identity.
func (a *Activities) Compose(ctx context.Context, input CompositionInput) (ComposeResult, error) {
	result, err := a.composer.Compose(ctx, ComposeRequest{
		SessionID:   input.SessionID,
		Mode:        input.Mode,
		CandidateID: input.CandidateID,
		TenantID:    input.TenantID,
		BlueprintID: input.BlueprintID,
	})
	if err != nil {
		var failure *ComposeFailure
		if errors.As(err, &failure) && !failure.Retryable {
			// Non-retryable by the contract's own declaration: retrying spends
			// provider budget to fail identically. The typed error carries the
			// code to the failure handler.
			return ComposeResult{}, temporal.NewNonRetryableApplicationError(
				failure.Message, failure.Code, failure)
		}
		return ComposeResult{}, err
	}
	return result, nil
}

// MarkReady records the bundle and moves the session to ready.
//
// Idempotent against replay: a session already ready with this digest means a
// previous attempt committed before the worker died, and the right answer is
// success, not a duplicate transition - the version guard would refuse one,
// and this turns that refusal back into the truth.
func (a *Activities) MarkReady(ctx context.Context, input CompositionInput, result ComposeResult) error {
	actor := Actor{ID: input.ActorID, Type: "service"}

	session, err := a.store.Get(ctx, input.SessionID, input.Mode, input.CandidateID, input.TenantID)
	if err != nil {
		return err
	}
	if session.State == StateReady && session.BundleDigest == result.BundleDigest {
		return nil // the previous attempt won; nothing to repeat
	}

	effects := Effects{
		BundleRef:      result.BundleRef,
		BundleDigest:   result.BundleDigest,
		BundleRevision: result.BundleRevision,
	}
	event, err := ReadyEvent(session, effects, actor)
	if err != nil {
		return err
	}
	effects.Event = event

	_, err = a.store.Transition(ctx, session, StateReady, effects, actor)
	return err
}

// MarkFailed records composition_failed with the failure's stable code.
func (a *Activities) MarkFailed(ctx context.Context, input CompositionInput, code string) error {
	actor := Actor{ID: input.ActorID, Type: "service"}

	session, err := a.store.Get(ctx, input.SessionID, input.Mode, input.CandidateID, input.TenantID)
	if err != nil {
		return err
	}
	if session.State == StateCompositionFailed {
		return nil // already recorded by a previous attempt
	}

	_, err = a.store.Transition(ctx, session, StateCompositionFailed,
		Effects{FailureCode: code}, actor)
	return err
}

// CompositionWorkflow drives one session from composing to ready or to
// composition_failed.
//
// Started with the session id as its workflow identity, per the transition
// contract in session-lifecycle.md: a duplicate compose command joins the
// running workflow instead of starting a second composition.
func CompositionWorkflow(ctx workflow.Context, input CompositionInput) error {
	options := workflow.ActivityOptions{
		// Composition calls a model provider; the ceiling matches the RPC
		// contract's 30s per attempt with room for retries around it.
		StartToCloseTimeout: 45 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    5,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, options)

	var activities *Activities

	var result ComposeResult
	composeErr := workflow.ExecuteActivity(ctx, activities.Compose, input).Get(ctx, &result)
	if composeErr == nil {
		return workflow.ExecuteActivity(ctx, activities.MarkReady, input, result).Get(ctx, nil)
	}

	// Composition is out of attempts or refused outright. Recording the
	// failure is not optional and gets its own retries: a session stuck in
	// composing because the failure could not be written is a session nobody
	// can retry from the interface.
	code := "COMPOSITION_FAILED"
	var applicationErr *temporal.ApplicationError
	if errors.As(composeErr, &applicationErr) && applicationErr.Type() != "" {
		code = applicationErr.Type()
	}
	if err := workflow.ExecuteActivity(ctx, activities.MarkFailed, input, code).Get(ctx, nil); err != nil {
		return err
	}
	return composeErr
}
