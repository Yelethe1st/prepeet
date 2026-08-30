// Command vulncheck turns a govulncheck report into a build gate.
//
// govulncheck exits zero when it is asked for JSON, so on its own it reports
// rather than gates. This reads that report on standard input and decides:
//
//   - A vulnerability this code calls in a module the project depends on fails
//     the build. That is the dependency audit PLT-02 asks for.
//   - A vulnerability nobody calls is printed and does not fail. Failing on the
//     dependency graph rather than on reachable code trains people to add
//     exceptions, and a gate with a habit of exceptions stops meaning anything.
//   - A standard library vulnerability is printed loudly and does not fail. It
//     is fixed by moving the Go toolchain, which is pinned in
//     services/platform/go.mod, so gating on it would hold every unrelated
//     change hostage to a toolchain bump. The report names the version that
//     clears the findings so the bump is a decision somebody makes rather than
//     a number nobody has.
//
// The exemption is deliberate and narrow, and it is the reason PLT-02's note
// says the pipeline does not gate standard library advisories.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

// stdlibModule is what govulncheck calls the standard library in a trace frame.
const stdlibModule = "stdlib"

// message is the part of govulncheck's JSON stream this gate reads. The stream
// carries config, progress, osv and finding messages; every field not needed
// here is left out so a new one upstream cannot break the decode.
type message struct {
	Config  *json.RawMessage `json:"config"`
	Finding *finding         `json:"finding"`
}

// finding is one vulnerability as govulncheck reports it. The first trace frame
// is the vulnerable symbol itself: it carries a function name only when the
// scan could trace a call to it, which is what separates "your code calls this"
// from "this is somewhere in your graph".
type finding struct {
	OSV          string `json:"osv"`
	FixedVersion string `json:"fixed_version"`
	Trace        []struct {
		Module   string `json:"module"`
		Package  string `json:"package"`
		Function string `json:"function"`
	} `json:"trace"`
}

// called reports whether the scan traced a call from this code to the
// vulnerable symbol.
func (f finding) called() bool {
	return len(f.Trace) > 0 && f.Trace[0].Function != ""
}

// module is the module the vulnerability lives in.
func (f finding) module() string {
	if len(f.Trace) == 0 {
		return "unknown"
	}
	return f.Trace[0].Module
}

// result is what the gate decided, kept separate from printing so the decision
// is testable without parsing output.
type result struct {
	// Blocking is every called vulnerability in a dependency. Non-empty means
	// the build fails.
	Blocking []finding
	// Toolchain counts called standard library vulnerabilities, which are
	// reported rather than gated.
	Toolchain int
	// ToolchainFix is the highest version that clears them, which is the one
	// worth upgrading to: the lowest would leave the others standing.
	ToolchainFix string
	// Unreachable counts vulnerabilities in the graph that nothing calls.
	Unreachable int
}

// gate reads a govulncheck JSON report and classifies its findings.
//
// An absent report is an error rather than a pass. A scanner that could not
// reach the vulnerability database, or a binary that is not installed, produces
// no output, and a filter that reads silence as "nothing found" is the failure
// this whole gate exists to avoid. Every real report opens with a config
// message, so its absence is the tell.
func gate(r io.Reader) (result, error) {
	decoder := json.NewDecoder(r)

	var (
		sawConfig bool
		// One advisory is reported many times: once as a fact about the
		// dependency graph with no trace, then once per call that could be
		// traced to it. The gate speaks about vulnerabilities rather than
		// reports, so findings are collected per advisory and decided after the
		// stream ends.
		//
		// Deciding on the first report instead is not a smaller version of this;
		// it is the opposite answer. The untraced report always comes first, so
		// first-wins classified every reachable advisory as unreachable and the
		// gate passed what it exists to catch. It did, against this repository,
		// until the count came out at zero and did not match govulncheck's own.
		worst = map[string]finding{}
		order []string
	)

	for {
		var msg message
		if err := decoder.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return result{}, fmt.Errorf("reading the govulncheck report: %w", err)
		}
		if msg.Config != nil {
			sawConfig = true
		}
		if msg.Finding == nil {
			continue
		}

		found := *msg.Finding
		previous, seen := worst[found.OSV]
		if !seen {
			order = append(order, found.OSV)
		}
		// A traced call is stronger evidence than its absence, and nothing
		// downgrades it.
		if !seen || (found.called() && !previous.called()) {
			worst[found.OSV] = found
		}
	}

	if !sawConfig {
		return result{}, errors.New("no govulncheck report on standard input: the scan did not run")
	}

	var out result
	for _, osv := range order {
		found := worst[osv]
		switch {
		case !found.called():
			out.Unreachable++
		case found.module() == stdlibModule:
			out.Toolchain++
			if semver.Compare(found.FixedVersion, out.ToolchainFix) > 0 {
				out.ToolchainFix = found.FixedVersion
			}
		default:
			out.Blocking = append(out.Blocking, found)
		}
	}

	sort.Slice(out.Blocking, func(i, j int) bool { return out.Blocking[i].OSV < out.Blocking[j].OSV })
	return out, nil
}

// Report renders the decision. A blocking finding names the advisory, the
// module and the version that fixes it, because a failure that does not say
// what to upgrade gets rerun rather than fixed.
func (r result) Report() string {
	var b strings.Builder

	for _, f := range r.Blocking {
		fmt.Fprintf(&b, "\033[31mFAIL\033[0m %s is called in %s, fixed in %s\n",
			f.OSV, f.module(), f.FixedVersion)
		fmt.Fprintf(&b, "     https://pkg.go.dev/vuln/%s\n", f.OSV)
	}

	if r.Toolchain > 0 {
		fmt.Fprintf(&b, "\033[33mWARN\033[0m %d standard library advisories are reachable, cleared by Go %s\n",
			r.Toolchain, strings.TrimPrefix(r.ToolchainFix, "v"))
		fmt.Fprintf(&b, "     Not gated: the toolchain is pinned in services/platform/go.mod.\n")
		fmt.Fprintf(&b, "     Run `make audit-go-verbose` for the traces.\n")
	}
	if r.Unreachable > 0 {
		fmt.Fprintf(&b, "     %d further advisories are in the dependency graph but are not called.\n", r.Unreachable)
	}

	if len(r.Blocking) == 0 {
		fmt.Fprintf(&b, "\033[32mPASS\033[0m no called vulnerability in a dependency\n")
	}
	return b.String()
}

func main() {
	decided, err := gate(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vulncheck: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(decided.Report())
	if len(decided.Blocking) > 0 {
		os.Exit(1)
	}
}
