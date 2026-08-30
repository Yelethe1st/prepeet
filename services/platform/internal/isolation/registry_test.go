// Package isolation_test is SEC-02: the adversarial tenant-isolation suite.
//
// It holds two things that belong together and are usually kept apart.
//
// The first is the structural gate in this file. Every table the migrations
// create is in one of three states, and each state has to be arrived at on
// purpose: defended by row-level security keyed to the caller, defended by a
// policy that deliberately admits everyone, or not defended at all. The last
// two have to be declared here with a reason. A tenant-scoped table added
// without row-level security breaks the build, and it breaks it without
// Docker, so the failure arrives in the red-green loop rather than four
// minutes into CI.
//
// The second is the attack suite in the integration-tagged files beside it:
// deliberate attempts to cross a tenant boundary through the HTTP handler,
// through the bounded context, and through the database, each aimed at a row
// that is known to exist under the other tenant, and each paired with the same
// operation performed legitimately, so that a refusal cannot be the attack
// having missed.
//
// Why a package of its own rather than a file in each context: an isolation
// claim is about the whole request path, and a suite that can see one context
// can test one layer of it. This package contains no production code and never
// will, which is the condition internal/architecture's boundary rule permits
// its imports on.
//
// What this suite does NOT cover is written in the ticket note. A security
// gate that overstates its scope is worse than none.
package isolation_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// migrationsDir holds every migration, which is the only place a table may be
// created. Read from disk rather than from a list, so a migration that is
// added is a migration this gate sees.
const migrationsDir = "../../platform/database/sql"

// unprotected names every table that carries no row-level security at all,
// with the reason it needs none.
//
// This map and unconditional below are the audit surface for tenant isolation
// at rest: to know what the database does not defend, read these two and
// nothing else. That only holds while every entry is a decision somebody made
// rather than a build failure somebody silenced, so each reason says what makes
// the table safe rather than that protecting it was inconvenient.
//
// The two shapes that recur:
//
//   - Identity is global by design. A person is not owned by a tenant: the same
//     candidate practises alone and screens for several employers, and sign-in
//     happens before any tenant is chosen. There is no tenant to scope these
//     rows to, so they are guarded by their query predicates and by the policy
//     layer instead. See migration 0002's header.
//   - A queue drained by a process that acts for no tenant. A tenant policy
//     would make the drainer see nothing, and the usual fix is a role that
//     bypasses row-level security, which would bypass every other policy too.
//
// exemption is one declared entry: why a table is not defended, and whether it
// is the one shape that has to be said out loud.
type exemption struct {
	reason string
	// tenantScoped admits that the table does carry a tenant_id and is still
	// not defended by row-level security. Every other entry is a table that
	// belongs to no tenant, where the absence needs no argument. This one does,
	// so the declaration has to make it in the open rather than by omission,
	// and the gate refuses an undeclared one.
	tenantScoped bool
}

