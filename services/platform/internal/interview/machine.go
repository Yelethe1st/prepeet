// Package interview owns the session lifecycle: which states exist, which
// transitions are legal, and what every transition must record.
//
// The machine here is docs/architecture/session-lifecycle.md's diagram,
// transcribed. The tests hold the two to each other edge by edge, so the spec
// cannot drift from the code without one of them failing.
//
// Implements SES-01.
package interview

import "fmt"

// State is one lifecycle state. A named type so a transition cannot be handed
// an arbitrary string that happens to be in scope.
type State string

// The lifecycle, from the spec's diagram.
const (
	StateDraft        State = "draft"
	StateComposing    State = "composing"
	StateReady        State = "ready"
	StateConnecting   State = "connecting"
	StateInProgress   State = "in_progress"
	StateReconnecting State = "reconnecting"
	StateFinalizing   State = "finalizing"
	StateEvaluating   State = "evaluating"
	StateReviewReady  State = "review_ready"
	StateArchived     State = "archived"

	// The exceptional terminals. The three *_failed states are retryable and
	// deliberately remain visible workflow state rather than being rewritten:
	// an operator looking at a session must see that composition failed and
	// was retried, not a history that pretends it composed first time.
	StateCancelled          State = "cancelled"
	StateExpired            State = "expired"
	StateCompositionFailed  State = "composition_failed"
	StateInterrupted        State = "interrupted"
	StateFinalizationFailed State = "finalization_failed"
	StateEvaluationFailed   State = "evaluation_failed"
)

// States returns every state, in a stable order.
func States() []State {
	return []State{
		StateDraft, StateComposing, StateReady, StateConnecting, StateInProgress,
		StateReconnecting, StateFinalizing, StateEvaluating, StateReviewReady,
		StateArchived, StateCancelled, StateExpired, StateCompositionFailed,
		StateInterrupted, StateFinalizationFailed, StateEvaluationFailed,
	}
}

// transitions is the diagram. An edge absent here does not exist, whatever a
// caller believes; adding one is a spec change first and a code change second.
var transitions = map[State][]State{
	StateDraft:              {StateComposing, StateCancelled},
	StateComposing:          {StateReady, StateCompositionFailed},
	StateCompositionFailed:  {StateComposing}, // authorized retry
	StateReady:              {StateConnecting, StateExpired},
	StateConnecting:         {StateInProgress, StateReady}, // aborted returns to ready
	StateInProgress:         {StateReconnecting, StateFinalizing},
	StateReconnecting:       {StateInProgress, StateFinalizing}, // resumed, or grace expired
	StateFinalizing:         {StateEvaluating, StateFinalizationFailed},
	StateFinalizationFailed: {StateFinalizing}, // retry
	StateEvaluating:         {StateReviewReady, StateEvaluationFailed},
	StateEvaluationFailed:   {StateEvaluating}, // retry
	StateReviewReady:        {StateArchived},
}

// TransitionError is a refused transition, with the stable code SES-01
// requires: a caller branches on Code, never on the message, and an invalid
// transition surfaces as a refusal rather than a 500.
type TransitionError struct {
	Code string
	From State
	To   State
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("interview: %s: no transition from %s to %s", e.Code, e.From, e.To)
}

// CanTransition reports whether the lifecycle permits from -> to.
func CanTransition(from, to State) error {
	for _, next := range transitions[from] {
		if next == to {
			return nil
		}
	}
	return &TransitionError{Code: "SESSION_TRANSITION_INVALID", From: from, To: to}
}
