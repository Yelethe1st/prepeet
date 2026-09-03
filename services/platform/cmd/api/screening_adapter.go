package main

import (
	"context"
	"errors"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
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
	sessions  *interview.Store
	results   *evaluation.Store
}

func newScreeningAdapter(store *recruiting.Store, id *identity.Service,
	settings *tenantadmin.SettingsStore, catalogue *catalog.Service,
	sessions *interview.Store, results *evaluation.Store) screeningAdapter {
	return screeningAdapter{store: store, identity: id, settings: settings,
		catalogue: catalogue, sessions: sessions, results: results}
}

var _ api.ScreeningInvitations = screeningAdapter{}
var _ api.CandidateAccommodations = screeningAdapter{}

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
	invitation, err := a.store.ResolveInvitationByToken(ctx, hash)
	if err != nil {
		return api.Session{}, screeningError(err)
	}

	// Provision before the consume, so the accept can record the candidate it
	// resolved to in the same update that sets the outcome. Provisioning is
	// idempotent and keyed to the recipient, so doing it before a consume that
	// might lose the race strands nothing: the winning accept resolved the same
	// address to the same account.
	candidateID, session, err := a.identity.ProvisionCandidateSession(ctx, invitation.Recipient)
	if err != nil {
		return api.Session{}, err
	}
	if _, err := a.store.AcceptInvitationByToken(ctx, hash, candidateID); err != nil {
		return api.Session{}, screeningError(err)
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

// StartScreeningSession creates the candidate's screening session for a
// campaign they accepted an invitation to.
//
// The accepted invitation is the authority, read as the candidate themselves:
// a candidate who accepted nothing here finds no invitation and is answered
// exactly like one naming a campaign that does not exist, so a signed-in person
// cannot start a session against a campaign they were never invited to. The
// campaign is then read under its own tenant, which the invitation named, and
// must still be open. The disclosure the candidate agreed to is recorded, and
// the session is created and moved to composing in the same shape a practice
// session is, so the worker composes it against the campaign's pins.
func (a screeningAdapter) StartScreeningSession(ctx context.Context, candidateID string, input api.ScreeningStart) (api.StartedScreeningSession, error) {
	accepted, err := a.store.AcceptedInvitationForCandidate(ctx, input.CampaignID, candidateID)
	if errors.Is(err, recruiting.ErrInvitationNotFound) {
		return api.StartedScreeningSession{}, api.ErrSessionMissing
	}
	if err != nil {
		return api.StartedScreeningSession{}, err
	}

	campaign, err := a.store.CampaignByID(ctx, accepted.TenantID, accepted.CampaignID)
	if err != nil {
		return api.StartedScreeningSession{}, err
	}
	if campaign.Status != recruiting.StatusOpen {
		return api.StartedScreeningSession{}, api.ErrCampaignNotOpen
	}

	// One session per accepted invitation. A candidate who already has a
	// screening session for this campaign is starting again, which the platform
	// never lets them do on their own: a recruiter must have authorized a
	// re-invitation, and starting claims it, so one authorization admits exactly
	// one further attempt. Without one, the restart is refused.
	newSessionID := id.New().String()
	if _, hasSession, err := a.sessions.ScreeningPhaseForCandidate(ctx, accepted.CampaignID, candidateID); err != nil {
		return api.StartedScreeningSession{}, err
	} else if hasSession {
		if err := a.store.ClaimReInvitation(ctx, accepted.CampaignID, candidateID, newSessionID); err != nil {
			if errors.Is(err, recruiting.ErrNoReInvitation) {
				return api.StartedScreeningSession{}, api.ErrReInvitationRequired
			}
			return api.StartedScreeningSession{}, err
		}
	}

	// Record what the candidate agreed to before the session exists, so a
	// crash between the two leaves an acceptance without a session rather than
	// a session nobody consented to.
	acceptance, err := recruiting.NewAcceptance(recruiting.AcceptanceRequest{
		TenantID: accepted.TenantID, CampaignID: accepted.CampaignID, CandidateID: candidateID,
		DisclosureVersion: input.DisclosureVersion, DisclosureDigest: input.DisclosureDigest,
	})
	if err != nil {
		return api.StartedScreeningSession{}, api.Invalid("disclosure_digest", "DISCLOSURE_INVALID", err.Error())
	}
	decisions := make([]recruiting.ConsentDecision, 0, len(input.Consents))
	for _, consent := range input.Consents {
		decisions = append(decisions, recruiting.ConsentDecision{Purpose: consent.Purpose, Granted: consent.Granted})
	}
	if err := a.store.RecordAcceptance(ctx, acceptance, decisions); err != nil {
		return api.StartedScreeningSession{}, err
	}

	// The session is created under the campaign's tenant, the scope screening
	// sessions live in, and moved straight to composing: creation is the request
	// to compose, and the worker starts the composition from the created event.
	session := interview.Session{
		ID: newSessionID, Mode: "screening", CandidateID: candidateID,
		TenantID: accepted.TenantID, CampaignID: accepted.CampaignID,
		// The campaign's pinned plan is the plan the composer uses; the blueprint
		// records which campaign the session belongs to rather than a second
		// source of configuration.
		BlueprintID:    "campaign/" + accepted.CampaignID,
		ConsentVersion: input.DisclosureVersion,
	}
	actor := interview.Actor{ID: candidateID, Type: "user"}
	if err := a.sessions.Create(ctx, session, actor); err != nil {
		return api.StartedScreeningSession{}, err
	}
	created, err := a.sessions.Get(ctx, session.ID, "screening", candidateID, accepted.TenantID)
	if err != nil {
		return api.StartedScreeningSession{}, err
	}
	composing, err := a.sessions.Transition(ctx, created, interview.StateComposing, interview.Effects{}, actor)
	if err != nil {
		return api.StartedScreeningSession{}, err
	}

	// Apply the accommodations already granted to this candidate onto the
	// session, so a grant is exercised on the interview rather than left as a
	// promise on a form. Each fulfilment re-checks the standing decision in the
	// store, so one withdrawn between grant and start is not applied; a
	// fulfilment that cannot be recorded does not fail the start, because the
	// session exists and the accommodation can be re-applied, while a session
	// lost to a fulfilment error could not.
	a.fulfilGrantedAccommodations(ctx, accepted.TenantID, accepted.CampaignID, candidateID, composing.ID)

	return api.StartedScreeningSession{SessionID: composing.ID, State: string(composing.State)}, nil
}

// fulfilGrantedAccommodations records every granted adjustment as applied to the
// session. It is best-effort against the store's own guard: only granted
// requests fulfil, and a failure is left for a later application rather than
// undoing a created session.
func (a screeningAdapter) fulfilGrantedAccommodations(ctx context.Context, tenantID, campaignID, candidateID, sessionID string) {
	views, err := a.store.AccommodationsFor(ctx, tenantID, campaignID, candidateID)
	if err != nil {
		return
	}
	for _, view := range views {
		if view.State != recruiting.RequestStateGranted {
			continue
		}
		_, _ = a.store.FulfilAccommodation(ctx, tenantID, view.Request.ID, sessionID)
	}
}

// Result composes SCR-07's read: the candidate's own screening session, the
// disclosure level its campaign pinned, and only as much of the evaluation as
// that level can ever show.
//
// The session read is the authority: GetScreeningForCandidate admits only the
// caller's own screening session, so everything after it acts on a session the
// candidate was already proven to own, and the tenant scope the evaluation
// reads under is that session's own. The disclosure fails closed on data, not
// on infrastructure: a campaign with no pinned determination, or one whose
// determination row is gone, discloses submission_only, while a database that
// cannot be read at all is an error, because an outage dressed as a narrow
// policy would hide itself.
func (a screeningAdapter) Result(ctx context.Context, candidateID, sessionID string) (api.ScreeningOutcome, error) {
	session, err := a.sessions.GetScreeningForCandidate(ctx, sessionID, candidateID)
	if errors.Is(err, interview.ErrNotFound) {
		return api.ScreeningOutcome{}, api.ErrSessionMissing
	}
	if err != nil {
		return api.ScreeningOutcome{}, err
	}

	outcome := api.ScreeningOutcome{State: string(session.State)}

	campaign, err := a.store.CampaignByID(ctx, session.TenantID, session.CampaignID)
	if err != nil {
		return api.ScreeningOutcome{}, err
	}
	if campaign.DeterminationID != "" {
		determination, err := a.store.DeterminationByID(ctx, campaign.DeterminationID)
		switch {
		case errors.Is(err, recruiting.ErrNoDetermination):
			// The pinned determination is gone: a data fault that discloses
			// nothing rather than everything. The empty level below is served
			// as submission_only.
		case err != nil:
			return api.ScreeningOutcome{}, err
		default:
			outcome.Disclosure = determination.ResultDisclosure
		}
	}

	ref := evaluation.SessionRef{
		SessionID: sessionID, Mode: "screening",
		CandidateID: candidateID, TenantID: session.TenantID,
	}
	result, err := a.results.ResultOf(ctx, ref)
	if errors.Is(err, evaluation.ErrNoResult) {
		return outcome, nil
	}
	if err != nil {
		return api.ScreeningOutcome{}, err
	}
	outcome.Evaluated = true
	outcome.Covered = result.Aggregation.CoveredCompetencies
	outcome.Total = result.Aggregation.TotalCompetencies

	// The wider data is read only when a level that can show it is in force:
	// data minimisation as a read pattern, not only as a filter.
	if outcome.Disclosure != api.DisclosureEvidenceWithoutBand &&
		outcome.Disclosure != api.DisclosureFullEvaluation {
		return outcome, nil
	}
	for _, competency := range result.Aggregation.Competencies {
		outcome.Competencies = append(outcome.Competencies, api.ScreeningCompetency{
			CompetencyID:  competency.CompetencyID,
			Status:        competency.Status,
			Band:          competency.Band,
			EvidenceCount: competency.EvidenceCount,
		})
	}
	spans, err := a.results.List(ctx, ref)
	if err != nil {
		return api.ScreeningOutcome{}, err
	}
	for _, span := range spans {
		outcome.Evidence = append(outcome.Evidence, api.ScreeningEvidence{
			CompetencyID: span.CompetencyID, Quote: span.Quote, Disposition: span.Kind,
		})
	}
	return outcome, nil
}

// screeningPhase maps a session's lifecycle state to the accommodation phase.
// No session is the earliest phase; a session not yet started is preparation;
// once it is running the interview is underway and a need is an incident; a
// finished or abandoned session is complete.
func screeningPhase(state interview.State, exists bool) recruiting.SessionPhase {
	if !exists {
		return recruiting.PhaseNoSession
	}
	switch string(state) {
	case "draft", "composing", "ready", "connecting":
		return recruiting.PhasePreparation
	case "in_progress", "reconnecting":
		return recruiting.PhaseUnderway
	default:
		return recruiting.PhaseComplete
	}
}

// RequestAccommodation records a candidate's request, its phase derived from
// where their own screening session for the campaign is. The authority is the
// accepted invitation; the request is then written under the campaign's tenant,
// the scope these rows live in, keyed to the candidate.
func (a screeningAdapter) RequestAccommodation(ctx context.Context, candidateID, campaignID, adjustment string) (api.Accommodation, error) {
	accepted, err := a.store.AcceptedInvitationForCandidate(ctx, campaignID, candidateID)
	if errors.Is(err, recruiting.ErrInvitationNotFound) {
		return api.Accommodation{}, api.ErrSessionMissing
	}
	if err != nil {
		return api.Accommodation{}, err
	}

	state, exists, err := a.sessions.ScreeningPhaseForCandidate(ctx, campaignID, candidateID)
	if err != nil {
		return api.Accommodation{}, err
	}

	request, err := recruiting.NewAccommodationRequest(recruiting.AccommodationRequestInput{
		TenantID: accepted.TenantID, CampaignID: campaignID, CandidateID: candidateID,
		Adjustment: recruiting.Adjustment(adjustment), Phase: screeningPhase(state, exists),
	})
	if err != nil {
		return api.Accommodation{}, accommodationError(err)
	}
	stored, err := a.store.RequestAccommodation(ctx, request)
	if err != nil {
		return api.Accommodation{}, err
	}
	return api.Accommodation{
		ID: stored.ID, CampaignID: stored.CampaignID, Adjustment: string(stored.Adjustment),
		State: string(recruiting.RequestStateRequested), RequestedAt: stored.RequestedAt,
	}, nil
}

// ListAccommodations answers the candidate's own requests for a campaign.
func (a screeningAdapter) ListAccommodations(ctx context.Context, candidateID, campaignID string) ([]api.Accommodation, error) {
	accepted, err := a.store.AcceptedInvitationForCandidate(ctx, campaignID, candidateID)
	if errors.Is(err, recruiting.ErrInvitationNotFound) {
		return nil, api.ErrSessionMissing
	}
	if err != nil {
		return nil, err
	}
	views, err := a.store.AccommodationsFor(ctx, accepted.TenantID, campaignID, candidateID)
	if err != nil {
		return nil, err
	}
	out := make([]api.Accommodation, 0, len(views))
	for _, view := range views {
		accommodation := api.Accommodation{
			ID: view.Request.ID, CampaignID: view.Request.CampaignID,
			Adjustment: string(view.Request.Adjustment), State: string(view.State),
			RequestedAt: view.Request.RequestedAt,
		}
		if view.DecidedAt != nil {
			accommodation.DecidedAt = view.DecidedAt
		}
		out = append(out, accommodation)
	}
	return out, nil
}

// accommodationError maps the domain's request refusals onto the surface.
func accommodationError(err error) error {
	switch {
	case errors.Is(err, recruiting.ErrUnknownAdjustment):
		return api.ErrAccommodationUnknownAdjustment
	case errors.Is(err, recruiting.ErrRequestTooLate):
		return api.ErrAccommodationTooLate
	}
	return err
}
