package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound means the row does not exist.
//
// The service turns this into an indistinguishable failure at the boundary,
// because a caller learning that an address is unknown is exactly the
// enumeration this package refuses to allow.
var ErrNotFound = errors.New("identity: not found")

// PostgresRepository is the PostgreSQL implementation of Repository.
//
// Nothing here is tenant scoped, because identity is not. A person is not owned
// by a tenant: the same candidate practises privately and may screen for several
// employers. Membership is what connects them, and that table carries the
// tenant and its row-level security policy. See ADR-0002.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// FindCredentialsByEmail returns the user and their stored password hash.
func (r *PostgresRepository) FindCredentialsByEmail(ctx context.Context, email string) (string, string, error) {
	var userID, hash string
	err := r.pool.QueryRow(ctx, `
		SELECT u.id::text, c.password_hash
		FROM identity.users u
		JOIN identity.credentials c ON c.user_id = u.id
		WHERE u.email = $1 AND u.status = 'active'`, email).Scan(&userID, &hash)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("identity: querying credentials: %w", err)
	}
	return userID, hash, nil
}

// CreateUserWithCredentials creates a user and their password in one
// transaction, so a user can never exist without a way to authenticate.
func (r *PostgresRepository) CreateUserWithCredentials(ctx context.Context, userID, email, passwordHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("identity: beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO identity.users (id, email, email_verified) VALUES ($1, $2, false)`,
		userID, email); err != nil {
		return fmt.Errorf("identity: inserting user: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO identity.credentials (user_id, password_hash) VALUES ($1, $2)`,
		userID, passwordHash); err != nil {
		return fmt.Errorf("identity: inserting credentials: %w", err)
	}
	return tx.Commit(ctx)
}

// UpdatePasswordHash replaces a stored hash, used when a successful login
// verified against outdated argon2 parameters.
func (r *PostgresRepository) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE identity.credentials SET password_hash = $2, updated_at = now() WHERE user_id = $1`,
		userID, passwordHash); err != nil {
		return fmt.Errorf("identity: updating password hash: %w", err)
	}
	return nil
}

// CreateSession records an issued token pair.
func (r *PostgresRepository) CreateSession(ctx context.Context, row SessionRow) error {
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO identity.sessions
			(id, user_id, family_id, session_token_hash, refresh_token_hash,
			 issued_at, expires_at, refresh_expires_at, authenticated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		row.ID, row.UserID, row.FamilyID, row.SessionTokenHash, row.RefreshTokenHash,
		row.IssuedAt, row.ExpiresAt, row.RefreshExpiresAt, row.AuthenticatedAt); err != nil {
		return fmt.Errorf("identity: inserting session: %w", err)
	}
	return nil
}

// FindSessionByToken finds a session by its session token hash.
func (r *PostgresRepository) FindSessionByToken(ctx context.Context, tokenHash string) (SessionRow, error) {
	return r.findSession(ctx, `session_token_hash = $1`, tokenHash)
}

// FindSessionByRefresh finds a session by its refresh token hash.
//
// This deliberately finds retired rows too. Finding one is exactly how reuse is
// detected, so excluding them would remove the signal.
func (r *PostgresRepository) FindSessionByRefresh(ctx context.Context, tokenHash string) (SessionRow, error) {
	return r.findSession(ctx, `refresh_token_hash = $1`, tokenHash)
}

func (r *PostgresRepository) findSession(ctx context.Context, predicate, tokenHash string) (SessionRow, error) {
	var row SessionRow
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, user_id::text, family_id::text,
		       session_token_hash, refresh_token_hash,
		       issued_at, expires_at, refresh_expires_at, authenticated_at,
		       retired_at, revoked_at
		FROM identity.sessions
		WHERE `+predicate, tokenHash).Scan(
		&row.ID, &row.UserID, &row.FamilyID,
		&row.SessionTokenHash, &row.RefreshTokenHash,
		&row.IssuedAt, &row.ExpiresAt, &row.RefreshExpiresAt, &row.AuthenticatedAt,
		&row.RetiredAt, &row.RevokedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return SessionRow{}, ErrNotFound
	}
	if err != nil {
		return SessionRow{}, fmt.Errorf("identity: querying session: %w", err)
	}
	return row, nil
}

// RetireSession marks a session superseded by a rotation.
//
// Retired is not revoked. A retired row stays valid to look up precisely so
// that presenting its refresh token can be recognised as reuse.
func (r *PostgresRepository) RetireSession(ctx context.Context, sessionID string, at time.Time) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE identity.sessions SET retired_at = $2 WHERE id = $1 AND retired_at IS NULL`,
		sessionID, at); err != nil {
		return fmt.Errorf("identity: retiring session: %w", err)
	}
	return nil
}

// RevokeFamily revokes every session descended from one login.
//
// Whole family rather than one row, because a reused refresh token means we
// cannot tell the legitimate client from the attacker, and revoking only the
// row that was reused would leave whichever of them holds the current pair.
func (r *PostgresRepository) RevokeFamily(ctx context.Context, familyID, reason string, at time.Time) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE identity.sessions
		SET revoked_at = $2, revoked_reason = $3
		WHERE family_id = $1 AND revoked_at IS NULL`,
		familyID, at, reason); err != nil {
		return fmt.Errorf("identity: revoking family: %w", err)
	}
	return nil
}
