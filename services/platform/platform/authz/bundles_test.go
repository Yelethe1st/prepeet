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
func TestAnOwnerHoldsEverythingAMemberDoes(t *testing.T) {
	t.Parallel()

	owner := authz.CapabilitiesOf(authz.RoleOwner)

	for _, capability := range authz.CapabilitiesOf(authz.RoleMember) {
		if !slices.Contains(owner, capability) {
			t.Errorf("a member holds %q and an owner does not, so the workspace administrator "+
				"cannot exercise or revoke it", capability)
		}
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

	granted := authz.CapabilitiesOf(authz.RoleMember)
	if len(granted) == 0 {
		t.Fatal("member grants nothing")
	}

	granted[0] = "tampered"

	if authz.CapabilitiesOf(authz.RoleMember)[0] == "tampered" {
		t.Error("the caller was handed the package's own slice, so a caller can rewrite a role")
	}
}
