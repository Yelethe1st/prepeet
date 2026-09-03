//go:build integration

package recruiting_test

// SCR-08: re-invitation is a named human's decision with a recorded reason, and
// each one admits exactly one further attempt.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

func TestReInvitationRequiresAReasonAndADecider(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	campaign, _ := store.CreateDraft(ctx, draftFor(tenantA, "Reinvite "+id.New().String()))
	candidateID := seedCandidate(t)

	if _, err := store.AuthorizeReInvitation(ctx, tenantA, campaign.ID, candidateID, "", seedCandidate(t), ""); !errors.Is(err, recruiting.ErrReInvitationReasonRequired) {
		t.Fatalf("empty reason error = %v, want ErrReInvitationReasonRequired", err)
	}
	if _, err := store.AuthorizeReInvitation(ctx, tenantA, campaign.ID, candidateID, "device failed", "", ""); !errors.Is(err, recruiting.ErrReInvitationDeciderRequired) {
		t.Fatalf("empty decider error = %v, want ErrReInvitationDeciderRequired", err)
	}
}

// One authorization admits exactly one further attempt: the first claim
// succeeds, a second finds nothing to claim.
func TestReInvitationIsClaimedOnce(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	campaign, _ := store.CreateDraft(ctx, draftFor(tenantA, "Reinvite "+id.New().String()))
	candidateID := seedCandidate(t)
	recruiterID := seedCandidate(t)

	authorized, err := store.AuthorizeReInvitation(ctx, tenantA, campaign.ID, candidateID, "connection dropped on their side", recruiterID, "")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if authorized.Reason == "" || authorized.DecidedBy != recruiterID {
		t.Fatalf("authorized wrong: %+v", authorized)
	}

	// The candidate, as themselves, claims it for a new session.
	if err := store.ClaimReInvitation(ctx, campaign.ID, candidateID, id.New().String()); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// A second claim finds no unclaimed authorization.
	if err := store.ClaimReInvitation(ctx, campaign.ID, candidateID, id.New().String()); !errors.Is(err, recruiting.ErrNoReInvitation) {
		t.Fatalf("second claim error = %v, want ErrNoReInvitation", err)
	}

	// The recruiter reads the audit: the authorization, now consumed.
	list, err := store.ReInvitationsForCandidate(ctx, tenantA, campaign.ID, candidateID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ConsumedSession == "" || !strings.Contains(list[0].Reason, "connection") {
		t.Fatalf("audit wrong: %+v", list)
	}
}

// A candidate with no authorization cannot claim one.
func TestClaimWithoutAuthorizationIsRefused(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	campaign, _ := store.CreateDraft(ctx, draftFor(tenantA, "Reinvite "+id.New().String()))

	if err := store.ClaimReInvitation(ctx, campaign.ID, seedCandidate(t), id.New().String()); !errors.Is(err, recruiting.ErrNoReInvitation) {
		t.Fatalf("claim error = %v, want ErrNoReInvitation", err)
	}
}
