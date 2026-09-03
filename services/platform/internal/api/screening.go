package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// ScreeningInvitationView is one invitation as the candidate holding its token
// sees it. It carries who invited them and whether the link still works, never
// the token and never anything about whether the address has an account.
type ScreeningInvitationView struct {
	Status     string
	Employer   string
	Role       string
	CampaignID string
	ExpiresAt  time.Time
}

// The refusals the candidate-facing surface translates. Each is its own
// sentinel because the candidate's situation differs: a token that names
// nothing is a dead end, while a link that lapsed or was withdrawn has an
// employer to go back to.
var (
	// ErrScreeningInvitationUnknown means the presented token resolves to no
	// invitation. Deliberately indistinguishable from a spent link, so a guess
	// learns nothing.
	ErrScreeningInvitationUnknown = errors.New("api: no invitation for this token")
	// ErrScreeningInvitationNotLive means the invitation cannot be acted on:
	// expired, revoked, or already answered.
	ErrScreeningInvitationNotLive = errors.New("api: the invitation is not live")
)

// ScreeningInvitations is what this package needs of the candidate side of
// recruiting. None of it is authenticated: the candidate arrives holding a
// token, which is the whole of their authority.
type ScreeningInvitations interface {
	// Resolve reads the invitation a token names, live or terminal, or
	// ErrScreeningInvitationUnknown for a token that names nothing.
	Resolve(ctx context.Context, token string) (ScreeningInvitationView, error)
	// Accept consumes the invitation and resolves the candidate to a session,
	// creating a passwordless account when the address has none. The session is
	// the same shape whether the account existed or not.
	Accept(ctx context.Context, token string) (Session, error)
	// Decline records the candidate's no and returns the invitation, declined.
	Decline(ctx context.Context, token string) (ScreeningInvitationView, error)
	// Result answers what one of the caller's own screening sessions may show
	// them, unfiltered; the handler applies the determination's disclosure
	// before the wire. A session that is not the caller's own screening
	// session is ErrSessionMissing, exactly like one that does not exist.
	Result(ctx context.Context, candidateID, sessionID string) (ScreeningOutcome, error)
	// StartScreeningSession creates the candidate's screening session for a
	// campaign they accepted an invitation to, records the disclosure they
	// agreed to, and returns the composing session. ErrSessionMissing when the
	// candidate holds no accepted invitation to the campaign, ErrCampaignNotOpen
	// when the campaign no longer admits sessions.
	StartScreeningSession(ctx context.Context, candidateID string, input ScreeningStart) (StartedScreeningSession, error)
}

// ScreeningStart is what a candidate supplies to begin their interview: which
// campaign, the disclosure version and digest they were shown, and their answer
// to each purpose it asked about. The candidate is the session, never the body.
type ScreeningStart struct {
	CampaignID        string
	DisclosureVersion string
	DisclosureDigest  string
	Consents          []ScreeningConsent
}

// ScreeningConsent is one candidate answer about one purpose.
type ScreeningConsent struct {
	Purpose string
	Granted bool
}

// StartedScreeningSession is the created session as the wire reports it.
type StartedScreeningSession struct {
	SessionID string
	State     string
}

// screeningHandlers serves SCR-05's candidate-facing surface.
type screeningHandlers struct {
	authentication *authentication
	invitations    ScreeningInvitations
}

