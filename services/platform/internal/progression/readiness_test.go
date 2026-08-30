package progression_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
)

// PRG-02's three rules, as properties of the calculation itself: a
// readiness that cannot name the standard and version it was computed
// against does not exist; two roles are two answers and never one
// average; and a competency nobody measured is its own outcome rather
// than a zero or a pass.

const (
	standardBody = `{
        "role": "rl_swe",
        "discipline": "software-engineering",
        "rubric_reference": "rubric/practice-default",
        "bands": ["developing", "solid", "strong"],
        "requirements": [
            {"competency_id": "systems-design", "target_band": "solid"},
            {"competency_id": "debugging", "target_band": "developing"}
        ]
    }`

	managerBody = `{
        "role": "rl_em",
        "discipline": "software-engineering",
        "rubric_reference": "rubric/practice-default",
        "bands": ["developing", "solid", "strong"],
        "requirements": [
            {"competency_id": "people-development", "target_band": "strong"}
        ]
    }`
)

func standardPin() progression.Pin {
	return progression.Pin{
		Reference: "role_standard/senior-backend",
		Version:   "2.1.0",
		Digest:    "sha256:9f1c",
	}
}

func backendStandard(t *testing.T) progression.Standard {
	t.Helper()
	standard, err := progression.ParseStandard(standardPin(), []byte(standardBody))
	if err != nil {
		t.Fatalf("parsing the standard: %v", err)
	}
	return standard
}

func managerStandard(t *testing.T) progression.Standard {
	t.Helper()
	pin := progression.Pin{
		Reference: "role_standard/engineering-manager",
		Version:   "1.0.0",
		Digest:    "sha256:2b7d",
	}
	standard, err := progression.ParseStandard(pin, []byte(managerBody))
	if err != nil {
		t.Fatalf("parsing the standard: %v", err)
	}
	return standard
}

// reading is one comparable observation of a competency.
func reading(id, competency, band string, at time.Time) progression.Observation {
	return progression.Observation{
		ID: id, CompetencyID: competency,
		Status: "assessed", Band: band, Confidence: "medium",
		RubricReference: "rubric/practice-default", RubricVersion: "1.1.0",
		ObservedAt: at,
	}
}

var observedAt = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

func outcomesOf(t *testing.T, readiness progression.Readiness) map[string]progression.CompetencyReadiness {
	t.Helper()
	byID := make(map[string]progression.CompetencyReadiness, len(readiness.Competencies))
	for _, competency := range readiness.Competencies {
		byID[competency.CompetencyID] = competency
	}
	return byID
}

func TestReadinessNamesTheStandardAndVersionItWasComputedAgainst(t *testing.T) {
	// Box 1. A readiness figure that cannot say what judged it is not
	// auditable, so the pin travels with the answer rather than beside it.
	computedAt := observedAt.Add(24 * time.Hour)
	readiness, err := progression.Compute(backendStandard(t), []progression.Observation{
		reading("o1", "systems-design", "solid", observedAt),
	}, computedAt)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	if readiness.Standard != standardPin() {
		t.Fatalf("standard = %+v, want the pin it was computed against", readiness.Standard)
	}
	if readiness.Role != "rl_swe" || readiness.Discipline != "software-engineering" {
		t.Fatalf("role/discipline = %q/%q", readiness.Role, readiness.Discipline)
	}
	if readiness.RubricReference != "rubric/practice-default" {
		t.Fatalf("rubric reference = %q; the comparability basis is part of the answer", readiness.RubricReference)
	}
	if !readiness.ComputedAt.Equal(computedAt) {
		t.Fatalf("computed at %v, want %v", readiness.ComputedAt, computedAt)
	}
}

func TestAStandardThatCannotNameItselfIsRefused(t *testing.T) {
	// The same box from the other side: there is no way to obtain a
	// readiness against an unnamed or unversioned standard, so an
	// unauditable number cannot be produced by mistake.
	pin := standardPin()
	for name, mutate := range map[string]func(*progression.Pin){
		"no reference": func(p *progression.Pin) { p.Reference = "" },
		"no version":   func(p *progression.Pin) { p.Version = "" },
		"no digest":    func(p *progression.Pin) { p.Digest = "" },
	} {
		t.Run(name, func(t *testing.T) {
			broken := pin
			mutate(&broken)
			if _, err := progression.ParseStandard(broken, []byte(standardBody)); !errors.Is(err, progression.ErrStandardIncoherent) {
				t.Fatalf("err = %v, want ErrStandardIncoherent", err)
			}
		})
	}
}

