package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/notification"
	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin"
	"github.com/Yelethe1st/prepeet/services/platform/platform/token"
)

// The invitation surface, composed.
//
// Four contexts meet here and only here: recruiting stores the invitation,
// notification carries the email and reports its delivery, tenantadmin names
// the employer a candidate will recognise, and catalog names the role. cmd is
// the only place allowed to see all four, so the token is minted here, the link
// is built here, and the plaintext never leaves this call: it goes into the
// email and into nothing else.
type invitationsAdapter struct {
	store     *recruiting.Store
	queue     *notification.Queue
	settings  *tenantadmin.SettingsStore
	catalogue *catalog.Service
	// baseURL is where an acceptance link points, such as https://app.prepeet.com.
	baseURL string
}

func newInvitationsAdapter(store *recruiting.Store, queue *notification.Queue,
	settings *tenantadmin.SettingsStore, catalogue *catalog.Service, baseURL string) invitationsAdapter {
	return invitationsAdapter{store: store, queue: queue, settings: settings, catalogue: catalogue, baseURL: baseURL}
}

var _ api.Invitations = invitationsAdapter{}

func (a invitationsAdapter) Issue(ctx context.Context, campaign api.Campaign, recipient, issuedBy string) (api.Invitation, error) {
	return a.issueFresh(ctx, campaign, recipient, issuedBy)
}

func (a invitationsAdapter) ListInvitations(ctx context.Context, tenantID, campaignID string) ([]api.Invitation, error) {
	invitations, err := a.store.InvitationsForCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return nil, invitationError(err)
	}
	out := make([]api.Invitation, 0, len(invitations))
	for _, invitation := range invitations {
		out = append(out, a.withDelivery(ctx, invitation))
	}
	return out, nil
}

func (a invitationsAdapter) Revoke(ctx context.Context, tenantID, campaignID, invitationID string) (api.Invitation, error) {
	revoked, err := a.store.RevokeInvitation(ctx, tenantID, campaignID, invitationID)
	if err != nil {
		return api.Invitation{}, invitationError(err)
	}
	return a.withDelivery(ctx, revoked), nil
}

// Resend reads the invitation, refuses one already answered or revoked, and
// issues a fresh link to the same recipient. The supersede that issuing does
// retires the old link, so a recipient never holds two working ones.
func (a invitationsAdapter) Resend(ctx context.Context, campaign api.Campaign, invitationID, issuedBy string) (api.Invitation, error) {
	existing, err := a.store.InvitationByID(ctx, campaign.TenantID, campaign.ID, invitationID)
	if err != nil {
		return api.Invitation{}, invitationError(err)
	}
	switch existing.Outcome {
	case recruiting.InvitationAccepted, recruiting.InvitationDeclined, recruiting.InvitationRevoked:
		// A resend here would hand a new link to someone whose answer, or whose
		// revocation, is already recorded. That is not a resend, it is a new
		// decision, and it is not this endpoint's to make.
		return api.Invitation{}, api.ErrInvitationNotResendable
	}
	return a.issueFresh(ctx, campaign, existing.Recipient, issuedBy)
}

// issueFresh is the shared body of Issue and Resend: resolve the names the
// email says, mint the token, build the link, and store the invitation with
// its email enqueued in the same transaction.
func (a invitationsAdapter) issueFresh(ctx context.Context, campaign api.Campaign, recipient, issuedBy string) (api.Invitation, error) {
	employer := employerName(ctx, a.settings, campaign.TenantID)
	role := roleTitle(ctx, a.catalogue, campaign.TenantID, campaign.RoleReference)

	minted, err := token.New(token.PurposeInvitation)
	if err != nil {
		return api.Invitation{}, fmt.Errorf("minting the invitation token: %w", err)
	}
	link := a.baseURL + "/screening/accept?token=" + minted.Plaintext
	expiry := recruiting.InvitationExpiry

	stored, err := a.store.IssueInvitation(ctx, recruiting.IssueInvitationInput{
		Campaign: recruiting.Campaign{
			ID: campaign.ID, TenantID: campaign.TenantID, Status: recruiting.Status(campaign.Status),
			Name: campaign.Name, RoleReference: campaign.RoleReference, Jurisdiction: campaign.Jurisdiction,
		},
		Recipient: recipient, IssuedBy: issuedBy,
		ExpiresAt: time.Now().Add(expiry), TokenHash: minted.Hash,
	}, func(tx pgx.Tx) (string, error) {
		return a.queue.Enqueue(ctx, tx, recipient, notification.ScreeningInvitation{
			Employer: employer, Role: role, Link: link,
			ExpiresHours: int(expiry.Hours()),
		})
	})
	if err != nil {
		return api.Invitation{}, invitationError(err)
	}
	return a.withDelivery(ctx, stored), nil
}

// withDelivery maps a stored invitation to the wire, attaching the delivery
// status of the email it was carried by. A status lookup failure is not fatal
// to reading the invitation: the delivery reads unknown and the rest stands,
// because a recruiter needs to see the invitation even when the mail record is
// briefly unreachable.
func (a invitationsAdapter) withDelivery(ctx context.Context, invitation recruiting.Invitation) api.Invitation {
	delivery := api.InvitationDelivery{Status: notification.DeliveryUnknown}
	if status, err := a.queue.DeliveryStatus(ctx, invitation.EmailID); err == nil {
		delivery = api.InvitationDelivery{
			Status: status.Status, Attempts: status.Attempts, LastError: status.LastError,
		}
	}
	return api.Invitation{
		ID: invitation.ID, CampaignID: invitation.CampaignID, Recipient: invitation.Recipient,
		Status: invitation.Status(time.Now()), IssuedAt: invitation.IssuedAt,
		ExpiresAt: invitation.ExpiresAt, Delivery: delivery,
	}
}

// employerName is what a candidate should recognise in the email: the
// workspace's display name, or its legal name, or a neutral fallback so an
// invitation is never signed by an empty string.
func employerName(ctx context.Context, settings *tenantadmin.SettingsStore, tenantID string) string {
	current, err := settings.Current(ctx, tenantID)
	if err == nil {
		if name := strings.TrimSpace(current.Settings.Organisation.DisplayName); name != "" {
			return name
		}
		if name := strings.TrimSpace(current.Settings.Organisation.LegalName); name != "" {
			return name
		}
	}
	return "The hiring team"
}

// roleTitle resolves the campaign's role reference to the human title the
// catalogue gives it, falling back to the reference itself when the catalogue
// cannot be read or does not name it.
func roleTitle(ctx context.Context, catalogue *catalog.Service, tenantID, reference string) string {
	cat, err := catalogue.Catalogue(ctx, tenantID)
	if err != nil {
		return reference
	}
	for _, role := range cat.Roles {
		if role.ID == reference {
			return role.Title
		}
	}
	return reference
}

// invitationError maps recruiting's refusals onto the surface's sentinels,
// keeping the original wrapped so its detail survives into the message.
func invitationError(err error) error {
	for domain, surface := range map[error]error{
		recruiting.ErrCampaignNotOpen:    api.ErrCampaignNotOpen,
		recruiting.ErrInvitationNotFound: api.ErrInvitationMissing,
		recruiting.ErrNoAccess:           api.ErrCampaignNoAccess,
	} {
		if errors.Is(err, domain) {
			return fmt.Errorf("%w: %s", surface, err.Error())
		}
	}
	return err
}
