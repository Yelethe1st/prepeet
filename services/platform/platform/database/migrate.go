// Package database owns the PostgreSQL schema and the tenant isolation that
// sits under every query.
//
// ADR-0002 decides the shape: one database, a schema per module, row-level
// security on every tenant-owned table, and tenant context set per transaction
// rather than per connection. This package implements that decision and the
// migration runner that applies it.
//
// The isolation here is defence in depth, not the primary control.
// Authorization still decides who may read what. What row-level security buys
// is that a handler which forgets to scope its query returns nothing rather
// than another tenant's rows, and a cross-tenant leak in this product means an
// employer seeing rehearsals a candidate believed were private.
//
// Implements part of PLT-03.
package database

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed sql/*.sql
var migrationFiles embed.FS

// Migration is one numbered, forward-only schema change.
type Migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

// Checksum returns the digest recorded against an applied migration.
//
// It is taken from the SQL itself and nothing else, because its only job is to
// detect that an already applied migration has been edited. Two environments
// running different SQL under the same version number is a difference nobody
// notices until it matters.
func Checksum(sql string) string {
	sum := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(sum[:])
}

// Migrations returns every embedded migration in version order.
//
// Files are named NNNN_description.sql. Ordering comes from the number rather
// than from whatever order the filesystem returns, because the latter is not
// guaranteed and a schema applied out of order is not a schema.
func Migrations() ([]Migration, error) {
	entries, err := migrationFiles.ReadDir("sql")
	if err != nil {
		return nil, fmt.Errorf("database: reading migrations: %w", err)
	}

	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		name := entry.Name()
		number, rest, found := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
		if !found {
			return nil, fmt.Errorf("database: migration %q is not named NNNN_description.sql", name)
		}
		version, err := strconv.Atoi(number)
		if err != nil {
			return nil, fmt.Errorf("database: migration %q has a non-numeric version: %w", name, err)
		}

		content, err := migrationFiles.ReadFile(path.Join("sql", name))
		if err != nil {
			return nil, fmt.Errorf("database: reading %q: %w", name, err)
		}

		migrations = append(migrations, Migration{
			Version:  version,
			Name:     rest,
			SQL:      string(content),
			Checksum: Checksum(string(content)),
		})
	}

	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}

// MigrateOptions configures a migration run.
type MigrateOptions struct {
	// AppPassword is the password for the prepeet_app role, created on first
	// run. In a deployed environment this comes from the secret store rather
	// than from configuration, per PLT-07.
	AppPassword string
}

// Migrate applies every outstanding migration.
//
// It runs as its own process rather than at api startup, so that several api
// replicas starting at once cannot race, and so a migration can be applied and
// verified before the code depending on it is deployed.
//
// Each migration runs inside its own transaction together with the row that
// records it, so a failure leaves neither a half-applied schema nor a history
// that disagrees with reality.
func Migrate(ctx context.Context, url string, opts MigrateOptions) error {
	migrations, err := Migrations()
	if err != nil {
		return err
	}

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("database: connecting: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS public.schema_migrations (
			version     integer     PRIMARY KEY,
			name        text        NOT NULL,
			checksum    text        NOT NULL,
			applied_at  timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("database: creating migration history: %w", err)
	}

	applied, err := appliedChecksums(ctx, conn)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if recorded, done := applied[migration.Version]; done {
			if recorded != migration.Checksum {
				return fmt.Errorf(
					"database: migration %d (%s) has changed since it was applied: "+
						"recorded %s, found %s. Migrations are forward only; add a new one",
					migration.Version, migration.Name, short(recorded), short(migration.Checksum))
			}
			continue
		}

		if err := apply(ctx, conn, migration, opts); err != nil {
			return err
		}
	}
	return nil
}

// apply runs one migration and records it in the same transaction.
func apply(ctx context.Context, conn *pgx.Conn, migration Migration, opts MigrateOptions) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("database: beginning migration %d: %w", migration.Version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sql := migration.SQL
	if opts.AppPassword != "" {
		// The application role's password is the one value in the schema that
		// cannot live in a checked-in file. The placeholder keeps the SQL
		// checksummable while the secret stays out of the repository.
		sql = strings.ReplaceAll(sql, ":'app_password'", quoteLiteral(opts.AppPassword))
	}

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("database: applying migration %d (%s): %w", migration.Version, migration.Name, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO public.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
		migration.Version, migration.Name, migration.Checksum); err != nil {
		return fmt.Errorf("database: recording migration %d: %w", migration.Version, err)
	}

	return tx.Commit(ctx)
}

func appliedChecksums(ctx context.Context, conn *pgx.Conn) (map[int]string, error) {
	rows, err := conn.Query(ctx, `SELECT version, checksum FROM public.schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("database: reading migration history: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]string)
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, fmt.Errorf("database: reading migration history: %w", err)
		}
		applied[version] = checksum
	}
	return applied, rows.Err()
}

// ErrNoTenant is returned when a tenant identifier is missing or unusable.
var ErrNoTenant = errors.New("database: a tenant is required")

// SetUser records who is asking, for the policies that scope a row to its owner
// rather than to a tenant.
//
// Listing which tenants a person belongs to cannot be answered from inside one
// tenant's scope, so tenancy.memberships carries a second policy keyed to this
// value. SET LOCAL for the same reason as the tenant: it must not survive the
// transaction and be inherited by the next request on this connection.
func SetUser(ctx context.Context, tx pgx.Tx, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("database: a user is required")
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, userID); err != nil {
		return fmt.Errorf("database: setting user context: %w", err)
	}
	return nil
}

// SetTenant scopes a transaction to one tenant.
//
// SET LOCAL rather than SET, deliberately: the value dies with the transaction
// and cannot be inherited by the next request that borrows the same pooled
// connection. A pooled connection carrying a previous request's tenant is
// exactly the bug row-level security exists to catch, so the mechanism must not
// create it.
//
// The identifier is passed as a parameter rather than interpolated. SET LOCAL
// does not accept a bind parameter directly, so set_config is used, which does.
func SetTenant(ctx context.Context, tx pgx.Tx, tenantID string) error {
	if strings.TrimSpace(tenantID) == "" {
		return ErrNoTenant
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenantID); err != nil {
		return fmt.Errorf("database: setting tenant context: %w", err)
	}
	return nil
}

// quoteLiteral escapes a value for inclusion in SQL as a literal. It is used
// only for the application role's password during migration, where a bind
// parameter is not available because CREATE ROLE does not accept one.
func quoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func short(checksum string) string {
	if len(checksum) <= 12 {
		return checksum
	}
	return checksum[:12]
}
