package interview

import (
	"errors"
	"testing"
)

// The state machine, tested from the spec's diagram.
//
// session-lifecycle.md's mermaid diagram is the contract; the table below is
// that diagram transcribed, and the machine must agree with it exactly - every
// edge allowed, everything else refused with the stable code SES-01 requires.

func TestEveryEdgeInTheSpecIsAllowed(t *testing.T) {
	t.Parallel()

	edges := []struct{ from, to State }{
		{StateDraft, StateComposing},
		{StateComposing, StateReady},
		{StateComposing, StateCompositionFailed},
		{StateCompositionFailed, StateComposing}, // authorized retry
		{StateReady, StateConnecting},
		{StateConnecting, StateInProgress},
		{StateConnecting, StateReady}, // aborted
		{StateInProgress, StateReconnecting},
		{StateReconnecting, StateInProgress}, // resumed
		{StateReconnecting, StateFinalizing}, // grace expired
		{StateInProgress, StateFinalizing},
		{StateFinalizing, StateEvaluating},
		{StateFinalizing, StateFinalizationFailed},
		{StateFinalizationFailed, StateFinalizing}, // retry
		{StateEvaluating, StateReviewReady},
		{StateEvaluating, StateEvaluationFailed},
		{StateEvaluationFailed, StateEvaluating}, // retry
		{StateReviewReady, StateArchived},
		{StateDraft, StateCancelled},
		{StateReady, StateExpired},
	}

	for _, edge := range edges {
		if err := CanTransition(edge.from, edge.to); err != nil {
			t.Errorf("the spec allows %s -> %s, the machine refuses it: %v", edge.from, edge.to, err)
		}
	}
}

func TestEverythingOutsideTheSpecIsRefused(t *testing.T) {
	t.Parallel()

	// The full cross product minus the spec's edges. Enumerated rather than
	// sampled: seventeen states is 289 pairs, cheap to check and each one a
	// transition somebody could accidentally wire.
	allowed := map[[2]State]bool{}
	for _, edge := range [][2]State{
		{StateDraft, StateComposing}, {StateComposing, StateReady},
		{StateComposing, StateCompositionFailed}, {StateCompositionFailed, StateComposing},
		{StateReady, StateConnecting}, {StateConnecting, StateInProgress},
		{StateConnecting, StateReady}, {StateInProgress, StateReconnecting},
		{StateReconnecting, StateInProgress}, {StateReconnecting, StateFinalizing},
		{StateInProgress, StateFinalizing}, {StateFinalizing, StateEvaluating},
		{StateFinalizing, StateFinalizationFailed}, {StateFinalizationFailed, StateFinalizing},
		{StateEvaluating, StateReviewReady}, {StateEvaluating, StateEvaluationFailed},
		{StateEvaluationFailed, StateEvaluating}, {StateReviewReady, StateArchived},
		{StateDraft, StateCancelled}, {StateReady, StateExpired},
	} {
		allowed[edge] = true
	}

	for _, from := range States() {
		for _, to := range States() {
			if allowed[[2]State{from, to}] {
				continue
			}
			err := CanTransition(from, to)
			if err == nil {
				t.Errorf("%s -> %s is not in the spec and was allowed", from, to)
				continue
			}
			// SES-01: a stable error code, not a 500 and not prose.
			var refused *TransitionError
			if !errors.As(err, &refused) {
				t.Errorf("%s -> %s refused without a typed error: %v", from, to, err)
				continue
			}
			if refused.Code != "SESSION_TRANSITION_INVALID" {
				t.Errorf("%s -> %s refused with code %q", from, to, refused.Code)
			}
		}
	}
}

func TestSelfTransitionsAreRefused(t *testing.T) {
	t.Parallel()
	// A no-op transition is how a duplicate command looks; idempotency is the
	// store's job via versions, not the machine's via silent acceptance.
	for _, state := range States() {
		if CanTransition(state, state) == nil {
			t.Errorf("%s -> %s was allowed", state, state)
		}
	}
}

func TestTerminalStatesAreTerminalExceptWhereRetryIsAuthorized(t *testing.T) {
	t.Parallel()

	// The spec: "Retryable failures should remain visible workflow state
	// rather than destructive lifecycle rewrites". The three *_failed states
	// retry to their working state; the other terminals go nowhere.
	terminal := []State{StateCancelled, StateExpired, StateArchived, StateInterrupted}
	for _, from := range terminal {
		for _, to := range States() {
			if CanTransition(from, to) == nil {
				t.Errorf("terminal state %s has an exit to %s", from, to)
			}
		}
	}
}
