package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// Campaign is one campaign as this surface serves it.
//
// Declared here rather than borrowed from the recruiting context, because
// internal/api and internal/recruiting are separate bounded contexts and only
// cmd may see both. The first draft of this file imported recruiting directly,
// reasoning that a second copy of the types would be a translation layer with
// nothing to translate; the architecture gate refused it, and the gate is
// right: the moment the two vocabularies need to differ, a shared type forces
// them to agree, and the place that absorbs the difference must be cmd, where
// every other pair of contexts already meets.
type Campaign struct {
	ID            string
	Name          string
	Status        string
	RoleReference string
	Jurisdiction  string
	OpenedAt      *time.Time
	CreatedAt     time.Time
	// CreatedBy is written by the handler from the session, never read from a
	// request. A request-supplied creator would let anybody put a campaign in
	// a colleague's name.
	CreatedBy string
	TenantID  string
}

// PinChoice is one artifact chosen for a campaign, by reference. Resolution to
// a digest happens behind the port, against the registry.
type PinChoice struct {
	Type      string
	Reference string
}

// The refusals this surface translates, each to its own code because the
// recruiter's next step differs per code: a missing determination is
// counsel's to fix, an unpublished artifact is the author's, and an
// incomplete configuration is their own.
var (
	// ErrCampaignNoAccess covers both a campaign the caller is not on and one
	// that does not exist, deliberately: telling those apart would let
	// anybody enumerate who is hiring for what.
	ErrCampaignNoAccess = errors.New("api: no such campaign")
	// ErrCampaignNoDetermination is ADR-0020's refusal: the jurisdiction has
	// no recorded legal determination, so nothing may open there.
	ErrCampaignNoDetermination = errors.New("api: no determination for this jurisdiction")
	// ErrCampaignArtifactUnpublished means a chosen reference does not
	// resolve to a published artifact.
	ErrCampaignArtifactUnpublished = errors.New("api: an artifact is not published")
	// ErrCampaignIncomplete means a required artifact kind is unchosen.
	ErrCampaignIncomplete = errors.New("api: the campaign configuration is incomplete")
	// ErrCampaignDuplicatePin means two choices name one artifact kind.
	ErrCampaignDuplicatePin = errors.New("api: two choices name the same artifact kind")
	// ErrCampaignNotDraft means the configuration is already frozen.
	ErrCampaignNotDraft = errors.New("api: the campaign is not a draft")
)

// Recruiting is what this package needs of the campaign context.
//
// Every method that concerns a particular campaign takes the caller, because
// per-campaign authority is the campaign_recruiter join rather than a
// capability: campaign.manage is scoped and Authorize never names a scope, so
// the database join is not a convenience here, it is the enforcement.
type Recruiting interface {
	CreateDraft(ctx context.Context, campaign Campaign) (Campaign, error)
	List(ctx context.Context, tenantID string) ([]Campaign, error)
	// CampaignForRecruiter answers ErrCampaignNoAccess both for a campaign
	// the caller is not on and for one that does not exist.
	CampaignForRecruiter(ctx context.Context, tenantID, campaignID, userID string) (Campaign, error)
	// Open resolves the choices against the registry and the jurisdiction's
	// determination, and freezes them. The caller must already have been
	// admitted by CampaignForRecruiter.
	Open(ctx context.Context, campaign Campaign, pins []PinChoice) (Campaign, error)
	GrantAccess(ctx context.Context, tenantID, campaignID, userID, grantedBy string) error
}

// campaignHandlers serves SCR-01's surface.
type campaignHandlers struct {
	authentication *authentication
	campaigns      Recruiting
}

