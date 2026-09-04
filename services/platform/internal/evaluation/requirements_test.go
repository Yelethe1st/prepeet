package evaluation_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
)

// EVL-06's rules, pinned: each job requirement is reported on its own with
// the evidence attached, the reasons evidence is absent stay distinguishable
// from one another, every gap carries a suggested human follow-up, and no
// number anywhere summarises the requirements into a compatibility score.

func mappingFixture() (
	[]evaluation.JobRequirement,
	[]evaluation.Competency,
	[]evaluation.CompetencyResult,
	[]evaluation.StoredSpan,
) {
	requirements := []evaluation.JobRequirement{
		{ID: "req-1", Text: "Five years of systems design at scale"},
		{ID: "req-2", Text: "Strong debugging under pressure"},
		{ID: "req-3", Text: "A current driving licence"},
		{ID: "req-4", Text: "Communication with stakeholders"},
	}
	competencies := []evaluation.Competency{
		{ID: "sd", Name: "Systems design"},
		{ID: "dbg", Name: "Debugging"},
		{ID: "comm", Name: "Communication"},
	}
	results := []evaluation.CompetencyResult{
		{
			CompetencyID: "sd", Status: "assessed", Band: "solid",
			EvidenceCount: 3, Supporting: 3,
			EvidenceIDs: []string{"sp-1", "sp-2", "sp-3"}, ReasonCodes: []string{},
		},
		{
			CompetencyID: "dbg", Status: "unassessed", Confidence: "not_assessable",
			EvidenceCount: 1, Supporting: 1,
			EvidenceIDs: []string{"sp-4"}, ReasonCodes: []string{"INSUFFICIENT_EVIDENCE"},
		},
		{
			CompetencyID: "comm", Status: "unassessed", Confidence: "not_assessable",
			EvidenceCount: 0, EvidenceIDs: []string{}, ReasonCodes: []string{"NOT_DISCUSSED"},
		},
	}
	spans := []evaluation.StoredSpan{
		{ID: "sp-1", Span: evaluation.Span{CompetencyID: "sd", Kind: "supporting", Quote: "sharded by clinic"}},
		{ID: "sp-2", Span: evaluation.Span{CompetencyID: "sd", Kind: "supporting", Quote: "optimistic version"}},
		{ID: "sp-3", Span: evaluation.Span{CompetencyID: "sd", Kind: "supporting", Quote: "failed closed"}},
		{ID: "sp-4", Span: evaluation.Span{CompetencyID: "dbg", Kind: "supporting", Quote: "bisected the release"}},
	}
	return requirements, competencies, results, spans
}

func TestEachRequirementIsReportedOnItsOwnWithItsEvidence(t *testing.T) {
	requirements, competencies, results, spans := mappingFixture()

	report := evaluation.MapRequirements(requirements, competencies, results, spans)

	if report.MapVersion != evaluation.RequirementMapVersion {
		t.Fatalf("map version = %q: a finding must be legible against the rules that made it", report.MapVersion)
	}
	if len(report.Requirements) != 4 {
		t.Fatalf("findings = %d, want one per requirement", len(report.Requirements))
	}
	byID := map[string]evaluation.RequirementFinding{}
	for _, finding := range report.Requirements {
		byID[finding.RequirementID] = finding
	}

	// A requirement naming a well-evidenced competency is evidenced, with
	// exactly that competency's spans attached: the evidence rides the
	// finding, never a summary of it.
	evidenced := byID["req-1"]
	if evidenced.Status != "evidenced" {
		t.Fatalf("req-1 = %+v", evidenced)
	}
	if len(evidenced.EvidenceIDs) != 3 || evidenced.EvidenceIDs[0] != "sp-1" {
		t.Fatalf("req-1 evidence = %v", evidenced.EvidenceIDs)
	}
	if len(evidenced.Competencies) != 1 || evidenced.Competencies[0] != "sd" {
		t.Fatalf("req-1 linked = %v: the link is inspectable, never implicit", evidenced.Competencies)
	}
}

