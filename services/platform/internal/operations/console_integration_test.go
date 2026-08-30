//go:build integration

package operations_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetevents"
	"github.com/Yelethe1st/prepeet/services/platform/internal/operations"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// OPS-03's third criterion against real PostgreSQL: every operator action here
// lands in the append-only trail, including the ones that changed nothing.
//
// The queue behind the console is the real outbox, reached through the port
// this package declares and adapted in the test exactly as cmd adapts it. A
// fake would prove the audit row is written and nothing about the property that
// matters, which is that the row and the transition share a transaction: an
// effect without its audit row is an operator action nobody can review, and an
// audit row without its effect is a trail that lies.

var (
	pool     *pgxpool.Pool
	adminURL string
)

// operatorID is the acting person. Audit rows carry a foreign key to
// identity.users, so an operator has to be a real account, which is the point:
// "somebody retried this" is only useful if somebody is nameable.
const operatorID = "00000000-0000-7000-8000-0000000000f1"

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

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed connect: %v\n", err)
		os.Exit(1)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO identity.users (id, email) VALUES ($1, $2)`,
		operatorID, "operator.ops03@example.com"); err != nil {
		fmt.Fprintf(os.Stderr, "seeding the operator: %v\n", err)
		os.Exit(1)
	}
	_ = conn.Close(ctx)

	code := m.Run()
	pool.Close()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating: %v\n", err)
	}
	os.Exit(code)
}

// outboxQueue adapts the outbox to the port this package declares.
//
// It is the same adapter cmd wires, written here because the test needs the
// real queue: every property below is about a transaction shared between the
// queue's transition and this package's audit row, and a fake queue would share
// nothing.
type outboxQueue struct{ store *outbox.Store }

func (q outboxQueue) Depth(ctx context.Context) (operations.Depth, error) {
	backlog, err := q.store.Backlog(ctx)
	if err != nil {
		return operations.Depth{}, err
	}
	return operations.Depth{
		Pending: backlog.Pending, Failed: backlog.Failed,
		OldestPending: backlog.OldestPending,
	}, nil
}

func (q outboxQueue) Failed(ctx context.Context, limit int) ([]operations.Item, error) {
	failed, err := q.store.FailedEvents(ctx, limit)
	if err != nil {
		return nil, err
	}
	items := make([]operations.Item, 0, len(failed))
	for _, event := range failed {
		items = append(items, operations.Item{
			ID: event.ID, Kind: event.Type, TenantID: event.TenantID,
			OccurredAt: event.OccurredAt, FailedAt: event.DeadAt,
			Attempts: event.Attempts, LastError: event.LastError,
		})
	}
	return items, nil
}

func (q outboxQueue) Recover(ctx context.Context, tx pgx.Tx, itemID string) (bool, error) {
	return q.store.Recover(ctx, tx, itemID)
}

func (q outboxQueue) Discard(ctx context.Context, tx pgx.Tx, itemID, reason string) (bool, error) {
	return q.store.Discard(ctx, tx, itemID, reason)
}

// console builds the thing under test over the real outbox.
func console() *operations.Console {
	return operations.NewConsole(pool, outboxQueue{store: outbox.New(pool)})
}

// deadEvent publishes an event and exhausts its delivery attempts, which is how
// work arrives in front of an operator.
func deadEvent(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	store := outbox.New(pool)

	definition := prepeetevents.Catalogue[prepeetevents.IdentityUserRegisteredV1]
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	eventID, err := store.Publish(ctx, tx, outbox.Event{
		Type:          string(prepeetevents.IdentityUserRegisteredV1),
		SchemaVersion: definition.SchemaVersion,
		Producer:      definition.Owner,
		Actor:         outbox.Actor{Type: "service", ID: "test"},
		OccurredAt:    time.Now().UTC(),
		Payload:       payloadFor(t, definition),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	for range outbox.MaxAttempts {
		if err := store.MarkFailed(ctx, eventID, "the endpoint refused"); err != nil {
			t.Fatalf("MarkFailed: %v", err)
		}
	}
	return eventID
}

// payloadFor builds a payload carrying every field the contract requires.
func payloadFor(t *testing.T, definition prepeetevents.Definition) []byte {
	t.Helper()
	payload := "{"
	for i, field := range definition.Required {
		if i > 0 {
			payload += ","
		}
		payload += fmt.Sprintf("%q:%q", field, "placeholder")
	}
	return []byte(payload + "}")
}

// auditRows returns the recorded actions against one item.
//
// Read through the superuser connection rather than the application pool
// because audit.events forces row-level security and its untenanted policy is
// written for the acting user. That policy is the subject of IAM-03's tests;
// what this suite needs is to see what was written, from outside.
func auditRows(t *testing.T, subjectID string) []struct {
	Action  string
	Outcome string
	Actor   string
	Request string
	Detail  string
} {
	t.Helper()
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	rows, err := conn.Query(ctx,
		`SELECT action, outcome, coalesce(actor_id::text, ''), coalesce(request_id, ''), detail::text
		 FROM audit.events WHERE subject_type = 'outbox_event' AND subject_id = $1
		 ORDER BY occurred_at`, subjectID)
	if err != nil {
		t.Fatalf("reading audit rows: %v", err)
	}
	defer rows.Close()

	var found []struct {
		Action  string
		Outcome string
		Actor   string
		Request string
		Detail  string
	}
	for rows.Next() {
		var row struct {
			Action  string
			Outcome string
			Actor   string
			Request string
			Detail  string
		}
		if err := rows.Scan(&row.Action, &row.Outcome, &row.Actor, &row.Request, &row.Detail); err != nil {
			t.Fatalf("scanning audit rows: %v", err)
		}
		found = append(found, row)
	}
	return found
}

// stillDead reports whether the item is still waiting for an operator.
func stillDead(t *testing.T, eventID string) bool {
	t.Helper()

	var dead bool
	if err := pool.QueryRow(context.Background(),
		`SELECT dead_at IS NOT NULL FROM integration.outbox WHERE id = $1`, eventID).Scan(&dead); err != nil {
		t.Fatalf("reading dead_at: %v", err)
	}
	return dead
}

var operator = operations.Operator{UserID: operatorID, RequestID: "req-ops03"}

func TestARetryIsAuditedWithItsActorAndReason(t *testing.T) {
	eventID := deadEvent(t)

	if err := console().Retry(context.Background(), operator, eventID, "the endpoint is back"); err != nil {
		t.Fatalf("Retry: %v", err)
	}

	rows := auditRows(t, eventID)
	if len(rows) != 1 {
		t.Fatalf("got %d audit rows, want exactly one", len(rows))
	}
	if rows[0].Action != "operations.work_retried" {
		t.Errorf("action = %q, want operations.work_retried", rows[0].Action)
	}
	if rows[0].Outcome != "allowed" {
		t.Errorf("outcome = %q, want allowed", rows[0].Outcome)
	}
	if rows[0].Actor != operatorID {
		t.Errorf("actor = %q, want the operator", rows[0].Actor)
	}
	if rows[0].Request != "req-ops03" {
		t.Errorf("request_id = %q, want the correlation identifier", rows[0].Request)
	}
	if want := "the endpoint is back"; !strings.Contains(rows[0].Detail, want) {
		t.Errorf("detail = %q, want it to carry %q", rows[0].Detail, want)
	}
}

// The retry criterion at this layer: the same failed item, retried twice, is
// recovered once. The second attempt is refused rather than quietly repeated,
// which is what stops two operators working the same queue from delivering the
// same work twice.
func TestRetryingTheSameItemTwiceRecoversItOnce(t *testing.T) {
	ctx := context.Background()
	eventID := deadEvent(t)

	if err := console().Retry(ctx, operator, eventID, "first"); err != nil {
		t.Fatalf("first Retry: %v", err)
	}
	err := console().Retry(ctx, operator, eventID, "second")
	if !errors.Is(err, operations.ErrNotRecoverable) {
		t.Fatalf("second Retry returned %v, want ErrNotRecoverable", err)
	}

	rows := auditRows(t, eventID)
	if len(rows) != 2 {
		t.Fatalf("got %d audit rows, want the retry and its refusal", len(rows))
	}
	var allowed int
	for _, row := range rows {
		if row.Outcome == "allowed" {
			allowed++
		}
	}
	if allowed != 1 {
		t.Errorf("%d retries were allowed, want exactly one", allowed)
	}
	// The refusal is audited too. During an incident it is usually the first
	// sign that two people are working the same queue.
	if rows[1].Outcome != "denied" {
		t.Errorf("the refused retry was recorded as %q, want denied", rows[1].Outcome)
	}
}

// The atomicity that makes "every operator action is audited" true rather than
// aspirational: if the audit row cannot be written, the work is not retried.
//
// The failure is provoked the way it would really happen, with an actor the
// audit table's foreign key does not recognise.
func TestWorkIsNotRetriedWhenItCannotBeAudited(t *testing.T) {
	ctx := context.Background()
	eventID := deadEvent(t)

	unknown := operations.Operator{UserID: "00000000-0000-7000-8000-00000000beef", RequestID: "req-ghost"}
	if err := console().Retry(ctx, unknown, eventID, "who am I"); err == nil {
		t.Fatal("a retry by an unknown operator succeeded, so an unauditable action is possible")
	}

	if !stillDead(t, eventID) {
		t.Error("the item was retried anyway, so the audit row and the transition are not atomic")
	}
	if rows := auditRows(t, eventID); len(rows) != 0 {
		t.Errorf("got %d audit rows for a failed action, want none", len(rows))
	}
}

func TestADiscardIsAuditedAndTerminal(t *testing.T) {
	ctx := context.Background()
	eventID := deadEvent(t)

	if err := console().Discard(ctx, operator, eventID, "the destination was decommissioned"); err != nil {
		t.Fatalf("Discard: %v", err)
	}

	rows := auditRows(t, eventID)
	if len(rows) != 1 || rows[0].Action != "operations.work_discarded" {
		t.Fatalf("got %v, want one operations.work_discarded row", rows)
	}
	if !strings.Contains(rows[0].Detail, "decommissioned") {
		t.Errorf("detail = %q, want the operator's reason", rows[0].Detail)
	}

	// Terminal: it cannot be brought back by the other action.
	if err := console().Retry(ctx, operator, eventID, "changed my mind"); !errors.Is(err, operations.ErrNotRecoverable) {
		t.Errorf("retrying discarded work returned %v, want ErrNotRecoverable", err)
	}
}

// A destructive action with no reason is an audit row nobody can review, so it
// is refused before anything is touched rather than recorded as a blank.
func TestAnActionWithoutAReasonIsRefusedBeforeItTouchesAnything(t *testing.T) {
	ctx := context.Background()
	eventID := deadEvent(t)

	if err := console().Discard(ctx, operator, eventID, "   "); !errors.Is(err, operations.ErrReasonRequired) {
		t.Fatalf("Discard without a reason returned %v, want ErrReasonRequired", err)
	}
	if err := console().Retry(ctx, operator, eventID, ""); !errors.Is(err, operations.ErrReasonRequired) {
		t.Fatalf("Retry without a reason returned %v, want ErrReasonRequired", err)
	}
	if !stillDead(t, eventID) {
		t.Error("the item moved despite the refusal")
	}
	if rows := auditRows(t, eventID); len(rows) != 0 {
		t.Errorf("got %d audit rows for an action that was never attempted, want none", len(rows))
	}
}

// An action nobody can be named for cannot be audited, so it cannot be taken.
func TestAnAnonymousActionIsRefused(t *testing.T) {
	ctx := context.Background()
	eventID := deadEvent(t)

	anonymous := operations.Operator{RequestID: "req-anon"}
	if err := console().Retry(ctx, anonymous, eventID, "trust me"); !errors.Is(err, operations.ErrOperatorRequired) {
		t.Fatalf("an anonymous retry returned %v, want ErrOperatorRequired", err)
	}
	if !stillDead(t, eventID) {
		t.Error("anonymous work was retried")
	}
}

// The console's read side: what an operator is looking at when they decide.
func TestTheConsoleShowsTheBacklogAndWhatFailed(t *testing.T) {
	ctx := context.Background()
	eventID := deadEvent(t)

	assessment, err := console().Backlog(ctx)
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	if !assessment.FailedBreached {
		t.Error("dead-lettered work is present and the assessment is not breaching")
	}

	items, err := console().Failed(ctx, 0)
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	var found bool
	for _, item := range items {
		if item.ID == eventID {
			found = true
			if item.Attempts != outbox.MaxAttempts {
				t.Errorf("Attempts = %d, want %d", item.Attempts, outbox.MaxAttempts)
			}
			if item.LastError == "" {
				t.Error("the item carries no failure reason, so an operator is deciding blind")
			}
		}
	}
	if !found {
		t.Errorf("%s is dead lettered and was not listed", eventID)
	}
}
