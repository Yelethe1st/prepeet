package api_test

import (
	"net/http"
	"testing"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
)

const reInvitePath = "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/candidates/00000000-0000-7000-8000-00000000dd01/re-invitation"

// Authorizing a re-invitation records the reason and names the caller as the
// decider, and needs the caller to be on the campaign.
func TestAuthorizeReInvitationRecordsReasonAndRequiresTheJoin(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	inv := defaultStubInvitations()
	handler := serveInvitations(t, rec, inv)

	status, _ := campaignRequest(t, handler, http.MethodPost, reInvitePath, `{"reason":"their connection dropped mid-interview"}`)
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", status)
	}
	if inv.sawReInviteReason != "their connection dropped mid-interview" {
		t.Fatalf("reason not forwarded: %q", inv.sawReInviteReason)
	}
}

func TestAuthorizeReInvitationRequiresBeingOnTheCampaign(t *testing.T) {
	rec := &stubRecruiting{detailErr: api.ErrCampaignNoAccess}
	inv := defaultStubInvitations()
	handler := serveInvitations(t, rec, inv)

	status, _ := campaignRequest(t, handler, http.MethodPost, reInvitePath, `{"reason":"x"}`)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if inv.sawReInviteReason != "" {
		t.Fatal("a re-invitation reached the port for a caller not on the campaign")
	}
}
