package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// The hiring decision: REV-03.
//
// Advance, reject or hold, from a named human with a required reason. The
// decider is the session, never the body: a request-supplied decider would
// let anybody put an outcome in a colleague's name, and an outcome without
// a person is exactly what responsible-hiring.md refuses. The evidence
// version is captured by the composition at decision time, and an override
// records the band the aggregation assessed alongside the reviewer's own
// and their rationale - the band from the stored result, never the request.

// The refusals this surface translates.
var (
	// ErrDecisionNotReady means no published evaluation exists to decide
	// against: a decision recorded before the evidence would be an outcome
	// the evidence never informed.
	ErrDecisionNotReady = errors.New("api: no published evaluation to decide against")
	// ErrOverrideUnknownCompetency means an override names a competency the
	// evaluation did not assess: there is no band to disagree with.
	ErrOverrideUnknownCompetency = errors.New("api: the override names a competency the evaluation did not assess")
	// ErrDecisionInvalid carries the domain's own validation refusals.
	ErrDecisionInvalid = errors.New("api: the decision is invalid")
	// ErrDecisionReasonMissing is the reason rule at the wire.
	ErrDecisionReasonMissing = errors.New("api: a decision requires a reason")
	// ErrOverrideIncompleteAPI is an override missing band or rationale.
	ErrOverrideIncompleteAPI = errors.New("api: an override requires its band and rationale")
)

// OverrideInput is one disagreement as the request states it: the
// reviewer's band and rationale. The recorded band is not here, on
// purpose.
type OverrideInput struct {
	CompetencyID string
	Band         string
	Rationale    string
}

// BandOverrideDecided is one disagreement as the record keeps it.
type BandOverrideDecided struct {
	CompetencyID string
	RecordedBand string
	OverrideBand string
	Rationale    string
}

// ReviewDecisionView is one decision as history serves it.
type ReviewDecisionView struct {
	ID           string
	SessionID    string
	DecidedBy    string
	Decision     string
	Reason       string
	EvaluationID string
	ResultDigest string
	RubricDigest string
	Overrides    []BandOverrideDecided
	DecidedAt    time.Time
}

// ReviewDecisions is what this surface needs of the composition in cmd,
// where the stored evaluation and the decision record meet.
type ReviewDecisions interface {
	// Record captures the evidence version and the recorded bands from the
	// stored result, then appends the decision. The named refusals above
	// are its contract.
	Record(ctx context.Context, tenantID, campaignID, sessionID, decidedBy, decision, reason string, overrides []OverrideInput) (ReviewDecisionView, error)
	History(ctx context.Context, tenantID, campaignID, sessionID string) ([]ReviewDecisionView, error)
}

// decisionHandlers serves REV-03's surface.
type decisionHandlers struct {
	authentication *authentication
	campaigns      Recruiting
	decisions      ReviewDecisions
}

func (h *decisionHandlers) caller(ctx context.Context) (Principal, *failure) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		refusal := h.authentication.rejectedSession(ctx)
		return Principal{}, &refusal
	}
	principal, err := h.authentication.identity.Authorize(ctx, presented, requiredCapabilityFrom(ctx))
	if err != nil {
		refusal := h.authentication.failed(ctx, err)
		return Principal{}, &refusal
	}
	return principal, nil
}

