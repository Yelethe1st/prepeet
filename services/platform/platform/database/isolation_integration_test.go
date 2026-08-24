//go:build integration

// Cross-tenant isolation tests, run against real PostgreSQL.
//
// These are the tests ADR-0002 exists for. They do not check that the
// application remembers to scope its queries; they check what happens when it
// forgets, which is the case row-level security is there to survive.
//
// docs/delivery/release-criteria.md makes a cross-tenant leak a stop-ship
// condition, so these run in CI on every change rather than as an audit.
package database_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

const postgresImage = "postgres:17-alpine"

var (
	// adminURL owns the schemas and is used only to run migrations and to set
	// up fixtures. Nothing at runtime connects with it.
	adminURL string
	// appPool connects as prepeet_app, which is the role the api and worker
	// use, and which cannot bypass row-level security.
	appPool *pgxpool.Pool

	tenantA = "00000000-0000-7000-8000-00000000000a"
	tenantB = "00000000-0000-7000-8000-00000000000b"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase("prepeet"),
		tcpostgres.WithUsername("prepeet_migrator"),
		tcpostgres.WithPassword("migrator-password"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp")),
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

	if err := database.Migrate(ctx, adminURL, database.MigrateOptions{
		AppPassword: "app-password",
	}); err != nil {
		fmt.Fprintf(os.Stderr, "migrating: %v\n", err)
		os.Exit(1)
	}

	appPool, err = pgxpool.New(ctx, appURL())
	if err != nil {
		fmt.Fprintf(os.Stderr, "app pool: %v\n", err)
		os.Exit(1)
	}

	if err := seedTenants(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "seeding: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	appPool.Close()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating PostgreSQL: %v\n", err)
	}
	os.Exit(code)
}

// appURL rewrites the admin connection string to connect as the application
// role, which is the one whose behaviour these tests care about.
func appURL() string {
	cfg, err := pgx.ParseConfig(adminURL)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("postgres://prepeet_app:app-password@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database)
}

// seedTenants inserts one row per tenant using the migrator, because the app
// role deliberately cannot write outside its own tenant context.
func seedTenants(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	for _, tenant := range []struct{ id, name, slug string }{
		{tenantA, "Northwind Health System", "northwind"},
		{tenantB, "Orbital Labs", "orbital"},
	} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO tenancy.tenants (id, name, slug, region) VALUES ($1, $2, $3, 'eu-west-2')`,
			tenant.id, tenant.name, tenant.slug); err != nil {
			return err
		}
		if _, err := conn.Exec(ctx,
			`INSERT INTO tenancy.tenant_settings (tenant_id, key, value)
			 VALUES ($1, 'retention_months', '18')`, tenant.id); err != nil {
			return err
		}
	}
	return nil
}

// withTenant runs fn inside a transaction scoped to one tenant, which is how
// every request in the product will talk to the database.
func withTenant(t *testing.T, tenantID string, fn func(pgx.Tx)) {
	t.Helper()
	ctx := context.Background()

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := database.SetTenant(ctx, tx, tenantID); err != nil {
		t.Fatalf("SetTenant: %v", err)
	}
	fn(tx)
}

// The central claim. A query that forgets to scope itself returns only the
// active tenant's rows rather than everyone's.
func TestUnscopedSelectSeesOnlyTheActiveTenant(t *testing.T) {
	ctx := context.Background()

	withTenant(t, tenantA, func(tx pgx.Tx) {
		rows, err := tx.Query(ctx, `SELECT id FROM tenancy.tenant_settings`)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			count++
		}
		if count != 1 {
			t.Errorf("unscoped select returned %d rows, want only this tenant's 1", count)
		}
	})
}

// With no tenant set, the policy compares against NULL, which is not true, so
// nothing matches. Forgetting to set the context fails closed.
func TestNoTenantContextSeesNothing(t *testing.T) {
	ctx := context.Background()

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tenancy.tenant_settings`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("count with no tenant context = %d, want 0: an unscoped read must fail closed", count)
	}
}

