//go:build integration

package identity_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
)

// TEN-02 against real PostgreSQL: the member lifecycle end to end, with the
// two properties the ticket's boxes are really about proven on live
// sessions - a role change takes effect on the very next request, and a
// revocation strips a session that is still signed in.

// workspace registers an organisation and returns its service, the owner's
// session token and the tenant id, with the owner's tenant selected. The
// offset keeps two workspaces in one test from colliding on the address.
func workspace(t *testing.T, n int) (*identity.Service, *identity.Members, string, string) {
	t.Helper()
	ctx := context.Background()
	service := newService(t)

	email := nthEmailFor(t, 90+n)
	outcome, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: goodPassword,
		AccountType: identity.AccountOrganisation, OrganisationName: "Members Test Org",
	})
	if err != nil {
		t.Fatalf("registering the organisation: %v", err)
	}
	session, err := service.Authenticate(ctx, email, goodPassword)
	if err != nil {
		t.Fatalf("authenticating the owner: %v", err)
	}
	if err := service.SelectTenant(ctx, session.SessionToken, outcome.TenantID); err != nil {
		t.Fatalf("selecting the workspace: %v", err)
	}
	return service, identity.NewMembers(identity.NewRepository(pool)), session.SessionToken, outcome.TenantID
}

// joiner registers a fresh candidate account and returns its email, user id
// and live session token.
func joiner(t *testing.T, service *identity.Service, n int) (string, identity.Session) {
	t.Helper()
	email := nthEmailFor(t, n)
	session := register(t, service, email)
	return email, session
}

func TestTheMemberLifecycleInviteAcceptWorkChangeRevoke(t *testing.T) {
	ctx := context.Background()
	service, members, ownerToken, tenantID := workspace(t, 0)
	owner, err := service.Lookup(ctx, ownerToken)
	if err != nil {
		t.Fatalf("owner lookup: %v", err)
	}

	email, session := joiner(t, service, 1)

	// Invited, visible immediately, holding nothing yet.
	invited, err := members.Invite(ctx, tenantID, owner.UserID, email, "recruiter")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if invited.Status != "invited" || invited.Role != "recruiter" || invited.Email != email {
		t.Fatalf("invited = %+v", invited)
	}

	// Selecting the workspace accepts the invitation.
	if err := service.SelectTenant(ctx, session.SessionToken, tenantID); err != nil {
		t.Fatalf("accepting by selecting: %v", err)
	}
	granted, err := service.Capabilities(ctx, session.SessionToken)
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if !slices.Contains(granted, authz.CampaignManage) {
		t.Fatal("the accepted recruiter holds no recruiting capability")
	}
	if slices.Contains(granted, authz.TenantMemberManage) {
		t.Fatal("a recruiter can administer members")
	}

	// The acceptance is in the trail.
	var accepted int
	if err := adminPool.QueryRow(ctx, `
		SELECT count(*) FROM audit.events
		WHERE action = 'identity.membership_accepted' AND actor_id = $1`,
		invited.UserID).Scan(&accepted); err != nil {
		t.Fatalf("counting acceptance audit: %v", err)
	}
	if accepted != 1 {
		t.Fatalf("%d acceptance audit rows, want 1", accepted)
	}

	// A role change lands on the next request of the member's LIVE session -
	// no redeploy, no re-login - and the audit row carries the previous role.
	changed, err := members.ChangeRole(ctx, tenantID, owner.UserID, invited.MembershipID, "viewer", invited.Version+1)
	if err != nil {
		t.Fatalf("changing role: %v", err)
	}
	if changed.Role != "viewer" {
		t.Fatalf("changed = %+v", changed)
	}
	granted, err = service.Capabilities(ctx, session.SessionToken)
	if err != nil {
		t.Fatalf("capabilities after change: %v", err)
	}
	if slices.Contains(granted, authz.CampaignManage) {
		t.Fatal("the demoted member still holds campaign.manage on their live session")
	}
	if !slices.Contains(granted, authz.CampaignRead) {
		t.Fatal("the viewer lost even reading")
	}

	var previousRole string
	if err := adminPool.QueryRow(ctx, `
		SELECT detail->>'previous_role' FROM audit.events
		WHERE action = 'tenant.member_role_changed' AND subject_id = $1`,
		invited.MembershipID).Scan(&previousRole); err != nil {
		t.Fatalf("reading the change audit: %v", err)
	}
	if previousRole != "recruiter" {
		t.Fatalf("the audit says the previous role was %q", previousRole)
	}

	// Revocation strips the live session on its very next request.
	if err := members.Revoke(ctx, tenantID, owner.UserID, invited.MembershipID, changed.Version); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	granted, err = service.Capabilities(ctx, session.SessionToken)
	if err != nil {
		t.Fatalf("capabilities after revoke: %v", err)
	}
	for _, capability := range granted {
		if slices.Contains(authz.CapabilitiesOf(authz.RoleViewer), capability) &&
			!slices.Contains(authz.CapabilitiesOf(authz.RoleCandidate), capability) {
			t.Fatalf("the revoked member still holds %s while signed in", capability)
		}
	}

	// And the one policy path refuses them by name.
	if _, err := service.Authorize(ctx, session.SessionToken, authz.CampaignRead); err == nil {
		t.Fatal("Authorize allowed a revoked member")
	}
}

