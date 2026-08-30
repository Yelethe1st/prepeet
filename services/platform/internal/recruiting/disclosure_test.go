package recruiting_test

import (
	"errors"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
)

// SCR-02's three criteria, and the one responsible-hiring.md adds on top of
// them: model improvement can never be a condition of taking an interview.

func requiredPurposes() []recruiting.Purpose {
	return []recruiting.Purpose{
		{Name: "interview_and_evaluation", Required: true},
		{Name: "recording", Required: true},
		{Name: "model_improvement", Required: false},
	}
}

const testDigest = "sha256:" +
	"1111111111111111111111111111111111111111111111111111111111111111"

func TestDecliningOptionalProcessingDoesNotBlockTheInterview(t *testing.T) {
	t.Parallel()

	// The criterion in one test. Every required purpose granted, the optional
	// one refused, and the candidate proceeds.
	decisions := []recruiting.ConsentDecision{
		{Purpose: "interview_and_evaluation", Required: true, Granted: true},
		{Purpose: "recording", Required: true, Granted: true},
		{Purpose: "model_improvement", Required: false, Granted: false},
	}

	if err := recruiting.MayProceed(requiredPurposes(), decisions); err != nil {
		t.Fatalf("declining optional processing blocked the interview: %v", err)
	}
}

func TestARefusedRequiredPurposeBlocksTheInterview(t *testing.T) {
	t.Parallel()

	decisions := []recruiting.ConsentDecision{
		{Purpose: "interview_and_evaluation", Required: true, Granted: true},
		{Purpose: "recording", Required: true, Granted: false},
		{Purpose: "model_improvement", Required: false, Granted: true},
	}

	err := recruiting.MayProceed(requiredPurposes(), decisions)

	if !errors.Is(err, recruiting.ErrConsentRefused) {
		t.Fatalf("a refused required consent was accepted: %v", err)
	}
}

func TestAnUnansweredRequiredPurposeIsNotTreatedAsAgreement(t *testing.T) {
	t.Parallel()

	// Silence is the failure mode worth guarding: an absent decision must never
	// read as a granted one, which is what a naive "no refusal found" check
	// would do.
	decisions := []recruiting.ConsentDecision{
		{Purpose: "interview_and_evaluation", Required: true, Granted: true},
		{Purpose: "model_improvement", Required: false, Granted: false},
	}

	err := recruiting.MayProceed(requiredPurposes(), decisions)

	if !errors.Is(err, recruiting.ErrConsentMissing) {
		t.Fatalf("an unanswered required consent passed as agreement: %v", err)
	}
}

func TestOptionalProcessingNeedNotBeAnsweredAtAll(t *testing.T) {
	t.Parallel()

	// Not answering an optional question is not a failure to consent; it is a
	// person who did not tick a box, and the interview proceeds.
	decisions := []recruiting.ConsentDecision{
		{Purpose: "interview_and_evaluation", Required: true, Granted: true},
		{Purpose: "recording", Required: true, Granted: true},
	}

	if err := recruiting.MayProceed(requiredPurposes(), decisions); err != nil {
		t.Fatalf("an unanswered optional purpose blocked the interview: %v", err)
	}
}

func TestModelImprovementCannotBeMadeRequired(t *testing.T) {
	t.Parallel()

	// responsible-hiring.md: consent is "not bundled with optional model
	// improvement". A campaign that marked it required would be bundling by
	// another name, so the refusal is here rather than in a review checklist.
	purposes := []recruiting.Purpose{
		{Name: "interview_and_evaluation", Required: true},
		{Name: "model_improvement", Required: true},
	}

	if err := recruiting.ValidatePurposes(purposes); !errors.Is(err, recruiting.ErrPurposeCannotBeRequired) {
		t.Fatalf("model improvement was allowed to be required: %v", err)
	}
}

func TestADisclosureMustCoverEveryRequiredArea(t *testing.T) {
	t.Parallel()

	// screen-mode.md and responsible-hiring.md both list what a candidate must
	// be told. A disclosure missing one of them is not a shorter disclosure, it
	// is an incomplete one, and it should fail before a candidate sees it.
	incomplete := map[string]string{}
	for _, area := range recruiting.RequiredDisclosureAreas() {
		incomplete[area] = "stated"
	}
	delete(incomplete, "retention")

	err := recruiting.ValidateDisclosure(incomplete)

	if !errors.Is(err, recruiting.ErrDisclosureIncomplete) {
		t.Fatalf("a disclosure with no retention section was accepted: %v", err)
	}
}

func TestACompleteDisclosureIsAccepted(t *testing.T) {
	t.Parallel()

	complete := map[string]string{}
	for _, area := range recruiting.RequiredDisclosureAreas() {
		complete[area] = "stated"
	}

	if err := recruiting.ValidateDisclosure(complete); err != nil {
		t.Fatalf("a complete disclosure was refused: %v", err)
	}
}

func TestAnEmptySectionIsNotACoveredArea(t *testing.T) {
	t.Parallel()

	// A key present with nothing behind it passes a "has every key" check and
	// tells the candidate nothing, which is the way this guard would most
	// plausibly be defeated by accident.
	blank := map[string]string{}
	for _, area := range recruiting.RequiredDisclosureAreas() {
		blank[area] = "stated"
	}
	blank["ai_involvement"] = "   "

	if err := recruiting.ValidateDisclosure(blank); !errors.Is(err, recruiting.ErrDisclosureIncomplete) {
		t.Fatalf("a blank section counted as a covered area: %v", err)
	}
}

func TestAcceptanceRecordsTheExactVersion(t *testing.T) {
	t.Parallel()

	acceptance, err := recruiting.NewAcceptance(recruiting.AcceptanceRequest{
		TenantID: "tenant-1", CampaignID: "campaign-1", CandidateID: "candidate-1",
		DisclosureDigest: testDigest, DisclosureVersion: "2.1.0",
	})
	if err != nil {
		t.Fatalf("NewAcceptance: %v", err)
	}

	if acceptance.DisclosureDigest != testDigest || acceptance.DisclosureVersion != "2.1.0" {
		t.Fatalf("the acceptance does not carry the version it was given: %+v", acceptance)
	}
}

func TestAnAcceptanceWithoutAVersionIsRefused(t *testing.T) {
	t.Parallel()

	// An acceptance that cannot say what was accepted is not evidence of
	// anything, and storing one would leave a record that looks like consent.
	_, err := recruiting.NewAcceptance(recruiting.AcceptanceRequest{
		TenantID: "tenant-1", CampaignID: "campaign-1", CandidateID: "candidate-1",
		DisclosureDigest: "", DisclosureVersion: "",
	})

	if err == nil {
		t.Fatal("an acceptance naming no disclosure was created")
	}
}

func TestAnAcceptanceRefusesADigestThatIsNotOne(t *testing.T) {
	t.Parallel()

	// A reference passed where a digest was meant would store something that
	// resolves to whatever is current, which is the whole failure this column's
	// format exists to prevent.
	_, err := recruiting.NewAcceptance(recruiting.AcceptanceRequest{
		TenantID: "tenant-1", CampaignID: "campaign-1", CandidateID: "candidate-1",
		DisclosureDigest: "disclosure/screening-v2", DisclosureVersion: "2.1.0",
	})

	if err == nil {
		t.Fatal("a reference was accepted where a digest was required")
	}
}