var unprotected = map[string]exemption{
	"identity.users": {reason: "A person is not owned by a tenant, and sign-in happens " +
		"before one is chosen. There is no tenant to scope this row to. See migration 0001."},
	"identity.credentials": {reason: "A password belongs to the person, not to a workspace, " +
		"and is read while no tenant context can exist yet. See migration 0002."},
	"identity.sessions": {reason: "A session is created before a tenant is selected and " +
		"switches between workspaces during its life, so it cannot be scoped to one. Its own " +
		"high-entropy token is what guards it. See migration 0002."},
	"identity.action_tokens": {reason: "Verification, recovery, magic-link and one-time-code " +
		"tokens are minted for a person before any workspace is chosen, and are stored only " +
		"as hashes. See migration 0010."},
	"identity.elevations": {reason: "A support elevation is granted to a platform operator, " +
		"who belongs to no tenant by definition; scoping it to one would defeat the grant it " +
		"records. Reads made under it are audited instead. See migration 0015."},
	"identity.oauth_states": {reason: "The row exists before anybody is known: minted when a " +
		"person starts a provider sign-in and consumed when they come back, with no session " +
		"and no identity in between to scope it by. See migration 0039."},
	"identity.oauth_identities": {reason: "The link between a provider account and a person, " +
		"which is a property of the person and outlives any workspace they belong to. See " +
		"migration 0039."},
	"integration.outbox": {tenantScoped: true, reason: "Carries a tenant_id and is still not " +
		"defended, which is the one entry here that needs the argument made. It is written " +
		"inside a tenant-scoped transaction and read only by the dispatcher, which acts for " +
		"no tenant and must see every row. A tenant policy would make the dispatcher see " +
		"nothing, and the usual fix for that is a role holding BYPASSRLS, which would bypass " +
		"every other policy in the database too. Payloads carry no restricted content. See " +
		"migration 0004."},
	"notification.emails": {reason: "Enqueued inside the transaction that wants the email " +
		"sent, and drained by a worker that acts for no tenant. Keyed by recipient address, " +
		"which is not a tenant-owned identifier. See migration 0009."},
	"interview.timing_policies": {reason: "Platform-wide reference data with a SELECT grant " +
		"and no write grant at all: rows arrive by migration and are immutable by trigger. " +
		"See migration 0032."},
	"public.security_rate_limit_counters": {reason: "Counts attempts by address and by " +
		"network. Carries no tenant_id deliberately: an attacker belongs to no tenant, and " +
		"counting per tenant would let one tenant's traffic exhaust another's allowance. See " +
		"migration 0005."},
	"public.schema_migrations": {reason: "The migration ledger. Which migrations have run is " +
		"a property of the database rather than of any tenant."},
}

// unconditional names every table whose row-level security is on and whose
// policies deliberately admit everyone.
//
// A separate list from unprotected because it is a different decision, and
// running the two together is how a gate stops distinguishing "this belongs to
// nobody" from "somebody forgot". An unconditional policy is a real choice
// here: under FORCE, a table with no policy at all hides every row, so a
// reference table that everyone may read has to say so out loud.
//
// A table carrying a tenant_id column can never appear in either list. That is
// enforced below, and it is what stops this from becoming the place a
// tenant-scoped table goes to have its missing isolation excused.
var unconditional = map[string]string{
	"recruiting.jurisdiction_determination": "One jurisdiction's legal determination " +
		"rather than any tenant's data: every tenant operating there is bound by the same " +
		"row, and a campaign pins the version it opened under. Readable by all, writable " +
		"by none through the application role, and immutable by trigger. See migration 0043.",
}

// createdOutsideMigrations names the tables that exist in a live database but
// appear in no migration, so the live schema and this scan can be compared for
// equality rather than for containment.
//
// Equality is the point. Containment would still pass if the scanner below
// quietly stopped recognising a CREATE TABLE, which is the one failure this
// file cannot detect on its own.
var createdOutsideMigrations = map[string]string{
	"public.schema_migrations": "Created by the migration runner itself, because it has " +
		"to exist before the first migration can be recorded as applied.",
}

// The caps on the two lists above.
//
// Not arithmetic: a forcing function. A list that grows one justified entry at
// a time becomes a list of everything. Raising a cap is a deliberate edit with
// a reason in the commit message, which is the review this rule actually needs.
const (
	maxUnprotected   = 12
	maxUnconditional = 1
)

// tableFacts is what the migrations say about one table's isolation.
type tableFacts struct {
	// createdIn is the migration that created it, carried for the failure
	// message: whoever adds a table needs to be told which file to fix.
	createdIn string
	enabled   bool
	forced    bool
	// policies holds each CREATE POLICY statement in full, because whether a
	// policy isolates anything is a property of its predicate rather than of
	// its existence.
	policies []string
	// tenantColumn says the table declares a tenant_id, which makes it
	// tenant-scoped beyond argument.
	tenantColumn bool
}

