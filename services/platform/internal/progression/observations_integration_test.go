//go:build integration

package progression_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// PRG-01 against real PostgreSQL: history only ever grows, every row
// carries its provenance, and a re-evaluation under a new rubric adds its
// reading without touching the earlier view.

var pool *pgxpool.Pool

const candidateID = "00000000-0000-7000-8000-0000000000c1"

func TestMain(m *testing.M) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("prepeet"),
		tcpostgres.WithUsername("prepeet_migrator"),
		tcpostgres.WithPassword("migrator-password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(2*time.Minute)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres: %v\n", err)
		os.Exit(1)
	}
	adminURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "url: %v\n", err)
		os.Exit(1)
	}
	if err := database.Migrate(ctx, adminURL, database.MigrateOptions{AppPassword: "app-password"}); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	cfg, _ := pgx.ParseConfig(adminURL)
	pool, err = pgxpool.New(ctx, fmt.Sprintf(
		"postgres://prepeet_app:app-password@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "pool: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

func owner() progression.Owner {
	return progression.Owner{Mode: "practice", CandidateID: candidateID}
}

func observation(sessionID, evaluationID, competency, rubricVersion string, observedAt time.Time) progression.Observation {
	return progression.Observation{
		SessionID: sessionID, EvaluationID: evaluationID, CompetencyID: competency,
		Status: "assessed", Band: "solid", Confidence: "medium",
		EvidenceCount: 3, Supporting: 2, Contradictory: 0, Unverified: 1, Gaps: 0,
		RubricReference: "rubric/practice-default", RubricVersion: rubricVersion,
		RubricDigest:       "sha256:" + rubricVersion,
		AggregationVersion: "aggregate-1", ExtractionVersion: "evidence-1",
		ModelVersion: "none", PolicyVersion: "none",
		ObservedAt: observedAt,
	}
}

func TestARedeliveredEvaluationAppendsHistoryExactlyOnce(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	sessionID, evaluationID := id.New().String(), id.New().String()
	batch := []progression.Observation{
		observation(sessionID, evaluationID, "systems-design", "1.1.0", time.Now().UTC()),
		observation(sessionID, evaluationID, "debugging", "1.1.0", time.Now().UTC()),
	}

	for run := 0; run < 3; run++ {
		if err := store.Append(ctx, owner(), batch); err != nil {
			t.Fatalf("append %d: %v", run, err)
		}
	}

	history, err := store.History(ctx, owner())
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	count := 0
	for _, row := range history {
		if row.EvaluationID == evaluationID {
			count++
			if row.RubricVersion != "1.1.0" || row.RubricDigest == "" ||
				row.AggregationVersion != "aggregate-1" || row.ModelVersion != "none" {
				t.Fatalf("provenance did not survive: %+v", row)
			}
		}
	}
	if count != 2 {
		t.Fatalf("three appends left %d rows for the evaluation, want 2", count)
	}
}

func TestHistoryOnlyEverGrows(t *testing.T) {
	// Box 1, attacked from inside the owner's scope so the trigger is
	// genuinely exercised: an unscoped attack matches zero rows and
	// proves nothing.
	ctx := context.Background()
	store := progression.NewStore(pool)
	evaluationID := id.New().String()
	if err := store.Append(ctx, owner(), []progression.Observation{
		observation(id.New().String(), evaluationID, "systems-design", "1.1.0", time.Now().UTC()),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	attack := func(statement string) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx,
			`SELECT set_config('app.user_id', $1, true), set_config('app.tenant_id', '', true)`,
			candidateID); err != nil {
			t.Fatalf("scoping: %v", err)
		}
		tag, err := tx.Exec(ctx, statement, evaluationID)
		if err == nil && tag.RowsAffected() == 0 {
			t.Fatal("the attack matched zero rows; the trigger was never exercised")
		}
		if err == nil {
			t.Fatalf("history accepted %q", statement)
		}
	}
	attack(`UPDATE progression.observations SET band = 'strong' WHERE evaluation_id = $1`)
	attack(`DELETE FROM progression.observations WHERE evaluation_id = $1`)
}

func TestReEvaluationAddsAReadingWithoutDestroyingTheEarlierView(t *testing.T) {
	// Box 3: the same session and competency, judged again under a new
	// rubric version, is a second row. Both stand; the chart chooses.
	ctx := context.Background()
	store := progression.NewStore(pool)
	sessionID := id.New().String()
	first, second := id.New().String(), id.New().String()
	base := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)

	if err := store.Append(ctx, owner(), []progression.Observation{
		observation(sessionID, first, "systems-design", "1.1.0", base),
	}); err != nil {
		t.Fatalf("first reading: %v", err)
	}
	reread := observation(sessionID, second, "systems-design", "2.0.0", base.Add(time.Hour))
	reread.Band = "strong"
	if err := store.Append(ctx, owner(), []progression.Observation{reread}); err != nil {
		t.Fatalf("re-evaluation: %v", err)
	}

	history, err := store.History(ctx, owner())
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	var readings []progression.Observation
	for _, row := range history {
		if row.SessionID == sessionID {
			readings = append(readings, row)
		}
	}
	if len(readings) != 2 {
		t.Fatalf("%d readings, want both", len(readings))
	}
	if readings[0].RubricVersion != "1.1.0" || readings[0].Band != "solid" {
		t.Fatalf("the earlier view changed: %+v", readings[0])
	}
	if readings[1].RubricVersion != "2.0.0" || readings[1].Band != "strong" {
		t.Fatalf("the new reading is wrong: %+v", readings[1])
	}
}

func TestACorrectionNamesWhatItSupersedes(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	sessionID, first, second := id.New().String(), id.New().String(), id.New().String()
	base := time.Now().UTC()

	if err := store.Append(ctx, owner(), []progression.Observation{
		observation(sessionID, first, "debugging", "1.1.0", base),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	history, _ := store.History(ctx, owner())
	var original progression.Observation
	for _, row := range history {
		if row.EvaluationID == first {
			original = row
		}
	}

	correction := observation(sessionID, second, "debugging", "1.1.0", base.Add(time.Minute))
	correction.Supersedes = original.ID
	if err := store.Append(ctx, owner(), []progression.Observation{correction}); err != nil {
		t.Fatalf("correction: %v", err)
	}

	history, _ = store.History(ctx, owner())
	var linked bool
	for _, row := range history {
		if row.EvaluationID == second && row.Supersedes == original.ID {
			linked = true
		}
		if row.ID == original.ID && row.EvaluationID != first {
			t.Fatal("the original changed")
		}
	}
	if !linked {
		t.Fatal("the correction does not name its predecessor")
	}
}

func TestAnotherCandidatesHistoryIsInvisible(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	if err := store.Append(ctx, owner(), []progression.Observation{
		observation(id.New().String(), id.New().String(), "systems-design", "1.1.0", time.Now().UTC()),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	stranger := progression.Owner{Mode: "practice", CandidateID: "00000000-0000-7000-8000-0000000000c2"}
	history, err := store.History(ctx, stranger)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("a stranger read %d observations", len(history))
	}
}
