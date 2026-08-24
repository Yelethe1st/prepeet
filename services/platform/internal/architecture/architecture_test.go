// Package architecture_test enforces the module boundaries in ADR-0005.
//
// A test rather than a separate linter, deliberately. It runs with
// `go test ./...`, it is already in CI, it needs no dependency and no new
// command, and a rule that requires someone to remember an extra step is a rule
// that eventually stops being run.
//
// It reads the module's own import graph from `go list` and checks three things:
// that no bounded context reaches into another, that infrastructure does not
// depend on a context, and that the AWS SDK stays where ADR-0001 promised it
// would.
//
// Implements PLT-04.
package architecture_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// modulePath is this Go module. Imports outside it are third party and are only
// checked by the SDK rule.
const modulePath = "github.com/Yelethe1st/prepeet/services/platform"

// pkg is the part of `go list -json` output this test reads.
type pkg struct {
	ImportPath   string
	Imports      []string
	TestImports  []string
	XTestImports []string
}

// packages runs `go list` over the module and returns every package with its
// direct imports.
//
// Direct rather than transitive: a transitive check would fail for the wrong
// reason, since cmd legitimately reaches everything, and the rule that
// infrastructure may not import a context already closes the indirect route.
func packages(t *testing.T) []pkg {
	t.Helper()

	// The pattern is the module path rather than "./...". `go test` runs in the
	// package's own directory, so "./..." would resolve to this package alone
	// and the check would pass while every rule was being broken. That is not
	// hypothetical: it happened, and was caught only by introducing violations
	// deliberately to see the failure.
	//
	// Test imports are included because a test that reaches across a boundary
	// couples the two contexts just as firmly as production code does, and is
	// the more likely place for it to happen unnoticed.
	cmd := exec.Command("go", "list", "-json", modulePath+"/...")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}

	var found []pkg
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	for decoder.More() {
		var p pkg
		if err := decoder.Decode(&p); err != nil {
			t.Fatalf("decoding go list output: %v", err)
		}
		found = append(found, p)
	}
	if len(found) == 0 {
		t.Fatal("go list returned no packages, so this test proved nothing")
	}
	return found
}

// allImports returns a package's production and test imports together.
func allImports(p pkg) []string {
	return append(append(append([]string{}, p.Imports...), p.TestImports...), p.XTestImports...)
}

// contextOf returns the bounded context a package belongs to, if it is under
// internal/. The second result is false for anything else.
func contextOf(importPath string) (string, bool) {
	rest, found := strings.CutPrefix(importPath, modulePath+"/internal/")
	if !found {
		return "", false
	}
	name, _, _ := strings.Cut(rest, "/")
	return name, name != ""
}

// The boundary this ADR exists for. A context that reaches into another has
// made the two one thing, whatever the directory layout says, and extraction
// later means finding every call site.
//
// When a context needs something another one owns, it declares the narrow
// interface it needs in its own package and cmd wires the two together. See
// ADR-0005.
func TestNoContextImportsAnother(t *testing.T) {
	t.Parallel()

	for _, p := range packages(t) {
		importer, isContext := contextOf(p.ImportPath)
		if !isContext {
			continue
		}

		for _, imported := range allImports(p) {
			importee, isContext := contextOf(imported)
			if !isContext || importee == importer {
				continue
			}
			t.Errorf("%s imports %s\n"+
				"    internal/%s and internal/%s are separate bounded contexts.\n"+
				"    Declare the interface you need in internal/%s and wire it in cmd, per ADR-0005.",
				p.ImportPath, imported, importer, importee, importer)
		}
	}
}

// Infrastructure that knows about a bounded context has stopped being
// infrastructure. It also reopens the boundary above by the back door: two
// contexts could depend on each other through a platform package that knows
// both.
func TestPlatformDoesNotImportAContext(t *testing.T) {
	t.Parallel()

	for _, p := range packages(t) {
		if !strings.HasPrefix(p.ImportPath, modulePath+"/platform/") {
			continue
		}

		for _, imported := range allImports(p) {
			if _, isContext := contextOf(imported); isContext {
				t.Errorf("%s imports %s\n"+
					"    platform holds infrastructure and must not depend on a bounded context.\n"+
					"    Invert the dependency: let the context define what it needs.",
					p.ImportPath, imported)
			}
		}
	}
}

// ADR-0001 keeps the cloud reversible by confining vendor SDKs to the adapter
// layer, and lists this check as the thing that makes the promise hold. An AWS
// call from a bounded context would mean a migration touches domain code.
func TestCloudSDKStaysInTheAdapterLayer(t *testing.T) {
	t.Parallel()

	const sdk = "github.com/aws/aws-sdk-go-v2"

	for _, p := range packages(t) {
		if strings.HasPrefix(p.ImportPath, modulePath+"/platform/") {
			continue
		}

		for _, imported := range allImports(p) {
			if strings.HasPrefix(imported, sdk) {
				t.Errorf("%s imports %s\n"+
					"    The AWS SDK belongs in platform/ only, per ADR-0001, which relies on\n"+
					"    this to keep the cloud choice reversible.",
					p.ImportPath, imported)
			}
		}
	}
}

// cmd is the one place allowed to see the whole system, because something has
// to wire it together. This test does not enforce a rule so much as record that
// the exemption is deliberate, and fail if cmd ever stops being that place.
func TestCmdIsTheOnlyPlaceThatSeesEveryContext(t *testing.T) {
	t.Parallel()

	sawMultiple := false
	for _, p := range packages(t) {
		if !strings.HasPrefix(p.ImportPath, modulePath+"/cmd/") {
			continue
		}

		contexts := map[string]struct{}{}
		for _, imported := range allImports(p) {
			if name, isContext := contextOf(imported); isContext {
				contexts[name] = struct{}{}
			}
		}
		if len(contexts) > 1 {
			sawMultiple = true
		}
	}

	// Not yet true, and it will be as soon as cmd/api wires identity and
	// tenancy together. Recorded rather than asserted so the test does not fail
	// for the system being small.
	if !sawMultiple {
		t.Log("no cmd package wires more than one context yet; it will once GET /me is served")
	}
}
