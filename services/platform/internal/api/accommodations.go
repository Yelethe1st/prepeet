package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// SCR-06's surface: a candidate asks for an adjustment to how their interview is
// conducted, a recruiter answers, and the grant is applied to the session. The
// adjustment is a named change, never a reason: there is no field here for one,
// so the form cannot ask for a medical condition even by accident.

// Accommodation is one request as the candidate sees it.
type Accommodation struct {
	ID          string
	CampaignID  string
	Adjustment  string
	State       string
	RequestedAt time.Time
	DecidedAt   *time.Time
}

// The refusals this surface owns.
var (
	// ErrAccommodationUnknownAdjustment means the adjustment named is outside
	// the fixed vocabulary.
	ErrAccommodationUnknownAdjustment = errors.New("api: not a named adjustment")
	// ErrAccommodationTooLate means the interview is underway or over: the need
	// is now an incident, not a request.
	ErrAccommodationTooLate = errors.New("api: the interview has begun")
	// ErrAccommodationMissing means the request named does not exist on this
	// campaign, collapsed with somebody else's so an id cannot be probed.
	ErrAccommodationMissing = errors.New("api: no such accommodation request")
)

// CandidateAccommodations is the candidate half of SCR-06.
type CandidateAccommodations interface {
	// RequestAccommodation records a candidate's request for a campaign they
	// accepted an invitation to, refusing one made once the interview is
	// underway. ErrSessionMissing when the candidate accepted no invitation.
	RequestAccommodation(ctx context.Context, candidateID, campaignID, adjustment string) (Accommodation, error)
	// ListAccommodations answers the candidate's own requests for a campaign.
	ListAccommodations(ctx context.Context, candidateID, campaignID string) ([]Accommodation, error)
}

// RecruiterAccommodations is the recruiter half: answering a request.
type RecruiterAccommodations interface {
	// DecideAccommodation grants or declines a request on a campaign the caller
	// is on, naming the caller as the decider. ErrAccommodationMissing when the
	// request is not on the campaign.
	DecideAccommodation(ctx context.Context, tenantID, campaignID, requestID, decidedBy string, granted bool) error
}

// RequestAccommodation records the candidate's request.
func (h *screeningHandlers) RequestAccommodation(ctx context.Context, request prepeetapi.RequestAccommodationRequestObject) (prepeetapi.RequestAccommodationResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		refusal := h.authentication.rejectedSession(ctx)
		return refusal, nil
	}
	principal, err := h.authentication.identity.Authorize(ctx, presented, requiredCapabilityFrom(ctx))
	if err != nil {
		return h.authentication.failed(ctx, err), nil
	}

	accommodation, err := h.accommodations.RequestAccommodation(ctx,
		principal.UserID, request.Body.CampaignID.String(), string(request.Body.Adjustment))
	if err != nil {
		return h.accommodationFailure(ctx, err), nil
	}
	return prepeetapi.RequestAccommodation201JSONResponse{
		Body:    accommodationBody(accommodation),
		Headers: prepeetapi.RequestAccommodation201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ListAccommodations answers the candidate's own requests.
func (h *screeningHandlers) ListAccommodations(ctx context.Context, request prepeetapi.ListAccommodationsRequestObject) (prepeetapi.ListAccommodationsResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		refusal := h.authentication.rejectedSession(ctx)
		return refusal, nil
	}
	principal, err := h.authentication.identity.Authorize(ctx, presented, requiredCapabilityFrom(ctx))
	if err != nil {
		return h.authentication.failed(ctx, err), nil
	}

	accommodations, err := h.accommodations.ListAccommodations(ctx, principal.UserID, request.Params.CampaignID.String())
	if err != nil {
		return h.accommodationFailure(ctx, err), nil
	}
	entries := make([]prepeetapi.Accommodation, 0, len(accommodations))
	for _, accommodation := range accommodations {
		entries = append(entries, accommodationBody(accommodation))
	}
	return prepeetapi.ListAccommodations200JSONResponse{
		Body:    prepeetapi.AccommodationList{Accommodations: entries},
		Headers: prepeetapi.ListAccommodations200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// DecideAccommodation is the recruiter answering, on the invitation handlers
// because it shares their campaign join.
func (h *invitationHandlers) DecideAccommodation(ctx context.Context, request prepeetapi.DecideAccommodationRequestObject) (prepeetapi.DecideAccommodationResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	// The decider is the session, never the body: an accommodation decision
	// names the human accountable for it, and that is who is asking.
	if err := h.recruiterAccommodations.DecideAccommodation(ctx,
		principal.ActiveTenantID, campaign.ID, request.RequestID.String(), principal.UserID, request.Body.Granted); err != nil {
		return h.decisionFailure(ctx, err), nil
	}
	return prepeetapi.DecideAccommodation204Response{
		Headers: prepeetapi.DecideAccommodation204ResponseHeaders{CacheControl: NoStore},
	}, nil
}

func (h *invitationHandlers) decisionFailure(ctx context.Context, err error) failure {
	base := h.authentication.failed(ctx, err)
	switch {
	case errors.Is(err, ErrCampaignNoAccess):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no campaign at that identifier."
	case errors.Is(err, ErrAccommodationMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no such accommodation request on this campaign."
	}
	return base
}

// accommodationFailure translates the candidate surface's refusals.
func (h *screeningHandlers) accommodationFailure(ctx context.Context, err error) failure {
	base := h.authentication.failed(ctx, err)
	switch {
	case errors.Is(err, ErrSessionMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no such campaign to request an accommodation for."
	case errors.Is(err, ErrAccommodationUnknownAdjustment):
		base.status = http.StatusBadRequest
		base.code = string(prepeetapi.VALIDATIONFAILED)
		base.message = "That is not an adjustment that can be requested."
	case errors.Is(err, ErrAccommodationTooLate):
		base.status = http.StatusConflict
		base.code = "INTERVIEW_UNDERWAY"
		base.message = "The interview has begun. Raise this with the interviewer or support instead."
	}
	return base
}

func accommodationBody(accommodation Accommodation) prepeetapi.Accommodation {
	body := prepeetapi.Accommodation{
		ID:          campaignUUID(accommodation.ID),
		Adjustment:  prepeetapi.AccommodationAdjustment(accommodation.Adjustment),
		State:       prepeetapi.AccommodationState(accommodation.State),
		RequestedAt: accommodation.RequestedAt,
	}
	if accommodation.CampaignID != "" {
		body.CampaignID = ptrUUID(accommodation.CampaignID)
	}
	if accommodation.DecidedAt != nil {
		decided := *accommodation.DecidedAt
		body.DecidedAt = &decided
	}
	return body
}

func (f failure) VisitRequestAccommodationResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitListAccommodationsResponse(w http.ResponseWriter) error   { return f.write(w) }
func (f failure) VisitDecideAccommodationResponse(w http.ResponseWriter) error  { return f.write(w) }
