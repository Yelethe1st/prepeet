package content

import (
	"errors"
	"testing"
)

// The artifact lifecycle from domain-model.md, held to the spec exactly as
// interview's machine is: every listed edge allowed, everything else refused
// with a stable code.

func TestEverySpecEdgeIsAllowed(t *testing.T) {
	t.Parallel()

	edges := [][2]Status{
		{StatusDraft, StatusValidating},
		{StatusValidating, StatusApproved},
		{StatusValidating, StatusDraft}, // validation rejected: back to the author
		{StatusApproved, StatusPublished},
		{StatusPublished, StatusDeprecated},
		{StatusDeprecated, StatusRetired},
	}
	for _, edge := range edges {
		if err := CanTransition(edge[0], edge[1]); err != nil {
			t.Errorf("the spec allows %s -> %s, the machine refuses: %v", edge[0], edge[1], err)
		}
	}
}

func TestEverythingElseIsRefused(t *testing.T) {
	t.Parallel()

	allowed := map[[2]Status]bool{
		{StatusDraft, StatusValidating}:     true,
		{StatusValidating, StatusApproved}:  true,
		{StatusValidating, StatusDraft}:     true,
		{StatusApproved, StatusPublished}:   true,
		{StatusPublished, StatusDeprecated}: true,
		{StatusDeprecated, StatusRetired}:   true,
	}

	for _, from := range Statuses() {
		for _, to := range Statuses() {
			if allowed[[2]Status{from, to}] {
				continue
			}
			err := CanTransition(from, to)
			var refused *TransitionError
			if !errors.As(err, &refused) || refused.Code != "ARTIFACT_TRANSITION_INVALID" {
				t.Errorf("%s -> %s: got %v, want ARTIFACT_TRANSITION_INVALID", from, to, err)
			}
		}
	}
}

func TestRetiredIsTheOnlyDeadEnd(t *testing.T) {
	t.Parallel()

	// Published and deprecated still move forward; retired is history. A
	// retired artifact coming back would be a version reappearing under a
	// digest sessions already pinned with a different meaning in mind.
	for _, to := range Statuses() {
		if CanTransition(StatusRetired, to) == nil {
			t.Errorf("retired has an exit to %s", to)
		}
	}
}