func TestIncoherentStandardsAreRefused(t *testing.T) {
	for name, body := range map[string]string{
		"no role":               `{"discipline":"d","rubric_reference":"r","bands":["a"],"requirements":[{"competency_id":"c","target_band":"a"}]}`,
		"no discipline":         `{"role":"r","rubric_reference":"r","bands":["a"],"requirements":[{"competency_id":"c","target_band":"a"}]}`,
		"no rubric reference":   `{"role":"r","discipline":"d","bands":["a"],"requirements":[{"competency_id":"c","target_band":"a"}]}`,
		"no bands":              `{"role":"r","discipline":"d","rubric_reference":"r","bands":[],"requirements":[{"competency_id":"c","target_band":"a"}]}`,
		"no requirements":       `{"role":"r","discipline":"d","rubric_reference":"r","bands":["a"],"requirements":[]}`,
		"a target off the band": `{"role":"r","discipline":"d","rubric_reference":"r","bands":["a"],"requirements":[{"competency_id":"c","target_band":"z"}]}`,
		"a repeated competency": `{"role":"r","discipline":"d","rubric_reference":"r","bands":["a"],"requirements":[{"competency_id":"c","target_band":"a"},{"competency_id":"c","target_band":"a"}]}`,
		"a repeated band":       `{"role":"r","discipline":"d","rubric_reference":"r","bands":["a","a"],"requirements":[{"competency_id":"c","target_band":"a"}]}`,
		"a nameless competency": `{"role":"r","discipline":"d","rubric_reference":"r","bands":["a"],"requirements":[{"competency_id":"","target_band":"a"}]}`,
		"not a document":        `{`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := progression.ParseStandard(standardPin(), []byte(body)); !errors.Is(err, progression.ErrStandardIncoherent) {
				t.Fatalf("err = %v, want ErrStandardIncoherent", err)
			}
		})
	}
}

func TestIncomparableRolesAreNeverAveraged(t *testing.T) {
	// Box 2. Two standards are two answers. There is no combined figure,
	// and one role's evidence never reaches the other's requirements even
	// when the candidate has plenty of it.
	observations := []progression.Observation{
		reading("o1", "systems-design", "strong", observedAt),
		reading("o2", "debugging", "strong", observedAt),
	}

	readinesses, err := progression.ComputeAll(
		[]progression.Standard{managerStandard(t), backendStandard(t)},
		observations, observedAt)
	if err != nil {
		t.Fatalf("compute all: %v", err)
	}
	if len(readinesses) != 2 {
		t.Fatalf("%d readinesses, want one per standard", len(readinesses))
	}

	byRole := map[string]progression.Readiness{}
	for _, readiness := range readinesses {
		byRole[readiness.Role] = readiness
	}
	backend, manager := byRole["rl_swe"], byRole["rl_em"]
	if backend.Met != 2 || backend.Below != 0 || backend.Unassessed != 0 {
		t.Fatalf("backend = %+v, want both requirements met", backend)
	}
	if manager.Met != 0 || manager.Unassessed != 1 {
		t.Fatalf("manager = %+v; a backend's evidence answered a manager's standard", manager)
	}
	if manager.Competencies[0].ObservationID != "" {
		t.Fatalf("a manager requirement resolved against %q", manager.Competencies[0].ObservationID)
	}
}

func TestProgressionIsGroupedByDisciplineAndRole(t *testing.T) {
	// The grouping is the answer's shape, not a caller's convention:
	// readiness comes back ordered by discipline then role, so a screen
	// cannot accidentally interleave two disciplines into one list.
	// Handed over in the wrong order deliberately: an assertion that
	// matches the order they went in would pass without any grouping.
	readinesses, err := progression.ComputeAll(
		[]progression.Standard{backendStandard(t), managerStandard(t)}, nil, observedAt)
	if err != nil {
		t.Fatalf("compute all: %v", err)
	}
	if readinesses[0].Role != "rl_em" || readinesses[1].Role != "rl_swe" {
		t.Fatalf("order = %q, %q; want discipline then role",
			readinesses[0].Role, readinesses[1].Role)
	}
}

func TestTwoStandardsForOneRoleAreRefused(t *testing.T) {
	// Two standards claiming the same role would give one role two
	// readiness answers with no way to choose, which is the ambiguity
	// that invites averaging them.
	standard := backendStandard(t)
	other := standard
	other.Reference = "role_standard/senior-backend-eu"
	if _, err := progression.ComputeAll([]progression.Standard{standard, other}, nil, observedAt); !errors.Is(err, progression.ErrIncomparable) {
		t.Fatalf("err = %v, want ErrIncomparable", err)
	}
}

func TestUnassessedIsNeitherMetNorBelow(t *testing.T) {
	// Box 3. A competency nobody measured carries no band, is counted in
	// its own total, and says why it is unassessed.
	unassessed := reading("o2", "debugging", "", observedAt)
	unassessed.Status, unassessed.Band = "unassessed", ""

	readiness, err := progression.Compute(backendStandard(t),
		[]progression.Observation{unassessed}, observedAt)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	byID := outcomesOf(t, readiness)
	if got := byID["systems-design"]; got.Outcome != progression.OutcomeUnassessed ||
		got.Reason != progression.ReasonNeverObserved || got.ObservedBand != "" {
		t.Fatalf("never observed = %+v, want unassessed with no band", got)
	}
	if got := byID["debugging"]; got.Outcome != progression.OutcomeUnassessed ||
		got.Reason != progression.ReasonNotAssessed || got.ObservedBand != "" {
		t.Fatalf("observed but unassessed = %+v", got)
	}
	if readiness.Met != 0 || readiness.Below != 0 || readiness.Unassessed != 2 {
		t.Fatalf("counts = %+v; silence was counted as a failure or a pass", readiness)
	}
}

