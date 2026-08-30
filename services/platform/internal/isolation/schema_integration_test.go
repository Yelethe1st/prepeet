//go:build integration

// The live half of the structural gate.
//
// registry_test.go reads the migrations. This reads the database they built,
// and the two must agree exactly. The static scan is a parser, and a parser
// that stops recognising a statement reports a clean bill of health forever:
// that is not a hypothetical failure mode, it is how migration 0005's
// unqualified CREATE UNLOGGED TABLE went unseen until these two were compared.
//
// So the rule here is set equality rather than containment, and the authority
// for what a policy actually says is pg_policies rather than the file it came
// from.
package isolation_test

import (
	"context"
	"sort"
	"strings"
	"testing"
)

// liveTable is what PostgreSQL says about one table's isolation.
type liveTable struct {
	rowSecurity  bool
	forced       bool
	tenantColumn bool
	// policies holds each policy's USING and WITH CHECK expressions as
	// PostgreSQL rewrote them, which is what is actually enforced. A policy
	// text that differs from the migration it came from is still the truth.
	policies []string
}

// liveSchema reads every ordinary table outside the system schemas.
func liveSchema(t *testing.T) map[string]liveTable {
	t.Helper()
	ctx := context.Background()

	tables := map[string]liveTable{}
	rows, err := adminPool.Query(ctx, `
		SELECT n.nspname || '.' || c.relname,
		       c.relrowsecurity,
		       c.relforcerowsecurity,
		       EXISTS (SELECT 1 FROM pg_attribute a
		               WHERE a.attrelid = c.oid AND a.attname = 'tenant_id' AND a.attnum > 0)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_toast%'`)
	if err != nil {
		t.Fatalf("reading the live schema: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var table liveTable
		if err := rows.Scan(&name, &table.rowSecurity, &table.forced, &table.tenantColumn); err != nil {
			t.Fatalf("scanning a table: %v", err)
		}
		tables[name] = table
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading tables: %v", err)
	}

	policies, err := adminPool.Query(ctx, `
		SELECT schemaname || '.' || tablename, coalesce(qual, '') || ' ' || coalesce(with_check, '')
		FROM pg_policies`)
	if err != nil {
		t.Fatalf("reading the live policies: %v", err)
	}
	defer policies.Close()

	for policies.Next() {
		var name, expression string
		if err := policies.Scan(&name, &expression); err != nil {
			t.Fatalf("scanning a policy: %v", err)
		}
		table := tables[name]
		table.policies = append(table.policies, expression)
		tables[name] = table
	}
	if err := policies.Err(); err != nil {
		t.Fatalf("reading policies: %v", err)
	}

	if len(tables) == 0 {
		t.Fatal("the live database has no tables, so this test proved nothing")
	}
	return tables
}

// The cross-check that keeps the static gate honest. A table the migrations
// created and this scanner did not see is a table the gate would never have
// judged.
func TestTheMigrationScanSeesEveryTableTheDatabaseHas(t *testing.T) {
	live := liveSchema(t)
	scanned := scanMigrations(t)

	var unseen, phantom []string
	for name := range live {
		if _, found := scanned[name]; found {
			continue
		}
		if _, expected := createdOutsideMigrations[name]; expected {
			continue
		}
		unseen = append(unseen, name)
	}
	for name := range scanned {
		if _, found := live[name]; !found {
			phantom = append(phantom, name)
		}
	}
	sort.Strings(unseen)
	sort.Strings(phantom)

	if len(unseen) > 0 {
		t.Errorf("the database has tables the migration scan did not find: %s\n"+
			"    Every rule in registry_test.go is silent about these, which is worse than a\n"+
			"    failing rule. Teach the scanner the statement that created them.",
			strings.Join(unseen, ", "))
	}
	if len(phantom) > 0 {
		t.Errorf("the migration scan found tables the database does not have: %s\n"+
			"    The scanner is matching something that is not a table.",
			strings.Join(phantom, ", "))
	}
}

// The same three-state rule as the static gate, decided on what PostgreSQL is
// actually enforcing rather than on what the migration file appeared to say.
func TestTheLiveDatabaseEnforcesWhatTheDeclarationsClaim(t *testing.T) {
	live := liveSchema(t)
	keyed := liveKeyedTables(live)

	for name, table := range live {
		declaredUnprotected, isUnprotected := unprotected[name]
		_, isUnconditional := unconditional[name]

		if !table.rowSecurity {
			if !isUnprotected {
				t.Errorf("%s has no row-level security in the live database and is not declared.",
					name)
				continue
			}
			// The declaration is only true while the table is really
			// undefended; if somebody protects it, the entry must go, or it
			// will excuse the protection being removed again later.
			if declaredUnprotected.tenantScoped && !table.tenantColumn {
				t.Errorf("%s is declared tenantScoped and has no tenant_id column", name)
			}
			continue
		}

		if isUnprotected {
			t.Errorf("%s is declared unprotected and the live database protects it. "+
				"Remove the entry.", name)
		}
		if !table.forced {
			t.Errorf("%s does not force row-level security, so the migrator that owns it "+
				"bypasses every policy on it.", name)
		}
		if len(table.policies) == 0 {
			t.Errorf("%s has row-level security and no policy, so it denies every read to "+
				"everyone.", name)
			continue
		}
		if !keyed[name] && !isUnconditional {
			t.Errorf("%s has row-level security and no policy that decides anything about who "+
				"is asking:\n    %s", name, strings.Join(table.policies, "\n    "))
		}
		if keyed[name] && isUnconditional {
			t.Errorf("%s is declared as admitting every caller and its live policies are keyed "+
				"to the caller. Remove the entry.", name)
		}
	}
}

// The strongest statement this gate can make, and the one the ticket is about:
// a table that holds one tenant's rows is defended by the database, whatever
// any declaration says. Only integration.outbox is allowed to answer for
// itself, and only because its entry says so in the open.
func TestEveryTableWithATenantColumnIsKeyedToTheTenant(t *testing.T) {
	live := liveSchema(t)
	keyed := liveKeyedTables(live)

	checked := 0
	for name, table := range live {
		if !table.tenantColumn {
			continue
		}
		if declared, is := unprotected[name]; is && declared.tenantScoped {
			t.Logf("%s carries a tenant_id and is deliberately undefended: %s", name, declared.reason)
			continue
		}
		checked++
		if !table.rowSecurity || !table.forced {
			t.Errorf("%s carries a tenant_id and does not enforce row-level security", name)
		}
		if !keyed[name] {
			t.Errorf("%s carries a tenant_id and no policy of its own decides who is asking:\n"+
				"    %s", name, strings.Join(table.policies, "\n    "))
		}
	}
	if checked == 0 {
		t.Fatal("no tenant-owned tables were checked, so this test proved nothing")
	}
}

// liveKeyedTables is keyedByCaller over what PostgreSQL is actually enforcing.
//
// pg_policies gives the expressions as the planner rewrote them, which is the
// text that matters: a policy that says one thing in the migration and another
// in the catalogue is the catalogue's version at runtime.
func liveKeyedTables(live map[string]liveTable) map[string]bool {
	policies := map[string][]string{}
	for name, table := range live {
		policies[name] = table.policies
	}
	return keyedByCaller(policies)
}
