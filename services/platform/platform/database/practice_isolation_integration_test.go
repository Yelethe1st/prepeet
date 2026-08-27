//go:build integration

package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

// The practice/screening separation, proven at the database.
//
// The policy layer's half is proven in platform/authz, where every owner
// capability in the catalogue is tested against a deliberately
// over-provisioned tenant subject. This file is the other half IAM-06
// requires: the same attempts against row-level security itself, so that a
// handler bug which skips the policy layer entirely still reads nothing.
//
// The cast: Amara is a candidate with practice history. Northwind (tenantA)
// is a tenant she screened for, and Priya is Northwind's recruiter. Nothing
// Priya or Northwind can set up a transaction with reaches Amara's rows.

const (
	amara = "00000000-0000-7000-8000-0000000000a1"
	priya = "00000000-0000-7000-8000-0000000000b2"
)

// seedPractice creates both people and Amara's practice history, as the
// migrator, because the fixtures are the world these tests probe rather than
// the behaviour under test.
func seedPractice(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	for _, user := range []struct{ id, email string }{
		{amara, "amara.practice@example.com"},
		{priya, "priya.recruiter@example.com"},
	} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO identity.users (id, email) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
			user.id, user.email); err != nil {
			t.Fatalf("seeding user: %v", err)
		}
	}

	// Priya belongs to Northwind, and Amara screened for it: the membership is
	// exactly the relationship the separation must not leak through.
	if _, err := conn.Exec(ctx,
		`INSERT INTO tenancy.memberships (id, tenant_id, user_id, status, role)
		 VALUES ('00000000-0000-7000-8000-0000000000c3', $1, $2, 'active', 'recruiter')
		 ON CONFLICT (id) DO NOTHING`, tenantA, priya); err != nil {
		t.Fatalf("seeding membership: %v", err)
	}

	if _, err := conn.Exec(ctx,
		`INSERT INTO candidate.practice_sessions (id, user_id, status)
		 VALUES ('00000000-0000-7000-8000-0000000000d4', $1, 'completed')
		 ON CONFLICT (id) DO NOTHING`, amara); err != nil {
		t.Fatalf("seeding practice: %v", err)
	}
}

// countPractice reads how many practice rows a transaction can see.
func countPractice(t *testing.T, tx pgx.Tx) int {
	t.Helper()
	var count int
	if err := tx.QueryRow(context.Background(),
		"SELECT count(*) FROM candidate.practice_sessions").Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return count
}

func TestTheOwnerSeesTheirOwnPractice(t *testing.T) {
	// Without this, every zero below could be the policy hiding everything
	// from everyone, which is a different bug wearing the right costume.
	seedPractice(t)
	ctx := context.Background()

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, amara); err != nil {
		t.Fatalf("SetUser: %v", err)
	}

	if got := countPractice(t, tx); got != 1 {
		t.Fatalf("the owner sees %d of their own practice rows, want 1", got)
	}
}

func TestTenantAuthorityReadsNoPracticeRows(t *testing.T) {
	seedPractice(t)
	ctx := context.Background()

	// Every context shape a tenant-side code path could plausibly run with.
	// Each must see zero rows: the policy has no tenant branch to satisfy.
	shapes := map[string]func(tx pgx.Tx) error{
		"tenant context alone": func(tx pgx.Tx) error {
			return database.SetTenant(ctx, tx, tenantA)
		},
		"the recruiter's own user context": func(tx pgx.Tx) error {
			return database.SetUser(ctx, tx, priya)
		},
		"tenant and recruiter together": func(tx pgx.Tx) error {
			if err := database.SetTenant(ctx, tx, tenantA); err != nil {
				return err
			}
			return database.SetUser(ctx, tx, priya)
		},
		"tenant context with the owner's own id": func(tx pgx.Tx) error {
			// The shape that leaked from the profiles table before the
			// absence clause was demanded: the owner's identity, reached
			// through a code path that also set tenant context. WITH CHECK
			// caught the write; this is the read half.
			if err := database.SetTenant(ctx, tx, tenantA); err != nil {
				return err
			}
			return database.SetUser(ctx, tx, amara)
		},
		"no context at all": func(pgx.Tx) error { return nil },
	}

	for name, arrange := range shapes {
		t.Run(name, func(t *testing.T) {
			tx, err := appPool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if err := arrange(tx); err != nil {
				t.Fatalf("arranging: %v", err)
			}

			if got := countPractice(t, tx); got != 0 {
				t.Fatalf("%s sees %d practice rows; practice must be invisible to it", name, got)
			}
		})
	}
}

func TestNamingTheRowDirectlyDoesNotHelp(t *testing.T) {
	// A leak through a detail endpoint rather than a list: the attacker knows
	// the id. The policy applies to the row, not to the query shape.
	seedPractice(t)
	ctx := context.Background()

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("SetTenant: %v", err)
	}
	if err := database.SetUser(ctx, tx, priya); err != nil {
		t.Fatalf("SetUser: %v", err)
	}

	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM candidate.practice_sessions
		 WHERE id = '00000000-0000-7000-8000-0000000000d4'`).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Fatal("a practice row was readable by id under tenant authority")
	}
}

func TestTenantAuthorityCannotWritePracticeRows(t *testing.T) {
	seedPractice(t)
	ctx := context.Background()

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("SetTenant: %v", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO candidate.practice_sessions (id, user_id) VALUES (gen_random_uuid(), $1)`, amara)
	if err == nil {
		t.Fatal("a practice row was written under tenant authority")
	}
}

