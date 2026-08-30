//go:build integration

package tenantadmin_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin"
)

// TEN-01 against real PostgreSQL. The three boxes this file can close are
// the two structural ones: a change is audited with what it was before, and
// a version pinned earlier keeps its own document after a later change.

// configured saves one valid document and returns the store and the tenant.
func configured(t *testing.T) (*tenantadmin.SettingsStore, string, string) {
	t.Helper()
	store := tenantadmin.NewSettingsStore(pool)
	tenantID := seedTenant(t)
	actorID := seedUser(t)

	settings := tenantadmin.DefaultSettings()
	settings.Organisation.LegalName = "Northwind Health System Ltd"
	settings.Organisation.DisplayName = "Northwind Health"
	if _, err := store.Save(context.Background(), tenantID, actorID, settings, 0); err != nil {
		t.Fatalf("first save: %v", err)
	}
	return store, tenantID, actorID
}

// A settings screen has to render for a workspace that has never saved
// anything, so absence answers with the default document rather than an error.
func TestAnUnconfiguredWorkspaceReadsTheDefaultDocument(t *testing.T) {
	store := tenantadmin.NewSettingsStore(pool)
	tenantID := seedTenant(t)

	current, err := store.Current(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if current.Version != 0 {
		t.Errorf("Version = %d, want 0 for a workspace that has saved nothing", current.Version)
	}
	if current.Settings != tenantadmin.DefaultSettings() {
		t.Error("an unconfigured workspace read something other than the default document")
	}
}

func TestTheFirstSaveIsVersionOneAndReadsBack(t *testing.T) {
	store, tenantID, _ := configured(t)

	current, err := store.Current(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if current.Version != 1 {
		t.Errorf("Version = %d, want 1", current.Version)
	}
	if current.Settings.Organisation.DisplayName != "Northwind Health" {
		t.Errorf("DisplayName = %q, want what was saved", current.Settings.Organisation.DisplayName)
	}
}

// Two administrators on the settings screen at once. The second must be
// refused rather than overwriting a change they never saw.
func TestASaveAgainstAStaleVersionIsRefusedAndChangesNothing(t *testing.T) {
	ctx := context.Background()
	store, tenantID, actorID := configured(t)

	current, err := store.Current(ctx, tenantID)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	first := current.Settings
	first.Organisation.Industry = "healthcare"
	if _, err := store.Save(ctx, tenantID, actorID, first, 1); err != nil {
		t.Fatalf("the winning save: %v", err)
	}

	second := current.Settings
	second.Organisation.Industry = "logistics"
	if _, err := store.Save(ctx, tenantID, actorID, second, 1); !errors.Is(err, tenantadmin.ErrSettingsStale) {
		t.Fatalf("the losing save = %v, want ErrSettingsStale", err)
	}

	after, err := store.Current(ctx, tenantID)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if after.Version != 2 {
		t.Errorf("Version = %d, want 2: the refused save must not have written a row", after.Version)
	}
	if after.Settings.Organisation.Industry != "healthcare" {
		t.Errorf("Industry = %q, want the winning save's value", after.Settings.Organisation.Industry)
	}
}

// Validation is the domain's, and it runs before storage: an invalid document
// must not reach the table, because the table has no way to remove it.
func TestAnInvalidDocumentNeverReachesStorage(t *testing.T) {
	ctx := context.Background()
	store, tenantID, actorID := configured(t)

	broken := tenantadmin.DefaultSettings()
	broken.Organisation.LegalName = "Northwind Health System Ltd"
	broken.Organisation.DisplayName = ""
	if _, err := store.Save(ctx, tenantID, actorID, broken, 1); !errors.Is(err, tenantadmin.ErrSettingsInvalid) {
		t.Fatalf("Save = %v, want ErrSettingsInvalid", err)
	}

	current, err := store.Current(ctx, tenantID)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if current.Version != 1 {
		t.Errorf("Version = %d, want 1: a refused document must write nothing", current.Version)
	}
}

// The first box: audited with actor and previous value. The audit row carries
// the actor and the fields that moved; the previous value itself is the
// previous version, which is still there and still says what it said.
func TestASaveIsAuditedWithTheActorAndTheFieldsThatMoved(t *testing.T) {
	ctx := context.Background()
	store, tenantID, actorID := configured(t)

	current, err := store.Current(ctx, tenantID)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	before := current.Settings
	next := before
	next.Organisation.DisplayName = "Northwind"
	next.Defaults.DeadlineDays = 14
	if _, err := store.Save(ctx, tenantID, actorID, next, 1); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var auditActor, detail string
	if err := admin(t).QueryRow(ctx, `
		SELECT actor_id::text, detail::text FROM audit.events
		WHERE tenant_id = $1 AND action = 'tenant.settings_changed'
		ORDER BY occurred_at DESC LIMIT 1`, tenantID).Scan(&auditActor, &detail); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if auditActor != actorID {
		t.Errorf("audit actor = %s, want the administrator who saved", auditActor)
	}
	for _, field := range []string{"organisation.display_name", "defaults.deadline_days"} {
		if !strings.Contains(detail, field) {
			t.Errorf("the audit detail does not name %q as changed: %s", field, detail)
		}
	}

	// And the previous value is still readable, exactly.
	previous, err := store.AtVersion(ctx, tenantID, 1)
	if err != nil {
		t.Fatalf("AtVersion(1): %v", err)
	}
	if previous.Settings != before {
		t.Error("version 1 no longer reads as what it was before version 2 was saved")
	}
}

// The second box, as a property rather than as a promise. A campaign pins the
// settings version it was created under; a later change writes a new row and
// leaves that one alone, so the pin re-reads what it always read.
func TestAPinnedVersionKeepsItsDefaultsAfterALaterChange(t *testing.T) {
	ctx := context.Background()
	store, tenantID, actorID := configured(t)

	pinned, err := store.Current(ctx, tenantID)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	wasDeadline := pinned.Settings.Defaults.DeadlineDays

	next := pinned.Settings
	next.Defaults.DeadlineDays = wasDeadline + 21
	next.Defaults.ReminderDays = 2
	if _, err := store.Save(ctx, tenantID, actorID, next, pinned.Version); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reread, err := store.AtVersion(ctx, tenantID, pinned.Version)
	if err != nil {
		t.Fatalf("AtVersion: %v", err)
	}
	if reread.Settings.Defaults.DeadlineDays != wasDeadline {
		t.Errorf("the pinned version's deadline is now %d, want the %d it was created under",
			reread.Settings.Defaults.DeadlineDays, wasDeadline)
	}
	if reread.Settings != pinned.Settings {
		t.Error("the pinned version's document changed under a later save")
	}
}

func TestHistoryShowsEveryVersionNewestFirst(t *testing.T) {
	ctx := context.Background()
	store, tenantID, actorID := configured(t)

	current, _ := store.Current(ctx, tenantID)
	next := current.Settings
	next.Organisation.Industry = "healthcare"
	if _, err := store.Save(ctx, tenantID, actorID, next, current.Version); err != nil {
		t.Fatalf("Save: %v", err)
	}

	history, err := store.History(ctx, tenantID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("History returned %d versions, want 2", len(history))
	}
	if history[0].Version != 2 || history[1].Version != 1 {
		t.Errorf("History versions = %d, %d; want 2 then 1", history[0].Version, history[1].Version)
	}
	if history[0].ChangedBy != actorID {
		t.Errorf("ChangedBy = %s, want the administrator who saved", history[0].ChangedBy)
	}
}

// The attack that means something: a row that genuinely exists under another
// workspace, named directly. An unscoped attack matches nothing whether the
// policy works or not.
func TestOneWorkspaceCannotReadAnothersSettings(t *testing.T) {
	ctx := context.Background()
	store, victimTenant, _ := configured(t)
	attackerTenant := seedTenant(t)

	// The row exists, under the victim.
	if victim, err := store.Current(ctx, victimTenant); err != nil || victim.Version != 1 {
		t.Fatalf("the row under attack must exist: version %d, err %v", victim.Version, err)
	}

	// Named directly, from inside the attacker's scope.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, attackerTenant); err != nil {
		t.Fatalf("scoping: %v", err)
	}

	var seen int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM tenancy.tenant_configuration WHERE tenant_id = $1`, victimTenant).Scan(&seen); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if seen != 0 {
		t.Errorf("an administrator of one workspace saw %d of another's settings versions", seen)
	}
}

func TestOneWorkspaceCannotWriteSettingsIntoAnother(t *testing.T) {
	ctx := context.Background()
	_, victimTenant, actorID := configured(t)
	attackerTenant := seedTenant(t)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, attackerTenant); err != nil {
		t.Fatalf("scoping: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tenancy.tenant_configuration (tenant_id, version, settings, changed_by)
		VALUES ($1, 99, '{}'::jsonb, $2)`, victimTenant, actorID); err == nil {
		t.Error("an administrator of one workspace wrote settings into another")
	}
}

