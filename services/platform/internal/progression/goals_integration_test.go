//go:build integration

package progression_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// PRG-03 against real PostgreSQL. The two claims worth attacking are that
// a goal's subject cannot move under its milestones, and that a goal
// exists under no employer authority at all.

// goalCandidateID is this file's own candidate, so its rows cannot be
// confused with the observation and readiness suites' rows.
const goalCandidateID = "00000000-0000-7000-8000-0000000000d1"

// otherCandidateID is a second person, used to aim an attack at a goal
// that is known to exist rather than at nothing.
const otherCandidateID = "00000000-0000-7000-8000-0000000000d2"

func goalOwner(candidate string) progression.Owner {
	return progression.Owner{Mode: "practice", CandidateID: candidate}
}

// storedGoal writes one goal and returns it with its identifier.
func storedGoal(t *testing.T, candidate, competency, target string) progression.Goal {
	t.Helper()
	goal := progression.Goal{
		ID:              id.New().String(),
		Origin:          progression.GoalFromGap,
		OriginReference: "readiness/backend-engineer",
		CompetencyID:    competency,
		TargetBand:      target,
		RubricReference: "rubric/practice-default",
		Bands:           []string{"emerging", "developing", "solid", "strong"},
		Status:          progression.GoalActive,
	}
	if err := progression.NewStore(pool).CreateGoal(
		context.Background(), goalOwner(candidate), goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	return goal
}

// observedBand appends one assessed observation for a candidate, so a
// milestone has a real row to point at.
func observedBand(t *testing.T, candidate, competency, band string, at time.Time) progression.Observation {
	t.Helper()
	observation := progression.Observation{
		SessionID: id.New().String(), EvaluationID: id.New().String(),
		CompetencyID: competency, Status: "assessed", Band: band,
		Confidence: "medium", EvidenceCount: 3, Supporting: 2,
		RubricReference: "rubric/practice-default", RubricVersion: "1.0.0",
		RubricDigest: "sha256:1", AggregationVersion: "aggregate-1",
		ExtractionVersion: "evidence-1", ModelVersion: "none", PolicyVersion: "none",
		ObservedAt: at,
	}
	store := progression.NewStore(pool)
	owner := goalOwner(candidate)
	if err := store.Append(context.Background(), owner,
		[]progression.Observation{observation}); err != nil {
		t.Fatalf("append observation: %v", err)
	}
	history, err := store.History(context.Background(), owner)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	for _, stored := range history {
		if stored.EvaluationID == observation.EvaluationID {
			return stored
		}
	}
	t.Fatal("the observation just appended is not in the history")
	return progression.Observation{}
}

func TestAGoalIsReadBackWithTheScaleItWasPinnedTo(t *testing.T) {
	ctx := context.Background()
	goal := storedGoal(t, goalCandidateID, "systems-design-read", "strong")

	goals, err := progression.NewStore(pool).Goals(ctx, goalOwner(goalCandidateID))
	if err != nil {
		t.Fatalf("goals: %v", err)
	}
	for _, stored := range goals {
		if stored.ID != goal.ID {
			continue
		}
		if len(stored.Bands) != 4 || stored.Bands[3] != "strong" {
			t.Errorf("bands = %v, want the pinned scale", stored.Bands)
		}
		if stored.RubricReference != goal.RubricReference {
			t.Errorf("rubric reference = %q", stored.RubricReference)
		}
		if err := stored.Validate(); err != nil {
			t.Errorf("a goal read back from the database is not valid: %v", err)
		}
		return
	}
	t.Fatal("the goal just created is not in the list")
}

func TestAGoalCanBePausedAndRetiredButNotUnretired(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	owner := goalOwner(goalCandidateID)
	goal := storedGoal(t, goalCandidateID, "systems-design-lifecycle", "solid")

	for _, status := range []string{progression.GoalPaused, progression.GoalActive, progression.GoalRetired} {
		if err := store.SetGoalStatus(ctx, owner, goal.ID, status); err != nil {
			t.Fatalf("setting %q: %v", status, err)
		}
	}
	err := store.SetGoalStatus(ctx, owner, goal.ID, progression.GoalActive)
	if err == nil {
		t.Fatal("a retired goal came back, so retiring is not a decision")
	}
	if !strings.Contains(err.Error(), "retired") {
		t.Errorf("the refusal does not say why: %v", err)
	}
}

func TestAGoalsSubjectCannotMoveUnderItsMilestones(t *testing.T) {
	goal := storedGoal(t, goalCandidateID, "systems-design-subject", "solid")

	// The trigger, attacked directly rather than through the store, because
	// the store simply has no call that would try this.
	_, err := scopedTo(t, goalOwner(goalCandidateID),
		`UPDATE progression.goals SET competency_id = 'something-else' WHERE id = $1`, goal.ID)
	if err == nil {
		t.Fatal("a goal changed what it measures, so its milestones now mean nothing")
	}
	if !strings.Contains(err.Error(), "subject is fixed") {
		t.Errorf("the refusal does not name the rule: %v", err)
	}
}

func TestReachedMilestonesSurviveRecomputationAndAreNeverWrittenTwice(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	owner := goalOwner(goalCandidateID)
	goal := storedGoal(t, goalCandidateID, "systems-design-milestones", "strong")
	first := observedBand(t, goalCandidateID, "systems-design-milestones", "developing", day(2))
	observedBand(t, goalCandidateID, "systems-design-milestones", "solid", day(9))

	for attempt := 0; attempt < 3; attempt++ {
		if _, err := store.TrackGoals(ctx, owner, day(10)); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}

	progress, err := store.TrackGoals(ctx, owner, day(10))
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	for _, tracked := range progress {
		if tracked.Goal.ID != goal.ID {
			continue
		}
		if len(tracked.Milestones) != 2 {
			t.Fatalf("milestones = %d after four passes, want 2: %+v",
				len(tracked.Milestones), tracked.Milestones)
		}
		if tracked.Milestones[0].ObservationID != first.ID {
			t.Errorf("the first milestone does not cite the reading that earned it")
		}
		if len(tracked.Fresh) != 0 {
			t.Errorf("a repeated pass earned milestones again: %+v", tracked.Fresh)
		}
		return
	}
	t.Fatal("the goal is missing from its own progress")
}

func TestOneCandidatesGoalsAreInvisibleToAnother(t *testing.T) {
	ctx := context.Background()
	goal := storedGoal(t, goalCandidateID, "systems-design-private", "solid")

	// Aimed at a goal known to exist under the first candidate. An unscoped
	// attack matches nothing whether the policy works or not.
	rows, err := scopedTo(t, goalOwner(otherCandidateID),
		`UPDATE progression.goals SET status = 'retired' WHERE id = $1`, goal.ID)
	if err != nil {
		t.Fatalf("the attack failed for the wrong reason: %v", err)
	}
	if rows != 0 {
		t.Fatalf("another candidate retired this goal, touching %d rows", rows)
	}

	goals, err := progression.NewStore(pool).Goals(ctx, goalOwner(otherCandidateID))
	if err != nil {
		t.Fatalf("goals: %v", err)
	}
	for _, seen := range goals {
		if seen.ID == goal.ID {
			t.Fatal("another candidate can read this goal through the store")
		}
	}

	// And the same goal is still visible, and still active, to its owner:
	// a refusal that also refused the legitimate read would prove nothing.
	mine, err := progression.NewStore(pool).Goals(ctx, goalOwner(goalCandidateID))
	if err != nil {
		t.Fatalf("owner read: %v", err)
	}
	for _, seen := range mine {
		if seen.ID == goal.ID && seen.Status == progression.GoalActive {
			return
		}
	}
	t.Fatal("the owner cannot see their own active goal")
}

func TestNoTenantAuthorityReachesAGoal(t *testing.T) {
	// PRG-03's privacy rule, and the one the schema makes structural: there
	// is no tenant column, so there is no employer scope under which a goal
	// row exists.
	//
	// The attack sets a tenant AND the goal owner's own user id, which is
	// the only version of it that proves anything. Setting the tenant alone
	// is refused by the candidate clause and would leave the tenant clause
	// untested: that was the first version of this test, and removing the
	// tenant clause did not fail it.
	ctx := context.Background()
	goal := storedGoal(t, goalCandidateID, "systems-design-employer", "solid")
	const employerTenantID = "00000000-0000-7000-8000-0000000000a9"

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.user_id', $1, true), set_config('app.tenant_id', $2, true)`,
		goalCandidateID, employerTenantID); err != nil {
		t.Fatalf("scoping: %v", err)
	}

	var visible int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM progression.goals WHERE id = $1`, goal.ID).Scan(&visible); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if visible != 0 {
		t.Fatalf("a request carrying a tenant read %d goal rows", visible)
	}

	tag, err := tx.Exec(ctx,
		`UPDATE progression.goals SET status = 'retired' WHERE id = $1`, goal.ID)
	if err != nil {
		t.Fatalf("the attack failed for the wrong reason: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Fatalf("a request carrying a tenant retired %d goals", tag.RowsAffected())
	}

	// The same read with the tenant cleared finds the row, so the refusal
	// above was the tenant clause and not the goal being absent.
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.tenant_id', '', true)`); err != nil {
		t.Fatalf("clearing the tenant: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM progression.goals WHERE id = $1`, goal.ID).Scan(&visible); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if visible != 1 {
		t.Fatalf("the goal is not visible to its own owner either, so the attack proved nothing")
	}
}
