package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
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
// FindUserByID reads the fields GET /me reports.
//
// Only those fields. A SELECT * here would put status and version into a struct
// that is one refactor away from being serialised to a browser, and a user's
// suspension status is not theirs to read from an endpoint about themselves.
func (r *PostgresRepository) FindUserByID(ctx context.Context, userID string) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, coalesce(email::text, ''), email_verified
		FROM identity.users
		WHERE id = $1 AND status <> 'deleted'`, userID).
		Scan(&user.ID, &user.Email, &user.EmailVerified)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// A deleted user is reported as absent rather than as deleted, so an
		// endpoint cannot become a way to confirm that an account once existed.
		return User{}, ErrNotFound
	case err != nil:
		return User{}, fmt.Errorf("identity: reading user: %w", err)
	}
	return user, nil
}

// FindMembershipsByUser lists the tenants a person belongs to.
//
// This is the one question that cannot be scoped by tenant, because it is asked
// before a tenant has been chosen. Migration 0007 answers it with a second
// policy rather than a WHERE clause: the transaction says who it is acting as,
// and row-level security decides which rows that person may see.
//
// The WHERE on user_id below is therefore not the security boundary. It is a
// filter for the query planner, and the policy would refuse the rows even if it
// were removed, which is the property that makes forgetting it survivable.
func (r *PostgresRepository) FindMembershipsByUser(ctx context.Context, userID string) ([]Membership, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("identity: beginning membership read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := database.SetUser(ctx, tx, userID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		SELECT m.tenant_id::text, t.name, m.status, m.role
		FROM tenancy.memberships m
		JOIN tenancy.tenants t ON t.id = m.tenant_id
		WHERE m.user_id = $1 AND m.status <> 'revoked'
		ORDER BY t.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("identity: listing memberships: %w", err)
	}
	defer rows.Close()

	// Empty rather than nil, so a caller serialising this gets [] rather than
	// null and nothing downstream has to handle two shapes for "no tenants".
	memberships := []Membership{}
	for rows.Next() {
		var membership Membership
		if err := rows.Scan(&membership.TenantID, &membership.TenantName,
			&membership.Status, &membership.Role); err != nil {
			return nil, fmt.Errorf("identity: reading membership: %w", err)
		}
		memberships = append(memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("identity: reading memberships: %w", err)
	}
	return memberships, nil
}

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
// maxSlugAttempts bounds the search for a free slug.
//
// Retry rather than a uniqueness check first, because checking then inserting
// is a race: two organisations of the same name registering at the same moment
// both see the slug free. The unique index is the arbiter, and this loop reacts
// to it.
//
// Eight attempts with a growing random suffix is far past what any realistic
// collision needs. Failing after that is better than looping forever on a bug.
const maxSlugAttempts = 8

// CreateOrganisationAccount writes the person, the workspace and the owning
// membership in one transaction.
//
// The transaction is the whole reason this is one method. Written separately, a
// failure between them leaves somebody who can verify their address, sign in,
// and find no workspace, with the address now taken by an account nobody can
// complete.
func (r *PostgresRepository) CreateOrganisationAccount(ctx context.Context, account OrganisationAccount) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("identity: beginning organisation account: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Creating a tenant is an act performed as that tenant.
	//
	// This is not a workaround. The policy on tenancy.tenants is written against
	// the primary key, since a tenants table carrying a tenant_id would be
	// circular, so a row can only be inserted by a transaction already scoped to
	// the identifier it is inserting. Nothing can create the first tenant from
	// outside, which is exactly the property wanted: there is no unscoped path
	// that writes tenant data.
	//
	// The scope covers the membership below too, whose policy needs the same
	// setting. identity.users and identity.credentials are global and carry no
	// policy, so the setting is inert for them.
	if err := database.SetTenant(ctx, tx, account.TenantID); err != nil {
		return "", err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO identity.users (id, email) VALUES ($1, $2)`,
		account.UserID, account.Email); err != nil {
		return "", fmt.Errorf("identity: creating user: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO identity.credentials (user_id, password_hash) VALUES ($1, $2)`,
		account.UserID, account.PasswordHash); err != nil {
		return "", fmt.Errorf("identity: creating credentials: %w", err)
	}

	slug, err := insertTenant(ctx, tx, account)
	if err != nil {
		return "", err
	}

	// The membership is what makes the workspace administrable. A tenant
	// without one is a row nobody can reach, including support.
	if _, err := tx.Exec(ctx,
		`INSERT INTO tenancy.memberships (id, tenant_id, user_id, status, role)
		 VALUES ($1, $2, $3, 'active', 'owner')`,
		account.MembershipID, account.TenantID, account.UserID); err != nil {
		return "", fmt.Errorf("identity: creating owning membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("identity: committing organisation account: %w", err)
	}
	return slug, nil
}

// insertTenant writes the workspace, finding a free slug.
//
// A savepoint per attempt, because a unique violation aborts the transaction in
// PostgreSQL and every later statement then fails with "current transaction is
// aborted". Without this the retry would appear to work and the commit would
// not.
func insertTenant(ctx context.Context, tx pgx.Tx, account OrganisationAccount) (string, error) {
	slug := account.Slug

	for attempt := range maxSlugAttempts {
		if attempt > 0 {
			// The suffix is random rather than a counter. A counter makes slugs
			// enumerable, so acme-2 existing tells anyone that two Acmes
			// registered, which is not theirs to learn.
			slug = fmt.Sprintf("%s-%s", account.Slug, id.Suffix())
		}

		if _, err := tx.Exec(ctx, "SAVEPOINT tenant_insert"); err != nil {
			return "", fmt.Errorf("identity: opening savepoint: %w", err)
		}

		_, err := tx.Exec(ctx,
			`INSERT INTO tenancy.tenants (id, name, slug, region) VALUES ($1, $2, $3, $4)`,
			account.TenantID, account.OrganisationName, slug, account.Region)
		if err == nil {
			if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT tenant_insert"); err != nil {
				return "", fmt.Errorf("identity: releasing savepoint: %w", err)
			}
			return slug, nil
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolation {
			return "", fmt.Errorf("identity: creating tenant: %w", err)
		}

		if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT tenant_insert"); err != nil {
			return "", fmt.Errorf("identity: rolling back to savepoint: %w", err)
		}
	}

	return "", fmt.Errorf("identity: no free slug for %q after %d attempts", account.Slug, maxSlugAttempts)
}

// uniqueViolation is PostgreSQL's SQLSTATE for a duplicate key.
const uniqueViolation = "23505"

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
