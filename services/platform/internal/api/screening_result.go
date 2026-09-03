package api

import (
	"context"
	"net/http"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// SCR-07: what a screening candidate may see of their own result, enforced
// here rather than by a hidden link. The campaign's jurisdiction determination
// names a level, and discloseScreening builds the response from the level up,
// starting from the least it may say and adding only what the level grants.
// Coaching, recruiter notes, comparison and hiring decisions have no fields in
// any of these types, so no level and no bug in level selection can leak them:
// their absence is structural.

// The four levels, most to least disclosing, exactly as
// recruiting.jurisdiction_determination stores them.
const (
	DisclosureFullEvaluation      = "full_evaluation"
	DisclosureEvidenceWithoutBand = "evidence_without_band"
	DisclosureCompletionStatus    = "completion_status"
	DisclosureSubmissionOnly      = "submission_only"
)

// ScreeningOutcome is everything the port answers about one screening session's
// result, before filtering. The handler filters it; the raw form never reaches
// the wire.
type ScreeningOutcome struct {
	// Disclosure is the campaign determination's result_disclosure. An empty or
	// unrecognised value is served as submission_only: a disclosure decision
	// the server cannot read is one it must not guess upward.
	Disclosure string
	// State is the session's lifecycle state, from which the status the level
	// permits is derived.
	State string
	// Evaluated says a stored result exists. Distinct from State because a
	// result can land while the session is still moving through review states.
	Evaluated    bool
	Competencies []ScreeningCompetency
	Evidence     []ScreeningEvidence
	Covered      int
	Total        int
}

// ScreeningCompetency is one competency as the candidate may see it. Band is
// stripped by the filter below full_evaluation.
type ScreeningCompetency struct {
	CompetencyID  string
	Status        string
	Band          string
	EvidenceCount int
}

// ScreeningEvidence is one of the candidate's own quoted answers.
type ScreeningEvidence struct {
	CompetencyID string
	Quote        string
	Disposition  string
}

// discloseScreening builds the wire response from the level up.
//
// It is written additively on purpose: the empty response is what
// submission_only serves, and each wider level adds fields to it. A filter
// written subtractively, starting from everything and removing what the level
// forbids, fails open the day a new field is added and nobody remembers to
// subtract it; this shape fails closed the same day, because the new field
// simply never appears until a level explicitly grants it.
func discloseScreening(outcome ScreeningOutcome) prepeetapi.ScreeningResult {
	level := prepeetapi.ScreeningResultDisclosure(outcome.Disclosure)
	if !level.Valid() {
		// Unknown is the narrowest level, not an error: the candidate holding
		// a finished interview should not meet a 500 because a determination
		// row carries a vocabulary this build predates.
		level = prepeetapi.SubmissionOnly
	}

	response := prepeetapi.ScreeningResult{
		Disclosure: level,
		// submission_only's whole answer: something was submitted. Even
		// whether the interview completed is not disclosed at this level.
		Status: prepeetapi.Submitted,
	}
	if level == prepeetapi.SubmissionOnly {
		return response
	}

	// completion_status and up: how far the interview actually got.
	response.Status = screeningStatus(outcome)
	if level == prepeetapi.CompletionStatus {
		return response
	}

	// evidence_without_band and up: the assessment's shape and the candidate's
	// own words, bands included only at full_evaluation.
	withBand := level == prepeetapi.FullEvaluation
	competencies := make([]struct {
		Band          *string `json:"band,omitempty"`
		CompetencyID  string  `json:"competency_id"`
		EvidenceCount int     `json:"evidence_count"`
		Status        string  `json:"status"`
	}, 0, len(outcome.Competencies))
	for _, competency := range outcome.Competencies {
		entry := struct {
			Band          *string `json:"band,omitempty"`
			CompetencyID  string  `json:"competency_id"`
			EvidenceCount int     `json:"evidence_count"`
			Status        string  `json:"status"`
		}{
			CompetencyID:  competency.CompetencyID,
			Status:        competency.Status,
			EvidenceCount: competency.EvidenceCount,
		}
		if withBand && competency.Band != "" {
			band := competency.Band
			entry.Band = &band
		}
		competencies = append(competencies, entry)
	}
	response.Competencies = &competencies

	evidence := make([]struct {
		CompetencyID string `json:"competency_id"`
		Disposition  string `json:"disposition"`
		Quote        string `json:"quote"`
	}, 0, len(outcome.Evidence))
	for _, span := range outcome.Evidence {
		evidence = append(evidence, struct {
			CompetencyID string `json:"competency_id"`
			Disposition  string `json:"disposition"`
			Quote        string `json:"quote"`
		}{CompetencyID: span.CompetencyID, Disposition: span.Disposition, Quote: span.Quote})
	}
	response.Evidence = &evidence

	response.Coverage = &struct {
		Covered int `json:"covered"`
		Total   int `json:"total"`
	}{Covered: outcome.Covered, Total: outcome.Total}

	return response
}

// screeningStatus derives the honest status for the levels that disclose it.
func screeningStatus(outcome ScreeningOutcome) prepeetapi.ScreeningResultStatus {
	if outcome.Evaluated {
		return prepeetapi.Evaluated
	}
	switch outcome.State {
	case "finalizing", "evaluating", "review_ready", "archived", "evaluation_failed":
		return prepeetapi.Completed
	}
	return prepeetapi.InProgress
}

// GetScreeningResult serves the candidate their permitted view.
func (h *screeningHandlers) GetScreeningResult(ctx context.Context, request prepeetapi.GetScreeningResultRequestObject) (prepeetapi.GetScreeningResultResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		refusal := h.authentication.rejectedSession(ctx)
		return refusal, nil
	}
	principal, err := h.authentication.identity.Authorize(ctx, presented, requiredCapabilityFrom(ctx))
	if err != nil {
		return h.authentication.failed(ctx, err), nil
	}

	// ErrSessionMissing, for a session that is not the caller's own screening
	// session, maps to the same 404 in failed() that an absent one gets, so
	// existence is not answered across candidates.
	outcome, err := h.invitations.Result(ctx, principal.UserID, request.SessionID.String())
	if err != nil {
		return h.authentication.failed(ctx, err), nil
	}
	return prepeetapi.GetScreeningResult200JSONResponse{
		Body:    discloseScreening(outcome),
		Headers: prepeetapi.GetScreeningResult200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

func (f failure) VisitGetScreeningResultResponse(w http.ResponseWriter) error { return f.write(w) }
