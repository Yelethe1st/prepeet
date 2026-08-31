package main

import (
	"context"
	"sort"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
)

// progressionAdapter presents the competency history as the port the API
// declared.
//
// The translation lives here rather than in either package because neither may
// see the other: progression knows nothing of HTTP, and internal/api knows
// nothing of the database. It is also where the owner is built, and the shape
// of that is the point. A candidate reading their own progression is always
// practice, so Mode is fixed and TenantID is deliberately left empty: the
// row-level security policy for practice data requires tenant context to be
// absent rather than merely different, so passing one through would not widen
// the read, it would return nothing at all.
type progressionAdapter struct {
	store *progression.Store
	// now is injectable so freshness can be tested against a fixed moment
	// rather than against whenever the suite happens to run.
	now func() time.Time
}

func newProgressionAdapter(store *progression.Store) progressionAdapter {
	return progressionAdapter{store: store, now: time.Now}
}

func (a progressionAdapter) owner(userID string) progression.Owner {
	return progression.Owner{Mode: "practice", CandidateID: userID}
}

// Skills folds the observation history into one standing per competency.
//
// History is append-only and holds every reading ever taken, so the newest per
// competency is the standing and the rest are the evidence behind it. Ordering
// is by discipline then role then competency, so a screen renders the same
// sequence twice running rather than whatever the database felt like.
func (a progressionAdapter) Skills(ctx context.Context, userID string) (api.SkillHistory, error) {
	observations, err := a.store.History(ctx, a.owner(userID))
	if err != nil {
		return api.SkillHistory{}, err
	}

	at := a.now()
	byCompetency := map[string][]progression.Observation{}
	for _, observation := range observations {
		byCompetency[observation.CompetencyID] = append(byCompetency[observation.CompetencyID], observation)
	}

	standings := make([]api.SkillStanding, 0, len(byCompetency))
	for competencyID, readings := range byCompetency {
		// Newest first: the standing is the most recent reading, and the
		// screen expands into the older ones beneath it.
		sort.Slice(readings, func(i, j int) bool {
			return readings[i].ObservedAt.After(readings[j].ObservedAt)
		})

		evidence := make([]api.SkillEvidence, 0, len(readings))
		for _, reading := range readings {
			freshness := progression.Freshness(reading.ObservedAt, at)
			evidence = append(evidence, api.SkillEvidence{
				ObservedAt: reading.ObservedAt, AgeDays: freshness.AgeDays,
				Standing: freshness.Standing, Band: reading.Band,
				RubricReference: reading.RubricReference, RubricVersion: reading.RubricVersion,
			})
		}

		standing := api.SkillStanding{
			CompetencyID: competencyID, Name: competencyID,
			Standing: progression.EvidenceNone,
		}
		if len(evidence) > 0 {
			standing.Standing = evidence[0].Standing
			standing.Band = evidence[0].Band
			standing.Evidence = evidence
		}
		standings = append(standings, standing)
	}

	sort.Slice(standings, func(i, j int) bool {
		if standings[i].Discipline != standings[j].Discipline {
			return standings[i].Discipline < standings[j].Discipline
		}
		if standings[i].Role != standings[j].Role {
			return standings[i].Role < standings[j].Role
		}
		return standings[i].CompetencyID < standings[j].CompetencyID
	})
	return api.SkillHistory{Competencies: standings}, nil
}

// Readiness maps each stored reading onto the port's shape.
//
// The assessed and unassessed lists are built from the outcome rather than from
// the presence of a band, because progression states the outcome explicitly and
// inferring it here would be a second place for the rule to live.
func (a progressionAdapter) Readiness(ctx context.Context, userID string) ([]api.RoleReadiness, error) {
	readings, err := a.store.Readiness(ctx, a.owner(userID))
	if err != nil {
		return nil, err
	}

	at := a.now()
	roles := make([]api.RoleReadiness, 0, len(readings))
	for _, reading := range readings {
		role := api.RoleReadiness{
			Role: reading.Role, Discipline: reading.Discipline,
			StandardReference: reading.Standard.Reference,
			StandardVersion:   reading.Standard.Version,
			StandardDigest:    reading.Standard.Digest,
			ComputedAt:        reading.ComputedAt,
		}
		for _, competency := range reading.Competencies {
			if competency.Outcome == "unassessed" {
				role.Unassessed = append(role.Unassessed, api.ReadinessCompetency{
					CompetencyID: competency.CompetencyID, Name: competency.CompetencyID,
				})
				continue
			}
			role.Assessed = append(role.Assessed, api.ReadinessCompetency{
				CompetencyID: competency.CompetencyID, Name: competency.CompetencyID,
				Band:     competency.ObservedBand,
				Standing: progression.Freshness(competency.ObservedAt, at).Standing,
			})
		}
		roles = append(roles, role)
	}

	sort.Slice(roles, func(i, j int) bool {
		if roles[i].Discipline != roles[j].Discipline {
			return roles[i].Discipline < roles[j].Discipline
		}
		return roles[i].Role < roles[j].Role
	})
	return roles, nil
}
