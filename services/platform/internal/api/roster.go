package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// The candidate roster: REV-01.
//
// Evidence first, decision second, and the decision belongs to a named
// person. The roster is where a reviewer sees who has been invited and
// where each candidate stands - never who is best. Access is the campaign
// join, exactly as it is for the campaign itself; the standing filter runs
// server-side, because a row hidden in a browser is a row that was still
// sent; and nothing here orders candidates by quality, because a roster
// that ranks is a decision the platform has quietly made.

// RosterEntry is one invited candidate as the roster serves them.
type RosterEntry struct {
	InvitationID string
	Recipient    string
	// Standing is the contract's closed vocabulary: invitation states
	// through interview phases to awaiting_review, with
	// insufficient_evidence as its own named standing rather than a low
	// score.
	Standing  string
	InvitedAt time.Time
	// SessionID is set once the candidate's screening session exists.
	SessionID string
	// SubmittedAt is set once the screening became reviewable.
	SubmittedAt *time.Time
}

// Roster is one campaign's candidates with the count a reviewer triages by.
type Roster struct {
	PendingReview int
	Candidates    []RosterEntry
}

// RosterReads is what this surface needs of the composition in cmd: the
// invitations, the interviews and the evaluation standing meet there,
// because only cmd may see all three contexts.
type RosterReads interface {
	CampaignRoster(ctx context.Context, tenantID, campaignID string) (Roster, error)
}

// rosterHandlers serves REV-01's surface.
type rosterHandlers struct {
	authentication *authentication
	campaigns      Recruiting
	roster         RosterReads
}

func (h *rosterHandlers) caller(ctx context.Context) (Principal, *failure) {
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

// ListCampaignCandidates answers the roster for a recruiter on the campaign.
func (h *rosterHandlers) ListCampaignCandidates(ctx context.Context, request prepeetapi.ListCampaignCandidatesRequestObject) (prepeetapi.ListCampaignCandidatesResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	// The campaign join is the per-campaign enforcement, exactly as it is
	// for reading the campaign itself.
	campaign, err := h.campaigns.CampaignForRecruiter(ctx, principal.ActiveTenantID, request.CampaignID.String(), principal.UserID)
	if err != nil {
		return h.rosterFailure(ctx, err), nil
	}

	roster, err := h.roster.CampaignRoster(ctx, principal.ActiveTenantID, campaign.ID)
	if err != nil {
		return h.rosterFailure(ctx, err), nil
	}

	// The standing filter is served, not suggested: a filtered roster
	// contains only the rows asked for, whoever renders it.
	entries := roster.Candidates
	if request.Params.Standing != nil {
		wanted := string(*request.Params.Standing)
		filtered := make([]RosterEntry, 0, len(entries))
		for _, entry := range entries {
			if entry.Standing == wanted {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}

	body := prepeetapi.CampaignRoster{
		PendingReview: roster.PendingReview,
		Candidates:    make([]prepeetapi.RosterEntry, 0, len(entries)),
	}
	for _, entry := range entries {
		row := prepeetapi.RosterEntry{
			InvitationID: campaignUUID(entry.InvitationID),
			Recipient:    openapi_types.Email(entry.Recipient),
			Standing:     prepeetapi.RosterStanding(entry.Standing),
			InvitedAt:    entry.InvitedAt,
		}
		if entry.SessionID != "" {
			id := campaignUUID(entry.SessionID)
			row.SessionID = &id
		}
		if entry.SubmittedAt != nil {
			submitted := *entry.SubmittedAt
			row.SubmittedAt = &submitted
		}
		body.Candidates = append(body.Candidates, row)
	}
	return prepeetapi.ListCampaignCandidates200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ListCampaignCandidates200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// rosterFailure translates the refusals the roster can meet: the campaign
// join's refusal is the one, and it stays indistinguishable from absence.
func (h *rosterHandlers) rosterFailure(ctx context.Context, err error) failure {
	base := h.authentication.failed(ctx, err)
	if errors.Is(err, ErrCampaignNoAccess) {
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no campaign at that identifier."
	}
	return base
}

func (f failure) VisitListCampaignCandidatesResponse(w http.ResponseWriter) error {
	return f.write(w)
}
