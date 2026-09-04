package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// Re-review: REV-06.
//
// An appeal is raised against the latest recorded decision, frozen at that
// moment, assigned to somebody other than the person whose decision it is,
// and answered once, whole. The requester and the resolver are the
// session, never the body, for the reason every actor on this surface is.

// The refusals this surface translates.
var (
	ErrAppealNothingDecided = errors.New("api: no decision exists to appeal")
	ErrAppealReasonMissing  = errors.New("api: an appeal requires a reason")
	ErrAppealSelfReviewAPI  = errors.New("api: the original reviewer cannot re-review their own decision")
	ErrAppealMissing        = errors.New("api: no such re-review on this campaign")
	ErrAppealDone           = errors.New("api: this re-review is already resolved")
	ErrAppealNotAssignee    = errors.New("api: only the assigned reviewer resolves a re-review")
	ErrAppealIncomplete     = errors.New("api: a resolution needs its outcome, rationale and disclosure")
)

// FrozenEvidenceView names what the appealed decision was informed by.
type FrozenEvidenceView struct {
	EvaluationID string
	ResultDigest string
	RubricDigest string
	BundleDigest string
}

// ResolutionView is an answered appeal's ending.
type ResolutionView struct {
	Outcome    string
	Rationale  string
	Disclosure string
	ResolvedBy string
	ResolvedAt time.Time
}

// ReReviewView is one appeal at the port.
type ReReviewView struct {
	ID               string
	SessionID        string
	RequestedBy      string
	Reason           string
	AppealedDecision string
	OriginalReviewer string
	Frozen           FrozenEvidenceView
	AssignedTo       string
	RaisedAt         time.Time
	DueAt            time.Time
	Resolution       *ResolutionView
}

// Appeals is what this surface needs of the composition in cmd.
type Appeals interface {
	Raise(ctx context.Context, tenantID, campaignID, sessionID, requestedBy, reason string) (ReReviewView, error)
	Assign(ctx context.Context, tenantID, campaignID, appealID, assignee string) (ReReviewView, error)
	Resolve(ctx context.Context, tenantID, campaignID, appealID, resolvedBy, outcome, rationale, disclosure string) (ReReviewView, error)
	List(ctx context.Context, tenantID, campaignID, sessionID string) ([]ReReviewView, error)
}

// appealHandlers serves REV-06's surface.
type appealHandlers struct {
	authentication *authentication
	campaigns      Recruiting
	appeals        Appeals
}

