package recruiting_test

// SCR-06's decision rules, provable without a database.
//
// What lives here is the shape of the record and the two refusals that shape
// exists to make: a request that is anything other than a named adjustment,
// and a fulfilment that nobody granted. The append-only behaviour and the
// tenant boundary are the schema's and are proven in the integration file.

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
)

func validRequestInput() recruiting.AccommodationRequestInput {
	return recruiting.AccommodationRequestInput{
		TenantID:    "tenant",
		CampaignID:  "campaign",
		CandidateID: "candidate",
		Adjustment:  recruiting.AdjustmentCaptions,
		Phase:       recruiting.PhaseNoSession,
	}
}

// The design constraint stated structurally: a request is for a named
// adjustment, never a diagnosis. The struct is asserted field by field so
// that adding a place for a condition, a reason or a medical note is a change
// to this test, made on purpose, in a diff, with a reviewer, rather than a
// column that quietly starts collecting health data.
func TestARequestIsANamedAdjustmentNotADiagnosis(t *testing.T) {
	allowed := map[string]bool{
		"ID": true, "TenantID": true, "CampaignID": true, "CandidateID": true,
		"Adjustment": true, "RequestedAt": true,
	}
	kind := reflect.TypeOf(recruiting.AccommodationRequest{})
	for i := 0; i < kind.NumField(); i++ {
		name := kind.Field(i).Name
		if !allowed[name] {
			t.Errorf("AccommodationRequest carries %q, which nothing in SCR-06 asks for; "+
				"a free-text or condition field is where a diagnosis gets demanded", name)
		}
	}
	if kind.NumField() != len(allowed) {
		t.Errorf("AccommodationRequest has %d fields, want %d", kind.NumField(), len(allowed))
	}
}

func TestAnUnknownAdjustmentIsRefused(t *testing.T) {
	input := validRequestInput()
	input.Adjustment = "quiet_room_with_a_doctor_note"
	if _, err := recruiting.NewAccommodationRequest(input); !errors.Is(err, recruiting.ErrUnknownAdjustment) {
		t.Fatalf("want ErrUnknownAdjustment, got %v", err)
	}
}

func TestARequestBeforeOrDuringPreparationIsAccepted(t *testing.T) {
	for _, phase := range []recruiting.SessionPhase{
		recruiting.PhaseNoSession, recruiting.PhasePreparation,
	} {
		input := validRequestInput()
		input.Phase = phase
		request, err := recruiting.NewAccommodationRequest(input)
		if err != nil {
			t.Fatalf("phase %s: %v", phase, err)
		}
		if request.Adjustment != recruiting.AdjustmentCaptions {
			t.Fatalf("phase %s: adjustment is %q", phase, request.Adjustment)
		}
		if request.RequestedAt.IsZero() {
			t.Fatalf("phase %s: a request without a time is not a record", phase)
		}
	}
}

func TestARequestDuringTheInterviewIsRefused(t *testing.T) {
	for _, phase := range []recruiting.SessionPhase{
		recruiting.PhaseUnderway, recruiting.PhaseComplete,
	} {
		input := validRequestInput()
		input.Phase = phase
		if _, err := recruiting.NewAccommodationRequest(input); !errors.Is(err, recruiting.ErrRequestTooLate) {
			t.Fatalf("phase %s: want ErrRequestTooLate, got %v", phase, err)
		}
	}
}

// An unstated phase fails closed. Reading silence as "before the session"
// would let a caller that forgot to ask the session where it is admit a
// request mid-interview, which is exactly the window the phase rule closes.
func TestAnUnstatedPhaseIsRefused(t *testing.T) {
	input := validRequestInput()
	input.Phase = ""
	if _, err := recruiting.NewAccommodationRequest(input); !errors.Is(err, recruiting.ErrUnknownPhase) {
		t.Fatalf("want ErrUnknownPhase, got %v", err)
	}
}

