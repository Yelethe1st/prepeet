package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// Invitation is one invitation as this surface serves it.
//
// Declared here rather than borrowed from recruiting, for the reason the
// Campaign type is: internal/api and internal/recruiting are separate contexts
// and only cmd sees both. The token is deliberately absent from this type at
// every layer: it exists for the one call that emails it and is never returned.
type Invitation struct {
	ID         string
	CampaignID string
	Recipient  string
	// Status is the recruiter-facing state: live, expired, accepted, declined,
	// revoked or superseded. Expiry is computed behind the port, so a live link
	// past its window already reads expired here.
	Status    string
	IssuedAt  time.Time
	ExpiresAt time.Time
	Delivery  InvitationDelivery
}

// InvitationDelivery is what became of the email that carried the link, so a
// delivery failure is visible and a recruiter knows whether to resend.
type InvitationDelivery struct {
	Status    string
	Attempts  int
	LastError string
}

// The refusals this surface translates. Each is its own sentinel because the
// recruiter's next step differs: a closed door is a 409 they can wait out, a
// missing invitation is a 404, and an ending a resend must not overwrite is a
// 409 that is not theirs to force.
var (
	// ErrCampaignNotOpen means an invitation was issued against a campaign that
	// is not open, whose configuration is therefore not fixed.
	ErrCampaignNotOpen = errors.New("api: the campaign is not open")
	// ErrInvitationMissing covers both an unknown invitation and one on another
	// campaign, deliberately collapsed so an id cannot be probed across
	// campaigns.
	ErrInvitationMissing = errors.New("api: no such invitation")
	// ErrInvitationNotResendable means the invitation already has an ending a
	// resend would overwrite: accepted, declined or revoked.
	ErrInvitationNotResendable = errors.New("api: the invitation cannot be resent")
)

// Invitations is what this package needs of the invitation half of recruiting.
//
// Issue and Resend take the resolved Campaign rather than an id because the
// caller has already been admitted to it by CampaignForRecruiter, and issuing
// needs the campaign's open state and the details the email names. List and
// Revoke take the tenant and campaign for the same join-scoped reads.
type Invitations interface {
	Issue(ctx context.Context, campaign Campaign, recipient, issuedBy string) (Invitation, error)
	ListInvitations(ctx context.Context, tenantID, campaignID string) ([]Invitation, error)
	Revoke(ctx context.Context, tenantID, campaignID, invitationID string) (Invitation, error)
	Resend(ctx context.Context, campaign Campaign, invitationID, issuedBy string) (Invitation, error)
}

// invitationHandlers serves SCR-04's surface.
//
// It holds both ports because every operation first resolves the campaign
// through the recruiter join, which is the per-campaign enforcement, and then
// acts on its invitations.
type invitationHandlers struct {
	authentication *authentication
	campaigns      Recruiting
	invitations    Invitations
}

