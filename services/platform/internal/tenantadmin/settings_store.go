package tenantadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// The settings half of tenant administration, durable. What lives here rather
// than in SQL: the default document an unconfigured workspace reads, the
// difference between two versions that the audit row carries, and the
// decision that validation runs before anything is written.

// Settings refusals. Callers branch on these; each is a rule with a name.
var (
	// ErrSettingsStale means another administrator saved first. Nothing was
	// written: the version they read is no longer the current one, and
	// overwriting a change they never saw is the failure this prevents.
	ErrSettingsStale = errors.New("tenantadmin: SETTINGS_STALE: the settings changed since they were read")
	// ErrSettingsVersionUnknown covers a version this workspace never saved,
	// and another workspace's version alike.
	ErrSettingsVersionUnknown = errors.New("tenantadmin: SETTINGS_VERSION_UNKNOWN: no such settings version")
)

// Configuration is one saved version of a workspace's settings.
type Configuration struct {
	// Version is 0 for a workspace that has never saved, whose document is
	// DefaultSettings. Every saved version is 1 or above.
	Version   int
	Settings  Settings
	ChangedBy string
	ChangedAt time.Time
}

// SettingsStore reads and appends a workspace's configuration versions.
type SettingsStore struct {
	pool *pgxpool.Pool
}

// NewSettingsStore builds the store.
func NewSettingsStore(pool *pgxpool.Pool) *SettingsStore {
	return &SettingsStore{pool: pool}
}