// ResolveScreeningInvitation shows the candidate what their token points at.
func (h *screeningHandlers) ResolveScreeningInvitation(ctx context.Context, request prepeetapi.ResolveScreeningInvitationRequestObject) (prepeetapi.ResolveScreeningInvitationResponseObject, error) {
	// A token guess has no address to count against, so the network carries it,
	// the same guard the magic-link and OTP consumes use.
	if err := h.authentication.limits.check(ctx, "resolve-invitation", "", networkFromContext(ctx)); err != nil {
		return h.screeningFailure(ctx, err), nil
	}
	view, err := h.invitations.Resolve(ctx, request.Body.Token)
	if err != nil {
		return h.screeningFailure(ctx, err), nil
	}
	return prepeetapi.ResolveScreeningInvitation200JSONResponse{
		Body:    screeningInvitationBody(view),
		Headers: prepeetapi.ResolveScreeningInvitation200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// AcceptScreeningInvitation consumes the invitation and signs the candidate in.
func (h *screeningHandlers) AcceptScreeningInvitation(ctx context.Context, request prepeetapi.AcceptScreeningInvitationRequestObject) (prepeetapi.AcceptScreeningInvitationResponseObject, error) {
	if err := h.authentication.limits.check(ctx, "accept-invitation", "", networkFromContext(ctx)); err != nil {
		return h.screeningFailure(ctx, err), nil
	}
	session, err := h.invitations.Accept(ctx, request.Body.Token)
	if err != nil {
		return h.screeningFailure(ctx, err), nil
	}
	issued, err := h.authentication.issued(session)
	if err != nil {
		return h.screeningFailure(ctx, err), nil
	}
	return issued, nil
}

// DeclineScreeningInvitation records the candidate's no.
func (h *screeningHandlers) DeclineScreeningInvitation(ctx context.Context, request prepeetapi.DeclineScreeningInvitationRequestObject) (prepeetapi.DeclineScreeningInvitationResponseObject, error) {
	if err := h.authentication.limits.check(ctx, "decline-invitation", "", networkFromContext(ctx)); err != nil {
		return h.screeningFailure(ctx, err), nil
	}
	view, err := h.invitations.Decline(ctx, request.Body.Token)
	if err != nil {
		return h.screeningFailure(ctx, err), nil
	}
	return prepeetapi.DeclineScreeningInvitation200JSONResponse{
		Body:    screeningInvitationBody(view),
		Headers: prepeetapi.DeclineScreeningInvitation200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// screeningFailure translates the candidate surface's refusals. It defers to
// the shared failed() for rate-limit and unknown errors, then narrows the two
// it owns.
func (h *screeningHandlers) screeningFailure(ctx context.Context, err error) failure {
	base := h.authentication.failed(ctx, err)
	switch {
	case errors.Is(err, ErrScreeningInvitationUnknown):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "This invitation link is not valid."
	case errors.Is(err, ErrScreeningInvitationNotLive):
		base.status = http.StatusConflict
		base.code = "INVITATION_NOT_LIVE"
		base.message = "This invitation can no longer be used. Resolve it again to see who to contact."
	}
	return base
}

// screeningInvitationBody maps the candidate view onto the wire. campaign_id,
// role and expires_at are present only when they mean something: a terminal
// invitation names its employer and status and leaves the rest out rather than
// carrying a campaign a candidate can no longer enter.
func screeningInvitationBody(view ScreeningInvitationView) prepeetapi.ScreeningInvitationView {
	body := prepeetapi.ScreeningInvitationView{
		Status:   prepeetapi.ScreeningInvitationViewStatus(view.Status),
		Employer: view.Employer,
	}
	if view.Role != "" {
		role := view.Role
		body.Role = &role
	}
	if view.CampaignID != "" {
		body.CampaignID = ptrUUID(view.CampaignID)
	}
	if !view.ExpiresAt.IsZero() {
		expires := view.ExpiresAt
		body.ExpiresAt = &expires
	}
	return body
}

// ptrUUID parses a stored identifier to a pointer, degrading a bad value to nil
// rather than a zero UUID, so an unparseable id is absent rather than a
// misleading all-zeros one.
func ptrUUID(id string) *openapi_types.UUID {
	parsed := campaignUUID(id)
	return &parsed
}

// The candidate surface renders through the shared writers: sessionIssued for
// the accept that signs in, and failure for every refusal.
func (r sessionIssued) VisitAcceptScreeningInvitationResponse(w http.ResponseWriter) error {
	return r.write(w)
}

func (f failure) VisitResolveScreeningInvitationResponse(w http.ResponseWriter) error {
	return f.write(w)
}

func (f failure) VisitAcceptScreeningInvitationResponse(w http.ResponseWriter) error {
	return f.write(w)
}

func (f failure) VisitDeclineScreeningInvitationResponse(w http.ResponseWriter) error {
	return f.write(w)
}

// StartScreeningSession begins the interview an accepted invitation admits.
func (h *screeningHandlers) StartScreeningSession(ctx context.Context, request prepeetapi.StartScreeningSessionRequestObject) (prepeetapi.StartScreeningSessionResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		refusal := h.authentication.rejectedSession(ctx)
		return refusal, nil
	}
	principal, err := h.authentication.identity.Authorize(ctx, presented, requiredCapabilityFrom(ctx))
	if err != nil {
		return h.authentication.failed(ctx, err), nil
	}

	input := ScreeningStart{
		CampaignID:        request.Body.CampaignID.String(),
		DisclosureVersion: request.Body.DisclosureVersion,
		DisclosureDigest:  request.Body.DisclosureDigest,
	}
	if request.Body.Consents != nil {
		for _, consent := range *request.Body.Consents {
			input.Consents = append(input.Consents, ScreeningConsent{Purpose: consent.Purpose, Granted: consent.Granted})
		}
	}

	// The candidate is the session, never the body: a body-supplied candidate
	// would let a signed-in person start an interview in someone else's name.
	started, err := h.invitations.StartScreeningSession(ctx, principal.UserID, input)
	if err != nil {
		return h.startScreeningFailure(ctx, err), nil
	}
	return prepeetapi.StartScreeningSession201JSONResponse{
		Body:    prepeetapi.ScreeningSession{SessionID: campaignUUID(started.SessionID), State: started.State},
		Headers: prepeetapi.StartScreeningSession201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// startScreeningFailure narrows the two refusals this start owns beyond the
// shared mapping: no accepted invitation is a 404 that does not reveal the
// campaign, and a campaign no longer open is a 409.
func (h *screeningHandlers) startScreeningFailure(ctx context.Context, err error) failure {
	base := h.authentication.failed(ctx, err)
	switch {
	case errors.Is(err, ErrSessionMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no such campaign to start."
	case errors.Is(err, ErrCampaignNotOpen):
		base.status = http.StatusConflict
		base.code = "CAMPAIGN_NOT_OPEN"
		base.message = "This campaign is not open, so it admits no new sessions."
	}
	return base
}

func (f failure) VisitStartScreeningSessionResponse(w http.ResponseWriter) error { return f.write(w) }
