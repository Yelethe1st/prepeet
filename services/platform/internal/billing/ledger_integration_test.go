//go:build integration

package billing_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/internal/billing"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// ADR-0014 against real PostgreSQL: the unit meters exactly once, the
// boundary holds under concurrency, corrections are appended and never
// edited, and the warning precedes any block.

var (
	pool     *pgxpool.Pool
	adminURL string
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

	code := m.Run()
	pool.Close()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating: %v\n", err)
	}
	os.Exit(code)
}

// tenant seeds one tenant and returns its id.
func tenant(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer conn.Close(ctx)

	tenantID := id.New().String()
	if _, err := conn.Exec(ctx, `
		INSERT INTO tenancy.tenants (id, name, slug, region)
		VALUES ($1, 'Ledger Test', $2, 'eu-west-2')`,
		tenantID, "ledger-"+tenantID[len(tenantID)-12:]); err != nil {
		t.Fatalf("seeding tenant: %v", err)
	}
	return tenantID
}

func TestTheUnitMetersExactlyOnce(t *testing.T) {
	ctx := context.Background()
	ledger := billing.NewLedger(pool)
	tenantID := tenant(t)
	sessionID := id.New().String()

	if err := ledger.ReserveStart(ctx, tenantID, sessionID, "screening"); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	// The retry a workflow or a crash produces converges, never doubles.
	if err := ledger.ReserveStart(ctx, tenantID, sessionID, "screening"); !errors.Is(err, billing.ErrAlreadyMetered) {
		t.Fatalf("second reserve = %v, want ErrAlreadyMetered", err)
	}

	usage, err := ledger.Usage(ctx, tenantID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if usage.Started != 1 || usage.Billable != 1 {
		t.Fatalf("usage = %+v; the unit counted %d for one start", usage, usage.Started)
	}
}

func TestTheBoundaryHoldsAndCreditsReopenIt(t *testing.T) {
	ctx := context.Background()
	ledger := billing.NewLedger(pool)
	tenantID := tenant(t)
	limit := 2
	if err := ledger.SetQuota(ctx, tenantID, &limit, 0.8); err != nil {
		t.Fatalf("quota: %v", err)
	}

	first, second := id.New().String(), id.New().String()
	if err := ledger.ReserveStart(ctx, tenantID, first, "screening"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := ledger.ReserveStart(ctx, tenantID, second, "screening"); err != nil {
		t.Fatalf("second: %v", err)
	}
	if err := ledger.ReserveStart(ctx, tenantID, id.New().String(), "screening"); !errors.Is(err, billing.ErrQuotaExhausted) {
		t.Fatalf("over the limit = %v, want ErrQuotaExhausted", err)
	}

	// The platform's own failure is never billed, and the credit reopens
	// exactly the capacity it returns.
	if err := ledger.CreditStart(ctx, tenantID, first, billing.ReasonPlatformInterrupted); err != nil {
		t.Fatalf("credit: %v", err)
	}
	if err := ledger.CreditStart(ctx, tenantID, first, billing.ReasonPlatformInterrupted); !errors.Is(err, billing.ErrAlreadyMetered) {
		t.Fatalf("double credit = %v, want ErrAlreadyMetered", err)
	}
	if err := ledger.ReserveStart(ctx, tenantID, id.New().String(), "screening"); err != nil {
		t.Fatalf("reserve after credit: %v", err)
	}

	usage, err := ledger.Usage(ctx, tenantID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if usage.Started != 3 || usage.Credited != 1 || usage.Billable != 2 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestTheWarningPrecedesTheBlock(t *testing.T) {
	ctx := context.Background()
	ledger := billing.NewLedger(pool)
	tenantID := tenant(t)
	limit := 5
	if err := ledger.SetQuota(ctx, tenantID, &limit, 0.8); err != nil {
		t.Fatalf("quota: %v", err)
	}

	ladder := []struct {
		afterStarts int
		want        billing.Warning
	}{
		{3, billing.WarningNone},
		{4, billing.WarningApproaching},
		{5, billing.WarningReached},
	}
	started := 0
	for _, step := range ladder {
		for started < step.afterStarts {
			if err := ledger.ReserveStart(ctx, tenantID, id.New().String(), "screening"); err != nil {
				t.Fatalf("reserve %d: %v", started+1, err)
			}
			started++
		}
		usage, err := ledger.Usage(ctx, tenantID)
		if err != nil {
			t.Fatalf("usage at %d: %v", started, err)
		}
		if usage.Warning != step.want {
			t.Fatalf("at %d of %d the warning is %q, want %q", started, limit, usage.Warning, step.want)
		}
		if step.want == billing.WarningApproaching {
			// The second criterion's whole point: the warning exists while
			// starts still succeed, so nobody's first notice is a refusal.
			if err := ledger.ReserveStart(ctx, tenantID, id.New().String(), "screening"); err != nil {
				t.Fatalf("a start under warning was refused: %v", err)
			}
			started++
		}
	}
}

func TestConcurrentStartsAtTheBoundaryAdmitExactlyOne(t *testing.T) {
	ctx := context.Background()
	ledger := billing.NewLedger(pool)
	tenantID := tenant(t)
	limit := 1
	if err := ledger.SetQuota(ctx, tenantID, &limit, 0.8); err != nil {
		t.Fatalf("quota: %v", err)
	}

	const racers = 8
	var wg sync.WaitGroup
	results := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			results[slot] = ledger.ReserveStart(ctx, tenantID, id.New().String(), "screening")
		}(i)
	}
	wg.Wait()

	admitted := 0
	for _, err := range results {
		switch {
		case err == nil:
			admitted++
		case errors.Is(err, billing.ErrQuotaExhausted):
		default:
			t.Fatalf("a racer failed with %v", err)
		}
	}
	if admitted != 1 {
		t.Fatalf("%d racers were admitted at a limit of 1", admitted)
	}
}

func TestTheLedgerIsAppendOnlyStructurally(t *testing.T) {
	ctx := context.Background()
	ledger := billing.NewLedger(pool)
	tenantID := tenant(t)
	sessionID := id.New().String()
	if err := ledger.ReserveStart(ctx, tenantID, sessionID, "practice"); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)

	// Even the most privileged ordinary path cannot edit or delete a row:
	// corrections are credit entries, and the trigger says so by name.
	_, err = admin.Exec(ctx,
		`UPDATE billing.usage_entries SET kind = 'start_credited' WHERE session_id = $1`, sessionID)
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("editing the ledger = %v, want the trigger's refusal", err)
	}
	_, err = admin.Exec(ctx,
		`DELETE FROM billing.usage_entries WHERE session_id = $1`, sessionID)
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("deleting from the ledger = %v, want the trigger's refusal", err)
	}
}

