package main

import (
	"errors"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
)

// REV-01's vocabulary mapping, pinned as tables. Every lifecycle state and
// every invitation status lands on exactly one roster standing, and the two
// decisions that carry weight are held to by name: a reviewable session
// with no reachable result stays awaiting_review rather than being guessed
// at, and a record that reached nothing is insufficient_evidence, never a
// low score.

func TestEveryInvitationStatusHasARosterStanding(t *testing.T) {
	cases := map[string]string{
		"live":     "invited",
		"accepted": "accepted",
		"declined": "declined",
		"revoked":  "revoked",
		"expired":  "expired",
	}
	for status, want := range cases {
		if got := standingOfInvitation(status); got != want {
			t.Errorf("standingOfInvitation(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestEverySessionStateHasARosterStanding(t *testing.T) {
	// The states short of review, decided without asking evaluation
	// anything: a nil results store proves no evaluation read happens.
	adapter := rosterAdapter{}
	cases := map[interview.State]string{
		interview.StateDraft:              "accepted",
		interview.StateComposing:          "accepted",
		interview.StateCompositionFailed:  "accepted",
		interview.StateReady:              "accepted",
		interview.StateConnecting:         "in_progress",
		interview.StateInProgress:         "in_progress",
		interview.StateReconnecting:       "in_progress",
		interview.StateFinalizing:         "processing",
		interview.StateFinalizationFailed: "processing",
		interview.StateEvaluating:         "processing",
		interview.StateEvaluationFailed:   "processing",
		interview.StateExpired:            "session_expired",
		interview.StateCancelled:          "session_expired",
		interview.StateInterrupted:        "session_expired",
	}
	for state, want := range cases {
		got := adapter.standingOfSession(t.Context(), "tenant",
			interview.CampaignSession{ID: "ses", CandidateID: "cand", State: state})
		if got != want {
			t.Errorf("standingOfSession(%s) = %q, want %q", state, got, want)
		}
	}
}

func TestAReviewableSessionIsNamedHonestly(t *testing.T) {
	covered := evaluation.Result{}
	covered.Aggregation.CoveredCompetencies = 3

	empty := evaluation.Result{}

	if got := reviewableStanding(covered, nil); got != "awaiting_review" {
		t.Fatalf("covered record = %q, want awaiting_review", got)
	}
	// The record reached nothing: its own named standing, awaiting the
	// same human review, never a low score sorted to the bottom.
	if got := reviewableStanding(empty, nil); got != "insufficient_evidence" {
		t.Fatalf("empty record = %q, want insufficient_evidence", got)
	}
	// Not yet readable is not insufficient: publication may be in flight.
	if got := reviewableStanding(evaluation.Result{}, evaluation.ErrNoResult); got != "awaiting_review" {
		t.Fatalf("no result yet = %q, want awaiting_review", got)
	}
	if got := reviewableStanding(evaluation.Result{}, errors.New("db down")); got != "awaiting_review" {
		t.Fatalf("unreadable result = %q, want awaiting_review", got)
	}
}
