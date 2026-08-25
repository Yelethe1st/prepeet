// Package content owns the artifact registry: versioned, digest-addressed
// interview artifacts with an approval step and a rollback path.
//
// ADR-0011 decides the shape. What this package enforces beyond the schema's
// own triggers: the lifecycle machine, the separation of duties on publish,
// and that resolving by digest is the read path for anything already pinned.
//
// Implements CAT-01.
package content

import "fmt"

// Status is one lifecycle state of an artifact version.
type Status string

// The lifecycle, from domain-model.md.
const (
	StatusDraft      Status = "draft"
	StatusValidating Status = "validating"
	StatusApproved   Status = "approved"
	StatusPublished  Status = "published"
	StatusDeprecated Status = "deprecated"
	StatusRetired    Status = "retired"
)

// Statuses returns every status, in lifecycle order.
func Statuses() []Status {
	return []Status{
		StatusDraft, StatusValidating, StatusApproved,
		StatusPublished, StatusDeprecated, StatusRetired,
	}
}

// transitions is the spec's chain plus the one backward edge: validation
// rejecting a draft back to its author. Nothing returns from retired, because
// a version reappearing would carry a digest sessions already pinned with a
// different standing in mind.
var transitions = map[Status][]Status{
	StatusDraft:      {StatusValidating},
	StatusValidating: {StatusApproved, StatusDraft},
	StatusApproved:   {StatusPublished},
	StatusPublished:  {StatusDeprecated},
	StatusDeprecated: {StatusRetired},
}

// TransitionError is a refused lifecycle move, with a stable code.
type TransitionError struct {
	Code string
	From Status
	To   Status
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("content: %s: no transition from %s to %s", e.Code, e.From, e.To)
}

// CanTransition reports whether the lifecycle permits from -> to.
func CanTransition(from, to Status) error {
	for _, next := range transitions[from] {
		if next == to {
			return nil
		}
	}
	return &TransitionError{Code: "ARTIFACT_TRANSITION_INVALID", From: from, To: to}
}