func TestCannotSelectAnotherTenantRowEvenWhenNamedDirectly(t *testing.T) {
	ctx := context.Background()

	withTenant(t, tenantA, func(tx pgx.Tx) {
		var count int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM tenancy.tenant_settings WHERE tenant_id = $1`, tenantB).Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("count = %d, want 0: naming another tenant explicitly must not reveal it", count)
		}
	})
}

func TestCannotInsertIntoAnotherTenant(t *testing.T) {
	ctx := context.Background()

	withTenant(t, tenantA, func(tx pgx.Tx) {
		_, err := tx.Exec(ctx,
			`INSERT INTO tenancy.tenant_settings (tenant_id, key, value) VALUES ($1, 'smuggled', 'x')`,
			tenantB)
		if err == nil {
			t.Error("insert into another tenant succeeded, want it refused by policy")
		}
	})
}

func TestCannotUpdateAnotherTenantRow(t *testing.T) {
	ctx := context.Background()

	withTenant(t, tenantA, func(tx pgx.Tx) {
		tag, err := tx.Exec(ctx,
			`UPDATE tenancy.tenant_settings SET value = 'tampered' WHERE tenant_id = $1`, tenantB)
		if err != nil {
			return // refused outright is also correct
		}
		if tag.RowsAffected() != 0 {
			t.Errorf("update affected %d rows in another tenant, want 0", tag.RowsAffected())
		}
	})
}

func TestCannotDeleteAnotherTenantRow(t *testing.T) {
	ctx := context.Background()

	withTenant(t, tenantA, func(tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `DELETE FROM tenancy.tenant_settings WHERE tenant_id = $1`, tenantB)
		if err != nil {
			return
		}
		if tag.RowsAffected() != 0 {
			t.Errorf("delete removed %d rows from another tenant, want 0", tag.RowsAffected())
		}
	})
}

// A pooled connection that kept the previous request's tenant is exactly the
// bug row-level security is meant to catch, so the mechanism must not create
// it. SET LOCAL dies with the transaction; SET would not.
func TestTenantContextDoesNotSurviveTheTransaction(t *testing.T) {
	ctx := context.Background()

	conn, err := appPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("SetTenant: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Same physical connection, new transaction, no context set.
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM tenancy.tenant_settings`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d after the transaction ended, want 0: tenant context leaked across transactions", count)
	}
}

// A role that can turn the policy off makes every other test in this file
// decorative.
func TestApplicationRoleCannotBypassRowLevelSecurity(t *testing.T) {
	ctx := context.Background()

	var isSuper, canBypass bool
	if err := appPool.QueryRow(ctx,
		`SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).
		Scan(&isSuper, &canBypass); err != nil {
		t.Fatalf("querying role: %v", err)
	}

	if isSuper {
		t.Error("the application role is a superuser, which defeats row-level security entirely")
	}
	if canBypass {
		t.Error("the application role has BYPASSRLS, which defeats row-level security entirely")
	}
}

// PostgreSQL exempts a table's owner from its own policies unless the table
// forces them. The migrator owns every table, so without FORCE the whole
// mechanism would be off for the role that runs DDL.
func TestEveryTenantOwnedTableForcesRowLevelSecurity(t *testing.T) {
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT n.nspname, c.relname, c.relrowsecurity, c.relforcerowsecurity
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant_id' AND a.attnum > 0
		WHERE c.relkind = 'r'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	checked := 0
	for rows.Next() {
		var schema, table string
		var enabled, forced bool
		if err := rows.Scan(&schema, &table, &enabled, &forced); err != nil {
			t.Fatalf("scan: %v", err)
		}
		checked++
		if !enabled {
			t.Errorf("%s.%s carries tenant_id but row-level security is not enabled", schema, table)
		}
		if !forced {
			t.Errorf("%s.%s does not force row-level security, so its owner bypasses the policy", schema, table)
		}
	}
	if checked == 0 {
		t.Error("no tenant-owned tables were found, so this test proved nothing")
	}
}

// Reapplying migrations must be safe: deployments retry, and two replicas may
// start at once.
func TestMigrationsAreIdempotent(t *testing.T) {
	ctx := context.Background()

	if err := database.Migrate(ctx, adminURL, database.MigrateOptions{AppPassword: "app-password"}); err != nil {
		t.Fatalf("re-running migrations: %v", err)
	}
}

// Two environments running different SQL under the same version number is a
// difference nobody notices until it matters.
func TestAnEditedAppliedMigrationIsRefused(t *testing.T) {
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var original string
	if err := conn.QueryRow(ctx,
		`SELECT checksum FROM public.schema_migrations ORDER BY version LIMIT 1`).Scan(&original); err != nil {
		t.Fatalf("reading checksum: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`UPDATE public.schema_migrations SET checksum = 'tampered' WHERE version = (
			SELECT min(version) FROM public.schema_migrations)`); err != nil {
		t.Fatalf("tampering: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(),
			`UPDATE public.schema_migrations SET checksum = $1 WHERE version = (
				SELECT min(version) FROM public.schema_migrations)`, original)
	})

	if err := database.Migrate(ctx, adminURL, database.MigrateOptions{AppPassword: "app-password"}); err == nil {
		t.Error("Migrate accepted a changed checksum, want it refused")
	}
}

// A person must be able to see which tenants they belong to, and that question
// cannot be answered from inside one tenant's scope. Policy 0003 adds a second
// way to see a membership row without widening the first.
func TestAUserCanReadTheirOwnMembershipsAcrossTenants(t *testing.T) {
	ctx := context.Background()

	userID := seedUserInBothTenants(t)

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// No tenant is active. This is the tenant switcher's question, asked before
	// a tenant has been chosen.
	if err := database.SetUser(ctx, tx, userID); err != nil {
		t.Fatalf("SetUser: %v", err)
	}

	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM tenancy.memberships WHERE user_id = $1`, userID).Scan(&count); err != nil {
		t.Fatalf("counting memberships: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2: a person must see every tenant they belong to", count)
	}
}

// The self-read policy is SELECT only. Discovering your own memberships must
// not become a way to create or revoke one.
func TestTheSelfReadPolicyDoesNotAllowWriting(t *testing.T) {
	ctx := context.Background()

	userID := seedUserInBothTenants(t)

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := database.SetUser(ctx, tx, userID); err != nil {
		t.Fatalf("SetUser: %v", err)
	}

	tag, err := tx.Exec(ctx,
		`UPDATE tenancy.memberships SET status = 'revoked' WHERE user_id = $1`, userID)
	if err != nil {
		return // refused outright is also correct
	}
	if tag.RowsAffected() != 0 {
		t.Errorf("the self-read policy allowed updating %d membership rows", tag.RowsAffected())
	}
}

// A user must not see somebody else's memberships just because no tenant is set.
func TestTheSelfReadPolicyShowsOnlyYourOwnMemberships(t *testing.T) {
	ctx := context.Background()

	mine := seedUserInBothTenants(t)
	theirs := seedUserInBothTenants(t)

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := database.SetUser(ctx, tx, mine); err != nil {
		t.Fatalf("SetUser: %v", err)
	}

	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM tenancy.memberships WHERE user_id = $1`, theirs).Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0: one person must not see another's memberships", count)
	}
}

// seedUserInBothTenants creates a user belonging to tenant A and tenant B.
func seedUserInBothTenants(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	// The address is unique per call rather than per test: one test seeds two
	// users, and deriving the address from the test name alone collided.
	var userID string
	if err := conn.QueryRow(ctx,
		`INSERT INTO identity.users (id, email)
		 VALUES (gen_random_uuid(), $1 || gen_random_uuid()::text || '@example.com')
		 RETURNING id::text`,
		strings.ToLower(t.Name())+"-").Scan(&userID); err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	for _, tenant := range []string{tenantA, tenantB} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO tenancy.memberships (id, tenant_id, user_id) VALUES (gen_random_uuid(), $1, $2)`,
			tenant, userID); err != nil {
			t.Fatalf("seeding membership: %v", err)
		}
	}
	return userID
}
