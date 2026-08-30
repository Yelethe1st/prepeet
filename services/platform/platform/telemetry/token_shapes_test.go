package telemetry_test

import (
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
	"github.com/Yelethe1st/prepeet/services/platform/platform/token"
)

// The bearer-token pattern was written with the prefixes that existed when it
// was written, and IAM-08 then issued two more. A PKCE verifier or an OAuth
// state quoted into an error went to the log intact, which is exactly what the
// pattern exists to prevent.
//
// Listing the prefixes in two places is what made that possible, so the
// pattern is now built from the token package. This test issues a real token
// for every purpose rather than restating the prefixes a third time: a
// prefix added to the map and nowhere else has to fail here.
func TestEveryIssuedTokenShapeIsRedacted(t *testing.T) {
	t.Parallel()

	for _, purpose := range token.Purposes() {
		issued, err := token.New(purpose)
		if err != nil {
			t.Fatalf("New(%q): %v", purpose, err)
		}
		if purpose == token.PurposeOTP {
			// A one-time code is six digits with no prefix, so there is no
			// shape to match on. It is redacted by never being logged, which
			// is a different guarantee and not this one's to make.
			continue
		}

		line := "refusing the presented credential " + issued.Plaintext

		if scrubbed := telemetry.Scrub(line); scrubbed == line {
			t.Errorf("a %s token survives Scrub intact", purpose)
		} else if strings.Contains(scrubbed, issued.Plaintext) {
			t.Errorf("a %s token is still present after Scrub", purpose)
		}

		if _, found := telemetry.FindRestricted(line); !found {
			t.Errorf("FindRestricted does not recognise a %s token, so a caller "+
				"that must refuse rather than redact would carry it onward", purpose)
		}
	}
}
