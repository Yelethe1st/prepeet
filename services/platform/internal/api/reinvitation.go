package api

import (
	"context"
	"net/http"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// SCR-08: re-invitation is a named human's decision, with a reason, to let a
// candidate take the interview again. The platform never re-invites on its own.

// ReInvitations is what this package needs of the re-invitation half of
// recruiting.
type ReInvitations interface {
	// AuthorizeReInvitation records the decision. The decider is the caller,
	// never the body.
	AuthorizeReInvitation(ctx context.Context, tenantID, campaignID, candidateID, reason, decidedBy, interruptedSession string) error
}

// AuthorizeReInvitation records a recruiter's decision to let a candidate try
// again, on the invitation handlers because it shares their campaign join.
func (h *invitationHandlers) AuthorizeReInvitation(ctx context.Context, request prepeetapi.AuthorizeReInvitationRequestObject) (prepeetapi.AuthorizeReInvitationResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, denied := h.admit(ctx, principal, request.CampaignID.String())
	if denied != nil {
		return *denied, nil
	}

	interrupted := ""
	if request.Body.InterruptedSession != nil {
		interrupted = request.Body.InterruptedSession.String()
	}
	// The decider is the session, never the body: a re-invitation names the
	// human accountable for it, and that is who is asking.
	if err := h.reInvitations.AuthorizeReInvitation(ctx,
		principal.ActiveTenantID, campaign.ID, request.CandidateID.String(),
		request.Body.Reason, principal.UserID, interrupted); err != nil {
		return h.authentication.failed(ctx, err), nil
	}
	return prepeetapi.AuthorizeReInvitation204Response{
		Headers: prepeetapi.AuthorizeReInvitation204ResponseHeaders{CacheControl: NoStore},
	}, nil
}

func (f failure) VisitAuthorizeReInvitationResponse(w http.ResponseWriter) error { return f.write(w) }
