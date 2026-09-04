package main

import (
	"context"
	"errors"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
)

// The roster, composed: REV-01.
//
// Three contexts meet here and only here. Recruiting says who was invited
// and how each invitation ended; interview says where each accepted
// candidate's screening stands; evaluation says whether a completed
// screening's record could support assessment at all. The join key is the
// account acceptance resolved the recipient to, which recruiting records
// exactly so that this composition never has to guess by email.
//
// What the composition refuses to produce is an ordering by quality. Rows
// keep recruiting's own order (invitation recency), a completed screening
// whose evidence fell short is its own named standing rather than a low
// score, and nothing here reads a band, a ratio or a confidence - the
// evidence screen behind REV-02 is where evaluations are read, audited.
type rosterAdapter struct {
	invitations *recruiting.Store
	sessions    *interview.Store
	results     *evaluation.Store
}

func newRosterAdapter(invitations *recruiting.Store, sessions *interview.Store, results *evaluation.Store) rosterAdapter {
	return rosterAdapter{invitations: invitations, sessions: sessions, results: results}
}

var _ api.RosterReads = rosterAdapter{}

func (a rosterAdapter) CampaignRoster(ctx context.Context, tenantID, campaignID string) (api.Roster, error) {
	invitations, err := a.invitations.InvitationsForCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return api.Roster{}, err
	}
	sessions, err := a.sessions.CampaignSessions(ctx, tenantID, campaignID)
	if err != nil {
		return api.Roster{}, err
	}
	byCandidate := make(map[string]interview.CampaignSession, len(sessions))
	for _, session := range sessions {
		byCandidate[session.CandidateID] = session
	}

	roster := api.Roster{Candidates: make([]api.RosterEntry, 0, len(invitations))}
	now := time.Now()
	for _, invitation := range invitations {
		if invitation.Outcome == recruiting.InvitationSuperseded {
			// A resend retired this link; the fresh one carries the person.
			continue
		}
		entry := api.RosterEntry{
			InvitationID: invitation.ID,
			Recipient:    invitation.Recipient,
			InvitedAt:    invitation.IssuedAt,
		}

		session, sat := byCandidate[invitation.AcceptedCandidate]
		if invitation.AcceptedCandidate == "" || !sat {
			entry.Standing = standingOfInvitation(invitation.Status(now))
		} else {
			entry.SessionID = session.ID
			entry.Standing = a.standingOfSession(ctx, tenantID, session)
			if entry.Standing == "awaiting_review" || entry.Standing == "insufficient_evidence" {
				submitted := session.StateChangedAt
				entry.SubmittedAt = &submitted
				roster.PendingReview++
			}
		}
		roster.Candidates = append(roster.Candidates, entry)
	}
	return roster, nil
}

// standingOfInvitation maps recruiting's computed status onto the roster's
// vocabulary for a candidate with no interview yet.
func standingOfInvitation(status string) string {
	switch status {
	case "live":
		return "invited"
	case "accepted":
		return "accepted"
	case "declined", "revoked", "expired":
		return status
	default:
		// A vocabulary recruiting grows later still renders honestly.
		return status
	}
}

// standingOfSession maps the interview's lifecycle onto the roster's
// vocabulary, asking evaluation one question for a reviewable session:
// could the record support assessment at all. Absence of a stored result
// for a reviewable session reads as awaiting_review, because publication
// may still be in flight and "not yet readable" is not "insufficient".
func (a rosterAdapter) standingOfSession(ctx context.Context, tenantID string, session interview.CampaignSession) string {
	switch session.State {
	case interview.StateReady, interview.StateDraft, interview.StateComposing,
		interview.StateCompositionFailed:
		return "accepted"
	case interview.StateConnecting, interview.StateInProgress, interview.StateReconnecting:
		return "in_progress"
	case interview.StateFinalizing, interview.StateEvaluating,
		interview.StateFinalizationFailed, interview.StateEvaluationFailed:
		return "processing"
	case interview.StateExpired, interview.StateCancelled, interview.StateInterrupted:
		return "session_expired"
	case interview.StateReviewReady, interview.StateArchived:
		result, err := a.results.ResultOf(ctx, evaluation.SessionRef{
			SessionID: session.ID, Mode: "screening",
			CandidateID: session.CandidateID, TenantID: tenantID,
		})
		return reviewableStanding(result, err)
	default:
		return "processing"
	}
}

// reviewableStanding decides what a reviewable session's row says. A record
// that reached no competency at all is insufficient_evidence: its own named
// standing awaiting the same human review, never a low score. A result that
// cannot be read yet - publication may still be in flight - is
// awaiting_review, because "not yet readable" is not "insufficient".
func reviewableStanding(result evaluation.Result, err error) string {
	if errors.Is(err, evaluation.ErrNoResult) || err != nil {
		return "awaiting_review"
	}
	if result.Aggregation.CoveredCompetencies == 0 {
		return "insufficient_evidence"
	}
	return "awaiting_review"
}
