package evaluation

import (
	"sort"
	"strings"
	"unicode"
)

// Job requirements against the recorded evidence: EVL-06, to screen-mode.md.
//
// Each requirement is reported on its own - evidenced, partial, not
// discussed, or not assessable - with the evidence attached and no headline
// number anywhere, because a compatibility percentage is the decision the
// platform must not make. The reasons evidence is absent stay
// distinguishable: not discussed is the interview plan's gap, not
// assessable means nothing the interview measured maps to the requirement
// at all, and both are about the process, never about the candidate.
//
// The mapping is the deterministic floor, and deliberately conservative: a
// requirement links to a competency only when the competency's whole name
// appears in the requirement's text, because a fragment match invents a
// connection nobody stated, and an invented link becomes invented
// evidence. A requirement nothing links to is not assessable - the honest
// floor - and every gap hands the reviewer a suggested human follow-up,
// which is where a missing answer belongs: in the reviewer's own next
// conversation, not in a guess.

// RequirementMapVersion names these rules, so a finding is always legible
// against exactly the rules that made it and a smarter mapper supersedes
// this one by name rather than silently.
const RequirementMapVersion = "requirement-map-1"

// JobRequirement is one reviewed requirement as recruiting froze it,
// carried here as identifiers and text: this context never reads
// recruiting's tables, and cmd hands the two vocabularies to each other.
type JobRequirement struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// RequirementFinding is one requirement's report.
type RequirementFinding struct {
	RequirementID string `json:"requirement_id"`
	// Status is evidenced, partial, not_discussed or not_assessable:
	// screen-mode.md's closed vocabulary, verbatim.
	Status string `json:"status"`
	// Competencies names the links the mapping made, so a reviewer can see
	// why the finding says what it says rather than trusting it.
	Competencies []string `json:"competencies"`
	// EvidenceIDs are the stored spans behind the finding - attached, never
	// summarised. Empty exactly when no linked competency recorded any.
	EvidenceIDs []string `json:"evidence_ids"`
	// FollowUp is the suggested human question for every status short of
	// evidenced; empty exactly when the record already answers.
	FollowUp string `json:"follow_up,omitempty"`
}

// RequirementsReport is the whole answer. It deliberately has no aggregate
// field: no count of requirements met, no fraction, nothing a screen could
// render as a headline number.
type RequirementsReport struct {
	MapVersion   string               `json:"map_version"`
	Requirements []RequirementFinding `json:"requirements"`
}

// MapRequirements reports each requirement against the aggregation and the
// stored evidence, deterministically.
func MapRequirements(
	requirements []JobRequirement,
	competencies []Competency,
	results []CompetencyResult,
	spans []StoredSpan,
) RequirementsReport {
	resultsByID := make(map[string]CompetencyResult, len(results))
	for _, result := range results {
		resultsByID[result.CompetencyID] = result
	}
	spansByCompetency := make(map[string][]string, len(spans))
	for _, span := range spans {
		spansByCompetency[span.CompetencyID] = append(spansByCompetency[span.CompetencyID], span.ID)
	}

	report := RequirementsReport{
		MapVersion:   RequirementMapVersion,
		Requirements: make([]RequirementFinding, 0, len(requirements)),
	}
	for _, requirement := range requirements {
		finding := RequirementFinding{
			RequirementID: requirement.ID,
			Competencies:  []string{},
			EvidenceIDs:   []string{},
		}

		linked := make([]CompetencyResult, 0, 2)
		for _, competency := range competencies {
			if !namePhraseIn(competency.Name, requirement.Text) {
				continue
			}
			finding.Competencies = append(finding.Competencies, competency.ID)
			if result, measured := resultsByID[competency.ID]; measured {
				linked = append(linked, result)
			}
			finding.EvidenceIDs = append(finding.EvidenceIDs, spansByCompetency[competency.ID]...)
		}
		sort.Strings(finding.EvidenceIDs)

		finding.Status = requirementStatus(linked)
		if finding.Status != "evidenced" {
			finding.FollowUp = followUpFor(finding.Status, requirement.Text)
		}
		report.Requirements = append(report.Requirements, finding)
	}
	return report
}

// requirementStatus folds the linked competencies' aggregation into the
// closed vocabulary. The order of the rules is the argument: no link at all
// is a question about the interview's design, everything unreached is a
// question about the plan, and only a record that is assessed throughout
// with nothing contradicting it has actually answered the requirement.
func requirementStatus(linked []CompetencyResult) string {
	if len(linked) == 0 {
		return "not_assessable"
	}
	heard := false
	settled := true
	for _, result := range linked {
		if result.EvidenceCount > 0 {
			heard = true
		}
		if result.Status != "assessed" || hasReasonCode(result, "CONTRADICTIONS_PRESENT") {
			settled = false
		}
	}
	if !heard {
		return "not_discussed"
	}
	if settled {
		return "evidenced"
	}
	return "partial"
}

// followUpFor phrases the suggested human question. Each names the
// requirement itself, because the reviewer's follow-up conversation is
// where the missing answer belongs.
func followUpFor(status, requirement string) string {
	switch status {
	case "not_assessable":
		return "Nothing this interview measured maps to \"" + requirement +
			"\". Verify it directly, outside the interview record."
	case "not_discussed":
		return "The interview never reached \"" + requirement +
			"\". Ask about it in a follow-up conversation."
	default:
		return "The recorded evidence for \"" + requirement +
			"\" is thin or contradicted. Probe it further before deciding."
	}
}

// hasReasonCode reports whether the aggregation flagged the code.
func hasReasonCode(result CompetencyResult, code string) bool {
	for _, reason := range result.ReasonCodes {
		if reason == code {
			return true
		}
	}
	return false
}

// namePhraseIn reports whether the competency's whole name appears in the
// requirement text on word boundaries, case-insensitively. Whole-phrase on
// purpose: "design" inside "redesign" is a fragment, not a statement.
func namePhraseIn(name, text string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	text = strings.ToLower(text)
	if name == "" {
		return false
	}
	for start := 0; ; {
		found := strings.Index(text[start:], name)
		if found < 0 {
			return false
		}
		at := start + found
		before := at == 0 || !isWordRune(rune(text[at-1]))
		afterIndex := at + len(name)
		after := afterIndex >= len(text) || !isWordRune(rune(text[afterIndex]))
		if before && after {
			return true
		}
		start = at + 1
	}
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
