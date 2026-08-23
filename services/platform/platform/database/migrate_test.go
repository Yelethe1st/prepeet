package database_test

import (
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

// Migrations are applied in filename order, so the order has to come from the
// name rather than from however the filesystem happened to return them.
func TestMigrationsAreOrderedByVersion(t *testing.T) {
	t.Parallel()

	migrations, err := database.Migrations()
	if err != nil {
		t.Fatalf("Migrations() returned error: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("Migrations() returned none, want at least the initial schema")
	}

	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].Version >= migrations[i].Version {
			t.Errorf("migration %d (%s) does not come before %d (%s)",
				migrations[i-1].Version, migrations[i-1].Name,
				migrations[i].Version, migrations[i].Name)
		}
	}
}

// Two migrations sharing a version would apply in an order that depends on the
// filesystem, and would record ambiguously in the history table.
func TestMigrationVersionsAreUnique(t *testing.T) {
	t.Parallel()

	migrations, err := database.Migrations()
	if err != nil {
		t.Fatalf("Migrations() returned error: %v", err)
	}

	seen := make(map[int]string, len(migrations))
	for _, m := range migrations {
		if previous, clash := seen[m.Version]; clash {
			t.Errorf("version %d is used by both %q and %q", m.Version, previous, m.Name)
		}
		seen[m.Version] = m.Name
	}
}

// The checksum is what makes an edited applied migration detectable. If it were
// derived from anything but the SQL itself, an edit could go unnoticed.
func TestChecksumChangesWithTheSQL(t *testing.T) {
	t.Parallel()

	first := database.Checksum("CREATE TABLE a (id uuid);")
	same := database.Checksum("CREATE TABLE a (id uuid);")
	different := database.Checksum("CREATE TABLE a (id uuid); -- a comment")

	if first != same {
		t.Error("Checksum is not deterministic for identical SQL")
	}
	if first == different {
		t.Error("Checksum did not change when the SQL changed")
	}
	if first == "" {
		t.Error("Checksum returned an empty string")
	}
}

func TestEveryMigrationHasSQL(t *testing.T) {
	t.Parallel()

	migrations, err := database.Migrations()
	if err != nil {
		t.Fatalf("Migrations() returned error: %v", err)
	}

	for _, m := range migrations {
		if m.SQL == "" {
			t.Errorf("migration %d (%s) has no SQL", m.Version, m.Name)
		}
		if m.Checksum == "" {
			t.Errorf("migration %d (%s) has no checksum", m.Version, m.Name)
		}
	}
}
