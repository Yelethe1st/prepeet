//go:build integration

package progression_test

import (
	"context"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// PRG-02 against real PostgreSQL. The three rules are guarded twice: once
// in the calculation and once in the schema, because a readiness row that
// cannot name its standard, or that folds the unmeasured into a pass, is
// unauditable however carefully the Go was written.

// candidateTwoID is a second practice candidate, used to prove one
// candidate's readiness is invisible to another.
const candidateTwoID = "00000000-0000-7000-8000-0000000000c3"

// screeningOwner is a tenant-scoped owner, so the cross-tenant attack can
// be aimed at a row that is known to exist under another tenant. An
// unscoped attack matches nothing and would prove nothing.
func screeningOwner(tenantID string) progression.Owner {
	return progression.Owner{
		Mode:        "screening",
		CandidateID: candidateID,
		TenantID:    tenantID,
	}
}

// savedReadiness computes and stores one readiness for the given owner.
func savedReadiness(t *testing.T, owner progression.Owner, standard progression.Standard,
	observations []progression.Observation) progression.Readiness {
	t.Helper()
	readiness, err := progression.Compute(standard, observations, time.Now().UTC())
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if err := progression.NewStore(pool).SaveReadiness(context.Background(), owner, readiness); err != nil {
		t.Fatalf("save: %v", err)
	}
	return readiness
}

// scopedTo runs a statement inside the row-level security context of one
// owner, and answers how many rows it touched.
func scopedTo(t *testing.T, owner progression.Owner, statement string, args ...any) (int64, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	user, tenant := owner.CandidateID, ""
	if owner.Mode != "practice" {
		user, tenant = "", owner.TenantID
	}
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.user_id', $1, true), set_config('app.tenant_id', $2, true)`,
		user, tenant); err != nil {
		t.Fatalf("scoping: %v", err)
	}
	tag, err := tx.Exec(ctx, statement, args...)
	return tag.RowsAffected(), err
}

func uniqueStandard(t *testing.T, suffix string) progression.Standard {
	t.Helper()
	standard := backendStandard(t)
	standard.Reference += "/" + suffix
	standard.Role += "-" + suffix
	return standard
}

func TestAStoredReadinessNamesTheStandardAndVersionItWasComputedAgainst(t *testing.T) {
	// Box 1 through the database: what comes back out still says exactly
	// what judged it.
	ctx := context.Background()
	standard := uniqueStandard(t, "named")
	owner := progression.Owner{Mode: "practice", CandidateID: candidateTwoID}
	savedReadiness(t, owner, standard, []progression.Observation{
		reading(id.New().String(), "systems-design", "solid", observedAt),
	})

	stored, err := progression.NewStore(pool).Readiness(ctx, owner)
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	var found *progression.Readiness
	for index := range stored {
		if stored[index].Standard.Reference == standard.Reference {
			found = &stored[index]
		}
	}
	if found == nil {
		t.Fatalf("the stored readiness did not come back: %+v", stored)
	}
	if found.Standard != standard.Pin {
		t.Fatalf("standard = %+v, want %+v", found.Standard, standard.Pin)
	}
	if found.Role != standard.Role || found.Discipline != standard.Discipline ||
		found.RubricReference != standard.RubricReference {
		t.Fatalf("readiness lost its grouping or comparability basis: %+v", found)
	}
}

