//go:build integration

// The container, the pools and the fixtures every tenant administration
// integration test shares.
//
// One PostgreSQL for the package, migrated once. The admin pool exists only
// to seed rows the application role deliberately cannot write and to attack
// the append-only triggers as the table's owner, which is the one attacker
// the REVOKE cannot stop.
package tenantadmin_test

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

	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

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
		// Not ForListeningPort, and the second occurrence rather than the
		// first: the image starts a temporary server for its initialisation
		// scripts and logs readiness for that one too.
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

// admin connects as the migrator, which owns the tables. Used to seed and to
// attack, never to exercise the code under test.
func admin(t *testing.T) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// seedTenant creates one workspace and returns its id.
func seedTenant(t *testing.T) string {
	t.Helper()
	conn := admin(t)
	tenantID := id.New().String()
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO tenancy.tenants (id, name, slug, region)
		VALUES ($1, 'Tenant Administration Test', $2, 'eu-west-2')`,
		tenantID, "tenantadmin-"+tenantID[len(tenantID)-12:]); err != nil {
		t.Fatalf("seeding tenant: %v", err)
	}
	return tenantID
}

// seedUser creates one person with a unique address.
func seedUser(t *testing.T) string {
	t.Helper()
	conn := admin(t)
	userID := id.New().String()
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO identity.users (id, email) VALUES ($1, $2)`,
		userID, userID+"@example.test"); err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return userID
}

// seedMember creates a person and their membership of a workspace, and
// returns both identifiers. Written with the migrator because the roster is
// identity's to write and this package only reads it through a port.
func seedMember(t *testing.T, tenantID, role string) (membershipID, userID string) {
	t.Helper()
	userID = seedUser(t)
	membershipID = id.New().String()
	if _, err := admin(t).Exec(context.Background(), `
		INSERT INTO tenancy.memberships (id, tenant_id, user_id, status, role)
		VALUES ($1, $2, $3, 'active', $4)`,
		membershipID, tenantID, userID, role); err != nil {
		t.Fatalf("seeding membership: %v", err)
	}
	return membershipID, userID
}
