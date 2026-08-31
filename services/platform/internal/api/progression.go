package api

import (
	"context"
	"time"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// Progression is what this package needs of a candidate's competency history.
//
// A consumer-defined port, per ADR-0005: internal/progression knows nothing
// about HTTP, and the adapter that joins them lives in cmd/api. It is
// deliberately two reads and no writes. Everything progression records is
// derived from evaluations rather than submitted, so there is nothing here for
// a request to change, and an interface that could would invite one.
type Progression interface {
	// Skills answers every competency the candidate has been observed on,
	// including the ones they have not, which is the distinction the screens
	// exist to preserve.
	Skills(ctx context.Context, userID string) (SkillHistory, error)
	// Readiness answers one reading per role. A list, because two roles are two
	// answers and averaging them is the thing PRG-02 forbids.
	Readiness(ctx context.Context, userID string) ([]RoleReadiness, error)
}

// SkillHistory is every competency and the evidence behind it.
type SkillHistory struct {
	Competencies []SkillStanding
}

// SkillStanding is one competency's current reading.
type SkillStanding struct {
	CompetencyID string
	Name         string
	Discipline   string
	Role         string
	// Standing is fresh, aging, stale or none. None means never observed and is
	// kept apart from stale for the reason that recurs through this product:
	// never measured and measured long ago are different facts.
	Standing string
	// Band is empty when Standing is none. An unobserved competency has no band
	// rather than a low one, and the response omits the field entirely so a
	// screen cannot render an unasked question as a poor answer.
	Band     string
	Evidence []SkillEvidence
}

// SkillEvidence is one reading, with what judged it and when.
type SkillEvidence struct {
	ObservedAt      time.Time
	AgeDays         int
	Standing        string
	Band            string
	RubricReference string
	RubricVersion   string
}

// RoleReadiness is one role's reading against the standard that produced it.
type RoleReadiness struct {
	Role       string
	Discipline string
	// The three fields that make a readiness auditable. A figure that cannot
	// say what judged it cannot be reproduced or argued with.
	StandardReference string
	StandardVersion   string
	StandardDigest    string
	ComputedAt        time.Time
	Assessed          []ReadinessCompetency
	Unassessed        []ReadinessCompetency
}

// ReadinessCompetency is one competency within a role's reading.
type ReadinessCompetency struct {
	CompetencyID string
	Name         string
	// Band and Standing are empty for an unassessed competency, which is why
	// the two lists are separate rather than one list with a flag.
	Band     string
	Standing string
}

// progression serves the candidate's own history.
type progression struct {
	authentication *authentication
	history        Progression
}

// GetMySkills answers every competency with the evidence behind it.
func (p *progression) GetMySkills(ctx context.Context, _ prepeetapi.GetMySkillsRequestObject) (prepeetapi.GetMySkillsResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return p.authentication.rejectedSession(ctx), nil
	}
	principal, err := p.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return p.authentication.failed(ctx, err), nil
	}

	history, err := p.history.Skills(ctx, principal.UserID)
	if err != nil {
		// Never an empty history. Telling a candidate they have practised
		// nothing because a query failed is worse than telling them it failed.
		return p.authentication.failed(ctx, err), nil
	}

	competencies := make([]prepeetapi.SkillStanding, 0, len(history.Competencies))
	for _, competency := range history.Competencies {
		entry := prepeetapi.SkillStanding{
			CompetencyID: competency.CompetencyID, Name: competency.Name,
			Discipline: competency.Discipline, Role: competency.Role,
			Standing: prepeetapi.EvidenceStanding(competency.Standing),
			Evidence: make([]prepeetapi.SkillEvidence, 0, len(competency.Evidence)),
		}
		// Omitted rather than empty when there is no observation, so the
		// absence survives serialization instead of arriving as "".
		if competency.Band != "" {
			band := competency.Band
			entry.Band = &band
		}
		for _, reading := range competency.Evidence {
			entry.Evidence = append(entry.Evidence, prepeetapi.SkillEvidence{
				AgeDays: reading.AgeDays, Band: reading.Band, ObservedAt: reading.ObservedAt,
				RubricReference: reading.RubricReference, RubricVersion: reading.RubricVersion,
				Standing: prepeetapi.EvidenceStanding(reading.Standing),
			})
		}
		competencies = append(competencies, entry)
	}

	return prepeetapi.GetMySkills200JSONResponse{
		Body:    prepeetapi.SkillHistory{Competencies: competencies},
		Headers: prepeetapi.GetMySkills200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// GetMyReadiness answers one reading per role.
func (p *progression) GetMyReadiness(ctx context.Context, _ prepeetapi.GetMyReadinessRequestObject) (prepeetapi.GetMyReadinessResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return p.authentication.rejectedSession(ctx), nil
	}
	principal, err := p.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return p.authentication.failed(ctx, err), nil
	}

	readings, err := p.history.Readiness(ctx, principal.UserID)
	if err != nil {
		return p.authentication.failed(ctx, err), nil
	}

	roles := make([]prepeetapi.RoleReadiness, 0, len(readings))
	for _, reading := range readings {
		role := prepeetapi.RoleReadiness{
			ComputedAt: reading.ComputedAt, Discipline: reading.Discipline, Role: reading.Role,
			StandardDigest: reading.StandardDigest, StandardReference: reading.StandardReference,
			StandardVersion: reading.StandardVersion,
			Assessed:        make([]prepeetapi.AssessedCompetency, 0, len(reading.Assessed)),
			Unassessed:      make([]prepeetapi.UnassessedCompetency, 0, len(reading.Unassessed)),
		}
		for _, competency := range reading.Assessed {
			role.Assessed = append(role.Assessed, prepeetapi.AssessedCompetency{
				Band: competency.Band, CompetencyID: competency.CompetencyID,
				Name: competency.Name, Standing: prepeetapi.EvidenceStanding(competency.Standing),
			})
		}
		// UnassessedCompetency has no band field at all, so an unasked question
		// cannot be reported as a scored answer even by a later mistake.
		for _, competency := range reading.Unassessed {
			role.Unassessed = append(role.Unassessed, prepeetapi.UnassessedCompetency{
				CompetencyID: competency.CompetencyID, Name: competency.Name,
			})
		}
		roles = append(roles, role)
	}

	return prepeetapi.GetMyReadiness200JSONResponse{
		Body:    prepeetapi.ReadinessByRole{Roles: roles},
		Headers: prepeetapi.GetMyReadiness200ResponseHeaders{CacheControl: NoStore},
	}, nil
}