// Append-only, against the one attacker the REVOKE cannot stop: the role that
// owns the table. Without the trigger a migration or a psql session could
// rewrite what a campaign was created under.
func TestTheTableOwnerCannotRewriteASavedVersion(t *testing.T) {
	ctx := context.Background()
	_, tenantID, _ := configured(t)
	conn := admin(t)

	if _, err := conn.Exec(ctx, `
		UPDATE tenancy.tenant_configuration SET settings = '{}'::jsonb
		WHERE tenant_id = $1 AND version = 1`, tenantID); err == nil {
		t.Error("the table owner rewrote a saved settings version")
	}
	if _, err := conn.Exec(ctx, `
		DELETE FROM tenancy.tenant_configuration WHERE tenant_id = $1 AND version = 1`,
		tenantID); err == nil {
		t.Error("the table owner deleted a saved settings version")
	}
}

// And against the role the application actually connects as, which the
// trigger would also catch but which must not even hold the grant.
func TestTheApplicationRoleHoldsNoUpdateOrDeleteGrantOnSettings(t *testing.T) {
	ctx := context.Background()
	for _, privilege := range []string{"UPDATE", "DELETE"} {
		var granted bool
		if err := admin(t).QueryRow(ctx,
			`SELECT has_table_privilege('prepeet_app', 'tenancy.tenant_configuration', $1)`,
			privilege).Scan(&granted); err != nil {
			t.Fatalf("checking %s: %v", privilege, err)
		}
		if granted {
			t.Errorf("prepeet_app holds %s on tenancy.tenant_configuration", privilege)
		}
	}
}
