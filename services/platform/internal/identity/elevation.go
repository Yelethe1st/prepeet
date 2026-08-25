package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/identity/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/httpserver"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Time-bound platform elevation. Implements IAM-07.
//
// The grant is exceptional by construction: it cannot exist without a reason
// and a ticket, it cannot outlive the cap, its expiry is a timestamp compared
// at read time rather than a job that might not run, and every authenticated
// request made while it is active writes its own audit row - from session
// lookup, the one choke point all reads pass, so a future endpoint cannot
// forget to be recorded under it.

// Stable refusals for the elevation flows.
var (
	// ErrElevationReason and ErrElevationTicket refuse a grant that could not
	// be audited: "why" and "under what" are the record, not decoration.
	ErrElevationReason = errors.New("identity: ELEVATION_REASON_REQUIRED: an elevation needs a reason")
	ErrElevationTicket = errors.New("identity: ELEVATION_TICKET_REQUIRED: an elevation needs a ticket")
	// ErrElevationGone means the grant to revoke is already expired, revoked
	// or never existed - one answer, because a revoker only needs to know the
	// grant is not alive.
	ErrElevationGone = errors.New("identity: ELEVATION_NOT_ACTIVE: that elevation is not active")
)

// MaxElevation caps how long a grant can live. An hour is long enough to
// work an incident and short enough that a forgotten grant dies the same
// morning it was made.
const MaxElevation = time.Hour

// DefaultElevation is the grant length when none is asked for.
const DefaultElevation = 30 * time.Minute

// Elevation is one active grant, as callers and the visibility list see it.
type Elevation struct {
	GrantID   string
	UserID    string
	Email     string
	Reason    string
	Ticket    string
	GrantedAt time.Time
	ExpiresAt time.Time
}

// Authz converts the grant into the shape the decision engine consumes.
func (e Elevation) Authz() *authz.Elevation {
	return &authz.Elevation{
		GrantID:   e.GrantID,
		Reason:    e.Reason,
		Ticket:    e.Ticket,
		ExpiresAt: e.ExpiresAt,
	}
}

// Elevate grants time-bound platform elevation to a user.
//
// The capability check (platform.privileged_elevate) is the API layer's; what
// is enforced here is what makes the grant auditable and bounded whatever the
// caller: reason, ticket, and the cap. A requested duration above the cap is
// refused rather than clamped, because silently shortening what an operator
// asked for leaves them believing they hold time they do not.
func (s *Service) Elevate(ctx context.Context, userID, reason, ticket string, duration time.Duration) (Elevation, error) {
	if reason == "" {
		return Elevation{}, ErrElevationReason
	}
	if ticket == "" {
		return Elevation{}, ErrElevationTicket
	}
	if duration == 0 {
		duration = DefaultElevation
	}
	if duration < 0 || duration > MaxElevation {
		return Elevation{}, fmt.Errorf(
			"identity: ELEVATION_TOO_LONG: an elevation lasts at most %s", MaxElevation)
	}

	grantID := id.New().String()
	expires := s.clock().Add(duration)
	if err := s.repo.CreateElevation(ctx, grantID, userID, reason, ticket, expires); err != nil {
		return Elevation{}, err
	}
	return Elevation{
		GrantID: grantID, UserID: userID, Reason: reason, Ticket: ticket,
		ExpiresAt: expires,
	}, nil
}

// RevokeElevation ends a grant before its expiry.
func (s *Service) RevokeElevation(ctx context.Context, grantID, revokedBy string) error {
	return s.repo.RevokeElevation(ctx, grantID, revokedBy)
}

// ActiveElevations lists every grant alive right now: the visibility the
// ticket requires, for the operator and for their team.
func (s *Service) ActiveElevations(ctx context.Context) ([]Elevation, error) {
	return s.repo.ListActiveElevations(ctx)
}

// ── repository half

