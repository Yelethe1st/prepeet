package main

import (
	"strings"
	"testing"
)

// A report govulncheck never produced is the failure mode this gate has to
// survive: if the scanner cannot reach the vulnerability database, or the
// binary is missing, its output is empty and a naive filter reads that as
// "nothing wrong". Every real report opens with a config object, so its absence
// is the signal that there was no scan.
func TestAnEmptyReportIsRefusedRatherThanRead(t *testing.T) {
	t.Parallel()

	_, err := gate(strings.NewReader(""))
	if err == nil {
		t.Fatal("gate accepted an empty report, which is a scan that did not happen")
	}
	if !strings.Contains(err.Error(), "no govulncheck report") {
		t.Errorf("error does not say the report was missing: %v", err)
	}
}

// The gate is about dependencies the project chooses. A vulnerability the code
// calls in one of them fails the build and names both the module and the
// advisory, because a failure that does not say what to upgrade gets ignored.
func TestACalledDependencyVulnerabilityFails(t *testing.T) {
	t.Parallel()

	report := configLine + calledFinding("GO-2025-0001", "v1.2.3", "github.com/example/lib", "Parse")

	result, err := gate(strings.NewReader(report))
	if err != nil {
		t.Fatalf("gate() returned error: %v", err)
	}
	if len(result.Blocking) != 1 {
		t.Fatalf("got %d blocking findings, want 1", len(result.Blocking))
	}
	summary := result.Report()
	for _, want := range []string{"GO-2025-0001", "github.com/example/lib", "v1.2.3"} {
		if !strings.Contains(summary, want) {
			t.Errorf("report does not name %q:\n%s", want, summary)
		}
	}
}

// A vulnerability nobody calls is a fact about the dependency graph, not about
// this code. Failing on it would train people to add exceptions, which is how a
// gate stops meaning anything.
func TestAnUncalledDependencyVulnerabilityDoesNotFail(t *testing.T) {
	t.Parallel()

	report := configLine + uncalledFinding("GO-2025-0002", "github.com/example/lib")

	result, err := gate(strings.NewReader(report))
	if err != nil {
		t.Fatalf("gate() returned error: %v", err)
	}
	if len(result.Blocking) != 0 {
		t.Fatalf("an uncalled finding blocked the build: %+v", result.Blocking)
	}
	if result.Unreachable != 1 {
		t.Errorf("got %d unreachable findings, want 1", result.Unreachable)
	}
}

// The standard library is fixed by moving the toolchain, which is pinned in
// services/platform/go.mod rather than chosen here. Gating on it would make
// every build red until an unrelated change lands, so it is reported loudly
// instead, with the version that would clear it.
func TestStandardLibraryFindingsAreReportedRatherThanGated(t *testing.T) {
	t.Parallel()

	report := configLine +
		calledFinding("GO-2026-0003", "v1.26.1", "stdlib", "Parse") +
		calledFinding("GO-2026-0004", "v1.26.6", "stdlib", "Verify")

	result, err := gate(strings.NewReader(report))
	if err != nil {
		t.Fatalf("gate() returned error: %v", err)
	}
	if len(result.Blocking) != 0 {
		t.Fatalf("a standard library finding blocked the build: %+v", result.Blocking)
	}
	if result.Toolchain != 2 {
		t.Errorf("got %d toolchain findings, want 2", result.Toolchain)
	}
	summary := result.Report()
	// The highest fixed version, because upgrading to the lowest one leaves the
	// other finding standing and looks like the gate did nothing.
	if !strings.Contains(summary, "1.26.6") {
		t.Errorf("report does not name the toolchain version that clears them:\n%s", summary)
	}
}

// govulncheck emits one JSON object per message with no enclosing array, so a
// reader that expects a document rather than a stream sees only the first
// message and every finding after it disappears.
func TestFindingsAfterTheFirstMessageAreRead(t *testing.T) {
	t.Parallel()

	report := configLine +
		`{"progress":{"message":"Scanning"}}` + "\n" +
		`{"osv":{"id":"GO-2025-0005"}}` + "\n" +
		calledFinding("GO-2025-0005", "v0.1.0", "github.com/example/other", "Do")

	result, err := gate(strings.NewReader(report))
	if err != nil {
		t.Fatalf("gate() returned error: %v", err)
	}
	if len(result.Blocking) != 1 {
		t.Fatalf("got %d blocking findings, want 1: the stream was not read past its first message", len(result.Blocking))
	}
}

// govulncheck reports the same advisory more than once: first as a fact about
// the dependency graph with no call trace, then once per call it managed to
// trace. Taking the first report and skipping the rest reads every called
// vulnerability as unreachable, which is a gate that passes exactly what it
// exists to catch. Found by running it against this repository, where all 24
// reachable advisories were being reported as "in the graph but not called".
func TestACalledFindingBeatsAnEarlierUncalledOneForTheSameAdvisory(t *testing.T) {
	t.Parallel()

	report := configLine +
		uncalledFinding("GO-2025-0006", "github.com/example/lib") +
		calledFinding("GO-2025-0006", "v1.2.3", "github.com/example/lib", "Parse")

	result, err := gate(strings.NewReader(report))
	if err != nil {
		t.Fatalf("gate() returned error: %v", err)
	}
	if len(result.Blocking) != 1 {
		t.Fatalf("got %d blocking findings, want 1: the call trace was discarded", len(result.Blocking))
	}
	if result.Unreachable != 0 {
		t.Errorf("the advisory was counted as unreachable as well as blocking")
	}
}

// The same advisory reported through several call traces is one vulnerability,
// not four. A count that inflates with the size of the call graph tells nobody
// how much is wrong.
func TestOneAdvisoryReportedManyTimesCountsOnce(t *testing.T) {
	t.Parallel()

	report := configLine +
		calledFinding("GO-2025-0007", "v1.0.0", "github.com/example/lib", "Parse") +
		calledFinding("GO-2025-0007", "v1.0.0", "github.com/example/lib", "Parse") +
		calledFinding("GO-2025-0007", "v1.0.0", "github.com/example/lib", "Decode")

	result, err := gate(strings.NewReader(report))
	if err != nil {
		t.Fatalf("gate() returned error: %v", err)
	}
	if len(result.Blocking) != 1 {
		t.Fatalf("got %d blocking findings, want 1", len(result.Blocking))
	}
}

const configLine = `{"config":{"protocol_version":"v1.0.0","scanner_name":"govulncheck"}}` + "\n"

// calledFinding builds a called finding: govulncheck marks a vulnerability as reached
// by giving the first frame of the trace a function name.
func calledFinding(osv, fixed, module, function string) string {
	return `{"finding":{"osv":"` + osv + `","fixed_version":"` + fixed + `","trace":[` +
		`{"module":"` + module + `","package":"example","function":"` + function + `"},` +
		`{"module":"github.com/Yelethe1st/prepeet/services/platform","package":"caller","function":"Handle"}` +
		`]}}` + "\n"
}

// uncalledFinding builds the shape govulncheck uses for a vulnerability it
// found in the graph but could not trace a call to: a single frame with no
// function.
func uncalledFinding(osv, module string) string {
	return `{"finding":{"osv":"` + osv + `","fixed_version":"v9.9.9","trace":[` +
		`{"module":"` + module + `"}]}}` + "\n"
}
