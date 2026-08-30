//go:build integration

// The attacks. Northwind, acting entirely within its rights, reaches for a
// row that belongs to Orbital.
//
// Every attack in this file obeys three rules, because an attack that breaks
// any of them can pass while the guard it claims to test is absent.
//
// It is scoped. The identifier it names is a row that exists, under the other
// tenant, right now. An unscoped attempt - list everything, delete where a
// condition matches nothing - reports zero rows whether it was refused or
// whether it simply found nothing, and this project has already been caught by
// exactly that.
//
// It is otherwise valid. The version guard is satisfied, the capability is
// held, the session is live, and the target is not an owner, since member
// administration refuses to touch an owner row for reasons that have nothing
// to do with tenancy. Every refusal an attack could earn for some other reason
// is removed first, so the only thing left to refuse it is the boundary.
//
// It has a control. The same operation, on the same row, with the same
// arguments, performed by the tenant that owns it, and it succeeds. A refusal
// without that is indistinguishable from an attack that missed.
package isolation_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

// invitations gives each freshly invited target its own address. Tests share a
// database, and a second invitation of the same person is a different flow
// with a different outcome.
var invitations = 0

// freshTarget invites a new recruiter into Orbital and returns the membership.
//
// One per attack rather than one for the file: the controls below deliberately
// succeed, and a shared row would leave a later attack aiming at a membership
// that an earlier control had already revoked or renamed - which is another way
// of aiming at nothing.
func freshTarget(t *testing.T) identity.Member {
	t.Helper()
	ctx := context.Background()

	invitations++
	email := fmt.Sprintf("recruit-%d@orbital.example", invitations)
	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: password, AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("registering the person to invite: %v", err)
	}

	invited, err := members.Invite(ctx, orbital.tenantID, orbital.ownerID, email, "recruiter")
	if err != nil {
		t.Fatalf("inviting into Orbital: %v", err)
	}
	if invited.Role == "owner" {
		t.Fatal("the target is an owner, which every operation below refuses for its own reasons")
	}
	return invited
}

// roleOf reads a membership's role from outside every policy, which is the
// only place from which "the row is untouched" can be asserted: the tenant
// that must not see it cannot see it, and the tenant that owns it would be
// answering about its own view.
func roleOf(t *testing.T, membershipID string) (role, status string) {
	t.Helper()
	if err := adminPool.QueryRow(context.Background(),
		`SELECT role, status FROM tenancy.memberships WHERE id = $1`, membershipID).
		Scan(&role, &status); err != nil {
		t.Fatalf("reading the membership as the migrator: %v", err)
	}
	return role, status
}

// ──────────────────────────────────────────────── layer 1: the HTTP handler

// The classic insecure direct object reference, with everything else correct.
//
// Northwind's owner holds tenant.member_manage, presents a live session, and
// sends the version the row is actually at. The only thing wrong with the
// request is whose membership it names.
func TestTheHandlerRefusesToChangeAForeignWorkspacesMember(t *testing.T) {
	victim := freshTarget(t)
	body := fmt.Sprintf(`{"role":"viewer","expected_version":%d}`, victim.Version)

	attack := request(t, northwind.sessionToken, http.MethodPatch,
		"/tenant/members/"+victim.MembershipID, body)

	if attack.Code != http.StatusNotFound {
		t.Errorf("PATCH of Orbital's membership from Northwind answered %d, want 404.\n"+
			"    body: %s", attack.Code, attack.Body.String())
	}
	if role, _ := roleOf(t, victim.MembershipID); role != "recruiter" {
		t.Errorf("the membership is now %q: the request that was refused still changed the row", role)
	}

	// The control. Same row, same version, same body: Orbital's own owner.
	// Without this, a 404 could mean the identifier was wrong, the version was
	// stale, or the row had never existed.
	control := request(t, orbital.sessionToken, http.MethodPatch,
		"/tenant/members/"+victim.MembershipID, body)
	if control.Code != http.StatusOK {
		t.Fatalf("the same request from the workspace that owns the row answered %d, want 200.\n"+
			"    The attack above proved nothing: this operation does not work.\n    body: %s",
			control.Code, control.Body.String())
	}
	if role, _ := roleOf(t, victim.MembershipID); role != "viewer" {
		t.Fatalf("the control did not change the row, so the attack had nothing to succeed at")
	}
}

