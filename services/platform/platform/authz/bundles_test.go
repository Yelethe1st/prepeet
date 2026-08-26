package authz_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
)

/*
Roles as bundles.

A role is never a check. Nothing in the product asks whether somebody is an
owner; it asks whether they hold a capability, and a role is only how they came
to hold it. These assert the properties that make that safe, across every bundle
rather than in the ones somebody thought to check.
*/

func TestEveryRoleGrantsSomething(t *testing.T) {
	t.Parallel()

	for _, role := range authz.Roles() {
		if len(authz.CapabilitiesOf(role)) == 0 {
			t.Errorf("role %q grants nothing, which is either a mistake or a role that should not exist", role)
		}
	}
}

func TestAnUnknownRoleGrantsNothing(t *testing.T) {
	t.Parallel()

	// Deny by default reaches here too. A typo in a role name must grant
	// nothing rather than everything or the first bundle in a map.
	if granted := authz.CapabilitiesOf(authz.Role("administrator")); len(granted) != 0 {
		t.Errorf("an unknown role granted %v", granted)
	}
}

/*
The property the product rests on, checked from the bundle side.

A candidate's own data is reachable only by that candidate. If any membership
role granted an owner-scoped capability, tenant authority would reach practice
history, which is the failure this product cannot have.
*/
func TestNoMembershipRoleGrantsAnOwnerCapability(t *testing.T) {
	t.Parallel()

	for _, role := range authz.Roles() {
		if !authz.RoleRequiresMembership(role) {
			continue
		}

		for _, capability := range authz.CapabilitiesOf(role) {
			requirement, _ := authz.RequirementOf(capability)
			if requirement.Owner {
				t.Errorf("role %q is held through a tenant membership and grants %q, which is "+
					"a candidate's own data", role, capability)
			}
		}
	}
}

// And the reverse: the bundle held with no tenant reaches only its holder.
func TestTheUntenantedRoleGrantsOnlyOwnCapabilities(t *testing.T) {
	t.Parallel()

	found := 0
	for _, role := range authz.Roles() {
		if authz.RoleRequiresMembership(role) {
			continue
		}
		found++

		for _, capability := range authz.CapabilitiesOf(role) {
			requirement, _ := authz.RequirementOf(capability)
			if !requirement.Owner {
				t.Errorf("role %q is held without a tenant and grants %q, which is not owner-scoped",
					role, capability)
			}
		}
	}

	if found == 0 {
		t.Fatal("no untenanted role exists, so a candidate belonging to no workspace can do nothing")
	}
}

/*
Platform authority is separate from tenant authority, never a senior form of it.

A tenant role granting a platform capability would make the owner of any
workspace a member of platform staff, which is exactly what the separation
exists to prevent.
*/
func TestNoRoleGrantsPlatformAuthority(t *testing.T) {
	t.Parallel()

	for _, role := range authz.Roles() {
		for _, capability := range authz.CapabilitiesOf(role) {
			requirement, _ := authz.RequirementOf(capability)
			if requirement.Platform {
				t.Errorf("role %q grants platform capability %q", role, capability)
			}
		}
	}
}

// An owner administers the workspace, so anything a member can do they can do.
// A member holding something an owner does not would be a privilege a workspace
// administrator cannot exercise or revoke.
func TestTheAdministratorHoldsEverythingAnyMembershipRoleDoes(t *testing.T) {
	t.Parallel()

	admin := authz.CapabilitiesOf(authz.RoleAdmin)

	for _, role := range []authz.Role{authz.RoleRecruiter, authz.RoleHiringManager, authz.RoleViewer} {
		for _, capability := range authz.CapabilitiesOf(role) {
			if !slices.Contains(admin, capability) {
				t.Errorf("a %s holds %q and the administrator does not, so the workspace "+
					"administrator cannot exercise or revoke it", role, capability)
			}
		}
	}
}

func TestOwnerAndAdminHoldIdenticalCapability(t *testing.T) {
	// What distinguishes an owner is being anchored to the workspace's
	// creation, not holding more: a capability only one of them held would
	// be authority that appears or vanishes on a rename.
	t.Parallel()

	owner := authz.CapabilitiesOf(authz.RoleOwner)
	admin := authz.CapabilitiesOf(authz.RoleAdmin)
	if len(owner) != len(admin) {
		t.Fatalf("owner holds %d capabilities and admin %d", len(owner), len(admin))
	}
	for _, capability := range owner {
		if !slices.Contains(admin, capability) {
			t.Errorf("owner holds %q and admin does not", capability)
		}
	}
}

func TestTheViewerCanChangeNothing(t *testing.T) {
	// Read-only means read-only: a viewer capability that manages, reviews,
	// raises or publishes would make oversight indistinguishable from
	// authority - the prototype's matrix shows the row as all reads.
	t.Parallel()

	for _, capability := range authz.CapabilitiesOf(authz.RoleViewer) {
		name := string(capability)
		if !strings.HasSuffix(name, ".read") && !strings.HasSuffix(name, ".read_screen") &&
			!strings.HasSuffix(name, "_read") {
			t.Errorf("the viewer holds %q, which is not a read", name)
		}
	}
}

func TestARecruiterRaisesButNeverResolves(t *testing.T) {
	// The matrix's one asymmetric row: raising a re-review is asking a
	// question; resolving one answers for the workspace and needs the
	// hiring manager or above.
	t.Parallel()

	recruiter := authz.CapabilitiesOf(authz.RoleRecruiter)
	if !slices.Contains(recruiter, authz.AppealRaise) {
		t.Error("a recruiter cannot raise a re-review")
	}
	if slices.Contains(recruiter, authz.AppealManage) {
		t.Error("a recruiter can resolve the re-reviews they raise")
	}
	if !slices.Contains(authz.CapabilitiesOf(authz.RoleHiringManager), authz.AppealManage) {
		t.Error("nobody below admin can resolve a re-review")
	}
}

// Comparison is off by default under responsible-hiring.md, and whether it ships
// at all is DEC-17. A bundle granting it would turn it on for every workspace.
func TestComparisonIsInNoBundle(t *testing.T) {
	t.Parallel()

	for _, role := range authz.Roles() {
		for _, capability := range authz.CapabilitiesOf(role) {
			if strings.Contains(string(capability), "compare") {
				t.Errorf("role %q grants %q, which responsible-hiring.md says is off by default",
					role, capability)
			}
		}
	}
}

// The bundles a caller reads must not be the ones the package uses, or a caller
// could grant itself authority by appending to what it was handed.
func TestCapabilitiesOfReturnsACopy(t *testing.T) {
	t.Parallel()

	granted := authz.CapabilitiesOf(authz.RoleRecruiter)
	if len(granted) == 0 {
		t.Fatal("member grants nothing")
	}

	granted[0] = "tampered"

	if authz.CapabilitiesOf(authz.RoleRecruiter)[0] == "tampered" {
		t.Error("the caller was handed the package's own slice, so a caller can rewrite a role")
	}
}
