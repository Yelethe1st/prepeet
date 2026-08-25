//go:build integration

package candidate_test

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

	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

// PRO-01 against real PostgreSQL: the profile is the owner's and nobody
// else's, at the store and below it, and a partial profile is a profile.

var (
	pool     *pgxpool.Pool
	adminURL string
)

const (
	amaraID = "00000000-0000-7000-8000-0000000000d1"
	priyaID = "00000000-0000-7000-8000-0000000000d2"
	tenant  = "00000000-0000-7000-8000-0000000000da"
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

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed connect: %v\n", err)
		os.Exit(1)
	}
	for _, user := range []struct{ id, email string }{
		{amaraID, "amara.profile@example.com"}, {priyaID, "priya.profile@example.com"},
	} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO identity.users (id, email) VALUES ($1, $2)`, user.id, user.email); err != nil {
			fmt.Fprintf(os.Stderr, "seeding: %v\n", err)
			os.Exit(1)
		}
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO tenancy.tenants (id, name, slug, region)
		 VALUES ($1, 'Northwind', 'northwind-p', 'eu-west-2')`, tenant); err != nil {
		fmt.Fprintf(os.Stderr, "seeding tenant: %v\n", err)
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

func service() *candidate.Service {
	return candidate.NewService(candidate.NewStore(pool))
}

func TestANeverSavedProfileIsTheEmptyProfileNotAnError(t *testing.T) {
	profile, err := service().GetProfile(context.Background(), priyaID)
	if err != nil {
		t.Fatalf("the first visit looks broken: %v", err)
	}
	if len(profile.Disciplines) != 0 || profile.Seniority != "" {
		t.Fatalf("an unsaved profile carries data: %+v", profile)
	}
	// The one default that is on before any save: practice reminders, which
	// the empty read must agree with the schema about.
	if !profile.NotifyPracticeReminders {
		t.Fatal("the empty profile disagrees with the schema's reminder default")
	}
}

func TestSaveAndReadBackRoundTrips(t *testing.T) {
	ctx := context.Background()

	saved, err := service().SaveProfile(ctx, amaraID, candidate.Profile{
		Disciplines:            []string{"Go", "distributed systems"},
		TargetRoles:            []string{"Staff Engineer"},
		Seniority:              "senior",
		CareerContext:          "ten years in payments, moving toward platform work",
		DefaultDurationMinutes: 30,
		DefaultPressure:        "standard",
		ExtendedTime:           true,
		Captions:               true,
		AccessibilityNotes:     "please avoid rapid-fire follow-ups",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	if len(saved.Disciplines) != 2 || saved.Disciplines[1] != "distributed systems" {
		t.Fatalf("disciplines = %v", saved.Disciplines)
	}
	if !saved.ExtendedTime || !saved.Captions {
		t.Fatal("the accessibility preferences did not survive the round trip")
	}
	if saved.UpdatedAt.IsZero() {
		t.Fatal("no updated_at came back")
	}

	// Saving again replaces whole-record: a cleared field stays cleared.
	again, err := service().SaveProfile(ctx, amaraID, candidate.Profile{
		Disciplines: []string{"Go"},
	})
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if len(again.TargetRoles) != 0 || again.CareerContext != "" || again.ExtendedTime {
		t.Fatalf("cleared fields survived the replace: %+v", again)
	}
}

func TestTheProfileIsInvisibleToEveryoneButItsOwner(t *testing.T) {
	// PRO-01's first criterion, attacked below the service: another user,
	// tenant context, and both together read nothing, even naming the row.
	ctx := context.Background()
	if _, err := service().SaveProfile(ctx, amaraID, candidate.Profile{
		Seniority: "senior",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	shapes := map[string]func(tx pgx.Tx) error{
		"another user": func(tx pgx.Tx) error {
			return database.SetUser(ctx, tx, priyaID)
		},
		"tenant context": func(tx pgx.Tx) error {
			return database.SetTenant(ctx, tx, tenant)
		},
		"tenant context with the owner's own id": func(tx pgx.Tx) error {
			if err := database.SetTenant(ctx, tx, tenant); err != nil {
				return err
			}
			return database.SetUser(ctx, tx, amaraID)
		},
		"no context": func(pgx.Tx) error { return nil },
	}
	for name, arrange := range shapes {
		t.Run(name, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if err := arrange(tx); err != nil {
				t.Fatalf("arranging: %v", err)
			}

			var count int
			if err := tx.QueryRow(ctx,
				"SELECT count(*) FROM candidate.profiles WHERE user_id = $1", amaraID).Scan(&count); err != nil {
				t.Fatalf("query: %v", err)
			}
			if count != 0 {
				t.Fatalf("%s read the profile", name)
			}
		})
	}
}

func TestTheTripwireRefusesAProfileWriteUnderTenantContext(t *testing.T) {
	// The owner's own write, inside a transaction that also carries tenant
	// context: WITH CHECK would pass it, the tripwire must not.
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenant); err != nil {
		t.Fatalf("SetTenant: %v", err)
	}
	if err := database.SetUser(ctx, tx, amaraID); err != nil {
		t.Fatalf("SetUser: %v", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO candidate.profiles (user_id) VALUES ($1)
		 ON CONFLICT (user_id) DO UPDATE SET seniority = 'rewritten'`, amaraID)
	if err == nil {
		t.Fatal("a profile write went through under tenant context; the tripwire is not on this table")
	}
}