// The same shape, destructively. Revocation is the operation an attacker would
// actually want: it removes a rival workspace's recruiter.
func TestTheHandlerRefusesToRevokeAForeignWorkspacesMember(t *testing.T) {
	victim := freshTarget(t)
	path := fmt.Sprintf("/tenant/members/%s?expectedVersion=%d", victim.MembershipID, victim.Version)

	attack := request(t, northwind.sessionToken, http.MethodDelete, path, "")

	if attack.Code != http.StatusNotFound {
		t.Errorf("DELETE of Orbital's membership from Northwind answered %d, want 404.\n"+
			"    body: %s", attack.Code, attack.Body.String())
	}
	if _, status := roleOf(t, victim.MembershipID); status == "revoked" {
		t.Error("the membership is revoked: the request that was refused still took effect")
	}

	control := request(t, orbital.sessionToken, http.MethodDelete, path, "")
	if control.Code != http.StatusNoContent {
		t.Fatalf("the same revocation from the workspace that owns the row answered %d, want 204.\n"+
			"    The attack above proved nothing.\n    body: %s", control.Code, control.Body.String())
	}
	if _, status := roleOf(t, victim.MembershipID); status != "revoked" {
		t.Fatal("the control did not revoke the row, so the attack had nothing to succeed at")
	}
}