func (h *appealHandlers) caller(ctx context.Context) (Principal, *failure) {
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

// admit resolves the campaign for the caller through the join.
func (h *appealHandlers) admit(ctx context.Context, principal Principal, campaignID string) (Campaign, *failure) {
	campaign, err := h.campaigns.CampaignForRecruiter(ctx, principal.ActiveTenantID, campaignID, principal.UserID)
	if err != nil {
		refusal := h.appealFailure(ctx, err)
		return Campaign{}, &refusal
	}
	return campaign, nil
}

// RaiseReReview opens the appeal, freezing at this moment.
func (h *appealHandlers) RaiseReReview(ctx context.Context, request prepeetapi.RaiseReReviewRequestObject) (prepeetapi.RaiseReReviewResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	raised, err := h.appeals.Raise(ctx, principal.ActiveTenantID, campaign.ID,
		request.SessionID.String(), principal.UserID, request.Body.Reason)
	if err != nil {
		return h.appealFailure(ctx, err), nil
	}
	return prepeetapi.RaiseReReview201JSONResponse{
		Body:    reReviewBody(raised),
		Headers: prepeetapi.RaiseReReview201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// AssignReReview seats the reviewer who answers.
func (h *appealHandlers) AssignReReview(ctx context.Context, request prepeetapi.AssignReReviewRequestObject) (prepeetapi.AssignReReviewResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	assigned, err := h.appeals.Assign(ctx, principal.ActiveTenantID, campaign.ID,
		request.ReReviewID.String(), request.Body.Assignee.String())
	if err != nil {
		return h.appealFailure(ctx, err), nil
	}
	return prepeetapi.AssignReReview200JSONResponse{
		Body:    reReviewBody(assigned),
		Headers: prepeetapi.AssignReReview200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ResolveReReview answers, once, as the session's user.
func (h *appealHandlers) ResolveReReview(ctx context.Context, request prepeetapi.ResolveReReviewRequestObject) (prepeetapi.ResolveReReviewResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	resolved, err := h.appeals.Resolve(ctx, principal.ActiveTenantID, campaign.ID,
		request.ReReviewID.String(), principal.UserID,
		string(request.Body.Outcome), request.Body.Rationale, request.Body.Disclosure)
	if err != nil {
		return h.appealFailure(ctx, err), nil
	}
	return prepeetapi.ResolveReReview200JSONResponse{
		Body:    reReviewBody(resolved),
		Headers: prepeetapi.ResolveReReview200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ListReReviews answers the session's appeals, oldest first.
func (h *appealHandlers) ListReReviews(ctx context.Context, request prepeetapi.ListReReviewsRequestObject) (prepeetapi.ListReReviewsResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	appeals, err := h.appeals.List(ctx, principal.ActiveTenantID, campaign.ID, request.SessionID.String())
	if err != nil {
		return h.appealFailure(ctx, err), nil
	}
	body := prepeetapi.ReReviewList{
		ReReviews: make([]prepeetapi.ReReviewView, 0, len(appeals)),
	}
	for _, appeal := range appeals {
		body.ReReviews = append(body.ReReviews, reReviewBody(appeal))
	}
	return prepeetapi.ListReReviews200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ListReReviews200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// appealFailure translates the refusals the appeal surface can meet.
func (h *appealHandlers) appealFailure(ctx context.Context, err error) failure {
	base := h.authentication.failed(ctx, err)
	switch {
	case errors.Is(err, ErrCampaignNoAccess):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no campaign at that identifier."
	case errors.Is(err, ErrAppealMissing), errors.Is(err, ErrReviewSessionMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no re-review at that identifier on this campaign."
	case errors.Is(err, ErrAppealNothingDecided):
		base.status = http.StatusConflict
		base.code = "APPEAL_NO_DECISION"
		base.message = "Nothing has been decided on this screening yet, so there is nothing to appeal."
	case errors.Is(err, ErrAppealReasonMissing):
		base.status = http.StatusConflict
		base.code = "REASON_REQUIRED"
		base.message = "An appeal requires a reason."
	case errors.Is(err, ErrAppealSelfReviewAPI):
		base.status = http.StatusConflict
		base.code = "SELF_REVIEW_FORBIDDEN"
		base.message = "The original reviewer cannot re-review their own decision."
	case errors.Is(err, ErrAppealDone):
		base.status = http.StatusConflict
		base.code = "APPEAL_RESOLVED"
		base.message = "This re-review is already resolved; its answer stands."
	case errors.Is(err, ErrAppealNotAssignee):
		base.status = http.StatusConflict
		base.code = "RESOLVER_NOT_ASSIGNED"
		base.message = "Only the assigned reviewer resolves a re-review."
	case errors.Is(err, ErrAppealIncomplete):
		base.status = http.StatusConflict
		base.code = "RESOLUTION_INCOMPLETE"
		base.message = "A resolution needs its outcome, rationale and the disclosure the candidate is permitted."
	}
	return base
}

// reReviewBody encodes one appeal for the wire.
func reReviewBody(appeal ReReviewView) prepeetapi.ReReviewView {
	body := prepeetapi.ReReviewView{
		ID:               campaignUUID(appeal.ID),
		SessionID:        campaignUUID(appeal.SessionID),
		RequestedBy:      campaignUUID(appeal.RequestedBy),
		Reason:           appeal.Reason,
		AppealedDecision: campaignUUID(appeal.AppealedDecision),
		OriginalReviewer: campaignUUID(appeal.OriginalReviewer),
		RaisedAt:         appeal.RaisedAt,
		DueAt:            appeal.DueAt,
	}
	body.Frozen.EvaluationID = campaignUUID(appeal.Frozen.EvaluationID)
	body.Frozen.ResultDigest = appeal.Frozen.ResultDigest
	body.Frozen.RubricDigest = appeal.Frozen.RubricDigest
	body.Frozen.BundleDigest = appeal.Frozen.BundleDigest
	if appeal.AssignedTo != "" {
		assignee := campaignUUID(appeal.AssignedTo)
		body.AssignedTo = &assignee
	}
	if appeal.Resolution != nil {
		resolution := struct {
			CandidateDisclosure string                                   `json:"candidate_disclosure"`
			Outcome             prepeetapi.ReReviewViewResolutionOutcome `json:"outcome"`
			Rationale           string                                   `json:"rationale"`
			ResolvedAt          time.Time                                `json:"resolved_at"`
			ResolvedBy          openapi_types.UUID                       `json:"resolved_by"`
		}{
			CandidateDisclosure: appeal.Resolution.Disclosure,
			Outcome:             prepeetapi.ReReviewViewResolutionOutcome(appeal.Resolution.Outcome),
			Rationale:           appeal.Resolution.Rationale,
			ResolvedAt:          appeal.Resolution.ResolvedAt,
			ResolvedBy:          campaignUUID(appeal.Resolution.ResolvedBy),
		}
		body.Resolution = &resolution
	}
	return body
}

func (f failure) VisitRaiseReReviewResponse(w http.ResponseWriter) error   { return f.write(w) }
func (f failure) VisitAssignReReviewResponse(w http.ResponseWriter) error  { return f.write(w) }
func (f failure) VisitResolveReReviewResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitListReReviewsResponse(w http.ResponseWriter) error   { return f.write(w) }
