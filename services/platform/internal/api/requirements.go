package api

import (
	"context"
	"errors"
	"net/http"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// SCR-03's recruiter surface: the job description a campaign draws on, and the
// requirements extracted from it, reviewable and correctable before the campaign
// opens and frozen once it does.

// Requirement is one requirement as this surface serves it, span-linked to the
// job context it came from.
type Requirement struct {
	ID        string
	Text      string
	SpanStart int
	SpanEnd   int
	Status    string
}

// The refusals this surface owns.
var (
	// ErrRequirementsFrozen means the campaign has opened, so its requirements
	// can no longer change.
	ErrRequirementsFrozen = errors.New("api: the campaign's requirements are frozen")
	// ErrRequirementMissing means the requirement named is not on the campaign,
	// collapsed with somebody else's so an id cannot be probed.
	ErrRequirementMissing = errors.New("api: no such requirement")
)

// Requirements is what this package needs of the job-context half of recruiting.
type Requirements interface {
	// SubmitJobContext stores the job description and returns the requirements
	// extracted from it. ErrRequirementsFrozen when the campaign has opened.
	SubmitJobContext(ctx context.Context, tenantID, campaignID, sourceText string) ([]Requirement, error)
	// ListRequirements answers a campaign's requirements.
	ListRequirements(ctx context.Context, tenantID, campaignID string) ([]Requirement, error)
	// CorrectRequirement changes a requirement's wording or rejects it.
	// ErrRequirementMissing when it is not on the campaign, ErrRequirementsFrozen
	// once the campaign has opened.
	CorrectRequirement(ctx context.Context, tenantID, campaignID, requirementID, text, status string) (Requirement, error)
}

// SubmitJobContext stores the job description and extracts its requirements.
func (h *invitationHandlers) SubmitJobContext(ctx context.Context, request prepeetapi.SubmitJobContextRequestObject) (prepeetapi.SubmitJobContextResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	requirements, err := h.requirements.SubmitJobContext(ctx, principal.ActiveTenantID, campaign.ID, request.Body.SourceText)
	if err != nil {
		return h.requirementFailure(ctx, err), nil
	}
	return prepeetapi.SubmitJobContext200JSONResponse{
		Body:    requirementList(requirements),
		Headers: prepeetapi.SubmitJobContext200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ListRequirements answers a campaign's requirements for review.
func (h *invitationHandlers) ListRequirements(ctx context.Context, request prepeetapi.ListRequirementsRequestObject) (prepeetapi.ListRequirementsResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	requirements, err := h.requirements.ListRequirements(ctx, principal.ActiveTenantID, campaign.ID)
	if err != nil {
		return h.requirementFailure(ctx, err), nil
	}
	return prepeetapi.ListRequirements200JSONResponse{
		Body:    requirementList(requirements),
		Headers: prepeetapi.ListRequirements200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// CorrectRequirement fixes or rejects one requirement.
func (h *invitationHandlers) CorrectRequirement(ctx context.Context, request prepeetapi.CorrectRequirementRequestObject) (prepeetapi.CorrectRequirementResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	text := ""
	if request.Body.Text != nil {
		text = *request.Body.Text
	}
	corrected, err := h.requirements.CorrectRequirement(ctx,
		principal.ActiveTenantID, campaign.ID, request.RequirementID.String(), text, string(request.Body.Status))
	if err != nil {
		return h.requirementFailure(ctx, err), nil
	}
	return prepeetapi.CorrectRequirement200JSONResponse{
		Body:    requirementBody(corrected),
		Headers: prepeetapi.CorrectRequirement200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// requirementFailure translates the requirement surface's refusals.
func (h *invitationHandlers) requirementFailure(ctx context.Context, err error) failure {
	base := h.authentication.failed(ctx, err)
	switch {
	case errors.Is(err, ErrCampaignNoAccess):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no campaign at that identifier."
	case errors.Is(err, ErrRequirementMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no such requirement on this campaign."
	case errors.Is(err, ErrRequirementsFrozen):
		base.status = http.StatusConflict
		base.code = "REQUIREMENTS_FROZEN"
		base.message = "This campaign has opened, so its requirements are fixed."
	}
	return base
}

func requirementBody(requirement Requirement) prepeetapi.Requirement {
	return prepeetapi.Requirement{
		ID:        campaignUUID(requirement.ID),
		Text:      requirement.Text,
		SpanStart: requirement.SpanStart,
		SpanEnd:   requirement.SpanEnd,
		Status:    prepeetapi.RequirementStatus(requirement.Status),
	}
}

func requirementList(requirements []Requirement) prepeetapi.RequirementList {
	entries := make([]prepeetapi.Requirement, 0, len(requirements))
	for _, requirement := range requirements {
		entries = append(entries, requirementBody(requirement))
	}
	return prepeetapi.RequirementList{Requirements: entries}
}

func (f failure) VisitSubmitJobContextResponse(w http.ResponseWriter) error   { return f.write(w) }
func (f failure) VisitListRequirementsResponse(w http.ResponseWriter) error   { return f.write(w) }
func (f failure) VisitCorrectRequirementResponse(w http.ResponseWriter) error { return f.write(w) }
