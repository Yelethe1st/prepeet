package recruiting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
)

// The three things SCR-01 is for, and the fourth ADR-0020 adds.
//
// These run against fakes rather than Postgres because what is being tested is
// the opening decision, not the storage. The storage half of the same rules
// lives in the migration as CHECK constraints and triggers, and is exercised by
// campaign_integration_test.go: both halves exist deliberately, because a rule
// enforced only in Go is a rule a future query can walk around, and a rule
// enforced only in the database produces an error nobody can act on.

// stubArtifacts answers what recruiting is allowed to know about an artifact.
//
// recruiting owns the recruiting schema and nothing else, so it cannot read
// content.artifacts and must be told. That is ADR-0005's consumer-defined port,
// and this is the test's implementation of it.
type stubArtifacts struct {
	published map[string]recruiting.Artifact
	err       error
}

func (s stubArtifacts) PublishedArtifact(_ context.Context, tenantID, reference string) (recruiting.Artifact, error) {
	if s.err != nil {
		return recruiting.Artifact{}, s.err
	}
	artifact, found := s.published[reference]
	if !found {
		return recruiting.Artifact{}, recruiting.ErrNotPublished
	}
	return artifact, nil
}

// stubDeterminations answers which jurisdictions have a recorded determination.
type stubDeterminations struct {
	byJurisdiction map[string]recruiting.Determination
}

func (s stubDeterminations) LatestDetermination(_ context.Context, jurisdiction string) (recruiting.Determination, error) {
	determination, found := s.byJurisdiction[jurisdiction]
	if !found {
		return recruiting.Determination{}, recruiting.ErrNoDetermination
	}
	return determination, nil
}

func publishedSet() map[string]recruiting.Artifact {
	return map[string]recruiting.Artifact{
		"rubric/backend-v3":   {Reference: "rubric/backend-v3", Type: "rubric", Digest: digest('a'), Version: "3.0.0"},
		"calibration/backend": {Reference: "calibration/backend", Type: "calibration", Digest: digest('b'), Version: "1.2.0"},
		"persona/neutral":     {Reference: "persona/neutral", Type: "persona", Digest: digest('c'), Version: "1.0.0"},
		"plan/standard-45":    {Reference: "plan/standard-45", Type: "plan", Digest: digest('d'), Version: "2.0.0"},
	}
}

// digest builds a digest in the shape content.DigestOf produces, which is
// "sha256:" followed by 64 hex characters. The shape matters: the campaign_pin
// constraint refuses anything else, so a test using a shorter placeholder would
// pass here and fail against the database.
func digest(fill byte) string {
	out := make([]byte, 64)
	for i := range out {
		out[i] = fill
	}
	return "sha256:" + string(out)
}

func gbDetermination() recruiting.Determination {
	return recruiting.Determination{
		ID: "det-gb-1", Jurisdiction: "GB", Version: 1,
		ResultDisclosure: "evidence_without_band", AppealStatus: "right",
		Approver: "A. Counsel", ApprovedAt: time.Now(),
	}
}

func newService(artifacts recruiting.Artifacts, determinations recruiting.Determinations) *recruiting.Service {
	return recruiting.NewService(artifacts, determinations)
}

func draft() recruiting.Campaign {
	return recruiting.Campaign{
		ID: "campaign-1", TenantID: "tenant-1", Name: "Backend hire",
		RoleReference: "role/backend", Jurisdiction: "GB", Status: recruiting.StatusDraft,
	}
}

func fullPins() []recruiting.PinRequest {
	return []recruiting.PinRequest{
		{Type: "rubric", Reference: "rubric/backend-v3"},
		{Type: "calibration", Reference: "calibration/backend"},
		{Type: "persona", Reference: "persona/neutral"},
		{Type: "plan", Reference: "plan/standard-45"},
	}
}

// Criterion one: a campaign cannot be opened against a draft or unpublished
// configuration.
func TestOpeningRefusesAnUnpublishedArtifact(t *testing.T) {
	t.Parallel()

	// The rubric this campaign names is not in the published set, which is what
	// an artifact still in draft or in validating looks like from here.
	available := publishedSet()
	delete(available, "rubric/backend-v3")
	service := newService(
		stubArtifacts{published: available},
		stubDeterminations{byJurisdiction: map[string]recruiting.Determination{"GB": gbDetermination()}},
	)

	_, err := service.ResolveOpening(context.Background(), draft(), fullPins())

	if !errors.Is(err, recruiting.ErrNotPublished) {
		t.Fatalf("an unpublished rubric was accepted: %v", err)
	}
}

func TestOpeningRefusesAMissingArtifactKind(t *testing.T) {
	t.Parallel()

	service := newService(
		stubArtifacts{published: publishedSet()},
		stubDeterminations{byJurisdiction: map[string]recruiting.Determination{"GB": gbDetermination()}},
	)

	// No persona. A campaign with no persona has no interviewer, and finding
	// that out at the candidate's first invitation is too late.
	incomplete := []recruiting.PinRequest{
		{Type: "rubric", Reference: "rubric/backend-v3"},
		{Type: "calibration", Reference: "calibration/backend"},
		{Type: "plan", Reference: "plan/standard-45"},
	}

	_, err := service.ResolveOpening(context.Background(), draft(), incomplete)

	if !errors.Is(err, recruiting.ErrIncompleteConfiguration) {
		t.Fatalf("a campaign with no persona was opened: %v", err)
	}
}