func TestStoredReadinessKeepsAssessedAndUnassessedApart(t *testing.T) {
	// Box 3 through the database. One requirement met, one never
	// observed: the unassessed one comes back with no band, no resolving
	// observation and a reason.
	ctx := context.Background()
	standard := uniqueStandard(t, "apart")
	owner := progression.Owner{Mode: "practice", CandidateID: candidateTwoID}
	observation := reading(id.New().String(), "systems-design", "strong", observedAt)
	savedReadiness(t, owner, standard, []progression.Observation{observation})

	stored, err := progression.NewStore(pool).Readiness(ctx, owner)
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	for _, readiness := range stored {
		if readiness.Standard.Reference != standard.Reference {
			continue
		}
		if readiness.Met != 1 || readiness.Below != 0 || readiness.Unassessed != 1 {
			t.Fatalf("counts = %+v, want one met and one unassessed", readiness)
		}
		byID := outcomesOf(t, readiness)
		met := byID["systems-design"]
		if met.Outcome != progression.OutcomeMet || met.ObservedBand != "strong" ||
			met.ObservationID != observation.ID || met.Reason != "" ||
			!met.ObservedAt.Equal(observedAt) {
			t.Fatalf("met = %+v; the resolving reading did not survive storage", met)
		}
		unassessed := byID["debugging"]
		if unassessed.Outcome != progression.OutcomeUnassessed ||
			unassessed.ObservedBand != "" || unassessed.ObservationID != "" ||
			unassessed.Reason != progression.ReasonNeverObserved ||
			!unassessed.ObservedAt.IsZero() {
			t.Fatalf("unassessed = %+v; silence acquired a band or a date", unassessed)
		}
		return
	}
	t.Fatalf("the stored readiness did not come back: %+v", stored)
}

func TestTheSchemaRefusesASnapshotThatCannotNameItsStandard(t *testing.T) {
	// Box 1 as a guard that outlives this package's Go. A stored
	// readiness whose standard, version or digest is missing is an
	// unauditable number about a person, so the row is refused rather
	// than defaulted.
	owner := progression.Owner{Mode: "practice", CandidateID: candidateTwoID}
	for name, values := range map[string][3]string{
		"no reference": {"", "1.0.0", "sha256:x"},
		"no version":   {"role_standard/x", "", "sha256:x"},
		"no digest":    {"role_standard/x", "1.0.0", ""},
		"no role":      {"role_standard/x", "1.0.0", "sha256:x"},
	} {
		t.Run(name, func(t *testing.T) {
			role := "rl_x"
			if name == "no role" {
				role = ""
			}
			_, err := scopedTo(t, owner,
				`INSERT INTO progression.readiness_snapshots
                     (id, candidate_id, mode, tenant_id, standard_reference,
                      standard_version, standard_digest, role_id, discipline_id,
                      rubric_reference, answer_digest, computed_at)
                 VALUES ($1::uuid, $2::uuid, 'practice', NULL, $3, $4, $5, $6,
                         'd_x', 'rubric/x', 'sha256:a', now())`,
				id.New().String(), candidateTwoID,
				values[0], values[1], values[2], role)
			if err == nil {
				t.Fatalf("the schema stored a readiness with %s", name)
			}
		})
	}
}

func TestTheSchemaRefusesAMetRequirementWithNoReading(t *testing.T) {
	// Box 3 from the other side: assessed and unassessed are mirror
	// shapes, so a requirement cannot be called met while carrying the
	// emptiness of one nobody measured.
	owner := progression.Owner{Mode: "practice", CandidateID: candidateTwoID}
	standard := uniqueStandard(t, "hollow-met")
	savedReadiness(t, owner, standard, nil)

	_, err := scopedTo(t, owner,
		`INSERT INTO progression.readiness_competencies
             (snapshot_id, candidate_id, mode, tenant_id, competency_id, target_band,
              outcome, observed_band, reason, observation_id, observed_at)
         SELECT id, $1::uuid, 'practice', NULL, 'hollow', 'solid',
                'met', '', '', NULL, NULL
         FROM progression.readiness_snapshots WHERE standard_reference = $2`,
		candidateTwoID, standard.Reference)
	if err == nil {
		t.Fatal("the schema called a requirement met with no band, no reading and no date")
	}
}

