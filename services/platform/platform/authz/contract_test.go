package authz_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
)

/*
The catalogue against the contract it is generated from.

The generator already guarantees they agree, since one produces the other. What
these check is what the generator cannot: that the contract still says what the
authorization model requires it to say, and that the properties the model
depends on hold across every entry rather than in the ones somebody thought to
test.
*/

// contract reads the source document. Read as text rather than parsed, because
// what is asserted here is about the document a person reviews.
func contract(t *testing.T) string {
	t.Helper()

	// Four levels up from platform/authz to the repository root.
	path := filepath.Join("..", "..", "..", "..", "packages", "contracts", "authz", "capabilities.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the capability contract: %v", err)
	}
	return string(raw)
}

var contractEntry = regexp.MustCompile(`(?m)^  - name: (\S+)`)

func contractNames(t *testing.T) map[string]bool {
	t.Helper()

	names := map[string]bool{}
	for _, match := range contractEntry.FindAllStringSubmatch(contract(t), -1) {
		names[match[1]] = true
	}
	if len(names) == 0 {
		t.Fatal("the contract declares no capabilities, so this test proves nothing")
	}
	return names
}

// Every capability the code knows is in the document, and the reverse. A
// capability in one and not the other is authority that either cannot be
// granted or cannot be reviewed.
func TestTheCatalogueAndTheContractDescribeTheSameCapabilities(t *testing.T) {
	t.Parallel()

	declared := contractNames(t)

	for _, capability := range authz.Catalogue() {
		if !declared[string(capability)] {
			t.Errorf("%s exists in code and not in the contract, so it is authority nobody reviewed",
				capability)
		}
		delete(declared, string(capability))
	}

	for name := range declared {
		t.Errorf("%s is in the contract and not in code, so it can never be granted", name)
	}
}

/*
Every capability carries a reason.

The generator refuses to emit one without a reason, so this is a second check on
the same rule, and it is worth having because the two fail differently: the
generator fails the build for whoever added the capability, and this fails for
whoever removes a reason later, when the capability already exists and the build
would otherwise be quite happy.
*/
func TestEveryCapabilityInTheContractCarriesAReason(t *testing.T) {
	t.Parallel()

	document := contract(t)
	blocks := strings.Split(document, "  - name: ")[1:]

	if len(blocks) == 0 {
		t.Fatal("no capability blocks were found")
	}

	for _, block := range blocks {
		name := strings.Fields(block)[0]
		if !strings.Contains(block, "reason:") {
			t.Errorf("%s has no reason, so its requirements are a rule nobody can argue against", name)
		}
	}
}

/*
The property the whole product rests on.

A candidate's practice data must be unreachable from tenant authority. In the
catalogue that means every capability over a candidate's own data requires
owner, and requires it alone: adding tenant to one of these would make it
satisfiable by an employer, which is the failure this product cannot have.
*/
func TestCandidateOwnDataCapabilitiesAreOwnerOnly(t *testing.T) {
	t.Parallel()

	found := 0
	for _, capability := range authz.Catalogue() {
		if !strings.HasPrefix(string(capability), "candidate.") {
			continue
		}
		found++

		requirement, known := authz.RequirementOf(capability)
		if !known {
			t.Fatalf("%s is in the catalogue and has no requirement", capability)
		}
		if !requirement.Owner {
			t.Errorf("%s does not require owner, so it is not the candidate's alone", capability)
		}
		if requirement.Tenant {
			t.Errorf("%s requires tenant, which makes a candidate's own data reachable from an employer",
				capability)
		}
	}

	if found == 0 {
		t.Fatal("no candidate capabilities were found, so this asserted nothing")
	}
}

/*
Platform authority is separate from tenant authority, never above it.

A platform capability that also required a tenant would be a way for platform
staff to act inside a workspace as though they were a member of it, which is
exactly the thing the separation exists to prevent.
*/
func TestPlatformCapabilitiesAreNotAlsoTenantCapabilities(t *testing.T) {
	t.Parallel()

	found := 0
	for _, capability := range authz.Catalogue() {
		requirement, _ := authz.RequirementOf(capability)
		if !requirement.Platform {
			continue
		}
		found++

		if requirement.Tenant {
			t.Errorf("%s is both platform and tenant authority, which makes platform staff members "+
				"of every workspace", capability)
		}
	}

	if found == 0 {
		t.Fatal("no platform capabilities were found")
	}
}

/*
A name describing an interface element would have to change when the element is
renamed, and the two have no reason to move together.

Matched on whole segments rather than substrings, which the first version got
wrong: "view" is inside "review", so evaluation.review was reported as named
after a UI element.

"screen" is deliberately absent from the list even though it is a UI word,
because in this product it is a domain one: a screening interview is a mode,
opposite to practice, and evaluation.read_screen is authority over a screening
evaluation rather than over a page. A check that forbade it would be forbidding
the vocabulary the product is written in.
*/
func TestNoCapabilityIsNamedAfterAnInterfaceElement(t *testing.T) {
	t.Parallel()

	forbidden := map[string]bool{
		"page": true, "button": true, "modal": true, "dialog": true,
		"tab": true, "menu": true, "sidebar": true, "dashboard": true,
		"widget": true, "panel": true, "view": true, "popup": true,
		"drawer": true, "banner": true, "card": true,
	}

	for _, capability := range authz.Catalogue() {
		for _, segment := range strings.FieldsFunc(string(capability), func(r rune) bool {
			return r == '.' || r == '_'
		}) {
			if forbidden[strings.ToLower(segment)] {
				t.Errorf("%s contains the segment %q, which names an interface element rather than authority",
					capability, segment)
			}
		}
	}
}

// The version exists so a consumer can tell whether it is reading a catalogue
// it understands. A version that never moves is a field nobody checks.
func TestTheCatalogueIsVersioned(t *testing.T) {
	t.Parallel()

	if authz.CatalogueVersion < 1 {
		t.Errorf("CatalogueVersion = %d, want at least 1", authz.CatalogueVersion)
	}
	if !strings.Contains(contract(t), "version:") {
		t.Error("the contract declares no version")
	}
}