// Current answers the workspace's settings as they stand.
//
// A workspace that has never saved reads DefaultSettings at version 0 rather
// than an error, because the settings screen has to render before anybody has
// pressed save, and an error there is a broken form rather than an empty one.
func (s *SettingsStore) Current(ctx context.Context, tenantID string) (Configuration, error) {
	tx, err := s.begin(ctx, tenantID)
	if err != nil {
		return Configuration{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := db.New(tx).CurrentConfiguration(ctx, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Configuration{Version: 0, Settings: DefaultSettings()}, nil
	}
	if err != nil {
		return Configuration{}, fmt.Errorf("tenantadmin: reading settings: %w", err)
	}
	return decodeConfiguration(int(row.Version), row.Settings, row.ChangedBy, row.ChangedAt)
}

// AtVersion answers what the workspace's settings were at one version.
//
// This is the read a campaign makes against the version it pinned when it was
// created, which is how "defaults apply to new campaigns only" holds without
// anybody copying the whole document into the campaign.
func (s *SettingsStore) AtVersion(ctx context.Context, tenantID string, version int) (Configuration, error) {
	if version == 0 {
		return Configuration{Version: 0, Settings: DefaultSettings()}, nil
	}

	tx, err := s.begin(ctx, tenantID)
	if err != nil {
		return Configuration{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := db.New(tx).ConfigurationAtVersion(ctx, db.ConfigurationAtVersionParams{
		TenantID: tenantID, Version: int32(version),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Configuration{}, ErrSettingsVersionUnknown
	}
	if err != nil {
		return Configuration{}, fmt.Errorf("tenantadmin: reading settings version: %w", err)
	}
	return decodeConfiguration(int(row.Version), row.Settings, row.ChangedBy, row.ChangedAt)
}

// History answers every version, newest first.
func (s *SettingsStore) History(ctx context.Context, tenantID string) ([]Configuration, error) {
	tx, err := s.begin(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := db.New(tx).ConfigurationHistory(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenantadmin: reading settings history: %w", err)
	}
	history := make([]Configuration, 0, len(rows))
	for _, row := range rows {
		version, err := decodeConfiguration(int(row.Version), row.Settings, row.ChangedBy, row.ChangedAt)
		if err != nil {
			return nil, err
		}
		history = append(history, version)
	}
	return history, nil
}

// Save appends a new version and audits what moved.
//
// expectedVersion is the version the administrator was looking at, and a save
// against a stale one is refused rather than merged: two people on the
// settings screen at once must not have one of them silently undo the other.
// The refusal comes from the primary key, so it holds under concurrency
// rather than under a read followed by a write.
//
// The audit row carries the actor and the names of the fields that moved. The
// previous values themselves are not copied into it: they are the previous
// version, which is still in the table and cannot be edited, and audit.events
// is exported to tenants and is deliberately not a place for candidate-facing
// copy. "What was it before" is answered by reading version n-1.
func (s *SettingsStore) Save(ctx context.Context, tenantID, actorID string, next Settings, expectedVersion int) (Configuration, error) {
	if err := next.Validate(); err != nil {
		return Configuration{}, err
	}

	previous, err := s.readAt(ctx, tenantID, expectedVersion)
	if err != nil {
		return Configuration{}, err
	}

	encoded, err := json.Marshal(next)
	if err != nil {
		return Configuration{}, fmt.Errorf("tenantadmin: encoding settings: %w", err)
	}
	changed, err := json.Marshal(map[string]any{
		"from_version": expectedVersion,
		"to_version":   expectedVersion + 1,
		"changed":      ChangedFields(previous, next),
	})
	if err != nil {
		return Configuration{}, fmt.Errorf("tenantadmin: encoding the audit detail: %w", err)
	}

	tx, err := s.begin(ctx, tenantID)
	if err != nil {
		return Configuration{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	if err := q.InsertConfiguration(ctx, db.InsertConfigurationParams{
		TenantID: tenantID, Version: int32(expectedVersion + 1),
		Settings: encoded, ChangedBy: actorID,
	}); err != nil {
		if isUniqueViolation(err) {
			return Configuration{}, ErrSettingsStale
		}
		return Configuration{}, fmt.Errorf("tenantadmin: saving settings: %w", err)
	}

	if err := q.InsertTenantAuditEvent(ctx, db.InsertTenantAuditEventParams{
		ID: id.New().String(), TenantID: tenantID, ActorID: actorID,
		Action: "tenant.settings_changed", SubjectType: "tenant_configuration",
		SubjectID: fmt.Sprintf("%s@%d", tenantID, expectedVersion+1), Detail: changed,
	}); err != nil {
		return Configuration{}, fmt.Errorf("tenantadmin: auditing the settings change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Configuration{}, err
	}
	return s.AtVersion(ctx, tenantID, expectedVersion+1)
}

// readAt returns the document a save is being made against, so the audit row
// can say what moved. A version the workspace never saved is a stale save
// rather than an unknown one: the administrator is working from something
// that is not this workspace's history.
func (s *SettingsStore) readAt(ctx context.Context, tenantID string, version int) (Settings, error) {
	configuration, err := s.AtVersion(ctx, tenantID, version)
	if errors.Is(err, ErrSettingsVersionUnknown) {
		return Settings{}, ErrSettingsStale
	}
	if err != nil {
		return Settings{}, err
	}
	return configuration.Settings, nil
}

// begin opens a transaction scoped to one workspace. Every read and write in
// this file goes through it, so a missing scope is a compile-time absence
// rather than a runtime leak.
func (s *SettingsStore) begin(ctx context.Context, tenantID string) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("tenantadmin: beginning: %w", err)
	}
	if err := database.SetTenant(ctx, tx, tenantID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

// decodeConfiguration turns a stored row into a document.
func decodeConfiguration(version int, body []byte, changedBy string, changedAt time.Time) (Configuration, error) {
	var settings Settings
	if err := json.Unmarshal(body, &settings); err != nil {
		return Configuration{}, fmt.Errorf("tenantadmin: decoding settings: %w", err)
	}
	return Configuration{
		Version: version, Settings: settings,
		ChangedBy: changedBy, ChangedAt: changedAt,
	}, nil
}

// isUniqueViolation reports whether an insert lost a race on a unique index.
//
// The concurrency guard is the primary key rather than a read-then-write,
// because a read-then-write has a window between the two in which the other
// administrator's save lands and neither of them is told.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}