// RecordReviewDecision appends one named human's decision.
func (h *decisionHandlers) RecordReviewDecision(ctx context.Context, request prepeetapi.RecordReviewDecisionRequestObject) (prepeetapi.RecordReviewDecisionResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, err := h.campaigns.CampaignForRecruiter(ctx, principal.ActiveTenantID, request.CampaignID.String(), principal.UserID)
	if err != nil {
		return h.decisionFailure(ctx, err), nil
	}

	overrides := []OverrideInput{}
	if request.Body.Overrides != nil {
		for _, override := range *request.Body.Overrides {
			overrides = append(overrides, OverrideInput{
				CompetencyID: override.CompetencyID,
				Band:         override.Band,
				Rationale:    override.Rationale,
			})
		}
	}

	// The decider is the session, never the body.
	decided, err := h.decisions.Record(ctx, principal.ActiveTenantID, campaign.ID,
		request.SessionID.String(), principal.UserID,
		string(request.Body.Decision), request.Body.Reason, overrides)
	if err != nil {
		return h.decisionFailure(ctx, err), nil
	}
	return prepeetapi.RecordReviewDecision201JSONResponse{
		Body:    decisionBody(decided),
		Headers: prepeetapi.RecordReviewDecision201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ListReviewDecisions answers the whole history, oldest first.
func (h *decisionHandlers) ListReviewDecisions(ctx context.Context, request prepeetapi.ListReviewDecisionsRequestObject) (prepeetapi.ListReviewDecisionsResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, err := h.campaigns.CampaignForRecruiter(ctx, principal.ActiveTenantID, request.CampaignID.String(), principal.UserID)
	if err != nil {
		return h.decisionFailure(ctx, err), nil
	}

	history, err := h.decisions.History(ctx, principal.ActiveTenantID, campaign.ID, request.SessionID.String())
	if err != nil {
		return h.decisionFailure(ctx, err), nil
	}
	body := prepeetapi.ReviewDecisionList{
		Decisions: make([]prepeetapi.ReviewDecisionView, 0, len(history)),
	}
	for _, decision := range history {
		body.Decisions = append(body.Decisions, decisionBody(decision))
	}
	return prepeetapi.ListReviewDecisions200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ListReviewDecisions200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// decisionFailure translates the refusals the decision surface can meet.
func (h *decisionHandlers) decisionFailure(ctx context.Context, err error) failure {
	base := h.authentication.failed(ctx, err)
	switch {
	case errors.Is(err, ErrCampaignNoAccess):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no campaign at that identifier."
	case errors.Is(err, ErrReviewSessionMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no screening session at that identifier on this campaign."
	case errors.Is(err, ErrDecisionNotReady):
		base.status = http.StatusConflict
		base.code = "REVIEW_NOT_READY"
		base.message = "No evaluation is published to decide against yet. The review appears the moment it is."
	case errors.Is(err, ErrDecisionReasonMissing):
		base.status = http.StatusConflict
		base.code = "REASON_REQUIRED"
		base.message = "A decision requires a reason. What you write is part of the record an appeal reads."
	case errors.Is(err, ErrOverrideIncompleteAPI):
		base.status = http.StatusConflict
		base.code = "OVERRIDE_INCOMPLETE"
		base.message = "An override needs the band you read and why. A disagreement without its reasoning is not recordable."
	case errors.Is(err, ErrOverrideUnknownCompetency):
		base.status = http.StatusConflict
		base.code = "OVERRIDE_UNKNOWN_COMPETENCY"
		base.message = "That override names a competency the evaluation did not assess, so there is no band to disagree with."
	case errors.Is(err, ErrDecisionInvalid):
		base.status = http.StatusConflict
		base.code = "DECISION_INVALID"
		base.message = "That is not a recordable decision."
	}
	return base
}

// decisionBody encodes one decision for the wire.
func decisionBody(decision ReviewDecisionView) prepeetapi.ReviewDecisionView {
	body := prepeetapi.ReviewDecisionView{
		ID:           campaignUUID(decision.ID),
		SessionID:    campaignUUID(decision.SessionID),
		DecidedBy:    campaignUUID(decision.DecidedBy),
		Decision:     prepeetapi.ReviewDecisionViewDecision(decision.Decision),
		Reason:       decision.Reason,
		EvaluationID: campaignUUID(decision.EvaluationID),
		ResultDigest: decision.ResultDigest,
		RubricDigest: decision.RubricDigest,
		Overrides:    make([]prepeetapi.BandOverrideView, 0, len(decision.Overrides)),
		DecidedAt:    decision.DecidedAt,
	}
	for _, override := range decision.Overrides {
		body.Overrides = append(body.Overrides, prepeetapi.BandOverrideView{
			CompetencyID: override.CompetencyID,
			RecordedBand: override.RecordedBand,
			OverrideBand: override.OverrideBand,
			Rationale:    override.Rationale,
		})
	}
	return body
}

func (f failure) VisitRecordReviewDecisionResponse(w http.ResponseWriter) error {
	return f.write(w)
}
func (f failure) VisitListReviewDecisionsResponse(w http.ResponseWriter) error {
	return f.write(w)
}
