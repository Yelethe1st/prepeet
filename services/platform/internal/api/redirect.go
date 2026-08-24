// Package api serves the public HTTP API.
//
// It implements the interface generated from packages/contracts/api/openapi.yaml,
// so a handler that does not match the contract does not compile. ADR-0004
// makes the contract the source; this package is where that becomes true rather
// than stated.
//
// Nothing here holds a product rule. Handlers translate between the wire and
// the modules that own the behaviour, and any decision worth testing belongs in
// one of those modules rather than here.
//
// Implements part of IAM-01 and CTR-01.
package api

import (
	"net/url"
	"strings"
)

// DefaultRedirect is where someone goes when the destination they asked for
// cannot be trusted. It is a real destination rather than an error, because
// refusing to redirect after a successful login is a worse experience than
// landing somewhere sensible.
const DefaultRedirect = "/candidate"

// SafeRedirect validates a post-login destination.
//
// The destination arrives in a query parameter on a page anyone can link to, so
// it is attacker controlled. Sending someone off-site immediately after they
// authenticate is exactly the moment they trust what they see, which is what
// makes an open redirect worth exploiting: a convincing fake login page, one
// hop after a real one.
//
// The rule is deliberately narrow. Only a path on this origin is accepted, and
// anything that could be read as a scheme, an authority, or a header break is
// refused rather than sanitised. Sanitising invites a parser difference between
// what we checked and what the browser follows.
func SafeRedirect(candidate string) (string, bool) {
	if candidate == "" || len(candidate) > 2048 {
		return "", false
	}

	// Leading whitespace and control characters are stripped by browsers before
	// the URL is parsed, so a value that looks harmless here can be an absolute
	// URL by the time it is followed. Refusing outright avoids having to model
	// which characters which browser strips.
	for _, r := range candidate {
		if r < 0x20 || r == 0x7F {
			return "", false
		}
	}

	// A path, not a URL. Anything else is refused before parsing, because the
	// parse itself is where the disagreements live.
	if !strings.HasPrefix(candidate, "/") {
		return "", false
	}
	// "//host" and "/\host" are both read as protocol-relative by some browsers,
	// which makes them absolute URLs wearing a path's clothing.
	if strings.HasPrefix(candidate, "//") || strings.HasPrefix(candidate, `/\`) {
		return "", false
	}
	if strings.Contains(candidate, `\`) {
		return "", false
	}

	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", false
	}
	// After all of the above these should be impossible, and they are checked
	// anyway: the cost of being wrong here is an account takeover, and the cost
	// of the check is nothing.
	if parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil {
		return "", false
	}

	return candidate, true
}