// Statement forms. Matched against whole statements rather than against lines,
// because a CREATE POLICY in these migrations often puts its ON clause on the
// following line, and a line-anchored pattern silently found no policy at all
// on the tables that most needed reading.
var (
	createTable = regexp.MustCompile(
		`(?is)^CREATE\s+(?:UNLOGGED\s+|TEMPORARY\s+|TEMP\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*(?:\.[a-z_][a-z0-9_]*)?)`)
	alterRowSecurity = regexp.MustCompile(
		`(?is)^ALTER\s+TABLE\s+([a-z_][a-z0-9_.]*)\s+(ENABLE|FORCE)\s+ROW\s+LEVEL\s+SECURITY`)
	createPolicy = regexp.MustCompile(
		`(?is)^CREATE\s+POLICY\s+[a-z_][a-z0-9_]*\s+ON\s+([a-z_][a-z0-9_.]*)`)
	tenantColumn = regexp.MustCompile(`(?im)^\s*tenant_id\s`)
)

// settingsThatIdentifyTheCaller are the two request-scoped settings every
// isolating policy in this schema is written against. A policy that mentions
// neither cannot be deciding anything about who is asking.
var settingsThatIdentifyTheCaller = []string{"app.tenant_id", "app.user_id"}

// qualify gives an unqualified table name the schema PostgreSQL would give it.
func qualify(name string) string {
	if strings.Contains(name, ".") {
		return name
	}
	return "public." + name
}

// statements splits a migration into SQL statements with its comments removed.
//
// Comments go first so that a sentence describing a statement is not mistaken
// for one: these migrations explain themselves at length, and several of those
// explanations name the statements they are about.
//
// Splitting on the semicolon also splits the body of a dollar-quoted function,
// which is harmless here only because no fragment of one begins with CREATE
// TABLE, ALTER TABLE or CREATE POLICY. The live cross-check in
// schema_integration_test.go is what makes that assumption safe to hold: if
// this parser ever stops seeing a table, the two disagree and the build fails.
func statements(sql string) []string {
	lines := strings.Split(sql, "\n")
	for i, line := range lines {
		if at := strings.Index(line, "--"); at >= 0 {
			lines[i] = line[:at]
		}
	}

	var out []string
	for _, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
		if trimmed := strings.TrimSpace(statement); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// scanMigrations reads every migration and reports what they say about each
// table's isolation.
func scanMigrations(t *testing.T) map[string]tableFacts {
	t.Helper()

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("reading %s: %v", migrationsDir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	// In order, so that a later migration protecting an earlier table is
	// applied after the migration that created it.
	sort.Strings(names)

	facts := map[string]tableFacts{}
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}

		for _, statement := range statements(string(raw)) {
			switch {
			case createTable.MatchString(statement):
				table := qualify(createTable.FindStringSubmatch(statement)[1])
				if existing, seen := facts[table]; seen {
					t.Errorf("%s is created twice, in %s and in %s",
						table, existing.createdIn, name)
					continue
				}
				facts[table] = tableFacts{
					createdIn:    name,
					tenantColumn: tenantColumn.MatchString(statement),
				}

			case alterRowSecurity.MatchString(statement):
				match := alterRowSecurity.FindStringSubmatch(statement)
				known, ok := facts[qualify(match[1])]
				if !ok {
					continue
				}
				if strings.EqualFold(match[2], "ENABLE") {
					known.enabled = true
				} else {
					known.forced = true
				}
				facts[qualify(match[1])] = known

			case createPolicy.MatchString(statement):
				table := qualify(createPolicy.FindStringSubmatch(statement)[1])
				if known, ok := facts[table]; ok {
					known.policies = append(known.policies, statement)
					facts[table] = known
				}
			}
		}
	}

	if len(facts) == 0 {
		t.Fatal("no tables were found in the migrations, so this gate proved nothing")
	}
	return facts
}

