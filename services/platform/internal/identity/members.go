package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/identity/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Member administration: TEN-02's flows, in the context that owns
// memberships.
//
// The rules the surface keeps: the owner row is untouchable here - the
// anchor exists so a workspace always has an administrator nobody inside it
// can remove - and this surface never assigns 'owner' either, because
// ownership transfer is a deliberate act with its own later flow, not a
// dropdown option. Every change is version-guarded against concurrent
// administrators and audited with what it changed FROM, since "who made
// Priya an admin, and what was she before" is the question the trail
// exists to answer. Revocation needs no propagation: capabilities are
// recomputed from the membership on every request, so a revoked member's
// very next request carries nothing.

// The roles this surface assigns. owner is deliberately absent.
var assignableRoles = map[string]bool{
	"admin": true, "recruiter": true, "hiring_manager": true, "viewer": true,
}

// Member administration refusals.
var (
	ErrMemberNotFound     = errors.New("identity: MEMBER_NOT_FOUND: no such membership in this workspace")
	ErrMemberExists       = errors.New("identity: MEMBER_EXISTS: that person is already a member")
	ErrMemberOwner        = errors.New("identity: MEMBER_OWNER: the owner cannot be changed or removed here")
	ErrMemberRoleInvalid  = errors.New("identity: MEMBER_ROLE_INVALID: that is not a role this surface assigns")
	ErrMemberUnknownEmail = errors.New("identity: MEMBER_UNKNOWN_EMAIL: no active account has that address")
	ErrMemberStale        = errors.New("identity: MEMBER_STALE: the membership changed since it was read")
)

// Member is one row of the workspace's people.
type Member struct {
	MembershipID string
	UserID       string
	Email        string
	Role         string
	Status       string
	Version      int
	CreatedAt    time.Time
}

// Members administers one workspace's memberships.
type Members struct {
	pool interface {
		Begin(ctx context.Context) (pgx.Tx, error)
	}
}

// NewMembers wires member administration over the identity database.
func NewMembers(repo *PostgresRepository) *Members {
	return &Members{pool: repo.pool}
}

