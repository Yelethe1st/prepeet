package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	// Aliased, because the generated package is identitydb so that no file can
	// import two modules' generated packages and have them collide, while
	// inside identity the only "db" there is is this one.
	db "github.com/Yelethe1st/prepeet/services/platform/internal/identity/db"
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
//
// The SQL lives in internal/identity/db/queries.sql and the access code beside
// it is generated, per ADR-0008. What stays here is everything sqlc cannot
// express: which statements share a transaction, what the transaction is scoped
// to, and how a database error becomes a domain one. That division is the point
// rather than an accident, because those three are where the rules are.
type PostgresRepository struct {
	pool *pgxpool.Pool
	// queries against the pool, for the reads that need no transaction. A
	// transactional method builds its own with db.New(tx), because the acting
	// user has to be set on the same connection the statement runs on.
	q *db.Queries
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, q: db.New(pool)}
}

// FindCredentialsByEmail returns the user and their stored password hash.
func (r *PostgresRepository) FindCredentialsByEmail(ctx context.Context, email string) (string, string, error) {
	row, err := r.q.FindCredentialsByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("identity: querying credentials: %w", err)
	}
	return row.UserID, row.PasswordHash, nil
}

// FindUserByID reads the fields GET /me reports.
func (r *PostgresRepository) FindUserByID(ctx context.Context, userID string) (User, error) {
	row, err := r.q.FindUserByID(ctx, userID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// A deleted user is reported as absent rather than as deleted, so an
		// endpoint cannot become a way to confirm that an account once existed.
		return User{}, ErrNotFound
	case err != nil:
		return User{}, fmt.Errorf("identity: reading user: %w", err)
	}
	return User{ID: row.ID, Email: row.Email, EmailVerified: row.EmailVerified}, nil
}

// SetActiveTenant verifies the membership, records the selection and writes the
// audit event, in one transaction.
//
// Three things in one method because they cannot be separated. A selection
// written without its membership check is an unauthorised scope; one written
// without its audit event is an authorisation decision nobody can review; and a
// refusal that is not audited loses exactly the record worth keeping, since a
// person trying workspaces they do not belong to is the shape of an account
// probing for access.
//
// The transaction is scoped to the acting user rather than to a tenant, because
// this runs before a tenant has been chosen. The membership policy from 0007
// makes their own memberships readable, and the audit policy makes an
// untenanted event writable by its own actor.
func (r *PostgresRepository) SetActiveTenant(ctx context.Context, sessionID, userID, tenantID, requestID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("identity: beginning tenant selection: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := database.SetUser(ctx, tx, userID); err != nil {
		return err
	}
	q := db.New(tx)

	if tenantID != "" {
		permitted, err := q.MembershipExists(ctx, db.MembershipExistsParams{UserID: userID, TenantID: tenantID})
		if err != nil {
			return fmt.Errorf("identity: checking membership: %w", err)
		}

		if !permitted {
			// The refusal is committed. Rolling back would discard the audit
			// event along with the rejected write, which loses the record of
			// somebody attempting a workspace they do not belong to.
			if err := writeAudit(ctx, q, auditEvent{
				actorID: userID, action: "identity.tenant_selected", outcome: "denied",
				subjectType: "tenant", subjectID: tenantID, requestID: requestID,
			}); err != nil {
				return err
			}
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("identity: committing refused selection: %w", err)
			}
			return ErrNoMembership
		}

		// Selecting the workspace is how an invitation is accepted. The
		// tenant scope is entered only now, after the membership check
		// passed - the same legitimation CreateOrganisationAccount uses -
		// because the invited row's own policy is tenant-scoped.
		if err := database.SetTenant(ctx, tx, tenantID); err != nil {
			return err
		}
		accepted, err := q.ActivateInvitedMembership(ctx, db.ActivateInvitedMembershipParams{
			UserID: userID, TenantID: tenantID,
		})
		if err != nil {
			return fmt.Errorf("identity: accepting the invitation: %w", err)
		}
		if accepted > 0 {
			if err := writeAudit(ctx, q, auditEvent{
				actorID: userID, action: "identity.membership_accepted", outcome: "allowed",
				subjectType: "tenant", subjectID: tenantID, requestID: requestID,
			}); err != nil {
				return err
			}
		}
	}

	if err := q.SetSessionActiveTenant(ctx, db.SetSessionActiveTenantParams{
		SessionID: sessionID, TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("identity: recording tenant selection: %w", err)
	}

	action := "identity.tenant_selected"
	if tenantID == "" {
		action = "identity.tenant_cleared"
	}
	if err := writeAudit(ctx, q, auditEvent{
		actorID: userID, action: action, outcome: "allowed",
		subjectType: "tenant", subjectID: tenantID, requestID: requestID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// auditEvent is one row of the audit trail.
type auditEvent struct {
	actorID     string
	action      string
	outcome     string
	subjectType string
	subjectID   string
	requestID   string
}

// writeAudit appends to the audit trail inside the caller's transaction.
//
// It takes the caller's *db.Queries rather than the pool, which is the whole
// point: an audit row that commits when the act it describes does not is a
// record of something that never happened, and one written afterwards is
// missing whenever the process dies in between. Neither is worth having.
func writeAudit(ctx context.Context, q *db.Queries, event auditEvent) error {
	if err := q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		ID:          id.New().String(),
		ActorID:     event.actorID,
		Action:      event.action,
		SubjectType: event.subjectType,
		SubjectID:   event.subjectID,
		Outcome:     event.outcome,
		RequestID:   event.requestID,
	}); err != nil {
		return fmt.Errorf("identity: writing audit event: %w", err)
	}
	return nil
}

// FindRole returns the role a person holds in one tenant.
//
// Scoped by the acting user rather than by tenant, using the policy from
// migration 0007, because this is asked while establishing what the session may
// do and there is no tenant context to set yet.
func (r *PostgresRepository) FindRole(ctx context.Context, userID, tenantID string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("identity: beginning role read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := database.SetUser(ctx, tx, userID); err != nil {
		return "", err
	}

	role, err := db.New(tx).FindRole(ctx, db.FindRoleParams{UserID: userID, TenantID: tenantID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", ErrNotFound
	case err != nil:
		return "", fmt.Errorf("identity: reading role: %w", err)
	}
	return role, nil
}

// FindMembershipsByUser lists the tenants a person belongs to.
//
// This is the one question that cannot be scoped by tenant, because it is asked
// before a tenant has been chosen. Migration 0007 answers it with a second
// policy rather than a WHERE clause: the transaction says who it is acting as,
// and row-level security decides which rows that person may see.
//
// The WHERE on user_id in the query is therefore not the security boundary. It
// is a filter for the planner, and the policy would refuse the rows even if it
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

	rows, err := db.New(tx).ListMembershipsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("identity: listing memberships: %w", err)
	}

	// Empty rather than nil, so a caller serialising this gets [] rather than
	// null and nothing downstream has to handle two shapes for "no tenants".
	memberships := make([]Membership, 0, len(rows))
	for _, row := range rows {
		memberships = append(memberships, Membership{
			TenantID:   row.TenantID,
			TenantName: row.TenantName,
			Status:     row.Status,
			Role:       row.Role,
		})
	}
	return memberships, nil
}

