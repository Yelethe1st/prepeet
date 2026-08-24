//go:build integration

// Rate limiter tests against real PostgreSQL.
//
// The shared contract in counter_suite_test.go covers the behaviour every
// counter must have. This file adds only what cannot be tested any other way:
// two instances standing in for two ECS tasks sharing one count, exact counting
// under concurrency, sweeping, and what happens when the database is
// unreachable.
//
// The in-memory counter passes the whole shared contract and fails the first of
// those, which is the entire argument for this implementation existing.
package ratelimit_test

import (
	"context"
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
	"github.com/Yelethe1st/prepeet/services/platform/platform/ratelimit"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("prepeet"),
		tcpostgres.WithUsername("prepeet_migrator"),
		tcpostgres.WithPassword("migrator-password"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp")),
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

// Postgres satisfies the same contract as every other counter.
func TestPostgresSatisfiesTheCounterContract(t *testing.T) {
	runCounterContract(t, func(rule ratelimit.Rule, now func() time.Time) ratelimit.Counter {
		return ratelimit.NewPostgres(pool, rule, now)
	})
}

// What follows is specific to counting in the database.

// The reason this implementation exists. Two counters standing in for two ECS
// tasks must share one count, or an attacker gets the limit multiplied by the
// task count and the limit means nothing.
func TestTwoInstancesShareOneCount(t *testing.T) {
	rule := ratelimit.Rule{Limit: 4, Window: time.Minute}

	taskOne := ratelimit.NewPostgres(pool, rule, time.Now)
	taskTwo := ratelimit.NewPostgres(pool, rule, time.Now)

	allowed := 0
	for i := range 8 {
		counter := ratelimit.Counter(taskOne)
		if i%2 == 1 {
			counter = taskTwo // alternate, as a load balancer would
		}
		if allow(t, counter, key(t)).Allowed {
			allowed++
		}
	}

	if allowed != 4 {
		t.Errorf("allowed %d of 8 attempts across two instances, want 4: the count is not shared", allowed)
	}
}

// Two requests landing at the same instant must not both read the count before
// either writes it, or the limit is a suggestion under exactly the load that
// matters. Replacing the atomic statement with a read followed by a write let
// all thirty through, which is how this test earned its place.
func TestConcurrentAttemptsAreCountedExactly(t *testing.T) {
	counter := ratelimit.NewPostgres(pool, ratelimit.Rule{Limit: 10, Window: time.Minute}, time.Now)

	var (
		mu      sync.Mutex
		allowed int
		wg      sync.WaitGroup
	)
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decision, err := counter.Allow(context.Background(), key(t))
			if err != nil {
				t.Errorf("Allow: %v", err)
				return
			}
			if decision.Allowed {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowed != 10 {
		t.Errorf("allowed %d of 30 concurrent attempts, want exactly 10", allowed)
	}
}

// If this database is unreachable, authentication cannot happen at all: the
// credential lookup uses the same store. So the limiter failing open costs
// nothing that is not already lost, and failing closed would turn a database
// blip into a total lockout. The error is still returned so the caller can
// alert on it.
func TestTheLimiterFailsOpenWhenTheDatabaseIsUnreachable(t *testing.T) {
	ctx := context.Background()

	unreachable, err := pgxpool.New(ctx, "postgres://nobody:nothing@127.0.0.1:1/nowhere?sslmode=disable")
	if err != nil {
		t.Fatalf("building an unreachable pool: %v", err)
	}
	defer unreachable.Close()

	counter := ratelimit.NewPostgres(unreachable, ratelimit.Rule{Limit: 5, Window: time.Minute}, time.Now)

	decision, err := counter.Allow(ctx, key(t))

	if !decision.Allowed {
		t.Error("the limiter refused the request when the database was unreachable, which would lock everyone out during a blip")
	}
	if err == nil {
		t.Error("the limiter reported no error, so nobody would know it had stopped counting")
	}
}

// Rows nobody will read again are only rows somebody has to store, and the keys
// here are email and network addresses, which is personal data.
func TestSweepRemovesOldWindows(t *testing.T) {
	ctx := context.Background()

	now := time.Now()
	counter := ratelimit.NewPostgres(pool,
		ratelimit.Rule{Limit: 5, Window: time.Minute},
		func() time.Time { return now })

	if _, err := counter.Allow(ctx, key(t)); err != nil {
		t.Fatalf("Allow: %v", err)
	}

	now = now.Add(time.Hour)
	removed, err := counter.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if removed == 0 {
		t.Error("Sweep removed nothing, so old windows accumulate forever")
	}
}
