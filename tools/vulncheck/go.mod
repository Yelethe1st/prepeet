// The Go dependency audit gate: the govulncheck it runs, pinned here, and the
// program in main.go that turns govulncheck's report into a pass or a failure.
//
// Separate from services/platform for the reason authzgen and eventgen are: an
// audit tool must not become a dependency of the service it audits. Pinning the
// scanner here rather than installing it at @latest in CI means it is the same
// scanner on a laptop and on a runner, so a finding cannot appear or disappear
// with whatever version a runner happened to fetch that morning. The
// vulnerability database is still fetched live, which is the point of it.
module github.com/Yelethe1st/prepeet/tools/vulncheck

go 1.25

tool golang.org/x/vuln/cmd/govulncheck

require golang.org/x/mod v0.22.0

require (
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/telemetry v0.0.0-20240522233618-39ace7a40ae7 // indirect
	golang.org/x/tools v0.29.0 // indirect
	golang.org/x/vuln v1.1.4 // indirect
)
