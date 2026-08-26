//go:build integration

package evaluation_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// The store's half of EVL-01's third box: the stage retries by wholesale
// replacement and converges instead of duplicating.

var pool *pgxpool.Pool

const evidenceCandidate = "00000000-0000-7000-8000-0000000000f1"

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
	// Sessions are referenced by candidate id only here; the evidence table
	// has no session FK, deliberately, because evaluation must outlive
	// whatever happens to the session row's lifecycle.
	conn, err := pgx.Connect(ctx, adminURL)
	if err == nil {
		_, _ = conn.Exec(ctx, `INSERT INTO identity.users (id, email) VALUES ($1, 'evidence@example.com')`, evidenceCandidate)
		_ = conn.Close(ctx)
	}

	code := m.Run()
	pool.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

func span(sequence int, competency, quote string) evaluation.Span {
	return evaluation.Span{
		CompetencyID: competency, Kind: "supporting", SegmentSequence: sequence,
		Quote: quote, CharStart: 0, CharEnd: len(quote),
		StartMs: 1000, EndMs: 2000, ExtractionVersion: "evidence-1",
	}
}

func TestARetriedStageConvergesInsteadOfDuplicating(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	ref := evaluation.SessionRef{
		SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate,
	}
	spans := []evaluation.Span{
		span(3, "systems-design", "I sharded by region."),
		span(5, "debugging", "I traced it to a lock."),
	}

	if err := store.Replace(ctx, ref, "evidence-1", spans, nil); err != nil {
		t.Fatalf("first store: %v", err)
	}
	// The retry a worker death produces: identical input, identical rows.
	if err := store.Replace(ctx, ref, "evidence-1", spans, nil); err != nil {
		t.Fatalf("retried store: %v", err)
	}

	stored, err := store.List(ctx, ref)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("%d spans after a retry, want 2: the stage duplicated evidence", len(stored))
	}
	// Every box-one field survives storage.
	first := stored[0]
	if first.SegmentSequence != 3 || first.Quote != "I sharded by region." ||
		first.CharEnd != len(first.Quote) || first.StartMs != 1000 || first.EndMs != 2000 ||
		first.ExtractionVersion != "evidence-1" {
		t.Fatalf("provenance did not survive: %+v", first)
	}

	// A new extractor version replaces its own spans, not another's; the
	// same version replaces wholesale.
	if err := store.Replace(ctx, ref, "evidence-1", spans[:1], nil); err != nil {
		t.Fatalf("narrowed store: %v", err)
	}
	narrowed, _ := store.List(ctx, ref)
	if len(narrowed) != 1 {
		t.Fatalf("wholesale replacement left %d spans", len(narrowed))
	}
	if !reflect.DeepEqual(narrowed[0].Span, spans[0]) {
		t.Fatalf("the surviving span changed: %+v", narrowed[0].Span)
	}
}

func TestAnotherCandidatesEvidenceIsInvisible(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	owner := evaluation.SessionRef{
		SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate,
	}
	if err := store.Replace(ctx, owner, "evidence-1",
		[]evaluation.Span{span(3, "systems-design", "private evidence")}, nil); err != nil {
		t.Fatalf("store: %v", err)
	}

	other := owner
	other.CandidateID = "00000000-0000-7000-8000-0000000000f2"
	stored, err := store.List(ctx, other)
	if err != nil {
		t.Fatalf("list as other: %v", err)
	}
	if len(stored) != 0 {
		t.Fatal("another candidate read the evidence")
	}
}

func TestContradictionsReplaceWholesaleAndReadBackBothSides(t *testing.T) {
	// EVL-04 against real PostgreSQL: the pair survives storage with both
	// quotes and clocks intact, the retried stage converges, and the
	// no-edit trigger holds when attacked from inside the owner's scope.
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	ref := evaluation.SessionRef{
		SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate,
	}
	pair := evaluation.Contradiction{
		Topic: []string{"migration", "payments", "team"},
		SideA: evaluation.ContradictionSide{
			SegmentSequence: 3, Quote: "I led the payments migration team of 5 engineers.",
			CharStart: 0, CharEnd: 49, StartMs: 5000, EndMs: 9000,
		},
		SideB: evaluation.ContradictionSide{
			SegmentSequence: 5, Quote: "The payments migration team I led was 12 people.",
			CharStart: 0, CharEnd: 48, StartMs: 15000, EndMs: 19000,
		},
		ExtractionVersion: "evidence-1",
	}

	for run := 0; run < 2; run++ {
		if err := store.Replace(ctx, ref, "evidence-1", nil, []evaluation.Contradiction{pair}); err != nil {
			t.Fatalf("replace %d: %v", run, err)
		}
	}

	stored, err := store.Contradictions(ctx, ref)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("two replaces left %d pairs, want 1", len(stored))
	}
	if !reflect.DeepEqual(stored[0], pair) {
		t.Fatalf("the pair changed in storage:\nstored %+v\nsent   %+v", stored[0], pair)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.user_id', $1, true), set_config('app.tenant_id', '', true)`,
		evidenceCandidate); err != nil {
		t.Fatalf("scoping: %v", err)
	}
	tag, err := tx.Exec(ctx,
		`UPDATE evaluation.contradictions SET a_quote = 'rewritten' WHERE session_id = $1`, ref.SessionID)
	if err == nil {
		if tag.RowsAffected() == 0 {
			t.Fatal("the attack matched zero rows; the trigger was never exercised")
		}
		t.Fatal("a stored contradiction accepted an edit")
	}
}

func TestAnotherCandidatesContradictionsAreInvisible(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	owner := evaluation.SessionRef{
		SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate,
	}
	pair := evaluation.Contradiction{
		Topic: []string{"cache", "latency"},
		SideA: evaluation.ContradictionSide{
			SegmentSequence: 3, Quote: "a", CharStart: 0, CharEnd: 1, StartMs: 1, EndMs: 2,
		},
		SideB: evaluation.ContradictionSide{
			SegmentSequence: 5, Quote: "b", CharStart: 0, CharEnd: 1, StartMs: 3, EndMs: 4,
		},
		ExtractionVersion: "evidence-1",
	}
	if err := store.Replace(ctx, owner, "evidence-1", nil, []evaluation.Contradiction{pair}); err != nil {
		t.Fatalf("store: %v", err)
	}

	stranger := evaluation.SessionRef{
		SessionID: owner.SessionID, Mode: "practice",
		CandidateID: "00000000-0000-7000-8000-0000000000f2",
	}
	stored, err := store.Contradictions(ctx, stranger)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("a stranger read %d pairs", len(stored))
	}
}