func TestTheReasonsEvidenceIsAbsentStayDistinguishable(t *testing.T) {
	requirements, competencies, results, spans := mappingFixture()

	report := evaluation.MapRequirements(requirements, competencies, results, spans)
	byID := map[string]evaluation.RequirementFinding{}
	for _, finding := range report.Requirements {
		byID[finding.RequirementID] = finding
	}

	// Thin evidence on a reached competency is partial: something was
	// heard, not enough to call evidenced.
	if byID["req-2"].Status != "partial" {
		t.Fatalf("req-2 = %+v, want partial", byID["req-2"])
	}
	if len(byID["req-2"].EvidenceIDs) != 1 {
		t.Fatalf("req-2 evidence = %v: partial still attaches what exists", byID["req-2"].EvidenceIDs)
	}

	// Nothing the interview measured maps to a driving licence: not
	// assessable, which is about the interview, never about the candidate.
	if byID["req-3"].Status != "not_assessable" {
		t.Fatalf("req-3 = %+v, want not_assessable", byID["req-3"])
	}

	// Communication was measured and never came up: not discussed, which
	// is the plan's gap, not the candidate's.
	if byID["req-4"].Status != "not_discussed" {
		t.Fatalf("req-4 = %+v, want not_discussed", byID["req-4"])
	}
}

func TestMissingEvidenceSuggestsAHumanFollowUp(t *testing.T) {
	requirements, competencies, results, spans := mappingFixture()

	report := evaluation.MapRequirements(requirements, competencies, results, spans)
	for _, finding := range report.Requirements {
		if finding.Status == "evidenced" {
			if finding.FollowUp != "" {
				t.Fatalf("%s carries a follow-up despite being evidenced: %q", finding.RequirementID, finding.FollowUp)
			}
			continue
		}
		// Every gap hands the reviewer a question to ask, naming the
		// requirement in the reviewer's own next conversation.
		if finding.FollowUp == "" {
			t.Fatalf("%s (%s) has no suggested follow-up", finding.RequirementID, finding.Status)
		}
		if !strings.Contains(finding.FollowUp, requirements[indexOf(requirements, finding.RequirementID)].Text) {
			t.Fatalf("%s follow-up does not name the requirement: %q", finding.RequirementID, finding.FollowUp)
		}
	}
}

func TestContradictedEvidenceIsPartialNeverEvidenced(t *testing.T) {
	requirements := []evaluation.JobRequirement{
		{ID: "req-1", Text: "Systems design experience"},
	}
	competencies := []evaluation.Competency{{ID: "sd", Name: "Systems design"}}
	results := []evaluation.CompetencyResult{{
		CompetencyID: "sd", Status: "assessed", Band: "solid",
		EvidenceCount: 4, Supporting: 3, Contradictory: 1,
		EvidenceIDs: []string{"sp-1"}, ReasonCodes: []string{"CONTRADICTIONS_PRESENT"},
	}}

	report := evaluation.MapRequirements(requirements, competencies, results, nil)

	if report.Requirements[0].Status != "partial" {
		t.Fatalf("contradicted = %+v, want partial: a contradiction is not a settled account", report.Requirements[0])
	}
}

func TestTheReportCarriesNoCompatibilityNumberAnywhere(t *testing.T) {
	requirements, competencies, results, spans := mappingFixture()

	report := evaluation.MapRequirements(requirements, competencies, results, spans)
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	// The whole document, searched by name: no field summarises the
	// requirements into a match, a score or a percentage, because a
	// headline number is the decision the platform must not make.
	for _, forbidden := range []string{"percent", "match", "score", "ratio", "compatib"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("the report carries %q: %s", forbidden, encoded)
		}
	}
}

func TestLinkingIsAWholeNamePhraseNeverAFragment(t *testing.T) {
	// "design" alone inside "redesign the onboarding" must not link the
	// Systems design competency: a fragment match invents a connection
	// nobody stated, and an invented link becomes invented evidence.
	requirements := []evaluation.JobRequirement{
		{ID: "req-1", Text: "Redesign the onboarding flow"},
	}
	competencies := []evaluation.Competency{{ID: "sd", Name: "Systems design"}}
	results := []evaluation.CompetencyResult{{
		CompetencyID: "sd", Status: "assessed", Band: "strong",
		EvidenceCount: 3, Supporting: 3, EvidenceIDs: []string{"sp-1"}, ReasonCodes: []string{},
	}}

	report := evaluation.MapRequirements(requirements, competencies, results, nil)

	if report.Requirements[0].Status != "not_assessable" {
		t.Fatalf("fragment linked: %+v", report.Requirements[0])
	}
	if len(report.Requirements[0].EvidenceIDs) != 0 {
		t.Fatalf("evidence attached to an invented link: %v", report.Requirements[0].EvidenceIDs)
	}
}

func indexOf(requirements []evaluation.JobRequirement, id string) int {
	for i, requirement := range requirements {
		if requirement.ID == id {
			return i
		}
	}
	return -1
}