func TestARequestNamesItsTenantCampaignAndCandidate(t *testing.T) {
	blank := func(mutate func(*recruiting.AccommodationRequestInput)) error {
		input := validRequestInput()
		mutate(&input)
		_, err := recruiting.NewAccommodationRequest(input)
		return err
	}
	if err := blank(func(i *recruiting.AccommodationRequestInput) { i.TenantID = " " }); err == nil {
		t.Fatal("a request without a tenant was accepted")
	}
	if err := blank(func(i *recruiting.AccommodationRequestInput) { i.CampaignID = "" }); err == nil {
		t.Fatal("a request without a campaign was accepted")
	}
	if err := blank(func(i *recruiting.AccommodationRequestInput) { i.CandidateID = "" }); err == nil {
		t.Fatal("a request without a candidate was accepted")
	}
}

// "By whom" is part of the record or there is no record. The same rule the
// jurisdiction determination applies to its approver.
func TestADecisionRequiresANamedHuman(t *testing.T) {
	if _, err := recruiting.NewAccommodationDecision("request", true, "  "); !errors.Is(err, recruiting.ErrNoDecider) {
		t.Fatalf("want ErrNoDecider, got %v", err)
	}
	decision, err := recruiting.NewAccommodationDecision("request", false, "reviewer")
	if err != nil {
		t.Fatalf("a named decline was refused: %v", err)
	}
	if decision.Granted {
		t.Fatal("a decline was recorded as a grant")
	}
	if decision.DecidedAt.IsZero() {
		t.Fatal("a decision without a time is not a record")
	}
}

func TestAFulfilmentRequiresAStandingGrant(t *testing.T) {
	declined := recruiting.AccommodationDecision{RequestID: "request", Granted: false, DecidedBy: "reviewer"}
	granted := recruiting.AccommodationDecision{RequestID: "request", Granted: true, DecidedBy: "reviewer"}

	if _, err := recruiting.NewFulfilment("request", "session", nil); !errors.Is(err, recruiting.ErrNotGranted) {
		t.Fatalf("no decision at all: want ErrNotGranted, got %v", err)
	}
	if _, err := recruiting.NewFulfilment("request", "session", &declined); !errors.Is(err, recruiting.ErrNotGranted) {
		t.Fatalf("standing decline: want ErrNotGranted, got %v", err)
	}
	fulfilment, err := recruiting.NewFulfilment("request", "session", &granted)
	if err != nil {
		t.Fatalf("a granted fulfilment was refused: %v", err)
	}
	if fulfilment.SessionID != "session" {
		t.Fatalf("the fulfilment lost its session: %q", fulfilment.SessionID)
	}
}

// A fulfilment that names no session is a promise, and SCR-06's third
// criterion is the difference between a promise and a record.
func TestAFulfilmentNamesItsSession(t *testing.T) {
	granted := recruiting.AccommodationDecision{RequestID: "request", Granted: true, DecidedBy: "reviewer"}
	if _, err := recruiting.NewFulfilment("request", "", &granted); err == nil {
		t.Fatal("a fulfilment without a session was accepted")
	}
}

// The state a candidate sees is derived from the append-only rows, never
// stored, so it cannot disagree with them.
func TestTheCandidateVisibleStateFollowsTheStandingDecision(t *testing.T) {
	granted := recruiting.AccommodationDecision{Granted: true}
	declined := recruiting.AccommodationDecision{Granted: false}

	if state := recruiting.StateOf(nil); state != recruiting.RequestStateRequested {
		t.Fatalf("no decision: want requested, got %q", state)
	}
	if state := recruiting.StateOf(&granted); state != recruiting.RequestStateGranted {
		t.Fatalf("grant: want granted, got %q", state)
	}
	if state := recruiting.StateOf(&declined); state != recruiting.RequestStateDeclined {
		t.Fatalf("decline: want declined, got %q", state)
	}
}

// The vocabulary is screen-mode.md's, verbatim. A fifth adjustment appearing
// here should be a change to the product document first.
func TestTheAdjustmentVocabularyIsScreenModes(t *testing.T) {
	want := []recruiting.Adjustment{
		recruiting.AdjustmentCaptions,
		recruiting.AdjustmentPushToTalk,
		recruiting.AdjustmentExtraTime,
		recruiting.AdjustmentAlternativePath,
	}
	got := recruiting.Adjustments()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("adjustments are %v, want %v", got, want)
	}
}
