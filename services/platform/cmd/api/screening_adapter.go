package main

import (
	"context"
	"errors"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin"
	"github.com/Yelethe1st/prepeet/services/platform/platform/token"
)

// The candidate-facing acceptance surface, composed.
//
// This is the one place recruiting, identity, tenantadmin and catalog meet for
// SCR-05. The candidate arrives holding a token: recruiting resolves and
// consumes it, identity turns the address it was sent to into a signed-in
// candidate, and tenantadmin and catalog name the employer and role the
// candidate sees. The token is hashed here and the hash is all that travels on;
// the plaintext is read once from the request and never stored.
type screeningAdapter struct {
	store     *recruiting.Store
	identity  *identity.Service
	settings  *tenantadmin.SettingsStore
	catalogue *catalog.Service
}

func newScreeningAdapter(store *recruiting.Store, id *identity.Service,
	settings *tenantadmin.SettingsStore, catalogue *catalog.Service) screeningAdapter {
	return screeningAdapter{store: store, identity: id, settings: settings, catalogue: catalogue}
}

var _ api.ScreeningInvitations = screeningAdapter{}

func (a screeningAdapter) Resolve(ctx context.Context, plaintext string) (api.ScreeningInvitationView, error) {
	invitation, err := a.store.ResolveInvitationByToken(ctx, token.HashOf(plaintext))
	if err != nil {
		return api.ScreeningInvitationView{}, screeningError(err)
	}
	return a.viewFor(ctx, invitation), nil
}

// Accept resolves first so an unknown token is a 404 rather than the 409 a
// spent one gets, then consumes the invitation and signs the candidate in.
//
// The consume is the single-use guard and comes before the session is issued:
// a candidate whose accept lost the race, or whose link lapsed between the
// resolve and the consume, is told so rather than being signed in against an
// invitation that was not theirs to accept. The address the session is for is
// the invitation's recipient, not anything the caller supplied.
func (a screeningAdapter) Accept(ctx context.Context, plaintext string) (api.Session, error) {
	hash := token.HashOf(plaintext)
	if _, err := a.store.ResolveInvitationByToken(ctx, hash); err != nil {
		return api.Session{}, screeningError(err)
	}
	accepted, err := a.store.AcceptInvitationByToken(ctx, hash)
	if err != nil {
		return api.Session{}, screeningError(err)
	}

	_, session, err := a.identity.ProvisionCandidateSession(ctx, accepted.Recipient)
	if err != nil {
		return api.Session{}, err
	}
	return sessionFrom(session), nil
}

// Decline resolves first for the same 404-versus-409 reason, then records the
// no. It issues no session and creates no account: declining ends the
// candidate's involvement.
func (a screeningAdapter) Decline(ctx context.Context, plaintext string) (api.ScreeningInvitationView, error) {
	hash := token.HashOf(plaintext)
	if _, err := a.store.ResolveInvitationByToken(ctx, hash); err != nil {
		return api.ScreeningInvitationView{}, screeningError(err)
	}
	declined, err := a.store.DeclineInvitationByToken(ctx, hash)
	if err != nil {
		return api.ScreeningInvitationView{}, screeningError(err)
	}
	return a.viewFor(ctx, declined), nil
}

// viewFor builds what the candidate sees: the status, the employer to contact,
// and the role. The campaign and expiry ride along only while the link is live,
// because they are what the flow after acceptance needs and mean nothing once
// the link is spent.
func (a screeningAdapter) viewFor(ctx context.Context, invitation recruiting.Invitation) api.ScreeningInvitationView {
	now := time.Now()
	view := api.ScreeningInvitationView{
		Status:   invitation.Status(now),
		Employer: employerName(ctx, a.settings, invitation.TenantID),
	}
	if campaign, err := a.store.CampaignByID(ctx, invitation.TenantID, invitation.CampaignID); err == nil {
		view.Role = roleTitle(ctx, a.catalogue, invitation.TenantID, campaign.RoleReference)
	}
	if invitation.Live(now) {
		view.CampaignID = invitation.CampaignID
		view.ExpiresAt = invitation.ExpiresAt
	}
	return view
}

// screeningError maps recruiting's token refusals onto the candidate surface's
// sentinels: a token that names nothing is unknown, and one that named
// something no longer live is not-live.
func screeningError(err error) error {
	switch {
	case errors.Is(err, recruiting.ErrInvitationUnknownToken):
		return api.ErrScreeningInvitationUnknown
	case errors.Is(err, recruiting.ErrInvitationNotLive):
		return api.ErrScreeningInvitationNotLive
	}
	return err
}
