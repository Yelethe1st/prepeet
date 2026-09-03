package api_test

import (
	"net/http"
	"testing"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
)

const accommodationsPath = "/api/v1/screening/accommodations"

// A candidate requests a named adjustment; the request is forwarded and the
// created request returned.
func TestRequestAccommodationForwardsTheAdjustment(t *testing.T) {
	stub := defaultStubScreening()
	stub.accommodation = api.Accommodation{
		ID: "00000000-0000-7000-8000-00000000aa01", Adjustment: "captions",
		State: "requested", RequestedAt: time.Now(),
	}
	handler := serveScreeningAuthed(t, stub)

	body := `{"campaign_id":"00000000-0000-7000-8000-00000000c123","adjustment":"captions"}`
	status, decoded := jsonPost(t, handler, accommodationsPath, body)
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", status)
	}
	if decoded["adjustment"] != "captions" || decoded["state"] != "requested" {
		t.Fatalf("body = %v", decoded)
	}
	if stub.sawAdjustment != "captions" {
		t.Fatalf("adjustment not forwarded: %q", stub.sawAdjustment)
	}
}

// A request once the interview is underway is a 409 that routes to the incident
// path rather than recording a request that can no longer be met in preparation.
func TestRequestAccommodationTooLateIs409(t *testing.T) {
	stub := defaultStubScreening()
	stub.requestErr = api.ErrAccommodationTooLate
	handler := serveScreeningAuthed(t, stub)

	body := `{"campaign_id":"00000000-0000-7000-8000-00000000c123","adjustment":"extra_time"}`
	status, decoded := jsonPost(t, handler, accommodationsPath, body)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if code, _ := decoded["error"].(map[string]any)["code"].(string); code != "INTERVIEW_UNDERWAY" {
		t.Fatalf("code = %q", code)
	}
}

// A candidate who accepted no invitation to the campaign gets a 404, the same
// answer a campaign that does not exist gets.
func TestRequestAccommodationWithoutInvitationIs404(t *testing.T) {
	stub := defaultStubScreening()
	stub.requestErr = api.ErrSessionMissing
	handler := serveScreeningAuthed(t, stub)

	body := `{"campaign_id":"00000000-0000-7000-8000-00000000c123","adjustment":"captions"}`
	status, _ := jsonPost(t, handler, accommodationsPath, body)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// The recruiter decision names the caller as decider, never the body, and is
// scoped to a campaign the caller is on.
func TestDecideAccommodationNamesTheCallerAndScopesToTheCampaign(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	inv := defaultStubInvitations()
	handler := serveInvitations(t, rec, inv)

	path := "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/accommodations/00000000-0000-7000-8000-00000000aa01/decision"
	status, _ := campaignRequest(t, handler, http.MethodPost, path, `{"granted":true}`)
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", status)
	}
	if inv.sawDecideID != "00000000-0000-7000-8000-00000000aa01" {
		t.Fatalf("request id not forwarded: %q", inv.sawDecideID)
	}
}

// A recruiter not on the campaign cannot decide: the join refuses first.
func TestDecideAccommodationRequiresBeingOnTheCampaign(t *testing.T) {
	rec := &stubRecruiting{detailErr: api.ErrCampaignNoAccess}
	inv := defaultStubInvitations()
	handler := serveInvitations(t, rec, inv)

	path := "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/accommodations/00000000-0000-7000-8000-00000000aa01/decision"
	status, _ := campaignRequest(t, handler, http.MethodPost, path, `{"granted":true}`)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if inv.sawDecideID != "" {
		t.Fatal("a decision reached the port for a caller not on the campaign")
	}
}
