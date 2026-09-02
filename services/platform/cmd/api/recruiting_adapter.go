package main

import (
	"context"
	"errors"
	"fmt"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
)

// The campaign surface, composed.
//
// Three contexts meet here and only here: the API's port, recruiting's store
// and opening service, and content's registry. The opening service asks two
// questions it deliberately cannot answer itself, whether an artifact is
// published and what a jurisdiction's determination says, and this file is
// where those ports get their real answers.

// recruitingAdapter satisfies api.Recruiting over the store and the service,
// translating both the types and the refusals. The translation is the point:
// internal/api and internal/recruiting are separate contexts, the architecture
// gate refuses either importing the other, and cmd is where every pair of
// contexts already meets.
type recruitingAdapter struct {
	store   *recruiting.Store
	service *recruiting.Service
}

// apiCampaign maps the domain campaign onto the surface's vocabulary.
func apiCampaign(campaign recruiting.Campaign) api.Campaign {
	return api.Campaign{
		ID: campaign.ID, TenantID: campaign.TenantID, Name: campaign.Name,
		Status: string(campaign.Status), RoleReference: campaign.RoleReference,
		Jurisdiction: campaign.Jurisdiction, OpenedAt: campaign.OpenedAt,
		CreatedAt: campaign.CreatedAt, CreatedBy: campaign.CreatedBy,
	}
}

// campaignError maps each domain refusal onto the surface's sentinel, keeping
// the original wrapped so its detail, which artifact, which kind, survives
// into the message a recruiter reads.
func campaignError(err error) error {
	for domain, surface := range map[error]error{
		recruiting.ErrNoAccess:                api.ErrCampaignNoAccess,
		recruiting.ErrNoDetermination:         api.ErrCampaignNoDetermination,
		recruiting.ErrNotPublished:            api.ErrCampaignArtifactUnpublished,
		recruiting.ErrIncompleteConfiguration: api.ErrCampaignIncomplete,
		recruiting.ErrDuplicateArtifact:       api.ErrCampaignDuplicatePin,
		recruiting.ErrNotDraft:                api.ErrCampaignNotDraft,
	} {
		if errors.Is(err, domain) {
			return fmt.Errorf("%w: %s", surface, err.Error())
		}
	}
	return err
}

func newRecruitingAdapter(store *recruiting.Store, artifacts *content.Store) recruitingAdapter {
	return recruitingAdapter{
		store: store,
		service: recruiting.NewService(
			publishedArtifacts{registry: artifacts},
			determinationSource{store: store},
		),
	}
}

func (a recruitingAdapter) CreateDraft(ctx context.Context, campaign api.Campaign) (api.Campaign, error) {
	created, err := a.store.CreateDraft(ctx, recruiting.Campaign{
		TenantID: campaign.TenantID, Name: campaign.Name,
		RoleReference: campaign.RoleReference, Jurisdiction: campaign.Jurisdiction,
		CreatedBy: campaign.CreatedBy,
	})
	if err != nil {
		return api.Campaign{}, campaignError(err)
	}
	return apiCampaign(created), nil
}

func (a recruitingAdapter) List(ctx context.Context, tenantID string) ([]api.Campaign, error) {
	campaigns, err := a.store.List(ctx, tenantID)
	if err != nil {
		return nil, campaignError(err)
	}
	out := make([]api.Campaign, 0, len(campaigns))
	for _, campaign := range campaigns {
		out = append(out, apiCampaign(campaign))
	}
	return out, nil
}

func (a recruitingAdapter) CampaignForRecruiter(ctx context.Context, tenantID, campaignID, userID string) (api.Campaign, error) {
	campaign, err := a.store.CampaignForRecruiter(ctx, tenantID, campaignID, userID)
	if err != nil {
		return api.Campaign{}, campaignError(err)
	}
	return apiCampaign(campaign), nil
}

// Open resolves and freezes in two steps that read as one to the caller.
//
// The service decides whether the campaign may open and against what; the
// store writes the decision. Resolution failures come back untouched because
// the handler translates each of them to its own code, and wrapping them here
// would break the errors.Is chain the translation depends on.
func (a recruitingAdapter) Open(ctx context.Context, campaign api.Campaign, pins []api.PinChoice) (api.Campaign, error) {
	domain := recruiting.Campaign{
		ID: campaign.ID, TenantID: campaign.TenantID, Name: campaign.Name,
		Status: recruiting.Status(campaign.Status), RoleReference: campaign.RoleReference,
		Jurisdiction: campaign.Jurisdiction, CreatedBy: campaign.CreatedBy,
	}
	requests := make([]recruiting.PinRequest, 0, len(pins))
	for _, pin := range pins {
		requests = append(requests, recruiting.PinRequest{Type: pin.Type, Reference: pin.Reference})
	}
	opening, err := a.service.ResolveOpening(ctx, domain, requests)
	if err != nil {
		return api.Campaign{}, campaignError(err)
	}
	opened, err := a.store.Open(ctx, domain, opening)
	if err != nil {
		return api.Campaign{}, campaignError(err)
	}
	return apiCampaign(opened), nil
}

func (a recruitingAdapter) GrantAccess(ctx context.Context, tenantID, campaignID, userID, grantedBy string) error {
	if err := a.store.GrantAccess(ctx, tenantID, campaignID, userID, grantedBy); err != nil {
		return campaignError(err)
	}
	return nil
}

// publishedArtifacts answers the opening service from the content registry.
type publishedArtifacts struct {
	registry *content.Store
}

// PublishedArtifact resolves a reference and refuses anything not published.
//
// The two cases are one answer on purpose, matching the port's contract: a
// caller that could ask "does it exist" apart from "is it published" would
// eventually ask only the first, and a campaign would pin a draft.
func (p publishedArtifacts) PublishedArtifact(ctx context.Context, tenantID, reference string) (recruiting.Artifact, error) {
	artifact, err := p.registry.Resolve(ctx, reference, tenantID)
	if err != nil {
		return recruiting.Artifact{}, fmt.Errorf("resolving %s: %w", reference, err)
	}
	if artifact.Status != content.StatusPublished {
		return recruiting.Artifact{}, fmt.Errorf("%s is %s, not published", reference, artifact.Status)
	}
	return recruiting.Artifact{
		Reference: artifact.Reference,
		Type:      artifact.Type,
		Digest:    artifact.Digest,
		Version:   artifact.Version,
	}, nil
}

// determinationSource answers the opening service from the determinations
// table recruiting already owns.
type determinationSource struct {
	store *recruiting.Store
}

func (d determinationSource) LatestDetermination(ctx context.Context, jurisdiction string) (recruiting.Determination, error) {
	determination, err := d.store.LatestDetermination(ctx, jurisdiction)
	if errors.Is(err, recruiting.ErrNoDetermination) {
		return recruiting.Determination{}, err
	}
	if err != nil {
		return recruiting.Determination{}, fmt.Errorf("reading the determination for %s: %w", jurisdiction, err)
	}
	return determination, nil
}