// List answers everyone in the workspace, whatever their status: an invited
// person is visible before they accept, and a revoked one keeps their row
// because the decisions they recorded stay attributed.
func (m *Members) List(ctx context.Context, tenantID string) ([]Member, error) {
	tx, err := m.begin(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := db.New(tx).ListMembers(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("identity: listing members: %w", err)
	}
	members := make([]Member, 0, len(rows))
	for _, row := range rows {
		members = append(members, Member{
			MembershipID: row.MembershipID, UserID: row.UserID, Email: row.Email,
			Role: row.Role, Status: row.Status, Version: int(row.Version),
			CreatedAt: row.CreatedAt,
		})
	}
	return members, nil
}

// Invite adds an existing account to the workspace as an invited member.
//
// The floor is deliberate: the address must already have an account, and the
// invitation is accepted by selecting the workspace, not by a token in an
// email - the email journey joins when the notification flows own it. A
// revoked person invited again gets their old row back as a fresh
// invitation, keeping the history that is the reason the row survived.
func (m *Members) Invite(ctx context.Context, tenantID, actorID, email, role string) (Member, error) {
	if !assignableRoles[role] {
		return Member{}, ErrMemberRoleInvalid
	}

	tx, err := m.begin(ctx, tenantID)
	if err != nil {
		return Member{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	userID, err := q.FindActiveUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrMemberUnknownEmail
	}
	if err != nil {
		return Member{}, fmt.Errorf("identity: resolving the address: %w", err)
	}

	existing, err := q.FindMembershipByUser(ctx, db.FindMembershipByUserParams{
		UserID: userID, TenantID: tenantID,
	})
	switch {
	case err == nil && existing.Status != "revoked":
		return Member{}, ErrMemberExists
	case err == nil:
		moved, err := q.ReinviteMembership(ctx, db.ReinviteMembershipParams{
			ID: existing.ID, Role: role,
		})
		if err != nil || moved == 0 {
			return Member{}, fmt.Errorf("identity: re-inviting: %w", err)
		}
		if err := m.audit(ctx, q, tenantID, actorID, "tenant.member_invited", existing.ID,
			map[string]string{"role": role, "previous_status": "revoked"}); err != nil {
			return Member{}, err
		}
		return m.commitAndRead(ctx, tx, tenantID, existing.ID)
	case !errors.Is(err, pgx.ErrNoRows):
		return Member{}, fmt.Errorf("identity: checking membership: %w", err)
	}

	membershipID := id.New().String()
	if err := q.InsertInvitedMembership(ctx, db.InsertInvitedMembershipParams{
		ID: membershipID, TenantID: tenantID, UserID: userID, Role: role,
	}); err != nil {
		return Member{}, fmt.Errorf("identity: inviting: %w", err)
	}
	if err := m.audit(ctx, q, tenantID, actorID, "tenant.member_invited", membershipID,
		map[string]string{"role": role}); err != nil {
		return Member{}, err
	}
	return m.commitAndRead(ctx, tx, tenantID, membershipID)
}

// ChangeRole moves one membership to another assignable role.
func (m *Members) ChangeRole(ctx context.Context, tenantID, actorID, membershipID, role string, expectedVersion int) (Member, error) {
	if !assignableRoles[role] {
		return Member{}, ErrMemberRoleInvalid
	}

	tx, err := m.begin(ctx, tenantID)
	if err != nil {
		return Member{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	current, err := q.FindMembershipInTenant(ctx, db.FindMembershipInTenantParams{
		ID: membershipID, TenantID: tenantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrMemberNotFound
	}
	if err != nil {
		return Member{}, fmt.Errorf("identity: reading membership: %w", err)
	}
	if current.Role == "owner" {
		return Member{}, ErrMemberOwner
	}

	moved, err := q.ChangeMembershipRole(ctx, db.ChangeMembershipRoleParams{
		ID: membershipID, Role: role, ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return Member{}, fmt.Errorf("identity: changing role: %w", err)
	}
	if moved == 0 {
		return Member{}, ErrMemberStale
	}

	// The box's exact demand: the audit row carries what the role WAS.
	if err := m.audit(ctx, q, tenantID, actorID, "tenant.member_role_changed", membershipID,
		map[string]string{"role": role, "previous_role": current.Role}); err != nil {
		return Member{}, err
	}
	return m.commitAndRead(ctx, tx, tenantID, membershipID)
}

// Revoke removes a member's access. Effective on their next request, because
// nothing caches what a membership grants.
func (m *Members) Revoke(ctx context.Context, tenantID, actorID, membershipID string, expectedVersion int) error {
	tx, err := m.begin(ctx, tenantID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	current, err := q.FindMembershipInTenant(ctx, db.FindMembershipInTenantParams{
		ID: membershipID, TenantID: tenantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMemberNotFound
	}
	if err != nil {
		return fmt.Errorf("identity: reading membership: %w", err)
	}
	if current.Role == "owner" {
		return ErrMemberOwner
	}
	if current.Status == "revoked" {
		return ErrMemberStale
	}

	moved, err := q.RevokeMembership(ctx, db.RevokeMembershipParams{
		ID: membershipID, ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return fmt.Errorf("identity: revoking: %w", err)
	}
	if moved == 0 {
		return ErrMemberStale
	}

	if err := m.audit(ctx, q, tenantID, actorID, "tenant.member_revoked", membershipID,
		map[string]string{"previous_role": current.Role, "previous_status": current.Status}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ── plumbing

func (m *Members) begin(ctx context.Context, tenantID string) (pgx.Tx, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("identity: beginning member administration: %w", err)
	}
	if err := database.SetTenant(ctx, tx, tenantID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func (m *Members) audit(ctx context.Context, q *db.Queries, tenantID, actorID, action, membershipID string, detail map[string]string) error {
	encoded, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("identity: encoding audit detail: %w", err)
	}
	if err := q.InsertTenantAuditEvent(ctx, db.InsertTenantAuditEventParams{
		ID: id.New().String(), TenantID: tenantID, ActorID: actorID,
		Action: action, SubjectType: "membership", SubjectID: membershipID,
		Outcome: "allowed", Detail: encoded,
	}); err != nil {
		return fmt.Errorf("identity: writing member audit: %w", err)
	}
	return nil
}

func (m *Members) commitAndRead(ctx context.Context, tx pgx.Tx, tenantID, membershipID string) (Member, error) {
	row, err := db.New(tx).FindMembershipInTenant(ctx, db.FindMembershipInTenantParams{
		ID: membershipID, TenantID: tenantID,
	})
	if err != nil {
		return Member{}, fmt.Errorf("identity: reading back: %w", err)
	}
	members, err := db.New(tx).ListMembers(ctx, tenantID)
	if err != nil {
		return Member{}, fmt.Errorf("identity: reading back: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Member{}, err
	}
	for _, member := range members {
		if member.MembershipID == row.ID {
			return Member{
				MembershipID: member.MembershipID, UserID: member.UserID,
				Email: member.Email, Role: member.Role, Status: member.Status,
				Version: int(member.Version), CreatedAt: member.CreatedAt,
			}, nil
		}
	}
	return Member{}, ErrMemberNotFound
}
