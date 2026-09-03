package main

import (
	"context"
	"errors"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
)

// requirementsAdapter presents SCR-03's job-context capture as the API's port.
//
// It carries the extractor, because choosing what extracts a job description's
// requirements is composition's decision, not the API's: the deterministic
// floor is wired here, and a model-backed extractor is a later swap of this one
// argument behind the same port.
type requirementsAdapter struct {
	store     *recruiting.Store
	extractor recruiting.RequirementExtractor
}

func newRequirementsAdapter(store *recruiting.Store, extractor recruiting.RequirementExtractor) requirementsAdapter {
	return requirementsAdapter{store: store, extractor: extractor}
}

var _ api.Requirements = requirementsAdapter{}

func (a requirementsAdapter) SubmitJobContext(ctx context.Context, tenantID, campaignID, sourceText string) ([]api.Requirement, error) {
	requirements, err := a.store.SubmitJobContext(ctx, tenantID, campaignID, sourceText, a.extractor)
	if err != nil {
		return nil, requirementError(err)
	}
	return apiRequirements(requirements), nil
}

func (a requirementsAdapter) ListRequirements(ctx context.Context, tenantID, campaignID string) ([]api.Requirement, error) {
	requirements, err := a.store.RequirementsForCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return nil, requirementError(err)
	}
	return apiRequirements(requirements), nil
}

func (a requirementsAdapter) CorrectRequirement(ctx context.Context, tenantID, campaignID, requirementID, text, status string) (api.Requirement, error) {
	corrected, err := a.store.CorrectRequirement(ctx, tenantID, campaignID, requirementID, text, status)
	if err != nil {
		return api.Requirement{}, requirementError(err)
	}
	return apiRequirement(corrected), nil
}

func apiRequirement(requirement recruiting.Requirement) api.Requirement {
	return api.Requirement{
		ID: requirement.ID, Text: requirement.Text,
		SpanStart: requirement.SpanStart, SpanEnd: requirement.SpanEnd, Status: requirement.Status,
	}
}

func apiRequirements(requirements []recruiting.Requirement) []api.Requirement {
	out := make([]api.Requirement, 0, len(requirements))
	for _, requirement := range requirements {
		out = append(out, apiRequirement(requirement))
	}
	return out
}

// requirementError maps recruiting's refusals onto the surface's sentinels.
func requirementError(err error) error {
	switch {
	case errors.Is(err, recruiting.ErrRequirementNotFound):
		return api.ErrRequirementMissing
	case errors.Is(err, recruiting.ErrRequirementsFrozen):
		return api.ErrRequirementsFrozen
	}
	return err
}