func TestTheRefusalsHoldTheLine(t *testing.T) {
	ctx := context.Background()
	service, members, ownerToken, tenantID := workspace(t, 0)
	owner, _ := service.Lookup(ctx, ownerToken)

	// An address with no account is refused; the invitation floor is honest.
	if _, err := members.Invite(ctx, tenantID, owner.UserID, "nobody@example.com", "recruiter"); !errors.Is(err, identity.ErrMemberUnknownEmail) {
		t.Fatalf("unknown address = %v, want ErrMemberUnknownEmail", err)
	}
	// The owner role is not assignable here.
	email, _ := joiner(t, service, 2)
	if _, err := members.Invite(ctx, tenantID, owner.UserID, email, "owner"); !errors.Is(err, identity.ErrMemberRoleInvalid) {
		t.Fatalf("assigning owner = %v, want ErrMemberRoleInvalid", err)
	}

	invited, err := members.Invite(ctx, tenantID, owner.UserID, email, "hiring_manager")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	// Inviting again is a conflict, not a duplicate row.
	if _, err := members.Invite(ctx, tenantID, owner.UserID, email, "viewer"); !errors.Is(err, identity.ErrMemberExists) {
		t.Fatalf("double invite = %v, want ErrMemberExists", err)
	}
	// A stale version is refused rather than silently overwriting.
	if _, err := members.ChangeRole(ctx, tenantID, owner.UserID, invited.MembershipID, "viewer", invited.Version+7); !errors.Is(err, identity.ErrMemberStale) {
		t.Fatalf("stale change = %v, want ErrMemberStale", err)
	}

	// The owner's own membership is untouchable through this surface.
	listed, err := members.List(ctx, tenantID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var ownerMembership identity.Member
	for _, member := range listed {
		if member.Role == "owner" {
			ownerMembership = member
		}
	}
	if ownerMembership.MembershipID == "" {
		t.Fatal("the owner is missing from the member list")
	}
	if _, err := members.ChangeRole(ctx, tenantID, owner.UserID, ownerMembership.MembershipID, "viewer", ownerMembership.Version); !errors.Is(err, identity.ErrMemberOwner) {
		t.Fatalf("demoting the owner = %v, want ErrMemberOwner", err)
	}
	if err := members.Revoke(ctx, tenantID, owner.UserID, ownerMembership.MembershipID, ownerMembership.Version); !errors.Is(err, identity.ErrMemberOwner) {
		t.Fatalf("revoking the owner = %v, want ErrMemberOwner", err)
	}
}

func TestARevokedMemberReinvitedGetsTheirRowBack(t *testing.T) {
	ctx := context.Background()
	service, members, ownerToken, tenantID := workspace(t, 0)
	owner, _ := service.Lookup(ctx, ownerToken)
	email, _ := joiner(t, service, 3)

	first, err := members.Invite(ctx, tenantID, owner.UserID, email, "recruiter")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if err := members.Revoke(ctx, tenantID, owner.UserID, first.MembershipID, first.Version); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	second, err := members.Invite(ctx, tenantID, owner.UserID, email, "viewer")
	if err != nil {
		t.Fatalf("re-invite: %v", err)
	}
	if second.MembershipID != first.MembershipID {
		t.Fatal("the re-invitation made a new row; the history the old one carried is now orphaned")
	}
	if second.Status != "invited" || second.Role != "viewer" {
		t.Fatalf("re-invited = %+v", second)
	}
}

func TestMemberAdministrationIsConfinedToItsTenant(t *testing.T) {
	ctx := context.Background()
	service, members, ownerToken, tenantID := workspace(t, 0)
	owner, _ := service.Lookup(ctx, ownerToken)
	_, otherMembers, otherToken, otherTenant := workspace(t, 1)
	other, _ := service.Lookup(ctx, otherToken)

	email, _ := joiner(t, service, 4)
	invited, err := members.Invite(ctx, tenantID, owner.UserID, email, "recruiter")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}

	// The other workspace's administrator cannot see or touch the row: the
	// tenant scope makes it not exist for them.
	listed, err := otherMembers.List(ctx, otherTenant)
	if err != nil {
		t.Fatalf("other list: %v", err)
	}
	for _, member := range listed {
		if member.MembershipID == invited.MembershipID {
			t.Fatal("a membership leaked across tenants")
		}
	}
	if _, err := otherMembers.ChangeRole(ctx, otherTenant, other.UserID, invited.MembershipID, "viewer", invited.Version); !errors.Is(err, identity.ErrMemberNotFound) {
		t.Fatalf("cross-tenant change = %v, want ErrMemberNotFound", err)
	}
}