// ADR-0020: no determination, no campaign.
func TestOpeningRefusesAJurisdictionWithNoDetermination(t *testing.T) {
	t.Parallel()

	service := newService(
		stubArtifacts{published: publishedSet()},
		// Determined for GB, and this campaign is not in GB.
		stubDeterminations{byJurisdiction: map[string]recruiting.Determination{"GB": gbDetermination()}},
	)
	elsewhere := draft()
	elsewhere.Jurisdiction = "DE"

	_, err := service.ResolveOpening(context.Background(), elsewhere, fullPins())

	if !errors.Is(err, recruiting.ErrNoDetermination) {
		t.Fatalf("a campaign opened where no legal determination exists: %v", err)
	}
}

func TestOpeningPinsTheDeterminationItWasCheckedAgainst(t *testing.T) {
	t.Parallel()

	service := newService(
		stubArtifacts{published: publishedSet()},
		stubDeterminations{byJurisdiction: map[string]recruiting.Determination{"GB": gbDetermination()}},
	)

	opening, err := service.ResolveOpening(context.Background(), draft(), fullPins())
	if err != nil {
		t.Fatalf("ResolveOpening: %v", err)
	}

	if opening.Determination.ID != "det-gb-1" {
		t.Fatalf("the campaign pinned %q, not the determination it was checked against",
			opening.Determination.ID)
	}
}

// Criterion two: publishing a new rubric version does not alter a running
// campaign. The structural half is that what gets pinned is the digest, because
// a reference resolves to whatever is current and a digest to what was chosen.
func TestOpeningPinsDigestsRatherThanReferences(t *testing.T) {
	t.Parallel()

	service := newService(
		stubArtifacts{published: publishedSet()},
		stubDeterminations{byJurisdiction: map[string]recruiting.Determination{"GB": gbDetermination()}},
	)

	opening, err := service.ResolveOpening(context.Background(), draft(), fullPins())
	if err != nil {
		t.Fatalf("ResolveOpening: %v", err)
	}

	if len(opening.Pins) != len(fullPins()) {
		t.Fatalf("pinned %d artifacts, want %d", len(opening.Pins), len(fullPins()))
	}
	for _, pin := range opening.Pins {
		if len(pin.Digest) != len("sha256:")+64 {
			t.Errorf("the %s pin carries no digest, so it names whatever is current: %+v",
				pin.Type, pin)
		}
	}
}

func TestARepublishedArtifactDoesNotMoveAnAlreadyOpenedCampaign(t *testing.T) {
	t.Parallel()

	available := publishedSet()
	determinations := stubDeterminations{byJurisdiction: map[string]recruiting.Determination{"GB": gbDetermination()}}
	artifacts := &mutableArtifacts{published: available}
	service := newService(artifacts, determinations)

	opened, err := service.ResolveOpening(context.Background(), draft(), fullPins())
	if err != nil {
		t.Fatalf("ResolveOpening: %v", err)
	}
	before := digestOf(t, opened.Pins, "rubric")

	// The rubric is republished at a new version, which is the ordinary thing
	// that happens to a rubric while a campaign is running.
	artifacts.published["rubric/backend-v3"] = recruiting.Artifact{
		Reference: "rubric/backend-v3", Type: "rubric", Digest: digest('z'), Version: "4.0.0",
	}

	after := digestOf(t, opened.Pins, "rubric")
	if before != after {
		t.Fatal("republishing a rubric changed what an already opened campaign points at")
	}
}

// mutableArtifacts is the published set as something a test can change
// underneath a campaign that has already opened.
type mutableArtifacts struct {
	published map[string]recruiting.Artifact
}

func (m *mutableArtifacts) PublishedArtifact(_ context.Context, tenantID, reference string) (recruiting.Artifact, error) {
	artifact, found := m.published[reference]
	if !found {
		return recruiting.Artifact{}, recruiting.ErrNotPublished
	}
	return artifact, nil
}

func digestOf(t *testing.T, pins []recruiting.Pin, kind string) string {
	t.Helper()
	for _, pin := range pins {
		if pin.Type == kind {
			return pin.Digest
		}
	}
	t.Fatalf("no %s pin", kind)
	return ""
}

func TestOpeningRefusesACampaignThatIsNotADraft(t *testing.T) {
	t.Parallel()

	service := newService(
		stubArtifacts{published: publishedSet()},
		stubDeterminations{byJurisdiction: map[string]recruiting.Determination{"GB": gbDetermination()}},
	)
	already := draft()
	already.Status = recruiting.StatusOpen

	// Reopening would re-resolve every pin against whatever is current now,
	// which is precisely the drift the digest pinning exists to prevent.
	if _, err := service.ResolveOpening(context.Background(), already, fullPins()); err == nil {
		t.Fatal("an already open campaign was opened again")
	}
}

func TestADuplicateArtifactKindIsRefused(t *testing.T) {
	t.Parallel()

	service := newService(
		stubArtifacts{published: publishedSet()},
		stubDeterminations{byJurisdiction: map[string]recruiting.Determination{"GB": gbDetermination()}},
	)
	twoRubrics := append(fullPins(), recruiting.PinRequest{
		Type: "rubric", Reference: "rubric/backend-v3",
	})

	// "One rubric" is the schema's primary key, and it is also a rule the
	// service should not need the database to discover for it.
	if _, err := service.ResolveOpening(context.Background(), draft(), twoRubrics); err == nil {
		t.Fatal("a campaign pinned two rubrics")
	}
}