// keyedTables reports which tables are actually scoped to the caller.
//
// The rule is every policy, not any policy. PostgreSQL ORs permissive policies
// together, so a table with one policy naming app.tenant_id and one saying
// USING (true) is a table every tenant can read in full. An "at least one"
// rule passes it, and that is not hypothetical: this rule was written that way
// first, and the mistake surfaced only when the memberships tenant policy was
// deliberately replaced with USING (true) to watch the database-layer attacks
// fail. They failed. The gate stayed green, because two other policies on that
// table still named app.user_id.
//
// A policy decides who is asking either by naming one of the request's own
// settings, or by delegating: admitting a row only where a row of an
// already-keyed table exists, which inherits that table's authority and is how
// interview.session_bundles is scoped without carrying a tenant_id. Delegation
// is followed to a fixpoint, because the table delegated to may delegate in
// turn.
//
// The alternative was to demand app.tenant_id in every policy. It would have
// been simpler and wrong: it would have failed correct tables and taught
// whoever hit it to write the predicate the gate wanted rather than the one
// the data needed.
func keyedTables(facts map[string]tableFacts) map[string]bool {
	policies := map[string][]string{}
	for table, known := range facts {
		policies[table] = known.policies
	}
	return keyedByCaller(policies)
}

// keyedByCaller is the rule itself, over each table's policy expressions.
//
// Shared by the migration scan and by the live check against pg_policies, so
// that the two cannot come to disagree about what counts as isolated.
func keyedByCaller(policies map[string][]string) map[string]bool {
	names := make([]string, 0, len(policies))
	for table := range policies {
		names = append(names, table)
	}

	decides := func(policy string, keyed map[string]bool, self string) bool {
		for _, setting := range settingsThatIdentifyTheCaller {
			if strings.Contains(policy, setting) {
				return true
			}
		}
		for _, other := range names {
			// A self-reference proves nothing: every policy names its own
			// table in its ON clause.
			if other != self && keyed[other] && strings.Contains(policy, other) {
				return true
			}
		}
		return false
	}

	keyed := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for table, own := range policies {
			if keyed[table] || len(own) == 0 {
				continue
			}
			all := true
			for _, policy := range own {
				if !decides(policy, keyed, table) {
					all = false
				}
			}
			if all {
				keyed[table] = true
				changed = true
			}
		}
	}
	return keyed
}

// The structural half of SEC-02, and the reason this file needs no database: a
// tenant-scoped table added without row-level security fails here, by name.
//
// Three properties, not one. ENABLE turns the mechanism on. FORCE makes it
// apply to the table's owner, which is the migrator, and without it every
// policy on the table is decorative for the role that runs DDL. A policy is
// what ENABLE enforces: a table with row-level security on and no policy denies
// everything, which fails closed but fails as an outage rather than as
// isolation, and is never what the author meant.
func TestEveryTableIsIsolatedOrDeclaredNotToBe(t *testing.T) {
	t.Parallel()

	facts := scanMigrations(t)
	keyed := keyedTables(facts)

	for table, known := range facts {
		_, noSecurity := unprotected[table]
		_, admitsEveryone := unconditional[table]

		if !known.enabled {
			if noSecurity {
				continue
			}
			t.Errorf("%s (created in %s) has no row-level security.\n"+
				"    Add ENABLE and FORCE ROW LEVEL SECURITY and a policy keyed to the active\n"+
				"    tenant, as migration 0043's campaign tables do. If the table belongs to no\n"+
				"    tenant, add it to unprotected in this file with the reason: a decision on\n"+
				"    the record rather than a line somebody deleted.",
				table, known.createdIn)
			continue
		}

		if !known.forced {
			t.Errorf("%s (created in %s) does not FORCE row-level security, so the migrator "+
				"that owns it bypasses every policy on it.", table, known.createdIn)
		}
		if len(known.policies) == 0 {
			t.Errorf("%s (created in %s) enables row-level security and defines no policy, so "+
				"it denies every read to everyone: an outage rather than isolation.",
				table, known.createdIn)
			continue
		}
		if !keyed[table] && !admitsEveryone {
			t.Errorf("%s (created in %s) has a policy that decides nothing about who is "+
				"asking.\n"+
				"    Every policy on a table has to be keyed, not just one of them: PostgreSQL\n"+
				"    ORs permissive policies, so one that admits everyone re-opens the table\n"+
				"    however well the others are written. A policy is keyed when it names\n"+
				"    app.tenant_id or app.user_id, or when it delegates to a table that is.\n"+
				"    Key it, or - if the table really does hold one thing every tenant shares -\n"+
				"    declare it in unconditional with the reason.\n    policies:\n      %s",
				table, known.createdIn, strings.Join(known.policies, "\n      "))
		}
	}
}

