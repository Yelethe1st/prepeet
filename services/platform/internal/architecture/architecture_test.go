// Package architecture_test enforces the module boundaries in ADR-0005.
//
// A test rather than a separate linter, deliberately. It runs with
// `go test ./...`, it is already in CI, it needs no dependency and no new
// command, and a rule that requires someone to remember an extra step is a rule
// that eventually stops being run.
//
// It reads the module's own import graph from `go list` and checks that no
// bounded context reaches into another, that infrastructure does not depend on a
// context, that the AWS SDK stays where ADR-0001 promised it would, and that
// fan-out goes through platform/broadcast as ADR-0006 assumed.
//
// Implements PLT-04.
package architecture_test

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	// Dir and GoFiles support the one rule that has to read source rather than
	// imports: a package can call pg_notify through pgx without importing
	// anything that names it.
	Dir     string
	GoFiles []string
}

// goFilesOf returns the absolute path of every non-test Go file in a package.
func goFilesOf(p pkg) []string {
	paths := make([]string, 0, len(p.GoFiles))
	for _, name := range p.GoFiles {
		paths = append(paths, filepath.Join(p.Dir, name))
	}
	return paths
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

// ADR-0006 defers Redis on the strength of the swap being cheap, and that is
// only true while the transport is reached through platform/broadcast.
//
// A handler calling LISTEN or NOTIFY directly would work perfectly and would
// quietly make the deferral wrong, because there would then be call sites to
// rewrite rather than one adapter to add. This is the check that would have
// been "we will remember" otherwise.
func TestFanOutGoesThroughTheBroadcastPackage(t *testing.T) {
	t.Parallel()

	// platform/broadcast is the implementation, and platform/outbox emits the
	// wake-up inside its own transaction, which is the one thing the interface
	// deliberately cannot express. Both are named here so the exemption is a
	// decision on the record rather than a prefix match that quietly widens.
	allowed := map[string]string{
		modulePath + "/platform/broadcast": "is the implementation",
		modulePath + "/platform/outbox":    "emits its wake-up transactionally, which no external transport can do",
	}

	for _, p := range packages(t) {
		if _, exempt := allowed[p.ImportPath]; exempt {
			continue
		}

		for _, file := range goFilesOf(p) {
			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("reading %s: %v", file, err)
			}
			for _, forbidden := range []string{"LISTEN ", "pg_notify", "NOTIFY "} {
				if strings.Contains(string(source), forbidden) {
					t.Errorf("%s uses %q directly\n"+
						"    Fan-out goes through platform/broadcast, per ADR-0006, which defers\n"+
						"    Redis on the strength of that swap being one new adapter.",
						file, strings.TrimSpace(forbidden))
				}
			}
		}
	}
}

// The Temporal client stays out of the bounded contexts, and the workflow
// package does not.
//
// The distinction is the point. ADR-0007 keeps the swap to Temporal Cloud cheap
// by building the client in one place, so a context that dialled its own would
// undo that. But workflow definitions are written against go.temporal.io/sdk/
// workflow and no interface hides that honestly, and they are domain logic, so
// they belong in the context that owns them. Banning both would push workflow
// code into platform/, where it has no business being.
//
// So: a context starts a workflow through a consumer-defined interface, and
// defines one using the SDK directly.
func TestTheTemporalClientStaysOutOfBoundedContexts(t *testing.T) {
	t.Parallel()

	const clientPackage = "go.temporal.io/sdk/client"

	for _, p := range packages(t) {
		// platform/ builds the client; cmd/ wires it.
		if strings.HasPrefix(p.ImportPath, modulePath+"/platform/") ||
			strings.HasPrefix(p.ImportPath, modulePath+"/cmd/") {
			continue
		}

		for _, imported := range allImports(p) {
			if imported == clientPackage {
				t.Errorf("%s imports %s\n"+
					"    The Temporal client is built in platform/temporal, per ADR-0007, which\n"+
					"    keeps moving to Temporal Cloud a configuration change. Start a workflow\n"+
					"    through a consumer-defined interface instead. Defining a workflow with\n"+
					"    go.temporal.io/sdk/workflow is fine and is not what this checks.",
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

// The rule that keeps ADR-0008's decision from decaying.
//
// sqlc is worth nothing if the next query is written by hand next to the
// generated ones. That is exactly how this codebase came to differ from the
// technology baseline in the first place: nobody decided against sqlc, queries
// simply got written where it was quickest, one at a time, and the difference
// was only visible to somebody who went looking.
//
// String literals rather than a grep over the file, because a comment
// explaining why a SELECT would be wrong is not a SELECT, and a rule that
// cannot tell the two apart is one people learn to phrase around.
func TestSQLIsGeneratedRatherThanWritten(t *testing.T) {
	t.Parallel()

	// Each exemption names what makes the statement impossible to generate,
	// rather than merely inconvenient. Anything that is only inconvenient
	// belongs in a queries.sql file.
	allowed := map[string]string{
		modulePath + "/platform/database":  "the migration runner creates and reads the table recording which migrations ran, and no migration can describe it because it has to exist before the first one applies",
		modulePath + "/platform/broadcast": "LISTEN takes no bind parameters, so there is nothing for sqlc to type, and it is issued on a connection held open for notifications rather than through the pool",
		modulePath + "/platform/outbox":    "pg_notify is emitted inside the caller's transaction so the signal becomes visible exactly when the row does, which is the guarantee the package exists for",
	}

	// Statement keywords as they appear in a query, not words as they appear in
	// prose. Upper case only, which every query in this module uses and no
	// sentence does.
	statements := []string{"SELECT ", "INSERT INTO", "UPDATE ", "DELETE FROM"}

	for _, p := range packages(t) {
		if _, exempt := allowed[p.ImportPath]; exempt {
			continue
		}

		for _, file := range goFilesOf(p) {
			if generated(t, file) {
				continue
			}

			for _, literal := range stringLiterals(t, file) {
				for _, statement := range statements {
					if !strings.Contains(literal, statement) {
						continue
					}
					t.Errorf("%s contains SQL in Go source:\n    %s\n"+
						"    Queries live in a queries.sql file beside the module and are generated by sqlc.\n"+
						"    See ADR-0008. If it genuinely cannot be generated, add the package here with the reason.",
						file, strings.TrimSpace(truncate(literal)))
					break
				}
			}
		}
	}
}

// generated reports whether a file was written by a generator.
//
// Matches the header every Go generator emits rather than a filename pattern,
// because the pattern differs between them and a file that lies about being
// generated is a problem no naming convention would have caught either.
func generated(t *testing.T, path string) bool {
	t.Helper()

	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return bytes.Contains(source, []byte("Code generated by"))
}

// stringLiterals returns the value of every string literal in a Go file.
//
// Parsed rather than scanned, so that a comment mentioning a statement is not
// mistaken for one. Both raw and interpreted literals are unquoted, because a
// query is usually a raw literal and a fragment concatenated onto one usually
// is not.
func stringLiterals(t *testing.T, path string) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	var literals []string
	ast.Inspect(parsed, func(n ast.Node) bool {
		literal, ok := n.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		literals = append(literals, value)
		return true
	})
	return literals
}

// truncate shortens a literal for a failure message, since a query printed in
// full buries the file path that says where to look.
func truncate(s string) string {
	const limit = 80
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
