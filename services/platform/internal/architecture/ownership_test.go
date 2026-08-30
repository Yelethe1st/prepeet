// Table ownership, PLT-04's last rule.
//
// The import rules in architecture_test.go stop a module calling another
// module's code. They say nothing about a module reading another module's
// tables, which is the same coupling with none of the compiler's help: sqlc
// generates whatever the query file asks for, and `SELECT ... FROM
// interview.sessions` inside internal/evaluation compiles perfectly.
//
// This is checkable at all because ADR-0010 puts every statement through sqlc.
// There is no hand-written SQL in internal, which is asserted here rather than
// assumed, so the query files are the complete surface: a module that reads a
// table reads it there or not at all.
//
// Implements PLT-04's fifth criterion.
package architecture_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// owns declares which schemas each module may name.
//
// A declaration rather than an inference. Deriving ownership from what the
// queries already do would make every existing crossing legal by definition and
// the check vacuous; written down, adding a schema to a module is a decision
// somebody makes on purpose, in a diff, with a reviewer.
var owns = map[string][]string{
	"billing":    {"billing"},
	"candidate":  {"candidate"},
	"content":    {"content"},
	"evaluation": {"evaluation"},
	// Tenancy is identity's. Memberships and the workspaces they point at are
	// the same subject as the people holding them, and IAM-03 makes identity
	// the one place that decides who may act under which tenant.
	"identity":     {"identity", "tenancy"},
	"interview":    {"interview"},
	"notification": {"notification"},
	"progression":  {"progression"},
	"recruiting":   {"recruiting"},
	// Operations owns no schema, and the empty list is the declaration rather
	// than an omission. It appends to the shared audit schema, which is
	// everybody's, and reaches the queue through platform/outbox's port rather
	// than by naming integration.outbox itself. A module that reads everything
	// and owns nothing is exactly the one worth stating outright, because the
	// next person to add a query here should have to think about which schema
	// it belongs to.
	"operations": {},
	// Tenant administration owns no whole schema either. Its tables live in
	// tenancy, which is identity's, so they are declared table by table in
	// ownsTables below, and the empty list here is what says it may name
	// nothing else there: not memberships, not tenants.
	"tenantadmin": {},
}

// ownsTables declares individual tables a module owns inside a schema that
// belongs to another module.
//
// It exists because the schema list in ADR-0002 is fixed and tenant
// administration is a bounded context whose tables are unarguably about the
// workspace: settings and the periodic access review both belong in tenancy,
// which identity owns because memberships and the people holding them are one
// subject. Giving a second module the whole schema would make the rule vacuous
// for both of them, so ownership is stated at the table instead.
//
// The exclusion runs both ways and is checked: a table named here is a
// crossing for every module except the one that owns it, the schema's owner
// included. That is what stops this from becoming a way to widen a claim
// rather than to narrow one.
var ownsTables = map[string][]string{
	"tenantadmin": {
		"tenancy.tenant_configuration",
		"tenancy.access_reviews",
		"tenancy.access_review_items",
	},
}

// auditSchema is everybody's, and by grant only to append to.
//
// Deliberately shared: a module that could not record what it did would be a
// module whose decisions are unreviewable. It is the one exception, it is named
// here rather than left implicit, and migration 0008 makes the table
// append-only by grant so that sharing it cannot mean editing it.
const auditSchema = "audit"

// schemaReference finds a schema-qualified name in a statement.
//
// Deliberately loose: it matches more than table references, a cast to a
// schema-qualified type among them. Matching too much is the safe direction,
// because that failure means somebody adds a schema to the declaration above,
// and the other failure means a crossing nobody sees.
var schemaReference = regexp.MustCompile(`\b([a-z][a-z_]*)\.[a-z_]+`)

// knownSchemas are the ones the migrations create. A dotted name that is not
// one of these is a table alias and its column, not a crossing.
var knownSchemas = map[string]bool{
	"audit": true, "billing": true, "candidate": true, "content": true,
	"evaluation": true, "identity": true, "integration": true, "interview": true,
	"media": true, "notification": true, "progression": true, "recruiting": true,
	"tenancy": true,
}

func TestAModuleNamesOnlyItsOwnSchemas(t *testing.T) {
	crossings := []string{}

	// Every table claimed by name, and by whom. Read once, so the loop below
	// can tell "that table is somebody else's" from "that schema is somebody
	// else's": different failures wearing the same shape.
	claimedBy := map[string]string{}
	for module, tables := range ownsTables {
		for _, table := range tables {
			if first, taken := claimedBy[table]; taken {
				t.Fatalf("%s is claimed by both internal/%s and internal/%s", table, first, module)
			}
			claimedBy[table] = module
		}
	}

	for module, allowed := range owns {
		permitted := map[string]bool{auditSchema: true}
		for _, schema := range allowed {
			permitted[schema] = true
		}

		path := filepath.Join("..", module, "db", "queries.sql")
		queries, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}

		for _, match := range schemaReference.FindAllStringSubmatch(string(queries), -1) {
			schema, table := match[1], match[0]
			if !knownSchemas[schema] {
				continue
			}
			// A table claimed by name overrides the schema claim in both
			// directions: its owner may name it wherever it lives, and the
			// schema's owner may not.
			if owner, claimed := claimedBy[table]; claimed {
				if owner != module {
					crossings = append(crossings, fmt.Sprintf(
						"internal/%s reads %s, which internal/%s owns", module, table, owner))
				}
				continue
			}
			if permitted[schema] {
				continue
			}
			crossings = append(crossings,
				fmt.Sprintf("internal/%s reads %s", module, table))
		}
	}

	sort.Strings(crossings)
	if len(crossings) > 0 {
		t.Fatalf("a module named a schema it does not own:\n  %s",
			strings.Join(crossings, "\n  "))
	}
}

// Every module with a query file is declared, or the rule above silently skips
// whichever module somebody adds next and reports nothing while doing it.
func TestEveryModuleWithQueriesDeclaresItsOwnership(t *testing.T) {
	entries, err := os.ReadDir("..")
	if err != nil {
		t.Fatalf("reading internal: %v", err)
	}

	undeclared := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join("..", entry.Name(), "db", "queries.sql")); err != nil {
			continue
		}
		if _, declared := owns[entry.Name()]; !declared {
			undeclared = append(undeclared, entry.Name())
		}
	}

	sort.Strings(undeclared)
	if len(undeclared) > 0 {
		t.Fatalf("these modules have queries and no declared ownership, so nothing checks them:\n  %s",
			strings.Join(undeclared, "\n  "))
	}
}

// The ownership rule is only complete because there is no other way to reach a
// table. If hand-written SQL appeared in a module, the check above would still
// pass and would be measuring nothing, which is exactly how a bug in this
// package's first rule survived: it read only its own package and would have
// passed forever.
func TestNoHandWrittenSQLBypassesTheOwnershipCheck(t *testing.T) {
	statement := regexp.MustCompile(`(?i)\b(select|insert\s+into|update|delete\s+from)\b`)
	offenders := []string{}

	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Generated code is sqlc's own output, and a test may build a fixture
		// however it likes. Neither is a module reaching for a table in the
		// code that runs in production.
		if strings.HasSuffix(path, ".gen.go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, literal := range stringLiterals(t, path) {
			if statement.MatchString(literal) && schemaReference.MatchString(literal) {
				offenders = append(offenders, fmt.Sprintf("%s: %s", path, truncate(literal)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}

	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Fatalf("SQL outside sqlc, which the ownership check cannot see:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}