// The rule that stops either declaration becoming somewhere to put a table
// whose isolation is missing.
//
// A tenant_id column is not evidence that a table is tenant-scoped; it is the
// thing itself. Declaring one away is therefore a claim that has to be made in
// the open: unprotected admits it only through the tenantScoped field, which
// exists so that the one table in this schema which needs that argument -
// integration.outbox - is a line somebody wrote rather than a line somebody
// left out. unconditional admits it not at all, because no table has ever
// needed it and the day one does, the argument belongs in review rather than
// in a boolean.
func TestATableCarryingTenantIDCannotBeDeclaredAwayQuietly(t *testing.T) {
	t.Parallel()

	facts := scanMigrations(t)

	for table, known := range facts {
		if !known.tenantColumn {
			continue
		}
		if declared, is := unprotected[table]; is && !declared.tenantScoped {
			t.Errorf("%s carries a tenant_id and is declared unprotected without saying so.\n"+
				"    Defend it, or set tenantScoped on the entry and argue for it there. A\n"+
				"    table holding one tenant's rows is not exempt by having a reason typed\n"+
				"    beside it.\n    The declared reason was: %s", table, declared.reason)
		}
		if reason, is := unconditional[table]; is {
			t.Errorf("%s carries a tenant_id and is declared unconditional, so its policy "+
				"admits every tenant to every row.\n    The declared reason was: %s", table, reason)
		}
	}

	// And the claim in the other direction: an entry that says it is
	// tenant-scoped, on a table that carries no tenant_id, is describing
	// something that is no longer there.
	for table, declared := range unprotected {
		if !declared.tenantScoped {
			continue
		}
		if known, exists := facts[table]; exists && !known.tenantColumn {
			t.Errorf("%s is declared tenantScoped and carries no tenant_id column. Drop the "+
				"field: the entry is describing a table that has changed under it.", table)
		}
	}
}

// A declaration naming a table that no longer exists is a claim nobody can
// check, and it hides the fact that the list has stopped being read.
func TestTheDeclarationsHaveNoStaleEntries(t *testing.T) {
	t.Parallel()

	facts := scanMigrations(t)
	missing := func(table string) bool {
		if _, inMigrations := facts[table]; inMigrations {
			return false
		}
		_, elsewhere := createdOutsideMigrations[table]
		return !elsewhere
	}

	for table, declared := range unprotected {
		if missing(table) {
			t.Errorf("unprotected names %s, which no migration creates.\n    Its reason was: %s",
				table, declared.reason)
		}
	}
	for table, reason := range unconditional {
		if missing(table) {
			t.Errorf("unconditional names %s, which no migration creates.\n    Its reason was: %s",
				table, reason)
		}
	}

	if len(unprotected) > maxUnprotected {
		t.Errorf("%d tables carry no row-level security and the cap is %d.\n"+
			"    Raising the cap is allowed. It is meant to be a visible decision.",
			len(unprotected), maxUnprotected)
	}
	if len(unconditional) > maxUnconditional {
		t.Errorf("%d tables admit every caller and the cap is %d.\n"+
			"    Raising the cap is allowed. It is meant to be a visible decision.",
			len(unconditional), maxUnconditional)
	}
}

// A table declared unprotected that has row-level security anyway means the
// declaration is describing a database that no longer exists. Left alone, the
// entry would go on excusing a table that had since been protected, and would
// excuse it again the day somebody removed the protection.
func TestNoDeclarationExcusesATableThatDefendsItself(t *testing.T) {
	t.Parallel()

	facts := scanMigrations(t)
	keyed := keyedTables(facts)

	for table, known := range facts {
		if _, declared := unprotected[table]; declared && known.enabled {
			t.Errorf("%s is declared unprotected, but %s enables row-level security on it. "+
				"Remove the entry.", table, known.createdIn)
		}
		if _, declared := unconditional[table]; declared && keyed[table] {
			t.Errorf("%s is declared as admitting every caller, but its policies are keyed to "+
				"the caller. Remove the entry.", table)
		}
	}
}
