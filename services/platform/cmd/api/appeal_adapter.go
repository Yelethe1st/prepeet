package main

import (
	"context"
	"errors"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
)

// The appeal, composed: REV-06.
//
// Recruiting owns the appeal record and the decision it is raised against;
// interview names the bundle the session ran, which the raise freezes
// alongside the decision's own evidence version. The one composition act
// here is resolving the session through the campaign's list and handing
// its bundle digest across, plus translating each refusal onto the wire's
// stable codes.
type appealAdapter struct {
	sessions *interview.Store
	appeals  *recruiting.Store
}

func newAppealAdapter(sessions *interview.Store, appeals *recruiting.Store) appealAdapter {
	return appealAdapter{sessions: sessions, appeals: appeals}
}

var _ api.Appeals = appealAdapter{}

func (a appealAdapter) Raise(ctx context.Context, tenantID, campaignID, sessionID, requestedBy, reason string) (api.ReReviewView, error) {
	sessions, err := a.sessions.CampaignSessions(ctx, tenantID, campaignID)
	if err != nil {
		return api.ReReviewView{}, err
	}
	bundleDigest := ""
	found := false
	for _, session := range sessions {
		if session.ID == sessionID {
			bundleDigest = session.BundleDigest
			found = true
			break
		}
	}
	if !found {
		return api.ReReviewView{}, api.ErrReviewSessionMissing
	}

	raised, err := a.appeals.RaiseReReview(ctx, tenantID, campaignID, sessionID,
		requestedBy, reason, bundleDigest)
	if err != nil {
		return api.ReReviewView{}, translateAppeal(err)
	}
	return appealView(raised), nil
}

func (a appealAdapter) Assign(ctx context.Context, tenantID, campaignID, appealID, assignee string) (api.ReReviewView, error) {
	assigned, err := a.appeals.AssignReReview(ctx, tenantID, campaignID, appealID, assignee)
	if err != nil {
		return api.ReReviewView{}, translateAppeal(err)
	}
	return appealView(assigned), nil
}

func (a appealAdapter) Resolve(ctx context.Context, tenantID, campaignID, appealID, resolvedBy, outcome, rationale, disclosure string) (api.ReReviewView, error) {
	resolved, err := a.appeals.ResolveReReview(ctx, tenantID, campaignID, appealID,
		resolvedBy, outcome, rationale, disclosure)
	if err != nil {
		return api.ReReviewView{}, translateAppeal(err)
	}
	return appealView(resolved), nil
}

func (a appealAdapter) List(ctx context.Context, tenantID, campaignID, sessionID string) ([]api.ReReviewView, error) {
	appeals, err := a.appeals.ReReviewsForSession(ctx, tenantID, campaignID, sessionID)
	if err != nil {
		return nil, translateAppeal(err)
	}
	out := make([]api.ReReviewView, 0, len(appeals))
	for _, appeal := range appeals {
		out = append(out, appealView(appeal))
	}
	return out, nil
}

// translateAppeal keeps each refusal's identity without api importing
// recruiting's sentinels.
func translateAppeal(err error) error {
	switch {
	case errors.Is(err, recruiting.ErrAppealNoDecision):
		return api.ErrAppealNothingDecided
	case errors.Is(err, recruiting.ErrAppealReasonRequired):
		return api.ErrAppealReasonMissing
	case errors.Is(err, recruiting.ErrAppealSelfReview):
		return api.ErrAppealSelfReviewAPI
	case errors.Is(err, recruiting.ErrAppealUnknown):
		return api.ErrAppealMissing
	case errors.Is(err, recruiting.ErrAppealResolved):
		return api.ErrAppealDone
	case errors.Is(err, recruiting.ErrAppealResolverNotAssigned):
		return api.ErrAppealNotAssignee
	case errors.Is(err, recruiting.ErrAppealOutcomeInvalid):
		return api.ErrAppealIncomplete
	}
	return err
}

func appealView(appeal recruiting.ReReview) api.ReReviewView {
	view := api.ReReviewView{
		ID:               appeal.ID,
		SessionID:        appeal.SessionID,
		RequestedBy:      appeal.RequestedBy,
		Reason:           appeal.Reason,
		AppealedDecision: appeal.AppealedDecision,
		OriginalReviewer: appeal.OriginalReviewer,
		Frozen: api.FrozenEvidenceView{
			EvaluationID: appeal.Frozen.EvaluationID,
			ResultDigest: appeal.Frozen.ResultDigest,
			RubricDigest: appeal.Frozen.RubricDigest,
			BundleDigest: appeal.BundleDigest,
		},
		AssignedTo: appeal.AssignedTo,
		RaisedAt:   appeal.RaisedAt,
		DueAt:      appeal.DueAt,
	}
	if appeal.Resolution != nil {
		view.Resolution = &api.ResolutionView{
			Outcome:    appeal.Resolution.Outcome,
			Rationale:  appeal.Resolution.Rationale,
			Disclosure: appeal.Resolution.Disclosure,
			ResolvedBy: appeal.Resolution.ResolvedBy,
			ResolvedAt: appeal.Resolution.ResolvedAt,
		}
	}
	return view
}