// The tripwire's own test, and the reason it exists: this is the one shape
// row-level security cannot catch.
func TestTheTripwireCatchesWhatThePolicyCannot(t *testing.T) {
	seedPractice(t)
	ctx := context.Background()

	// The owner writing their own row - WITH CHECK passes - but inside a
	// transaction that also carries tenant context. That combination means a
	// tenant-scoped code path is touching practice data, which is the bug, and
	// the exception is the stop-ship alarm going off.
	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("SetTenant: %v", err)
	}
	if err := database.SetUser(ctx, tx, amara); err != nil {
		t.Fatalf("SetUser: %v", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO candidate.practice_sessions (id, user_id) VALUES (gen_random_uuid(), $1)`, amara)
	if err == nil {
		t.Fatal("the owner's own write went through under tenant context; the tripwire is dead")
	}
	if !strings.Contains(err.Error(), "stop-ship") {
		t.Fatalf("the refusal does not announce itself as stop-ship: %v", err)
	}
}

// ─────────────────────────────────────────────── the structural guards

// Every candidate table, current and future, must carry the shape 0011
// established. These read the catalog rather than name tables, so the rules
// apply to tables that do not exist yet.

func TestEveryCandidateTableIsOwnerScopedAndTenantFree(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity,
		       EXISTS (SELECT 1 FROM pg_attribute a
		               WHERE a.attrelid = c.oid AND a.attname = 'tenant_id' AND NOT a.attisdropped) AS has_tenant,
		       coalesce((SELECT string_agg(coalesce(p.qual, '') || coalesce(p.with_check, ''), ' ')
		                 FROM pg_policies p
		                 WHERE p.schemaname = 'candidate' AND p.tablename = c.relname), '') AS policies
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'candidate' AND c.relkind = 'r'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	checked := 0
	for rows.Next() {
		var table, policies string
		var enabled, forced, hasTenant bool
		if err := rows.Scan(&table, &enabled, &forced, &hasTenant, &policies); err != nil {
			t.Fatalf("scan: %v", err)
		}
		checked++

		if !enabled || !forced {
			t.Errorf("candidate.%s does not force row-level security", table)
		}
		if hasTenant {
			// The tenant-owned table check would then demand a tenant policy,
			// and a tenant policy on practice data is the leak itself.
			t.Errorf("candidate.%s grew a tenant_id column; practice data has no tenant dimension", table)
		}
		if !strings.Contains(policies, "app.user_id") {
			t.Errorf("candidate.%s has no owner-scoped policy", table)
		}
		// Tenant context may appear only as a required absence. An equality
		// would be a tenant path into practice data; NO mention at all leaves
		// the mixed-context read open - the owner's own rows, reached through
		// a code path that also set tenant context - which is how the
		// profiles table leaked before this clause was demanded.
		if strings.Contains(policies, "app.tenant_id") &&
			!strings.Contains(policies, "IS NULL") {
			t.Errorf("candidate.%s has a policy that consults tenant context as an authority", table)
		}
		if !strings.Contains(policies, "app.tenant_id") {
			t.Errorf("candidate.%s does not require tenant-context absence, so a mixed-context transaction reads it", table)
		}
	}
	if checked == 0 {
		t.Error("no candidate tables were found, so this test proved nothing")
	}
}

// The projections rule: analytics and search carry the same separation. No
// projection exists yet, so the guard is proven against a planted offender and
// then run against the real schema, where it must find nothing.
func TestNoViewJoinsPracticeDataToTenantData(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	// The detector: views (and materialized views) whose definition references
	// both a candidate-schema table and any table carrying tenant_id.
	const detector = `
		SELECT n.nspname || '.' || c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('v', 'm')
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND EXISTS (
		      SELECT 1 FROM pg_depend d
		      JOIN pg_rewrite r ON r.oid = d.objid
		      JOIN pg_class dep ON dep.oid = d.refobjid
		      JOIN pg_namespace depn ON depn.oid = dep.relnamespace
		      WHERE r.ev_class = c.oid AND depn.nspname = 'candidate' AND dep.relkind = 'r')
		  AND EXISTS (
		      SELECT 1 FROM pg_depend d
		      JOIN pg_rewrite r ON r.oid = d.objid
		      JOIN pg_class dep ON dep.oid = d.refobjid
		      JOIN pg_attribute a ON a.attrelid = dep.oid AND a.attname = 'tenant_id' AND NOT a.attisdropped
		      WHERE r.ev_class = c.oid AND dep.relkind = 'r')`

	// First, prove the detector detects: plant exactly the projection the rule
	// forbids and expect it found. A guard first proven against nothing would
	// be a green light wired to nothing.
	if _, err := conn.Exec(ctx, `
		CREATE VIEW public.iam06_probe AS
		SELECT p.id FROM candidate.practice_sessions p
		JOIN tenancy.memberships m ON m.user_id = p.user_id`); err != nil {
		t.Fatalf("planting the probe view: %v", err)
	}
	var offenders []string
	collect := func() []string {
		found := []string{}
		rows, err := conn.Query(ctx, detector)
		if err != nil {
			t.Fatalf("detector: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatalf("scan: %v", err)
			}
			found = append(found, name)
		}
		return found
	}
	offenders = collect()
	if _, err := conn.Exec(ctx, "DROP VIEW public.iam06_probe"); err != nil {
		t.Fatalf("dropping the probe view: %v", err)
	}
	if len(offenders) != 1 || offenders[0] != "public.iam06_probe" {
		t.Fatalf("the detector did not find the planted offender: %v", offenders)
	}

	// Then the real schema, which must be clean.
	if offenders = collect(); len(offenders) != 0 {
		t.Fatalf("these projections join practice data to tenant data: %v", offenders)
	}
}
