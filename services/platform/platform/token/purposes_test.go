package token_test

import (
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/token"
)

// Purposes was written as a hand-maintained list beside the map it was meant
// to enumerate, and IAM-08 added two OAuth purposes to the map without adding
// them to the list. Nothing failed: every test that iterates Purposes simply
// stopped covering the new ones, so the prefix-uniqueness and round-trip
// guarantees quietly did not apply to the OAuth tokens at all.
//
// The list is now derived, and this is the test that says so.
func TestEveryIssuablePurposeIsEnumerated(t *testing.T) {
	t.Parallel()

	declared := []token.Purpose{
		token.PurposeSession, token.PurposeRefresh, token.PurposeEmailVerify,
		token.PurposePasswordReset, token.PurposeMagicLink, token.PurposeOTP,
		token.PurposeInvitation, token.PurposeOAuthState, token.PurposeOAuthVerifier,
	}

	enumerated := make(map[token.Purpose]bool, len(token.Purposes()))
	for _, purpose := range token.Purposes() {
		enumerated[purpose] = true
	}

	for _, purpose := range declared {
		if !enumerated[purpose] {
			t.Errorf("Purposes() omits %q, so every test that ranges over it skips that purpose",
				purpose)
		}
	}
	if len(token.Purposes()) != len(declared) {
		t.Errorf("Purposes() has %d entries and %d purposes are declared here; "+
			"a new purpose needs adding to this list too",
			len(token.Purposes()), len(declared))
	}
}

// A purpose that New refuses is a purpose no caller can use, which is the
// failure mode of adding a constant and forgetting its prefix.
func TestEveryEnumeratedPurposeCanBeIssued(t *testing.T) {
	t.Parallel()

	for _, purpose := range token.Purposes() {
		issued, err := token.New(purpose)
		if err != nil {
			t.Errorf("New(%q): %v", purpose, err)
			continue
		}
		if !strings.Contains(issued.Plaintext, "_") {
			t.Errorf("token for %q carries no prefix: it cannot be recognised in a log",
				purpose)
		}
	}
}