func TestTheSchemaRefusesAnUnassessedRequirementWearingABand(t *testing.T) {
	ctx := context.Background()
	owner := progression.Owner{Mode: "practice", CandidateID: candidateTwoID}
	standard := uniqueStandard(t, "band")
	savedReadiness(t, owner, standard, nil)

	var snapshotID string
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.user_id', $1, true), set_config('app.tenant_id', '', true)`,
		candidateTwoID); err != nil {
		t.Fatalf("scoping: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT id::text FROM progression.readiness_snapshots WHERE standard_reference = $1`,
		standard.Reference).Scan(&snapshotID); err != nil {
		t.Fatalf("finding the snapshot: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO progression.readiness_competencies
             (snapshot_id, candidate_id, mode, tenant_id, competency_id, target_band,
              outcome, observed_band, reason, observation_id, observed_at)
         VALUES ($1::uuid, $2::uuid, 'practice', NULL, 'smuggled', 'solid',
                 'unassessed', 'strong', 'never_observed', NULL, NULL)`,
		snapshotID, candidateTwoID); err == nil {
		t.Fatal("the schema let an unassessed requirement carry a band")
	}
}

func TestRecomputingAnUnchangedAnswerDoesNotAppendHistory(t *testing.T) {
	// A readiness snapshot is a change in what is known, not a poll. Two
	// identical answers are one fact, and a new observation that changes
	// the answer appends rather than overwrites.
	owner := progression.Owner{Mode: "practice", CandidateID: candidateTwoID}
	standard := uniqueStandard(t, "history")
	first := []progression.Observation{
		reading(id.New().String(), "systems-design", "developing", observedAt),
	}
	savedReadiness(t, owner, standard, first)
	savedReadiness(t, owner, standard, first)

	if got := snapshotCount(t, owner, standard); got != 1 {
		t.Fatalf("%d snapshots after two identical computations, want 1", got)
	}

	improved := append(append([]progression.Observation{}, first...),
		reading(id.New().String(), "systems-design", "strong", observedAt.Add(time.Hour)))
	savedReadiness(t, owner, standard, improved)
	if got := snapshotCount(t, owner, standard); got != 2 {
		t.Fatalf("%d snapshots after the answer changed, want 2", got)
	}

	stored, err := progression.NewStore(pool).Readiness(context.Background(), owner)
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	for _, readiness := range stored {
		if readiness.Standard.Reference != standard.Reference {
			continue
		}
		if readiness.Met != 1 {
			t.Fatalf("the read did not return the newest answer: %+v", readiness)
		}
		return
	}
	t.Fatal("the standard vanished from the read")
}

// snapshotCount counts one standard's snapshots inside the owner's scope.
func snapshotCount(t *testing.T, owner progression.Owner, standard progression.Standard) int {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.user_id', $1, true), set_config('app.tenant_id', '', true)`,
		owner.CandidateID); err != nil {
		t.Fatalf("scoping: %v", err)
	}
	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*)::int FROM progression.readiness_snapshots WHERE standard_reference = $1`,
		standard.Reference).Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return count
}

func TestTwoRolesStayTwoStoredAnswers(t *testing.T) {
	// Box 2 through the database: two standards are two rows and two
	// answers, grouped by discipline and role, with nothing combining
	// them.
	owner := progression.Owner{Mode: "practice", CandidateID: candidateTwoID}
	backend := uniqueStandard(t, "grouped-backend")
	manager := managerStandard(t)
	manager.Reference += "/grouped-manager"
	observations := []progression.Observation{
		reading(id.New().String(), "systems-design", "strong", observedAt),
		reading(id.New().String(), "debugging", "strong", observedAt),
	}
	savedReadiness(t, owner, backend, observations)
	savedReadiness(t, owner, manager, observations)

	stored, err := progression.NewStore(pool).Readiness(context.Background(), owner)
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	byReference := map[string]progression.Readiness{}
	for _, readiness := range stored {
		byReference[readiness.Standard.Reference] = readiness
	}
	if got := byReference[backend.Reference]; got.Met != 2 || got.Unassessed != 0 {
		t.Fatalf("backend = %+v", got)
	}
	if got := byReference[manager.Reference]; got.Met != 0 || got.Unassessed != 1 {
		t.Fatalf("manager = %+v; a backend's evidence answered a manager's standard", got)
	}

	previous := ""
	for _, readiness := range stored {
		key := readiness.Discipline + "/" + readiness.Role
		if key < previous {
			t.Fatalf("readiness came back ungrouped: %q after %q", key, previous)
		}
		previous = key
	}
}

func TestReadinessSnapshotsOnlyEverGrow(t *testing.T) {
	// Attacked from inside the owner's scope, so the trigger is genuinely
	// exercised: an unscoped attack matches zero rows and proves nothing.
	owner := progression.Owner{Mode: "practice", CandidateID: candidateTwoID}
	standard := uniqueStandard(t, "append-only")
	savedReadiness(t, owner, standard, nil)

	for _, statement := range []string{
		`UPDATE progression.readiness_snapshots SET standard_version = '9.9.9' WHERE standard_reference = $1`,
		`DELETE FROM progression.readiness_snapshots WHERE standard_reference = $1`,
	} {
		rows, err := scopedTo(t, owner, statement, standard.Reference)
		if err == nil && rows == 0 {
			t.Fatalf("the attack matched zero rows, so the trigger was never exercised: %s", statement)
		}
		if err == nil {
			t.Fatalf("readiness history accepted %q", statement)
		}
	}
}

func TestAnotherTenantsReadinessIsInvisible(t *testing.T) {
	// Scoped at a row known to exist under the other tenant. An unscoped
	// attack would match nothing whether or not the policy worked.
	ctx := context.Background()
	const tenantA = "00000000-0000-7000-8000-0000000000a1"
	const tenantB = "00000000-0000-7000-8000-0000000000a2"

	standard := uniqueStandard(t, "tenant")
	savedReadiness(t, screeningOwner(tenantA), standard, []progression.Observation{
		reading(id.New().String(), "systems-design", "strong", observedAt),
	})

	var snapshotID string
	rows, err := scopedTo(t, screeningOwner(tenantA),
		`SELECT id FROM progression.readiness_snapshots WHERE standard_reference = $1`,
		standard.Reference)
	if err != nil || rows == 0 {
		t.Fatalf("tenant A cannot see its own row (%d rows, %v), so the attack would prove nothing", rows, err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.tenant_id', $1, true), set_config('app.user_id', '', true)`,
		tenantA); err != nil {
		t.Fatalf("scoping: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT id::text FROM progression.readiness_snapshots WHERE standard_reference = $1`,
		standard.Reference).Scan(&snapshotID); err != nil {
		t.Fatalf("finding tenant A's row: %v", err)
	}
	_ = tx.Rollback(ctx)

	for _, attack := range []struct {
		name      string
		statement string
	}{
		{"the snapshot", `SELECT id FROM progression.readiness_snapshots WHERE id = $1::uuid`},
		{"its competencies", `SELECT competency_id FROM progression.readiness_competencies WHERE snapshot_id = $1::uuid`},
	} {
		matched, err := scopedTo(t, screeningOwner(tenantB), attack.statement, snapshotID)
		if err != nil {
			t.Fatalf("%s: %v", attack.name, err)
		}
		if matched != 0 {
			t.Fatalf("tenant B read %d rows of tenant A's %s", matched, attack.name)
		}
	}

	// And the store, asked as the other tenant, answers nothing at all.
	stored, err := progression.NewStore(pool).Readiness(ctx, screeningOwner(tenantB))
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	for _, readiness := range stored {
		if readiness.Standard.Reference == standard.Reference {
			t.Fatalf("tenant B read tenant A's readiness: %+v", readiness)
		}
	}
}

func TestAnotherCandidatesReadinessIsInvisible(t *testing.T) {
	ctx := context.Background()
	owner := progression.Owner{Mode: "practice", CandidateID: candidateTwoID}
	standard := uniqueStandard(t, "private")
	savedReadiness(t, owner, standard, nil)

	stranger := progression.Owner{Mode: "practice", CandidateID: candidateID}
	stored, err := progression.NewStore(pool).Readiness(ctx, stranger)
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	for _, readiness := range stored {
		if readiness.Standard.Reference == standard.Reference {
			t.Fatalf("a stranger read another candidate's readiness: %+v", readiness)
		}
	}
}
