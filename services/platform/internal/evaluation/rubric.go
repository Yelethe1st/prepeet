package evaluation

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// Rubric aggregation: EVL-02's pure half.
//
// aggregate-1 is a deterministic function from evidence spans and a pinned
// rubric to competency results, which is what makes "reconstruct an old
// session" a property instead of a project: the same spans and the same
// pinned rubric body produce the same result bytes, whatever the registry
// currently publishes.
//
// The rules are the spec's: eligible evidence only, coverage and evidence
// count per competency, a sufficiency threshold BEFORE any scoring, and
// unassessed kept strictly apart from poor - silence about a competency is
// a fact about coverage, never a low band.

// AggregationVersion names this calculation on every result.
const AggregationVersion = "aggregate-1"

// ErrRubricIncoherent refuses a rubric that cannot judge.
var ErrRubricIncoherent = errors.New("evaluation: the rubric document is incoherent")

// Rubric is one parsed, coherent rubric body.
type Rubric struct {
	Sufficiency struct {
		// MinSupporting is how much supporting evidence a competency needs
		// before any band may be assigned.
		MinSupporting int `json:"min_supporting"`
	} `json:"sufficiency"`
	// Bands in ascending order of the supporting ratio that earns them.
	Bands []Band `json:"bands"`
}

// Band is one qualitative level and the ratio floor that earns it.
type Band struct {
	ID       string  `json:"id"`
	MinRatio float64 `json:"min_ratio"`
}

// ParseRubric decodes and coheres a rubric document. The loader runs this
// as the artifact's validating step, so an incoherent rubric never
// publishes.
func ParseRubric(body []byte) (Rubric, error) {
	var rubric Rubric
	if err := json.Unmarshal(body, &rubric); err != nil {
		return Rubric{}, fmt.Errorf("%w: %v", ErrRubricIncoherent, err)
	}
	if rubric.Sufficiency.MinSupporting < 1 {
		return Rubric{}, fmt.Errorf("%w: a sufficiency threshold below one scores silence", ErrRubricIncoherent)
	}
	if len(rubric.Bands) == 0 {
		return Rubric{}, fmt.Errorf("%w: a rubric with no bands judges nothing", ErrRubricIncoherent)
	}
	previous := -1.0
	for _, band := range rubric.Bands {
		if band.ID == "" {
			return Rubric{}, fmt.Errorf("%w: a band without a name", ErrRubricIncoherent)
		}
		if band.MinRatio < 0 || band.MinRatio > 1 {
			return Rubric{}, fmt.Errorf("%w: band %q's ratio floor is outside [0,1]", ErrRubricIncoherent, band.ID)
		}
		if band.MinRatio <= previous {
			return Rubric{}, fmt.Errorf("%w: bands must ascend strictly; %q does not", ErrRubricIncoherent, band.ID)
		}
		previous = band.MinRatio
	}
	if rubric.Bands[0].MinRatio != 0 {
		return Rubric{}, fmt.Errorf("%w: the lowest band must start at zero or sufficient evidence could earn no band", ErrRubricIncoherent)
	}
	return rubric, nil
}

// CompetencyResult is one competency's aggregation.
type CompetencyResult struct {
	CompetencyID string `json:"competency_id"`
	// Status is assessed or unassessed; Band is set only when assessed.
	// The separation is the rule: unknown is never a low score.
	Status string `json:"status"`
	Band   string `json:"band,omitempty"`

	EvidenceCount int `json:"evidence_count"`
	Supporting    int `json:"supporting"`
	Contradictory int `json:"contradictory"`
	Unverified    int `json:"unverified"`
	Gaps          int `json:"gaps"`

	ReasonCodes []string `json:"reason_codes"`
}

// Aggregation is the whole result's content.
type Aggregation struct {
	Competencies []CompetencyResult `json:"competencies"`
	// Coverage is how many competencies had any evidence at all, over the
	// total asked about.
	CoveredCompetencies int `json:"covered_competencies"`
	TotalCompetencies   int `json:"total_competencies"`
}

// Aggregate runs aggregate-1: spans and a rubric in, competency results
// out, deterministically.
func Aggregate(rubric Rubric, competencies []Competency, spans []Span) Aggregation {
	byCompetency := map[string][]Span{}
	for _, span := range spans {
		byCompetency[span.CompetencyID] = append(byCompetency[span.CompetencyID], span)
	}

	results := make([]CompetencyResult, 0, len(competencies))
	covered := 0
	for _, competency := range competencies {
		evidence := byCompetency[competency.ID]
		result := CompetencyResult{
			CompetencyID:  competency.ID,
			EvidenceCount: len(evidence),
			ReasonCodes:   []string{},
		}
		for _, span := range evidence {
			switch span.Kind {
			case "supporting":
				result.Supporting++
			case "contradictory":
				result.Contradictory++
			case "claim_unverified":
				result.Unverified++
			case "gap":
				result.Gaps++
			}
		}
		if result.EvidenceCount > 0 {
			covered++
		}

		// Sufficiency before scoring: below the threshold there is no band,
		// and the reason says so by name.
		if result.Supporting < rubric.Sufficiency.MinSupporting {
			result.Status = "unassessed"
			result.ReasonCodes = append(result.ReasonCodes, "INSUFFICIENT_EVIDENCE")
			if result.Gaps > 0 {
				result.ReasonCodes = append(result.ReasonCodes, "GAPS_ACKNOWLEDGED")
			}
			results = append(results, result)
			continue
		}

		// The band is earned by the supporting share of eligible evidence.
		// Unverified claims count against the ratio without being treated
		// as contradiction: a claim nobody verified is weaker support, not
		// evidence of absence.
		eligible := result.Supporting + result.Contradictory + result.Unverified
		ratio := float64(result.Supporting) / float64(eligible)
		result.Status = "assessed"
		for _, band := range rubric.Bands {
			if ratio >= band.MinRatio {
				result.Band = band.ID
			}
		}
		if result.Contradictory > 0 {
			result.ReasonCodes = append(result.ReasonCodes, "CONTRADICTIONS_PRESENT")
		}
		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CompetencyID < results[j].CompetencyID
	})
	return Aggregation{
		Competencies:        results,
		CoveredCompetencies: covered,
		TotalCompetencies:   len(competencies),
	}
}