// CreateElevation stores the grant and its audit row in one transaction.
func (r *PostgresRepository) CreateElevation(ctx context.Context, grantID, userID, reason, ticket string, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("identity: beginning elevation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The untenanted audit policy binds the row to the acting user.
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return err
	}

	q := db.New(tx)
	if err := q.InsertElevation(ctx, db.InsertElevationParams{
		ID: grantID, UserID: userID, Reason: reason, Ticket: ticket, ExpiresAt: expiresAt,
	}); err != nil {
		return fmt.Errorf("identity: storing elevation: %w", err)
	}

	detail, err := json.Marshal(map[string]string{
		"reason": reason, "ticket": ticket, "expires_at": expiresAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("identity: encoding elevation detail: %w", err)
	}
	if err := q.InsertElevationAudit(ctx, db.InsertElevationAuditParams{
		ID: id.New().String(), ActorID: userID, Action: "identity.elevation_granted",
		GrantID: grantID, Outcome: "allowed", Detail: detail,
	}); err != nil {
		return fmt.Errorf("identity: auditing elevation: %w", err)
	}
	return tx.Commit(ctx)
}

// RevokeElevation ends a live grant and audits the ending together.
func (r *PostgresRepository) RevokeElevation(ctx context.Context, grantID, revokedBy string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("identity: beginning revocation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, revokedBy); err != nil {
		return err
	}

	q := db.New(tx)
	revoked, err := q.RevokeElevation(ctx, db.RevokeElevationParams{
		ID: grantID, RevokedBy: revokedBy,
	})
	if err != nil {
		return fmt.Errorf("identity: revoking elevation: %w", err)
	}
	if revoked == 0 {
		return ErrElevationGone
	}

	if err := q.InsertElevationAudit(ctx, db.InsertElevationAuditParams{
		ID: id.New().String(), ActorID: revokedBy, Action: "identity.elevation_revoked",
		GrantID: grantID, Outcome: "allowed", Detail: []byte(`{}`),
	}); err != nil {
		return fmt.Errorf("identity: auditing revocation: %w", err)
	}
	return tx.Commit(ctx)
}

// ListActiveElevations returns every grant alive right now.
func (r *PostgresRepository) ListActiveElevations(ctx context.Context) ([]Elevation, error) {
	rows, err := r.q.ListActiveElevations(ctx)
	if err != nil {
		return nil, fmt.Errorf("identity: listing elevations: %w", err)
	}
	active := make([]Elevation, 0, len(rows))
	for _, row := range rows {
		active = append(active, Elevation{
			GrantID: row.ID, UserID: row.UserID, Email: row.Email,
			Reason: row.Reason, Ticket: row.Ticket,
			GrantedAt: row.GrantedAt, ExpiresAt: row.ExpiresAt,
		})
	}
	return active, nil
}

// AuditElevatedRequest records one authenticated request made under a grant.
//
// Called from session lookup for every request while the grant is alive: the
// record that access happened, whether or not anything was read. The request
// id ties the row to the trace, so "what did this elevation touch" is
// answerable request by request.
func (r *PostgresRepository) AuditElevatedRequest(ctx context.Context, userID, grantID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("identity: beginning elevated-request audit: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return err
	}

	if err := db.New(tx).InsertElevatedRequestAudit(ctx, db.InsertElevatedRequestAuditParams{
		ID: id.New().String(), ActorID: userID, GrantID: grantID,
		Detail:    []byte(`{}`),
		RequestID: httpserver.RequestIDFrom(ctx),
	}); err != nil {
		return fmt.Errorf("identity: auditing elevated request: %w", err)
	}
	return tx.Commit(ctx)
}

// ActiveElevationFor returns the user's live grant, or ErrNotFound.
func (r *PostgresRepository) ActiveElevationFor(ctx context.Context, userID string) (Elevation, error) {
	row, err := r.q.ActiveElevationByUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Elevation{}, ErrNotFound
	}
	if err != nil {
		return Elevation{}, fmt.Errorf("identity: reading elevation: %w", err)
	}
	return Elevation{
		GrantID: row.ID, UserID: userID, Reason: row.Reason, Ticket: row.Ticket,
		GrantedAt: row.GrantedAt, ExpiresAt: row.ExpiresAt,
	}, nil
}
