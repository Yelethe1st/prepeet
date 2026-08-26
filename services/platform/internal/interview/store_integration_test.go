//go:build integration

package interview_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// SES-01 against real PostgreSQL: the three criteria, plus the separation the
// sessions table carries because practice and screening share it.

var (
	pool     *pgxpool.Pool
	adminURL string
)

const (
	candidateID = "00000000-0000-7000-8000-0000000000e1"
	recruiterID = "00000000-0000-7000-8000-0000000000e2"
	tenantID    = "00000000-0000-7000-8000-0000000000ea"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("prepeet"),
		tcpostgres.WithUsername("prepeet_migrator"),
		tcpostgres.WithPassword("migrator-password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting PostgreSQL: %v\n", err)
		os.Exit(1)
	}

	adminURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "connection string: %v\n", err)
		os.Exit(1)
	}
	if err := database.Migrate(ctx, adminURL, database.MigrateOptions{AppPassword: "app-password"}); err != nil {
		fmt.Fprintf(os.Stderr, "migrating: %v\n", err)
		os.Exit(1)
	}

	cfg, err := pgx.ParseConfig(adminURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing: %v\n", err)
		os.Exit(1)
	}
	pool, err = pgxpool.New(ctx, fmt.Sprintf(
		"postgres://prepeet_app:app-password@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "app pool: %v\n", err)
		os.Exit(1)
	}

	if err := seed(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "seeding: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating: %v\n", err)
	}
	os.Exit(code)
}

func seed(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	for _, user := range []struct{ id, email string }{
		{candidateID, "amara.sessions@example.com"},
		{recruiterID, "priya.sessions@example.com"},
	} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO identity.users (id, email) VALUES ($1, $2)`, user.id, user.email); err != nil {
			return err
		}
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO tenancy.tenants (id, name, slug, region)
		 VALUES ($1, 'Northwind', 'northwind-s', 'eu-west-2')`, tenantID); err != nil {
		return err
	}
	return nil
}

// candidate is the actor every practice command here runs as.
var candidate = interview.Actor{ID: candidateID, Type: "user"}

// createPractice writes a fresh draft practice session and returns it.
func createPractice(t *testing.T) interview.Session {
	t.Helper()
	store := interview.NewStore(pool)
	session := interview.Session{
		ID:          id.New().String(),
		Mode:        "practice",
		CandidateID: candidateID,
		BlueprintID: "bp_backend_v1",
	}
	if err := store.Create(context.Background(), session, candidate); err != nil {
		t.Fatalf("create: %v", err)
	}
	created, err := store.Get(context.Background(), session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return created
}

func TestCreateWritesTheSessionItsEventAndItsAudit(t *testing.T) {
	ctx := context.Background()
	session := createPractice(t)

	if session.State != interview.StateDraft || session.Version != 1 {
		t.Fatalf("created session is %s v%d, want draft v1", session.State, session.Version)
	}

	// The event and the audit row, read as the migrator because the outbox
	// and audit tables are not the aggregate's scope.
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var events int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM integration.outbox
		WHERE event_type = 'interview.session_created.v1'
		  AND payload->>'session_id' = $1`, session.ID).Scan(&events); err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if events != 1 {
		t.Errorf("create published %d session_created events, want 1", events)
	}

	var audits int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM audit.events
		WHERE action = 'interview.session_created' AND subject_id = $1`, session.ID).Scan(&audits); err != nil {
		t.Fatalf("counting audit: %v", err)
	}
	if audits != 1 {
		t.Errorf("create wrote %d audit rows, want 1", audits)
	}
}

func TestAnInvalidTransitionIsAStableRefusalNotA500(t *testing.T) {
	session := createPractice(t)
	store := interview.NewStore(pool)

	// draft -> evaluating exists on no edge of the spec's diagram.
	_, err := store.Transition(context.Background(), session, interview.StateEvaluating,
		interview.Effects{}, candidate)

	var refused *interview.TransitionError
	if !errors.As(err, &refused) {
		t.Fatalf("an invalid transition returned %v, want a TransitionError", err)
	}
	if refused.Code != "SESSION_TRANSITION_INVALID" {
		t.Fatalf("code = %q", refused.Code)
	}
}

