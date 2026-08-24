//go:build integration

// Outbox tests against real PostgreSQL.
//
// The property this exists to prove is not that events can be written and read.
// It is that an event and the state change it describes either both happen or
// neither does, and that two dispatchers running at once do not deliver the same
// event twice. Both are properties of the database rather than of the code, so
// neither can be tested against a fake.
package outbox_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("prepeet"),
		tcpostgres.WithUsername("prepeet_migrator"),
		tcpostgres.WithPassword("migrator-password"),
		// Not ForListeningPort. PostgreSQL accepts TCP connections before it
		// will answer them, so that strategy returns while the server is still
		// replying "the database system is starting up" and the first
		// connection fails. It made this suite flaky rather than broken, which
		// is worse: a failure that looks like the code under test.
		//
		// The occurrence matters as much as the log line. The official image
		// starts a temporary server to run its initialisation scripts and logs
		// readiness for that one too, so waiting for the first occurrence waits
		// for a server that is about to be shut down.
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting PostgreSQL: %v\n", err)
		os.Exit(1)
	}

	adminURL, err := container.ConnectionString(ctx, "sslmode=disable")
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
		fmt.Fprintf(os.Stderr, "parsing connection string: %v\n", err)
		os.Exit(1)
	}
	pool, err = pgxpool.New(ctx, fmt.Sprintf(
		"postgres://prepeet_app:app-password@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "app pool: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating PostgreSQL: %v\n", err)
	}
	os.Exit(code)
}

func event(t *testing.T, eventType string) outbox.Event {
	t.Helper()
	return outbox.Event{
		Type:          eventType,
		SchemaVersion: "1.0",
		OccurredAt:    time.Now().UTC(),
		Producer:      "identity",
		Actor:         outbox.Actor{Type: "service", ID: "test"},
		Payload:       json.RawMessage(`{"user_id":"usr_1"}`),
	}
}

