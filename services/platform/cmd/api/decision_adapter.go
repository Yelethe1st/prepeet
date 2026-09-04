package main

import (
	"context"
	"errors"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
)

// The decision, composed: REV-03.
//
// Recruiting owns the record; evaluation owns the evidence it was informed
// by. This adapter is where the two meet: the session is resolved through
// the campaign's own list so an id from another campaign answers as
// absence, the published result supplies the evidence version and the
// recorded band each override disagrees with - from the stored result,
// never from the request - and only then does the append-only record take
// the decision.
type decisionAdapter struct {
	sessions  *interview.Store
	results   *evaluation.Store
	decisions *recruiting.Store
}

func newDecisionAdapter(sessions *interview.Store, results *evaluation.Store,
	decisions *recruiting.Store) decisionAdapter {
	return decisionAdapter{sessions: sessions, results: results, decisions: decisions}
}

var _ api.ReviewDecisions = decisionAdapter{}

func (a decisionAdapter) Record(ctx context.Context, tenantID, campaignID, sessionID, decidedBy, decision, reason string, overrides []api.OverrideInput) (api.ReviewDecisionView, error) {
	session, err := a.sessionOn(ctx, tenantID, campaignID, sessionID)
	if err != nil {
		return api.ReviewDecisionView{}, err
	}

	// The evidence the decision is informed by. No published evaluation
	// means nothing to decide against: an outcome the evidence never
	// informed is refused, not defaulted.
	result, err := a.results.ResultOf(ctx, evaluation.SessionRef{
		SessionID: sessionID, Mode: "screening",
		CandidateID: session.CandidateID, TenantID: tenantID,
	})
	if errors.Is(err, evaluation.ErrNoResult) {
		return api.ReviewDecisionView{}, api.ErrDecisionNotReady
	}
	if err != nil {
		return api.ReviewDecisionView{}, err
	}

	// The recorded band comes from the stored result, never the request: an
	// override against a competency the evaluation did not assess has no
	// band to disagree with and is refused by name.
	bands := map[string]string{}
	for _, competency := range result.Aggregation.Competencies {
		if competency.Status == "assessed" {
			bands[competency.CompetencyID] = competency.Band
		}
	}
	enriched := make([]recruiting.BandOverride, 0, len(overrides))
	for _, override := range overrides {
		recorded, assessed := bands[override.CompetencyID]
		if !assessed {
			return api.ReviewDecisionView{}, api.ErrOverrideUnknownCompetency
		}
		enriched = append(enriched, recruiting.BandOverride{
			CompetencyID: override.CompetencyID,
			RecordedBand: recorded,
			OverrideBand: override.Band,
			Rationale:    override.Rationale,
		})
	}

	recorded, err := a.decisions.RecordReviewDecision(ctx, tenantID, campaignID,
		sessionID, decidedBy, decision, reason,
		recruiting.EvidenceVersion{
			EvaluationID: result.ID,
			ResultDigest: result.ResultDigest,
			RubricDigest: result.RubricDigest,
		}, enriched)
	switch {
	case errors.Is(err, recruiting.ErrDecisionReasonRequired):
		return api.ReviewDecisionView{}, api.ErrDecisionReasonMissing
	case errors.Is(err, recruiting.ErrOverrideIncomplete):
		return api.ReviewDecisionView{}, api.ErrOverrideIncompleteAPI
	case errors.Is(err, recruiting.ErrDecisionUnknown):
		return api.ReviewDecisionView{}, api.ErrDecisionInvalid
	case err != nil:
		return api.ReviewDecisionView{}, err
	}
	return decisionView(recorded), nil
}

func (a decisionAdapter) History(ctx context.Context, tenantID, campaignID, sessionID string) ([]api.ReviewDecisionView, error) {
	if _, err := a.sessionOn(ctx, tenantID, campaignID, sessionID); err != nil {
		return nil, err
	}
	history, err := a.decisions.ReviewDecisionsForSession(ctx, tenantID, campaignID, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]api.ReviewDecisionView, 0, len(history))
	for _, decision := range history {
		out = append(out, decisionView(decision))
	}
	return out, nil
}

// sessionOn resolves the session through the campaign's own list, so an id
// from another campaign answers as absence.
func (a decisionAdapter) sessionOn(ctx context.Context, tenantID, campaignID, sessionID string) (interview.CampaignSession, error) {
	sessions, err := a.sessions.CampaignSessions(ctx, tenantID, campaignID)
	if err != nil {
		return interview.CampaignSession{}, err
	}
	for _, session := range sessions {
		if session.ID == sessionID {
			return session, nil
		}
	}
	return interview.CampaignSession{}, api.ErrReviewSessionMissing
}

func decisionView(decision recruiting.ReviewDecision) api.ReviewDecisionView {
	view := api.ReviewDecisionView{
		ID:           decision.ID,
		SessionID:    decision.SessionID,
		DecidedBy:    decision.DecidedBy,
		Decision:     decision.Decision,
		Reason:       decision.Reason,
		EvaluationID: decision.Evidence.EvaluationID,
		ResultDigest: decision.Evidence.ResultDigest,
		RubricDigest: decision.Evidence.RubricDigest,
		Overrides:    make([]api.BandOverrideDecided, 0, len(decision.Overrides)),
		DecidedAt:    decision.CreatedAt,
	}
	for _, override := range decision.Overrides {
		view.Overrides = append(view.Overrides, api.BandOverrideDecided{
			CompetencyID: override.CompetencyID,
			RecordedBand: override.RecordedBand,
			OverrideBand: override.OverrideBand,
			Rationale:    override.Rationale,
		})
	}
	return view
}
