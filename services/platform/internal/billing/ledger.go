// Package billing owns the usage ledger and quotas, to ADR-0014.
//
// The unit is a started session, metered exactly once; corrections are
// credit entries beside the start, never edits, so an invoice is a sum
// over immutable rows. Quota is enforced by reservation at start: the
// reserve transaction locks the tenant's quota row, counts the ledger and
// either appends the start or refuses. Nothing in this package - or
// anywhere - consults quota after a session has started, which is how "a
// candidate is never interrupted mid-interview by a quota event" is a
// structure rather than a promise.
package billing

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/billing/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Credit reasons, from ADR-0014. A credit always names its cause.
const (
	// ReasonEarlyAbandon: the session ended inside the first minute.
	ReasonEarlyAbandon = "early_abandon"
	// ReasonPlatformInterrupted: our transport or agent failed the session.
	ReasonPlatformInterrupted = "interrupted_by_platform"
)

// Stable refusals.
var (
	// ErrQuotaExhausted refuses a new start at the limit. Existing sessions
	// are untouched by construction: nothing rechecks after reservation.
	ErrQuotaExhausted = errors.New("billing: QUOTA_EXHAUSTED: the workspace is at its session limit")
	// ErrAlreadyMetered means this session already has this entry; the
	// ledger's exactly-once guard refused a duplicate.
	ErrAlreadyMetered = errors.New("billing: ALREADY_METERED: this session is already recorded")
)

// Warning is where a tenant stands against its quota.
type Warning string

const (
	WarningNone        Warning = "none"
	WarningApproaching Warning = "approaching"
	WarningReached     Warning = "reached"
)

// Usage is what the tenant reads: the same numbers the invoice will use.
type Usage struct {
	Started  int
	Credited int
	// Billable is started minus credited: the number the unit prices.
	Billable int
	// Limit is nil when no quota is configured.
	Limit         *int
	WarnThreshold float64
	Warning       Warning
	Remaining     *int
}

// Ledger meters starts and answers usage.
type Ledger struct {
	pool *pgxpool.Pool
}

// NewLedger wires the ledger.
func NewLedger(pool *pgxpool.Pool) *Ledger {
	return &Ledger{pool: pool}
}

// ReserveStart admits one session under the quota, or refuses it.
//
// The quota row is locked first, so two concurrent starts at limit minus
// one queue rather than both passing. The inserted entry IS the
// reservation and the metering: one row, appended before the session may
// move to in_progress, unique per session. Re-reserving the same session
// is ErrAlreadyMetered, which a retrying caller treats as success already
// achieved.
func (l *Ledger) ReserveStart(ctx context.Context, tenantID, sessionID, mode string) error {
	tx, err := l.begin(ctx, tenantID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	quota, err := q.LockQuota(ctx, tenantID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// No quota row: unlimited, and no lock is needed because there is
		// no boundary to race over.
	case err != nil:
		return fmt.Errorf("billing: locking quota: %w", err)
	case quota.SessionLimit.Valid:
		billable, err := q.CountBillableStarts(ctx, tenantID)
		if err != nil {
			return fmt.Errorf("billing: counting: %w", err)
		}
		if int(billable) >= int(quota.SessionLimit.Int32) {
			return ErrQuotaExhausted
		}
	}

	if err := q.InsertUsageEntry(ctx, db.InsertUsageEntryParams{
		ID: id.New().String(), TenantID: tenantID, SessionID: sessionID,
		Kind: "session_started", Reason: "", Mode: mode,
	}); err != nil {
		if isUniqueViolation(err) {
			return ErrAlreadyMetered
		}
		return fmt.Errorf("billing: metering the start: %w", err)
	}
	return tx.Commit(ctx)
}

// CreditStart appends the correction ADR-0014 defines, exactly once.
func (l *Ledger) CreditStart(ctx context.Context, tenantID, sessionID, reason string) error {
	if reason != ReasonEarlyAbandon && reason != ReasonPlatformInterrupted {
		return fmt.Errorf("billing: %q is not a credit the unit defines", reason)
	}

	tx, err := l.begin(ctx, tenantID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	// A credit corrects a start; crediting a session that never started
	// would invent negative usage.
	started, err := q.HasEntry(ctx, db.HasEntryParams{SessionID: sessionID, Kind: "session_started"})
	if err != nil {
		return fmt.Errorf("billing: checking the start: %w", err)
	}
	if !started {
		return fmt.Errorf("billing: session %s has no start to credit", sessionID)
	}

	if err := q.InsertUsageEntry(ctx, db.InsertUsageEntryParams{
		ID: id.New().String(), TenantID: tenantID, SessionID: sessionID,
		Kind: "start_credited", Reason: reason, Mode: "screening",
	}); err != nil {
		if isUniqueViolation(err) {
			return ErrAlreadyMetered
		}
		return fmt.Errorf("billing: crediting: %w", err)
	}
	return tx.Commit(ctx)
}

// Usage answers where the tenant stands, warning included.
//
// The warning exists so approaching and reaching the limit are both
// visible before anything is blocked: approaching begins at the configured
// threshold, reached means the next start will be refused.
func (l *Ledger) Usage(ctx context.Context, tenantID string) (Usage, error) {
	tx, err := l.begin(ctx, tenantID)
	if err != nil {
		return Usage{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	summary, err := q.UsageSummary(ctx, tenantID)
	if err != nil {
		return Usage{}, fmt.Errorf("billing: summarising: %w", err)
	}
	usage := Usage{
		Started:  int(summary.Started),
		Credited: int(summary.Credited),
		Billable: int(summary.Started) - int(summary.Credited),
		Warning:  WarningNone,
		// The default threshold applies even without a quota row, so the
		// answer is stable when a limit is configured later.
		WarnThreshold: 0.80,
	}

	quota, err := q.GetQuota(ctx, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return usage, nil
	}
	if err != nil {
		return Usage{}, fmt.Errorf("billing: reading quota: %w", err)
	}
	usage.WarnThreshold = quota.WarnThreshold
	if !quota.SessionLimit.Valid {
		return usage, nil
	}

	limit := int(quota.SessionLimit.Int32)
	usage.Limit = &limit
	remaining := max(limit-usage.Billable, 0)
	usage.Remaining = &remaining
	switch {
	case usage.Billable >= limit:
		usage.Warning = WarningReached
	case float64(usage.Billable) >= float64(limit)*quota.WarnThreshold:
		usage.Warning = WarningApproaching
	}
	return usage, nil
}

// SetQuota writes configuration. Ledger entries are never touched.
// A nil limit is unlimited.
func (l *Ledger) SetQuota(ctx context.Context, tenantID string, limit *int, warnThreshold float64) error {
	tx, err := l.begin(ctx, tenantID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	encoded := int32(-1)
	if limit != nil {
		encoded = int32(*limit)
	}
	if err := db.New(tx).UpsertQuota(ctx, db.UpsertQuotaParams{
		TenantID: tenantID, SessionLimit: encoded, WarnThreshold: warnThreshold,
	}); err != nil {
		return fmt.Errorf("billing: writing quota: %w", err)
	}
	return tx.Commit(ctx)
}

func (l *Ledger) begin(ctx context.Context, tenantID string) (pgx.Tx, error) {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("billing: beginning: %w", err)
	}
	if err := database.SetTenant(ctx, tx, tenantID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}
