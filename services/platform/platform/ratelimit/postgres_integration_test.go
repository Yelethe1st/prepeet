//go:build integration

// Rate limiter tests against real PostgreSQL.
//
// The property that matters here cannot be tested any other way: two instances
// of the limiter, standing in for two ECS tasks, must share one count. The
// in-memory limiter passes every other test in this package and fails this one,
// which is exactly why this file exists.
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

func keyFor(t *testing.T) string {
	t.Helper()
	return t.Name() + "@example.com"
}

func TestPostgresCounterAllowsUnderTheLimit(t *testing.T) {
	ctx := context.Background()
	counter := ratelimit.NewPostgres(pool, ratelimit.Rule{Limit: 5, Window: time.Minute}, time.Now)

	for i := range 5 {
		decision, err := counter.Allow(ctx, keyFor(t))
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		if !decision.Allowed {
			t.Errorf("request %d was refused while under the limit", i+1)
		}
	}
}

func TestPostgresCounterRefusesOverTheLimit(t *testing.T) {
	ctx := context.Background()
	counter := ratelimit.NewPostgres(pool, ratelimit.Rule{Limit: 3, Window: time.Minute}, time.Now)

	for range 3 {
		if _, err := counter.Allow(ctx, keyFor(t)); err != nil {
			t.Fatalf("Allow: %v", err)
		}
	}

	decision, err := counter.Allow(ctx, keyFor(t))
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if decision.Allowed {
		t.Error("the fourth request was allowed under a limit of three")
	}
	if decision.RetryAfter <= 0 {
		t.Error("RetryAfter is not positive, and a client needs to know when to try again")
	}
}

// The reason this file exists. Two limiters standing in for two ECS tasks must
// share one count, or an attacker simply gets the limit multiplied by the task
// count and the limit means nothing.
func TestTwoInstancesShareOneCount(t *testing.T) {
	ctx := context.Background()
	rule := ratelimit.Rule{Limit: 4, Window: time.Minute}

	taskOne := ratelimit.NewPostgres(pool, rule, time.Now)
	taskTwo := ratelimit.NewPostgres(pool, rule, time.Now)

	allowed := 0
	for i := range 8 {
		counter := taskOne
		if i%2 == 1 {
			counter = taskTwo // alternate, as a load balancer would
		}
		decision, err := counter.Allow(ctx, keyFor(t))
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		if decision.Allowed {
			allowed++
		}
	}

	if allowed != 4 {
		t.Errorf("allowed %d of 8 attempts across two instances, want 4: the count is not shared", allowed)
	}
}

// Two requests landing at the same instant must not both read the count before
// either writes it, or the limit is a suggestion under exactly the load that
// matters.
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
			decision, err := counter.Allow(context.Background(), keyFor(t))
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

func TestPostgresCounterRecoversAsTheWindowPasses(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	clock := func() time.Time { return now }
	counter := ratelimit.NewPostgres(pool, ratelimit.Rule{Limit: 2, Window: time.Minute}, clock)

	for range 2 {
		if _, err := counter.Allow(ctx, keyFor(t)); err != nil {
			t.Fatalf("Allow: %v", err)
		}
	}
	refused, err := counter.Allow(ctx, keyFor(t))
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if refused.Allowed {
		t.Fatal("the third request was allowed under a limit of two")
	}

	now = now.Add(time.Minute + time.Second)

	recovered, err := counter.Allow(ctx, keyFor(t))
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !recovered.Allowed {
		t.Error("the limit did not recover after the window passed")
	}
}

// One person exhausting their attempts must not lock out everyone else.
func TestPostgresCounterSeparatesKeys(t *testing.T) {
	ctx := context.Background()
	counter := ratelimit.NewPostgres(pool, ratelimit.Rule{Limit: 2, Window: time.Minute}, time.Now)

	for range 2 {
		if _, err := counter.Allow(ctx, "exhausted-"+keyFor(t)); err != nil {
			t.Fatalf("Allow: %v", err)
		}
	}

	decision, err := counter.Allow(ctx, "untouched-"+keyFor(t))
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !decision.Allowed {
		t.Error("one key exhausting its limit refused a different key")
	}
}

// If this database is unreachable, authentication cannot happen at all: the
// credential lookup uses the same store. So the limiter failing open costs
// nothing that is not already lost, and failing closed would turn a database
// blip into a total lockout. The error is still returned so the caller can
// alert on it.
func TestTheLimiterFailsOpenWhenTheDatabaseIsUnreachable(t *testing.T) {
	ctx := context.Background()

	closed, err := pgxpool.New(ctx, "postgres://nobody:nothing@127.0.0.1:1/nowhere?sslmode=disable")
	if err != nil {
		t.Fatalf("building an unreachable pool: %v", err)
	}
	defer closed.Close()

	counter := ratelimit.NewPostgres(closed, ratelimit.Rule{Limit: 5, Window: time.Minute}, time.Now)

	decision, err := counter.Allow(ctx, keyFor(t))

	if !decision.Allowed {
		t.Error("the limiter refused the request when the database was unreachable, which would lock everyone out during a blip")
	}
	if err == nil {
		t.Error("the limiter reported no error, so nobody would know it had stopped counting")
	}
}

// Rows nobody will read again are only rows somebody has to store, and these
// keys are email and network addresses, which is personal data.
func TestSweepRemovesOldWindows(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	clock := func() time.Time { return now }
	counter := ratelimit.NewPostgres(pool, ratelimit.Rule{Limit: 5, Window: time.Minute}, clock)

	if _, err := counter.Allow(ctx, keyFor(t)); err != nil {
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

// The counter must not be able to tell a registered address from an unknown
// one, because it runs before the credential check and a limiter that behaved
// differently would enumerate accounts on its own.
func TestThePostgresCounterNeverLooksAKeyUp(t *testing.T) {
	ctx := context.Background()
	counter := ratelimit.NewPostgres(pool, ratelimit.Rule{Limit: 2, Window: time.Minute}, time.Now)

	known, err := counter.Allow(ctx, "registered-"+keyFor(t))
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	unknown, err := counter.Allow(ctx, "never-seen-"+keyFor(t))
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}

	if known.Allowed != unknown.Allowed || known.Remaining != unknown.Remaining {
		t.Error("the counter treated two keys differently, and it has no way to know which is registered")
	}
}