func TestTheCountsPartitionTheRequirements(t *testing.T) {
	// The arithmetic that makes box 3 checkable downstream: met, below
	// and unassessed add up to the requirements and never overlap, so no
	// reader can quietly fold the unknown into either of the others.
	readiness, err := progression.Compute(backendStandard(t), []progression.Observation{
		reading("o1", "systems-design", "developing", observedAt),
	}, observedAt)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if readiness.Met+readiness.Below+readiness.Unassessed != len(readiness.Competencies) {
		t.Fatalf("%+v does not partition %d requirements", readiness, len(readiness.Competencies))
	}
	if readiness.Below != 1 || readiness.Unassessed != 1 {
		t.Fatalf("counts = %+v, want one below and one unassessed", readiness)
	}
}

func TestAReadingUnderAnotherRubricIsNotComparable(t *testing.T) {
	// The rubric is part of what makes two numbers comparable, so a
	// reading from a different rubric is named as incomparable rather
	// than being counted or silently dropped.
	foreign := reading("o1", "systems-design", "strong", observedAt)
	foreign.RubricReference = "rubric/screening-default"

	readiness, err := progression.Compute(backendStandard(t),
		[]progression.Observation{foreign}, observedAt)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	got := outcomesOf(t, readiness)["systems-design"]
	if got.Outcome != progression.OutcomeUnassessed || got.Reason != progression.ReasonIncomparableRubric {
		t.Fatalf("foreign rubric = %+v, want unassessed and named incomparable", got)
	}
}

func TestABandOutsideThePinnedScaleIsNotComparable(t *testing.T) {
	// A band the standard's scale does not contain cannot be placed
	// against a target. Guessing where it sits would invent a result.
	readiness, err := progression.Compute(backendStandard(t), []progression.Observation{
		reading("o1", "systems-design", "exceptional", observedAt),
	}, observedAt)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	got := outcomesOf(t, readiness)["systems-design"]
	if got.Outcome != progression.OutcomeUnassessed || got.Reason != progression.ReasonIncomparableBand {
		t.Fatalf("unknown band = %+v", got)
	}
}

func TestTheLatestComparableReadingResolvesARequirement(t *testing.T) {
	readiness, err := progression.Compute(backendStandard(t), []progression.Observation{
		reading("o1", "systems-design", "developing", observedAt),
		reading("o2", "systems-design", "strong", observedAt.Add(time.Hour)),
	}, observedAt)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	got := outcomesOf(t, readiness)["systems-design"]
	if got.Outcome != progression.OutcomeMet || got.ObservationID != "o2" ||
		got.ObservedBand != "strong" || !got.ObservedAt.Equal(observedAt.Add(time.Hour)) {
		t.Fatalf("resolved by %+v, want the later reading", got)
	}
}

func TestASupersededReadingDoesNotResolveARequirement(t *testing.T) {
	// A correction supersedes rather than replaces (PRG-01), so the
	// superseded row is still in history. Reading it as current would
	// make readiness disagree with the corrected record.
	correction := reading("o2", "systems-design", "developing", observedAt.Add(-time.Hour))
	correction.Supersedes = "o1"

	readiness, err := progression.Compute(backendStandard(t), []progression.Observation{
		reading("o1", "systems-design", "strong", observedAt),
		correction,
	}, observedAt)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	got := outcomesOf(t, readiness)["systems-design"]
	if got.ObservationID != "o2" || got.Outcome != progression.OutcomeBelow {
		t.Fatalf("resolved by %+v, want the correction rather than what it superseded", got)
	}
}

func TestObservationsOutsideTheStandardAreIgnored(t *testing.T) {
	readiness, err := progression.Compute(backendStandard(t), []progression.Observation{
		reading("o1", "people-development", "strong", observedAt),
	}, observedAt)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if len(readiness.Competencies) != 2 || readiness.Unassessed != 2 {
		t.Fatalf("%+v; a competency the standard does not require entered the answer", readiness)
	}
}

func TestComputeRefusesAStandardItCannotName(t *testing.T) {
	// Compute is reachable with a hand-built Standard, so it re-checks
	// rather than trusting that ParseStandard was used.
	standard := backendStandard(t)
	standard.Version = ""
	if _, err := progression.Compute(standard, nil, observedAt); !errors.Is(err, progression.ErrStandardIncoherent) {
		t.Fatalf("err = %v, want ErrStandardIncoherent", err)
	}
}