func TestAStaleWriteIsRefusedRatherThanSilentlyOverwriting(t *testing.T) {
	ctx := context.Background()
	session := createPractice(t)
	store := interview.NewStore(pool)

	// Two readers hold version 1. The first transitions; the second tries to
	// act on the world it remembers.
	first, err := store.Transition(ctx, session, interview.StateComposing, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("first transition: %v", err)
	}
	if first.Version != 2 {
		t.Fatalf("version after transition = %d, want 2", first.Version)
	}

	_, err = store.Transition(ctx, session, interview.StateCancelled, interview.Effects{}, candidate)
	if !errors.Is(err, interview.ErrStaleVersion) {
		t.Fatalf("a stale write returned %v, want ErrStaleVersion", err)
	}

	// And the stale write changed nothing: the session is still composing.
	current, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if current.State != interview.StateComposing {
		t.Fatalf("the stale write moved the session to %s", current.State)
	}
}

func TestTheBundleArrivesWithReadyAndTheEventCarriesIt(t *testing.T) {
	ctx := context.Background()
	session := createPractice(t)
	store := interview.NewStore(pool)

	composing, err := store.Transition(ctx, session, interview.StateComposing, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("to composing: %v", err)
	}

	effects := interview.Effects{
		BundleRef:      "bundles/" + session.ID,
		BundleDigest:   "sha256:abc123",
		BundleRevision: 1,
	}
	event, err := interview.ReadyEvent(composing, effects, candidate)
	if err != nil {
		t.Fatalf("building the ready event: %v", err)
	}
	effects.Event = event

	ready, err := store.Transition(ctx, composing, interview.StateReady, effects, candidate)
	if err != nil {
		t.Fatalf("to ready: %v", err)
	}
	if ready.BundleDigest != "sha256:abc123" || ready.BundleRevision != 1 {
		t.Fatalf("ready session carries bundle %q rev %d", ready.BundleDigest, ready.BundleRevision)
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	var digest string
	if err := conn.QueryRow(ctx, `
		SELECT payload->>'bundle_digest' FROM integration.outbox
		WHERE event_type = 'interview.session_ready.v1' AND payload->>'session_id' = $1`,
		session.ID).Scan(&digest); err != nil {
		t.Fatalf("the ready event was not published: %v", err)
	}
	if digest != "sha256:abc123" {
		t.Fatalf("the event carries digest %q", digest)
	}
}

func TestPracticeSessionsAreInvisibleToTenantScope(t *testing.T) {
	// The IAM-06 separation on the one table both modes share.
	ctx := context.Background()
	session := createPractice(t)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantID); err != nil {
		t.Fatalf("SetTenant: %v", err)
	}
	if err := database.SetUser(ctx, tx, candidateID); err != nil {
		t.Fatalf("SetUser: %v", err)
	}

	// Even naming the row, even knowing the candidate: a transaction carrying
	// tenant context sees no practice sessions at all.
	var count int
	if err := tx.QueryRow(ctx,
		"SELECT count(*) FROM interview.sessions WHERE id = $1", session.ID).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Fatal("a practice session was visible to a tenant-scoped transaction")
	}
}

func TestTheSchemaRefusesAPracticeSessionWithATenant(t *testing.T) {
	// The CHECK is the last line: even code that lied its way past everything
	// else cannot record the linkage.
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, `
		INSERT INTO interview.sessions (id, mode, candidate_id, tenant_id, blueprint_id)
		VALUES ($1, 'practice', $2, $3, 'bp')`, id.New().String(), candidateID, tenantID)
	if err == nil {
		t.Fatal("a practice session with a tenant was inserted; the CHECK is gone")
	}
}