// CreateUserWithCredentials creates a user and their password in one
// transaction, so a user can never exist without a way to authenticate.
func (r *PostgresRepository) CreateUserWithCredentials(ctx context.Context, userID, email, passwordHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("identity: beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	if err := q.InsertUser(ctx, db.InsertUserParams{ID: userID, Email: email}); err != nil {
		return fmt.Errorf("identity: inserting user: %w", err)
	}
	if err := q.InsertCredentials(ctx, db.InsertCredentialsParams{
		UserID: userID, PasswordHash: passwordHash,
	}); err != nil {
		return fmt.Errorf("identity: inserting credentials: %w", err)
	}
	return tx.Commit(ctx)
}

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
	q := db.New(tx)

	if err := q.InsertUser(ctx, db.InsertUserParams{ID: account.UserID, Email: account.Email}); err != nil {
		return "", fmt.Errorf("identity: creating user: %w", err)
	}

	if err := q.InsertCredentials(ctx, db.InsertCredentialsParams{
		UserID: account.UserID, PasswordHash: account.PasswordHash,
	}); err != nil {
		return "", fmt.Errorf("identity: creating credentials: %w", err)
	}

	slug, err := insertTenant(ctx, tx, q, account)
	if err != nil {
		return "", err
	}

	// The membership is what makes the workspace administrable. A tenant
	// without one is a row nobody can reach, including support.
	if err := q.InsertOwningMembership(ctx, db.InsertOwningMembershipParams{
		ID: account.MembershipID, TenantID: account.TenantID, UserID: account.UserID,
	}); err != nil {
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
//
// The savepoints are the one thing here that stays raw. They are transaction
// control rather than a query, take no parameters, and sqlc has nothing to say
// about them; pgx's nested Begin emits the same statements.
func insertTenant(ctx context.Context, tx pgx.Tx, q *db.Queries, account OrganisationAccount) (string, error) {
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

		err := q.InsertTenant(ctx, db.InsertTenantParams{
			ID:     account.TenantID,
			Name:   account.OrganisationName,
			Slug:   slug,
			Region: account.Region,
		})
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

// UpdatePasswordHash replaces a stored hash, used when a successful login
// verified against outdated argon2 parameters.
func (r *PostgresRepository) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	if err := r.q.UpdatePasswordHash(ctx, db.UpdatePasswordHashParams{
		UserID: userID, PasswordHash: passwordHash,
	}); err != nil {
		return fmt.Errorf("identity: updating password hash: %w", err)
	}
	return nil
}

// CreateSession records an issued token pair.
func (r *PostgresRepository) CreateSession(ctx context.Context, row SessionRow) error {
	if err := r.q.InsertSession(ctx, db.InsertSessionParams{
		ID:               row.ID,
		UserID:           row.UserID,
		FamilyID:         row.FamilyID,
		SessionTokenHash: row.SessionTokenHash,
		RefreshTokenHash: row.RefreshTokenHash,
		IssuedAt:         row.IssuedAt,
		ExpiresAt:        row.ExpiresAt,
		RefreshExpiresAt: row.RefreshExpiresAt,
		AuthenticatedAt:  row.AuthenticatedAt,
	}); err != nil {
		return fmt.Errorf("identity: inserting session: %w", err)
	}
	return nil
}

// FindSessionByToken finds a session by its session token hash.
//
// The conversion from the generated row is direct rather than field by field,
// which Go permits only while the two structs agree exactly. That is deliberate:
// a column added, reordered or retyped in the query stops the build here rather
// than being silently dropped on the way to the service.
func (r *PostgresRepository) FindSessionByToken(ctx context.Context, tokenHash string) (SessionRow, error) {
	row, err := r.q.FindSessionByToken(ctx, tokenHash)
	if err != nil {
		return SessionRow{}, sessionError(err)
	}
	return SessionRow(row), nil
}

// FindSessionByRefresh finds a session by its refresh token hash.
//
// This deliberately finds retired rows too. Finding one is exactly how reuse is
// detected, so excluding them would remove the signal.
//
// Two named queries rather than one with an interpolated predicate. The old
// shape built its WHERE clause from a string, which sqlc cannot check and which
// is one careless caller away from being the place SQL is injected.
func (r *PostgresRepository) FindSessionByRefresh(ctx context.Context, tokenHash string) (SessionRow, error) {
	row, err := r.q.FindSessionByRefresh(ctx, tokenHash)
	if err != nil {
		return SessionRow{}, sessionError(err)
	}
	return SessionRow(row), nil
}

// sessionError maps a session read failure onto the domain error.
func sessionError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return fmt.Errorf("identity: querying session: %w", err)
}