func TestACreditNeedsAStartAndADefinedReason(t *testing.T) {
	ctx := context.Background()
	ledger := billing.NewLedger(pool)
	tenantID := tenant(t)

	if err := ledger.CreditStart(ctx, tenantID, id.New().String(), billing.ReasonEarlyAbandon); err == nil {
		t.Fatal("crediting a session that never started invented negative usage")
	}

	sessionID := id.New().String()
	if err := ledger.ReserveStart(ctx, tenantID, sessionID, "screening"); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := ledger.CreditStart(ctx, tenantID, sessionID, "felt_like_it"); err == nil {
		t.Fatal("an undefined credit reason was accepted; the ledger must explain the invoice")
	}
}

func TestQuotaChangesNeverTouchExistingEntries(t *testing.T) {
	// The third criterion's structural half: shrinking a quota below usage
	// refuses NEW starts and changes nothing about what already ran.
	ctx := context.Background()
	ledger := billing.NewLedger(pool)
	tenantID := tenant(t)

	for i := 0; i < 3; i++ {
		if err := ledger.ReserveStart(ctx, tenantID, id.New().String(), "screening"); err != nil {
			t.Fatalf("reserve %d: %v", i, err)
		}
	}
	limit := 1
	if err := ledger.SetQuota(ctx, tenantID, &limit, 0.8); err != nil {
		t.Fatalf("shrinking quota: %v", err)
	}

	if err := ledger.ReserveStart(ctx, tenantID, id.New().String(), "screening"); !errors.Is(err, billing.ErrQuotaExhausted) {
		t.Fatalf("new start after shrink = %v, want ErrQuotaExhausted", err)
	}
	usage, err := ledger.Usage(ctx, tenantID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if usage.Started != 3 || usage.Billable != 3 {
		t.Fatalf("the shrink rewrote history: %+v", usage)
	}
	if usage.Warning != billing.WarningReached {
		t.Fatalf("warning = %q, want reached", usage.Warning)
	}
}