func TestTheBundleDocumentPersistsWithReadyAndFreezes(t *testing.T) {
	// CAT-02's middle box at the aggregate: the ready transition persists the
	// bundle document in its own transaction, and from that moment the
	// document is immutable and permanent even to the table's owner.
	ctx := context.Background()
	session := createPractice(t)
	store := interview.NewStore(pool)

	composing, err := store.Transition(ctx, session, interview.StateComposing, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("to composing: %v", err)
	}

	effects := interview.Effects{
		BundleRef:      "sessions/" + session.ID + "/bundle",
		BundleDigest:   "sha256:bundledoc",
		BundleRevision: 1,
		BundleBody:     []byte(`{"pinned_inputs":[{"reference":"bp_backend_v1","digest":"sha256:plan"}]}`),
	}
	event, err := interview.ReadyEvent(composing, effects, candidate)
	if err != nil {
		t.Fatalf("event: %v", err)
	}
	effects.Event = event
	if _, err := store.Transition(ctx, composing, interview.StateReady, effects, candidate); err != nil {
		t.Fatalf("to ready: %v", err)
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var digest, body string
	if err := conn.QueryRow(ctx,
		`SELECT digest, body::text FROM interview.session_bundles WHERE session_id = $1`,
		session.ID).Scan(&digest, &body); err != nil {
		t.Fatalf("the bundle document was not persisted: %v", err)
	}
	if digest != "sha256:bundledoc" || body == "" {
		t.Fatalf("persisted bundle: digest=%q body=%q", digest, body)
	}

	if _, err := conn.Exec(ctx,
		`UPDATE interview.session_bundles SET body = '{}' WHERE session_id = $1`, session.ID); err == nil {
		t.Fatal("a persisted bundle was edited; what the session ran against just changed after the fact")
	}
	if _, err := conn.Exec(ctx,
		`DELETE FROM interview.session_bundles WHERE session_id = $1`, session.ID); err == nil {
		t.Fatal("a persisted bundle was deleted; a review has nothing to reconstruct from")
	}
}

func TestEveryTransitionCarriesItsActorIntoTheAudit(t *testing.T) {
	// SES-01's third criterion, walked edge by edge: each transition writes
	// exactly one audit row, the row names the actor who carried authority -
	// person or automation - and a transition the catalogue defines no event
	// for still audits without inventing one.
	ctx := context.Background()
	session := createPractice(t)
	store := interview.NewStore(pool)

	automation := interview.Actor{ID: candidateID, Type: "service"}

	composing, err := store.Transition(ctx, session, interview.StateComposing, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("to composing: %v", err)
	}
	failed, err := store.Transition(ctx, composing, interview.StateCompositionFailed,
		interview.Effects{FailureCode: "FAILURE_CODE_PROVIDER_UNAVAILABLE"}, automation)
	if err != nil {
		t.Fatalf("to composition_failed: %v", err)
	}
	if _, err := store.Transition(ctx, failed, interview.StateComposing, interview.Effects{}, candidate); err != nil {
		t.Fatalf("retry to composing: %v", err)
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT action, actor_id::text, actor_type, outcome
		FROM audit.events WHERE subject_id = $1 ORDER BY occurred_at, action`, session.ID)
	if err != nil {
		t.Fatalf("reading audit: %v", err)
	}
	defer rows.Close()

	type auditRow struct{ action, actorID, actorType, outcome string }
	var trail []auditRow
	for rows.Next() {
		var row auditRow
		if err := rows.Scan(&row.action, &row.actorID, &row.actorType, &row.outcome); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		trail = append(trail, row)
	}

	// One row per act: the create and three transitions. No transition is
	// unaccounted for, and none is double-counted.
	if len(trail) != 4 {
		t.Fatalf("the trail holds %d rows for 4 acts: %+v", len(trail), trail)
	}
	counts := map[string]int{}
	for _, row := range trail {
		counts[row.action]++
		if row.actorID != candidateID {
			t.Errorf("%s recorded actor %s, want the acting user", row.action, row.actorID)
		}
		if row.outcome != "allowed" {
			t.Errorf("%s recorded outcome %s", row.action, row.outcome)
		}
	}
	if counts["interview.session_composing"] != 2 || counts["interview.session_composition_failed"] != 1 {
		t.Fatalf("actions = %v", counts)
	}

	// The automation's transition says so: acting FOR the person, AS a
	// service, and the distinction survives into the row.
	for _, row := range trail {
		if row.action == "interview.session_composition_failed" && row.actorType != "service" {
			t.Errorf("the automated transition recorded actor_type %q, want service", row.actorType)
		}
		if row.action == "interview.session_created" && row.actorType != "user" {
			t.Errorf("the person's own act recorded actor_type %q, want user", row.actorType)
		}
	}

	// Eventless transitions invented no events: the outbox holds only the
	// created event for this session - composing and composition_failed have
	// no catalogued event and must not fabricate one.
	var events int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM integration.outbox
		WHERE payload->>'session_id' = $1`, session.ID).Scan(&events); err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if events != 1 {
		t.Fatalf("%d events for a created+composing+failed+retried session, want only session_created", events)
	}
}
