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

	if err := store.Replace(ctx, ref, "evidence-1", spans); err != nil {
		t.Fatalf("first store: %v", err)
	}
	// The retry a worker death produces: identical input, identical rows.
	if err := store.Replace(ctx, ref, "evidence-1", spans); err != nil {
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
	if err := store.Replace(ctx, ref, "evidence-1", spans[:1]); err != nil {
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
		[]evaluation.Span{span(3, "systems-design", "private evidence")}); err != nil {
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