// The property the outbox exists for. Publishing takes the transaction, so an
// event and the state change it describes commit together or not at all.
func TestPublishingJoinsTheCallersTransaction(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	published, err := store.Publish(ctx, tx, event(t, "identity.user_registered.v1"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Roll back, as the state change would if it failed.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM integration.outbox WHERE id = $1`, published).Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 0 {
		t.Error("the event survived a rolled back transaction, so it describes a state change that never happened")
	}
}

func TestACommittedEventIsReadable(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	published, err := store.Publish(ctx, tx, event(t, "identity.user_registered.v1"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	pending, err := store.Claim(ctx, 10)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	var found bool
	for _, e := range pending {
		if e.ID == published {
			found = true
			if e.Type != "identity.user_registered.v1" {
				t.Errorf("Type = %q, want the type it was published with", e.Type)
			}
		}
	}
	if !found {
		t.Error("a committed event was not returned by Claim")
	}
}

// Two dispatchers is the ordinary case: one is deploying while the other is
// still serving. If both could claim the same row, a webhook would fire twice
// and a tenant's ATS would see one candidate submitted twice.
func TestTwoDispatchersNeverClaimTheSameEvent(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	const events = 40
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	for range events {
		if _, err := store.Publish(ctx, tx, event(t, "concurrency.probe.v1")); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var (
		mu      sync.Mutex
		claimed = map[string]int{}
		wg      sync.WaitGroup
	)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				batch, err := store.Claim(context.Background(), 10)
				if err != nil {
					t.Errorf("Claim: %v", err)
					return
				}
				mu.Lock()
				for _, e := range batch {
					claimed[e.ID]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	for id, times := range claimed {
		if times > 1 {
			t.Errorf("event %s was claimed %d times, want once", id, times)
		}
	}
}

// Marking delivered is what stops redelivery. Until it happens the event stays
// pending, which is the at-least-once guarantee working as intended.
func TestADeliveredEventIsNotClaimedAgain(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	published := publishOne(t, store, "delivery.probe.v1")

	if err := store.MarkDelivered(ctx, published); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}

	for range 3 {
		batch, err := store.Claim(ctx, 50)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		for _, e := range batch {
			if e.ID == published {
				t.Fatal("a delivered event was claimed again")
			}
		}
	}
}

// A failure is not a loss. The event goes back to pending with a later attempt
// time, so a provider that is down for a minute does not cost the event.
func TestAFailedDeliveryIsRetriedLater(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	published := publishOne(t, store, "retry.probe.v1")

	if err := store.MarkFailed(ctx, published, "the endpoint returned 503"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	var attempts int
	var nextAttempt time.Time
	if err := pool.QueryRow(ctx,
		`SELECT attempts, next_attempt_at FROM integration.outbox WHERE id = $1`,
		published).Scan(&attempts, &nextAttempt); err != nil {
		t.Fatalf("reading state: %v", err)
	}

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	if !nextAttempt.After(time.Now()) {
		t.Error("next_attempt_at is not in the future, so the retry would be immediate")
	}
}

// Backoff has to grow, or a permanently broken endpoint is retried thousands of
// times a minute and the failure becomes a load problem as well.
func TestBackoffGrowsWithEachFailure(t *testing.T) {
	t.Parallel()

	first := outbox.Backoff(1)
	second := outbox.Backoff(2)
	tenth := outbox.Backoff(10)

	if second <= first {
		t.Errorf("Backoff(2) = %s is not longer than Backoff(1) = %s", second, first)
	}
	if tenth <= second {
		t.Errorf("Backoff(10) = %s is not longer than Backoff(2) = %s", tenth, second)
	}
	if tenth > time.Hour {
		t.Errorf("Backoff(10) = %s, want it capped so a recovered endpoint is retried within the hour", tenth)
	}
}

// An event nobody can deliver is an operational fact somebody needs to see,
// rather than a row retried silently until the end of time.
func TestAnEventIsDeadLetteredAfterEnoughFailures(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	published := publishOne(t, store, "deadletter.probe.v1")

	for range outbox.MaxAttempts {
		if err := store.MarkFailed(ctx, published, "the endpoint is gone"); err != nil {
			t.Fatalf("MarkFailed: %v", err)
		}
	}

	var dead *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT dead_at FROM integration.outbox WHERE id = $1`, published).Scan(&dead); err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if dead == nil {
		t.Fatalf("the event was not dead lettered after %d failures", outbox.MaxAttempts)
	}

	batch, err := store.Claim(ctx, 100)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	for _, e := range batch {
		if e.ID == published {
			t.Error("a dead lettered event was claimed again")
		}
	}
}

// A restricted payload would reach integrations and analytics, where the
// retention and access rules belong to somebody else.
func TestPublishRefusesAnEventWithoutItsEnvelope(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	for name, broken := range map[string]outbox.Event{
		"no type":     {SchemaVersion: "1.0", Producer: "identity", Actor: outbox.Actor{Type: "service", ID: "x"}},
		"no version":  {Type: "a.b.v1", Producer: "identity", Actor: outbox.Actor{Type: "service", ID: "x"}},
		"no producer": {Type: "a.b.v1", SchemaVersion: "1.0", Actor: outbox.Actor{Type: "service", ID: "x"}},
		"no actor":    {Type: "a.b.v1", SchemaVersion: "1.0", Producer: "identity"},
	} {
		t.Run(name, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			if _, err := store.Publish(ctx, tx, broken); err == nil {
				t.Errorf("Publish accepted an event with %s", name)
			}
		})
	}
}

// The event type carries its contract version, which is what consumers
// subscribe against. An unversioned type would leave them no way to tell a
// breaking change from an additive one.
func TestPublishRequiresAVersionedEventType(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	unversioned := event(t, "identity.user_registered")
	if _, err := store.Publish(ctx, tx, unversioned); err == nil {
		t.Error("Publish accepted an event type with no version suffix")
	}
}

func publishOne(t *testing.T, store *outbox.Store, eventType string) string {
	t.Helper()
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	published, err := store.Publish(ctx, tx, event(t, eventType))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return published
}