// caller authorizes the request with the capability the contract declares and
// answers the principal, or the refusal to return.
func (h *campaignHandlers) caller(ctx context.Context) (Principal, *failure) {
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

// CreateCampaign starts a draft.
func (h *campaignHandlers) CreateCampaign(ctx context.Context, request prepeetapi.CreateCampaignRequestObject) (prepeetapi.CreateCampaignResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}

	// The creator is the session, never the body. A request-supplied creator
	// would let anybody put a campaign in a colleague's name, and the creator
	// is who joins the campaign in the transaction that creates it.
	created, err := h.campaigns.CreateDraft(ctx, Campaign{
		TenantID:      principal.ActiveTenantID,
		Name:          request.Body.Name,
		RoleReference: request.Body.RoleReference,
		Jurisdiction:  request.Body.Jurisdiction,
		CreatedBy:     principal.UserID,
	})
	if err != nil {
		return h.authentication.failed(ctx, err), nil
	}
	return prepeetapi.CreateCampaign201JSONResponse{
		Body:    campaignBody(created),
		Headers: prepeetapi.CreateCampaign201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ListCampaigns answers every campaign in the caller's workspace.
func (h *campaignHandlers) ListCampaigns(ctx context.Context, _ prepeetapi.ListCampaignsRequestObject) (prepeetapi.ListCampaignsResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}

	campaigns, err := h.campaigns.List(ctx, principal.ActiveTenantID)
	if err != nil {
		return h.authentication.failed(ctx, err), nil
	}
	entries := make([]prepeetapi.Campaign, 0, len(campaigns))
	for _, campaign := range campaigns {
		// No pins on the tenant-wide list. Existence is what campaign.read
		// grants; a campaign's configuration is for the recruiters on it.
		entries = append(entries, campaignBody(campaign))
	}
	return prepeetapi.ListCampaigns200JSONResponse{
		Body:    prepeetapi.CampaignList{Campaigns: entries},
		Headers: prepeetapi.ListCampaigns200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// GetCampaign answers one campaign, for a recruiter on it.
func (h *campaignHandlers) GetCampaign(ctx context.Context, request prepeetapi.GetCampaignRequestObject) (prepeetapi.GetCampaignResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}

	campaign, err := h.campaigns.CampaignForRecruiter(ctx,
		principal.ActiveTenantID, request.CampaignID.String(), principal.UserID)
	if err != nil {
		return h.campaignFailure(ctx, err), nil
	}
	return prepeetapi.GetCampaign200JSONResponse{
		Body:    campaignBody(campaign),
		Headers: prepeetapi.GetCampaign200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// OpenCampaign freezes a draft's configuration.
func (h *campaignHandlers) OpenCampaign(ctx context.Context, request prepeetapi.OpenCampaignRequestObject) (prepeetapi.OpenCampaignResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}

	// Admission first, through the join. Without it a member could open a
	// colleague's draft by knowing its identifier, and the write would be
	// guarded more loosely than the read.
	campaign, err := h.campaigns.CampaignForRecruiter(ctx,
		principal.ActiveTenantID, request.CampaignID.String(), principal.UserID)
	if err != nil {
		return h.campaignFailure(ctx, err), nil
	}

	pins := make([]PinChoice, 0, len(request.Body.Pins))
	for _, pin := range request.Body.Pins {
		pins = append(pins, PinChoice{Type: pin.Type, Reference: pin.Reference})
	}
	opened, err := h.campaigns.Open(ctx, campaign, pins)
	if err != nil {
		return h.campaignFailure(ctx, err), nil
	}
	return prepeetapi.OpenCampaign200JSONResponse{
		Body:    campaignBody(opened),
		Headers: prepeetapi.OpenCampaign200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// GrantCampaignAccess puts a member on the campaign.
func (h *campaignHandlers) GrantCampaignAccess(ctx context.Context, request prepeetapi.GrantCampaignAccessRequestObject) (prepeetapi.GrantCampaignAccessResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}

	// Only a recruiter already on the campaign may add another, decided by the
	// same join that scopes reading it.
	if _, err := h.campaigns.CampaignForRecruiter(ctx,
		principal.ActiveTenantID, request.CampaignID.String(), principal.UserID); err != nil {
		return h.campaignFailure(ctx, err), nil
	}

	if err := h.campaigns.GrantAccess(ctx,
		principal.ActiveTenantID, request.CampaignID.String(),
		request.Body.UserID.String(), principal.UserID); err != nil {
		return h.authentication.failed(ctx, err), nil
	}
	return prepeetapi.GrantCampaignAccess204Response{
		Headers: prepeetapi.GrantCampaignAccess204ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// campaignFailure translates the campaign context's refusals.
//
// Each opening failure keeps its own code because the recruiter's next step
// differs per code: a missing determination is counsel's to fix, an unpublished
// artifact is the author's, and an incomplete configuration is their own.
func (h *campaignHandlers) campaignFailure(ctx context.Context, err error) failure {
	base := h.authentication.failed(ctx, err)
	switch {
	case errors.Is(err, ErrCampaignNoAccess):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no campaign at that identifier."
	case errors.Is(err, ErrCampaignNoDetermination):
		base.status = http.StatusUnprocessableEntity
		base.code = "NO_DETERMINATION"
		base.message = "Screening has no recorded legal determination for this jurisdiction yet, " +
			"so no campaign can open there. This is not something you can fix in the product."
	case errors.Is(err, ErrCampaignArtifactUnpublished):
		base.status = http.StatusUnprocessableEntity
		base.code = "ARTIFACT_NOT_PUBLISHED"
		base.message = "One of the chosen artifacts is not published: " + err.Error()
	case errors.Is(err, ErrCampaignIncomplete):
		base.status = http.StatusUnprocessableEntity
		base.code = "CONFIGURATION_INCOMPLETE"
		base.message = "The campaign is missing a required choice: " + err.Error()
	case errors.Is(err, ErrCampaignDuplicatePin):
		base.status = http.StatusUnprocessableEntity
		base.code = "CONFIGURATION_DUPLICATE"
		base.message = "Two choices name the same artifact kind: " + err.Error()
	case errors.Is(err, ErrCampaignNotDraft):
		base.status = http.StatusConflict
		base.code = "CAMPAIGN_NOT_DRAFT"
		base.message = "This campaign has already opened, so its configuration is frozen."
	}
	return base
}

// campaignBody maps the domain campaign onto the wire.
func campaignBody(campaign Campaign) prepeetapi.Campaign {
	body := prepeetapi.Campaign{
		ID:            campaignUUID(campaign.ID),
		Name:          campaign.Name,
		Status:        prepeetapi.CampaignStatus(campaign.Status),
		RoleReference: campaign.RoleReference,
		Jurisdiction:  campaign.Jurisdiction,
		CreatedAt:     campaign.CreatedAt,
	}
	if campaign.OpenedAt != nil {
		opened := *campaign.OpenedAt
		body.OpenedAt = &opened
	}
	return body
}

// campaignUUID parses a stored identifier, degrading to the zero UUID.
//
// Identifiers here are UUIDv7 strings minted by platform/id, so a parse
// failure means a test fixture rather than production data, and the zero UUID
// it degrades to is visible in any assertion rather than being a panic.
func campaignUUID(id string) openapi_types.UUID {
	var out openapi_types.UUID
	_ = out.UnmarshalText([]byte(id))
	return out
}