// RetireSession marks a session superseded by a rotation.
//
// Retired is not revoked. A retired row stays valid to look up precisely so
// that presenting its refresh token can be recognised as reuse.
func (r *PostgresRepository) RetireSession(ctx context.Context, sessionID string, at time.Time) error {
	if err := r.q.RetireSession(ctx, db.RetireSessionParams{ID: sessionID, RetiredAt: &at}); err != nil {
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
	if err := r.q.RevokeFamily(ctx, db.RevokeFamilyParams{
		FamilyID: familyID, RevokedAt: at, Reason: reason,
	}); err != nil {
		return fmt.Errorf("identity: revoking family: %w", err)
	}
	return nil
}

// CreateOAuthState records one in-flight authorisation.
func (r *PostgresRepository) CreateOAuthState(ctx context.Context, state OAuthState) error {
	return r.q.InsertOAuthState(ctx, db.InsertOAuthStateParams{
		ID: state.ID, Provider: state.Provider, StateHash: state.StateHash,
		CodeVerifier: state.CodeVerifier, RedirectTo: state.RedirectTo,
		ExpiresAt: state.ExpiresAt,
	})
}

// ConsumeOAuthState takes the state exactly once.
//
// The single-use guarantee is the UPDATE's own `used_at IS NULL`, so two
// callbacks racing cannot both win: one updates the row and the other updates
// nothing and is told the state is unknown, which is what a replay is.
func (r *PostgresRepository) ConsumeOAuthState(ctx context.Context, stateHash string) (OAuthState, error) {
	row, err := r.q.ConsumeOAuthState(ctx, stateHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return OAuthState{}, ErrNotFound
	}
	if err != nil {
		return OAuthState{}, fmt.Errorf("identity: consuming oauth state: %w", err)
	}
	return OAuthState{
		ID: row.ID, Provider: row.Provider, CodeVerifier: row.CodeVerifier,
		RedirectTo: row.RedirectTo, ExpiresAt: row.ExpiresAt,
	}, nil
}

// FindOAuthIdentity answers which person a provider account signs in as, or
// the empty string when nothing is linked.
func (r *PostgresRepository) FindOAuthIdentity(ctx context.Context, provider, subject string) (string, error) {
	row, err := r.q.FindOAuthIdentity(ctx, db.FindOAuthIdentityParams{
		Provider: provider, Subject: subject,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("identity: finding oauth identity: %w", err)
	}
	return row.UserID, nil
}

// LinkOAuthIdentity links a provider account to a person, or refreshes what
// is known about a link that already exists.
func (r *PostgresRepository) LinkOAuthIdentity(ctx context.Context, userID, provider, subject, email string) error {
	return r.q.LinkOAuthIdentity(ctx, db.LinkOAuthIdentityParams{
		ID: id.New().String(), UserID: userID, Provider: provider,
		Subject: subject, Email: email,
	})
}