func (h *invitationHandlers) caller(ctx context.Context) (Principal, *failure) {
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

// admit resolves the campaign for the caller through the join, or the refusal.
func (h *invitationHandlers) admit(ctx context.Context, principal Principal, campaignID string) (Campaign, *failure) {
	campaign, err := h.campaigns.CampaignForRecruiter(ctx, principal.ActiveTenantID, campaignID, principal.UserID)
	if err != nil {
		refusal := h.invitationFailure(ctx, err)
		return Campaign{}, &refusal
	}
	return campaign, nil
}

// ListInvitations answers one campaign's roster, for a recruiter on it.
func (h *invitationHandlers) ListInvitations(ctx context.Context, request prepeetapi.ListInvitationsRequestObject) (prepeetapi.ListInvitationsResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	invitations, err := h.invitations.ListInvitations(ctx, principal.ActiveTenantID, campaign.ID)
	if err != nil {
		return h.invitationFailure(ctx, err), nil
	}
	entries := make([]prepeetapi.Invitation, 0, len(invitations))
	for _, invitation := range invitations {
		entries = append(entries, invitationBody(invitation))
	}
	return prepeetapi.ListInvitations200JSONResponse{
		Body:    prepeetapi.InvitationList{Invitations: entries},
		Headers: prepeetapi.ListInvitations200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// IssueInvitation mints a link and emails it, to a recipient on an open campaign.
func (h *invitationHandlers) IssueInvitation(ctx context.Context, request prepeetapi.IssueInvitationRequestObject) (prepeetapi.IssueInvitationResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	// The issuer is the session, never the body, the same rule that governs a
	// campaign's creator: who issued an invitation is an accountability record.
	issued, err := h.invitations.Issue(ctx, campaign, string(request.Body.Recipient), principal.UserID)
	if err != nil {
		return h.invitationFailure(ctx, err), nil
	}
	return prepeetapi.IssueInvitation201JSONResponse{
		Body:    invitationBody(issued),
		Headers: prepeetapi.IssueInvitation201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// RevokeInvitation stops a live link without deleting the record it leaves.
func (h *invitationHandlers) RevokeInvitation(ctx context.Context, request prepeetapi.RevokeInvitationRequestObject) (prepeetapi.RevokeInvitationResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	revoked, err := h.invitations.Revoke(ctx, principal.ActiveTenantID, campaign.ID, request.InvitationID.String())
	if err != nil {
		return h.invitationFailure(ctx, err), nil
	}
	return prepeetapi.RevokeInvitation200JSONResponse{
		Body:    invitationBody(revoked),
		Headers: prepeetapi.RevokeInvitation200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ResendInvitation sends a fresh link to the same recipient, superseding the old.
func (h *invitationHandlers) ResendInvitation(ctx context.Context, request prepeetapi.ResendInvitationRequestObject) (prepeetapi.ResendInvitationResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	fresh, err := h.invitations.Resend(ctx, campaign, request.InvitationID.String(), principal.UserID)
	if err != nil {
		return h.invitationFailure(ctx, err), nil
	}
	return prepeetapi.ResendInvitation201JSONResponse{
		Body:    invitationBody(fresh),
		Headers: prepeetapi.ResendInvitation201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// invitationFailure translates the refusals the invitation surface can meet.
func (h *invitationHandlers) invitationFailure(ctx context.Context, err error) failure {
	base := h.authentication.failed(ctx, err)
	switch {
	case errors.Is(err, ErrCampaignNoAccess):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no campaign at that identifier."
	case errors.Is(err, ErrInvitationMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no such invitation on this campaign."
	case errors.Is(err, ErrCampaignNotOpen):
		base.status = http.StatusConflict
		base.code = "CAMPAIGN_NOT_OPEN"
		base.message = "This campaign is not open, so it cannot issue invitations yet."
	case errors.Is(err, ErrInvitationNotResendable):
		base.status = http.StatusConflict
		base.code = "INVITATION_NOT_RESENDABLE"
		base.message = "This invitation already has an outcome a resend would overwrite. " +
			"Issue a new invitation instead."
	}
	return base
}

// invitationBody maps the domain invitation onto the wire.
func invitationBody(invitation Invitation) prepeetapi.Invitation {
	body := prepeetapi.Invitation{
		ID:         campaignUUID(invitation.ID),
		CampaignID: campaignUUID(invitation.CampaignID),
		Recipient:  openapi_types.Email(invitation.Recipient),
		Status:     prepeetapi.InvitationStatus(invitation.Status),
		IssuedAt:   invitation.IssuedAt,
		ExpiresAt:  invitation.ExpiresAt,
		Delivery: prepeetapi.InvitationDelivery{
			Status: prepeetapi.InvitationDeliveryStatus(invitation.Delivery.Status),
		},
	}
	// Attempts and last error are present only when they say something: a zero
	// attempt count and an empty error are the absence of news, not news.
	if invitation.Delivery.Attempts > 0 {
		attempts := invitation.Delivery.Attempts
		body.Delivery.Attempts = &attempts
	}
	if invitation.Delivery.LastError != "" {
		lastError := invitation.Delivery.LastError
		body.Delivery.LastError = &lastError
	}
	return body
}

// The invitation surface's failures all render through the shared failure
// writer, so each operation needs only its Visit method.
func (f failure) VisitListInvitationsResponse(w http.ResponseWriter) error  { return f.write(w) }
func (f failure) VisitIssueInvitationResponse(w http.ResponseWriter) error  { return f.write(w) }
func (f failure) VisitRevokeInvitationResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitResendInvitationResponse(w http.ResponseWriter) error { return f.write(w) }