// Listing is the leak that needs no identifier: one endpoint, and the answer
// either contains the other workspace's people or it does not.
func TestTheHandlerListsOnlyTheSessionsOwnWorkspace(t *testing.T) {
	victim := freshTarget(t)

	listed := request(t, northwind.sessionToken, http.MethodGet, "/tenant/members", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("listing Northwind's members answered %d: %s", listed.Code, listed.Body.String())
	}

	var body struct {
		Members []struct {
			MembershipID string `json:"membership_id"`
			Email        string `json:"email"`
		} `json:"members"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the member list: %v", err)
	}
	if len(body.Members) == 0 {
		t.Fatal("Northwind's own list is empty, so this test would pass with every row hidden")
	}
	for _, member := range body.Members {
		if member.MembershipID == victim.MembershipID {
			t.Errorf("Northwind's member list contains Orbital's membership %s", member.MembershipID)
		}
		if member.Email == orbital.ownerEmail {
			t.Errorf("Northwind's member list contains Orbital's owner")
		}
	}

	// The control: the row this list must not contain is one Orbital's own
	// list does contain.
	owned := request(t, orbital.sessionToken, http.MethodGet, "/tenant/members", "")
	if err := json.Unmarshal(owned.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding Orbital's member list: %v", err)
	}
	found := false
	for _, member := range body.Members {
		if member.MembershipID == victim.MembershipID {
			found = true
		}
	}
	if !found {
		t.Fatal("Orbital cannot see its own invited recruiter, so the assertion above was vacuous")
	}
}

// The other way in: rather than naming a foreign row, name the foreign
// workspace and have the session act under it. This is the one endpoint that
// takes a tenant identifier from the client at all, which is why the
// membership check lives here.
func TestTheHandlerRefusesToActUnderAWorkspaceYouDoNotBelongTo(t *testing.T) {
	ctx := context.Background()
	body := fmt.Sprintf(`{"tenant_id":%q}`, orbital.tenantID)

	attack := request(t, northwind.sessionToken, http.MethodPut, "/me/active-tenant", body)

	// 403 rather than 404, per the contract: the workspace exists, and saying
	// otherwise would make this endpoint a test for which identifiers are real.
	if attack.Code != http.StatusForbidden {
		t.Errorf("selecting Orbital from a Northwind session answered %d, want 403.\n    body: %s",
			attack.Code, attack.Body.String())
	}

	// The session must still be acting under Northwind. A refusal that left the
	// selection changed would be worse than one that answered 200.
	after, err := service.Lookup(ctx, northwind.sessionToken)
	if err != nil {
		t.Fatalf("looking the session up afterwards: %v", err)
	}
	if after.ActiveTenantID != northwind.tenantID {
		t.Errorf("the session now acts under %q, want Northwind's %q",
			after.ActiveTenantID, northwind.tenantID)
	}

	// The attempt is on the record. A probe for workspaces somebody does not
	// belong to is the shape this trail exists to show.
	var denials int
	if err := adminPool.QueryRow(ctx, `
		SELECT count(*) FROM audit.events
		WHERE actor_id = $1 AND action = 'identity.tenant_selected'
		  AND outcome = 'denied' AND subject_id = $2`,
		northwind.ownerID, orbital.tenantID).Scan(&denials); err != nil {
		t.Fatalf("counting the refusal in the audit trail: %v", err)
	}
	if denials == 0 {
		t.Error("the refused selection left no audit row, so a campaign of probes would be invisible")
	}

	// The control: the same call, for a workspace the caller does belong to.
	control := request(t, orbital.sessionToken, http.MethodPut, "/me/active-tenant", body)
	if control.Code != http.StatusOK {
		t.Fatalf("selecting Orbital from Orbital's own session answered %d, want 200.\n"+
			"    The refusal above proved nothing.\n    body: %s", control.Code, control.Body.String())
	}
}

// ────────────────────────────────────── layer 2: the bounded context

// Under the handler, the context is called with a tenant and a membership that
// do not belong together. This is what a bug in any future handler looks like
// from here, and what an internal caller could do by hand.
func TestTheContextRefusesToChangeAMembershipInAnotherTenant(t *testing.T) {
	ctx := context.Background()
	victim := freshTarget(t)

	_, err := members.ChangeRole(ctx, northwind.tenantID, northwind.ownerID,
		victim.MembershipID, "viewer", victim.Version)
	if !errors.Is(err, identity.ErrMemberNotFound) {
		t.Errorf("ChangeRole across tenants returned %v, want ErrMemberNotFound", err)
	}
	if role, _ := roleOf(t, victim.MembershipID); role != "recruiter" {
		t.Errorf("the membership is now %q, so the refusal was not a refusal", role)
	}

	changed, err := members.ChangeRole(ctx, orbital.tenantID, orbital.ownerID,
		victim.MembershipID, "viewer", victim.Version)
	if err != nil {
		t.Fatalf("the same change under the owning tenant failed with %v.\n"+
			"    The attack above proved nothing: these arguments do not work.", err)
	}
	if changed.Role != "viewer" {
		t.Fatalf("the control changed nothing, so there was nothing to defend")
	}
}

func TestTheContextRefusesToRevokeAMembershipInAnotherTenant(t *testing.T) {
	ctx := context.Background()
	victim := freshTarget(t)

	err := members.Revoke(ctx, northwind.tenantID, northwind.ownerID,
		victim.MembershipID, victim.Version)
	if !errors.Is(err, identity.ErrMemberNotFound) {
		t.Errorf("Revoke across tenants returned %v, want ErrMemberNotFound", err)
	}
	if _, status := roleOf(t, victim.MembershipID); status == "revoked" {
		t.Error("the membership is revoked, so the refusal was not a refusal")
	}

	if err := members.Revoke(ctx, orbital.tenantID, orbital.ownerID,
		victim.MembershipID, victim.Version); err != nil {
		t.Fatalf("the same revocation under the owning tenant failed with %v.\n"+
			"    The attack above proved nothing.", err)
	}
	if _, status := roleOf(t, victim.MembershipID); status != "revoked" {
		t.Fatal("the control revoked nothing, so there was nothing to defend")
	}
}

// The refusals above are indistinguishable from absence, and deliberately so:
// a distinguishable one would answer whether an identifier is real to somebody
// who may not know.
func TestACrossTenantMembershipIsRefusedExactlyLikeOneThatDoesNotExist(t *testing.T) {
	ctx := context.Background()
	victim := freshTarget(t)

	foreign := members.Revoke(ctx, northwind.tenantID, northwind.ownerID,
		victim.MembershipID, victim.Version)
	imaginary := members.Revoke(ctx, northwind.tenantID, northwind.ownerID,
		"00000000-0000-7000-8000-0000000000ff", victim.Version)

	if foreign == nil || imaginary == nil {
		t.Fatalf("one of the two was allowed: foreign=%v imaginary=%v", foreign, imaginary)
	}
	if foreign.Error() != imaginary.Error() {
		t.Errorf("a real membership in another tenant refuses differently from one that does "+
			"not exist:\n    foreign:   %v\n    imaginary: %v\n"+
			"    The difference is an oracle for which identifiers are real.", foreign, imaginary)
	}
}

func TestTheContextListsOnlyTheTenantItWasAsked(t *testing.T) {
	ctx := context.Background()
	victim := freshTarget(t)

	listed, err := members.List(ctx, northwind.tenantID)
	if err != nil {
		t.Fatalf("listing Northwind: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("Northwind has no members at all, so this test would pass with everything hidden")
	}
	for _, member := range listed {
		if member.MembershipID == victim.MembershipID {
			t.Errorf("listing Northwind returned Orbital's membership %s", member.MembershipID)
		}
	}

	owned, err := members.List(ctx, orbital.tenantID)
	if err != nil {
		t.Fatalf("listing Orbital: %v", err)
	}
	found := false
	for _, member := range owned {
		if member.MembershipID == victim.MembershipID {
			found = true
		}
	}
	if !found {
		t.Fatal("Orbital's own list does not contain the row, so the assertion above was vacuous")
	}
}

// ──────────────────────────────────────── layer 3: the database under RLS

// withTenant runs fn in a transaction scoped to one tenant, which is how every
// request in the product talks to the database, and rolls it back.
func withTenant(t *testing.T, tenantID string, fn func(pgx.Tx)) {
	t.Helper()
	ctx := context.Background()

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := database.SetTenant(ctx, tx, tenantID); err != nil {
		t.Fatalf("SetTenant: %v", err)
	}
	fn(tx)
}

// The layer that has to hold when the two above have not. Every statement here
// names the other tenant's row by primary key, which is the strongest form the
// attack has: nothing is being filtered out by a predicate the attacker chose.
func TestRowLevelSecurityHidesAForeignMembershipNamedByItsPrimaryKey(t *testing.T) {
	ctx := context.Background()
	victim := freshTarget(t)

	withTenant(t, northwind.tenantID, func(tx pgx.Tx) {
		var seen int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM tenancy.memberships WHERE id = $1`, victim.MembershipID).
			Scan(&seen); err != nil {
			t.Fatalf("counting: %v", err)
		}
		if seen != 0 {
			t.Errorf("Northwind can see %d of Orbital's membership rows by primary key", seen)
		}
	})

	// The control, in a transaction that is rolled back: the same statement,
	// under the tenant that owns the row, finds it.
	withTenant(t, orbital.tenantID, func(tx pgx.Tx) {
		var seen int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM tenancy.memberships WHERE id = $1`, victim.MembershipID).
			Scan(&seen); err != nil {
			t.Fatalf("counting: %v", err)
		}
		if seen != 1 {
			t.Fatalf("Orbital sees %d rows at that identifier, want 1: the attack above was "+
				"aimed at nothing", seen)
		}
	})
}

func TestRowLevelSecurityRefusesAWriteToAForeignMembership(t *testing.T) {
	ctx := context.Background()
	victim := freshTarget(t)

	withTenant(t, northwind.tenantID, func(tx pgx.Tx) {
		// The write an attacker would want most: promote a membership they
		// control, or demote one they do not.
		tag, err := tx.Exec(ctx,
			`UPDATE tenancy.memberships SET role = 'owner' WHERE id = $1`, victim.MembershipID)
		if err != nil {
			return // refused outright is also correct
		}
		if tag.RowsAffected() != 0 {
			t.Errorf("Northwind updated %d of Orbital's membership rows", tag.RowsAffected())
		}
	})

	withTenant(t, northwind.tenantID, func(tx pgx.Tx) {
		tag, err := tx.Exec(ctx,
			`DELETE FROM tenancy.memberships WHERE id = $1`, victim.MembershipID)
		if err != nil {
			return
		}
		if tag.RowsAffected() != 0 {
			t.Errorf("Northwind deleted %d of Orbital's membership rows", tag.RowsAffected())
		}
	})

	if role, status := roleOf(t, victim.MembershipID); role != "recruiter" || status != "invited" {
		t.Errorf("the row is now role=%q status=%q, so something above went through", role, status)
	}

	// The control: under Orbital the same update finds the row, then is rolled
	// back so the world is as the next test expects it.
	withTenant(t, orbital.tenantID, func(tx pgx.Tx) {
		tag, err := tx.Exec(ctx,
			`UPDATE tenancy.memberships SET role = 'viewer' WHERE id = $1`, victim.MembershipID)
		if err != nil {
			t.Fatalf("the owning tenant could not update its own row: %v", err)
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("the owning tenant updated %d rows, want 1: the attacks above were aimed "+
				"at a row nothing can write", tag.RowsAffected())
		}
	})
}

// Writing into another tenant, rather than over it: the row would be
// Northwind's to create and Orbital's to live with.
func TestRowLevelSecurityRefusesAnInsertIntoAnotherTenant(t *testing.T) {
	ctx := context.Background()

	withTenant(t, northwind.tenantID, func(tx pgx.Tx) {
		_, err := tx.Exec(ctx,
			`INSERT INTO tenancy.memberships (id, tenant_id, user_id, status, role)
			 VALUES (gen_random_uuid(), $1, $2, 'active', 'admin')`,
			orbital.tenantID, northwind.ownerID)
		if err == nil {
			t.Error("Northwind's owner granted themselves an admin membership of Orbital")
		}
	})

	var intruders int
	if err := adminPool.QueryRow(ctx,
		`SELECT count(*) FROM tenancy.memberships WHERE tenant_id = $1 AND user_id = $2`,
		orbital.tenantID, northwind.ownerID).Scan(&intruders); err != nil {
		t.Fatalf("checking Orbital's memberships: %v", err)
	}
	if intruders != 0 {
		t.Errorf("Northwind's owner holds %d memberships of Orbital", intruders)
	}
}

// The audit trail is the record of what a workspace's administrators did, and
// is at least as sensitive as the rows it describes. The attack names a row
// that exists, by primary key, again.
func TestRowLevelSecurityHidesAForeignWorkspacesAuditTrail(t *testing.T) {
	ctx := context.Background()
	victim := freshTarget(t)

	// The invitation above wrote Orbital's audit row. Read its identifier from
	// outside every policy, so the attack can name it.
	var eventID string
	if err := adminPool.QueryRow(ctx, `
		SELECT id::text FROM audit.events
		WHERE tenant_id = $1 AND subject_id = $2 AND action = 'tenant.member_invited'`,
		orbital.tenantID, victim.MembershipID).Scan(&eventID); err != nil {
		t.Fatalf("finding Orbital's audit row: %v", err)
	}

	withTenant(t, northwind.tenantID, func(tx pgx.Tx) {
		var seen int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM audit.events WHERE id = $1`, eventID).Scan(&seen); err != nil {
			t.Fatalf("counting: %v", err)
		}
		if seen != 0 {
			t.Errorf("Northwind can read %d rows of Orbital's audit trail by identifier", seen)
		}
	})

	withTenant(t, orbital.tenantID, func(tx pgx.Tx) {
		var seen int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM audit.events WHERE id = $1`, eventID).Scan(&seen); err != nil {
			t.Fatalf("counting: %v", err)
		}
		if seen != 1 {
			t.Fatalf("Orbital sees %d of its own audit rows at that identifier, want 1: the "+
				"attack above was aimed at nothing", seen)
		}
	})
}
