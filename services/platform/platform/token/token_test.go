package token_test

import (
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/token"
)

func TestNewProducesAPrefixedToken(t *testing.T) {
	t.Parallel()

	issued, err := token.New(token.PurposeSession)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if !strings.HasPrefix(issued.Plaintext, "ses_") {
		t.Errorf("Plaintext = %q, want a ses_ prefix", issued.Plaintext)
	}
	if issued.Hash == "" {
		t.Error("Hash is empty, and the plaintext is never stored")
	}
}

// Every purpose gets its own prefix, so a token found in a log or a support
// ticket says what it is without anyone having to look it up.
func TestPurposesHaveDistinctPrefixes(t *testing.T) {
	t.Parallel()

	seen := make(map[string]token.Purpose)
	for _, purpose := range token.Purposes() {
		issued, err := token.New(purpose)
		if err != nil {
			t.Fatalf("New(%q) returned error: %v", purpose, err)
		}
		prefix, _, found := strings.Cut(issued.Plaintext, "_")
		if !found {
			t.Errorf("token for %q has no prefix: %q", purpose, issued.Plaintext)
			continue
		}
		if previous, clash := seen[prefix]; clash {
			t.Errorf("purposes %q and %q share the prefix %q", previous, purpose, prefix)
		}
		seen[prefix] = purpose
	}
}

func TestTokensAreUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, 5000)
	for range 5000 {
		issued, err := token.New(token.PurposeSession)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		if _, duplicate := seen[issued.Plaintext]; duplicate {
			t.Fatalf("New produced a duplicate token after %d values", len(seen))
		}
		seen[issued.Plaintext] = struct{}{}
	}
}

// A token has to carry enough entropy that guessing one is not a strategy. The
// prefix is not entropy, so it is excluded from the measure.
func TestTokenCarriesEnoughEntropy(t *testing.T) {
	t.Parallel()

	issued, err := token.New(token.PurposeSession)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, secret, _ := strings.Cut(issued.Plaintext, "_")
	// base64url of 32 random bytes is 43 characters.
	if len(secret) < 43 {
		t.Errorf("secret is %d characters, want at least 43 for 256 bits", len(secret))
	}
}

// The database stores the hash. A read of the session table must not yield
// anything that can be presented as a credential.
func TestHashIsNotThePlaintext(t *testing.T) {
	t.Parallel()

	issued, err := token.New(token.PurposeSession)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if strings.Contains(issued.Hash, issued.Plaintext) {
		t.Error("the stored hash contains the plaintext token")
	}
	_, secret, _ := strings.Cut(issued.Plaintext, "_")
	if strings.Contains(issued.Hash, secret) {
		t.Error("the stored hash contains the token secret")
	}
}

func TestHashIsDeterministicSoALookupCanFindIt(t *testing.T) {
	t.Parallel()

	issued, err := token.New(token.PurposeSession)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if token.HashOf(issued.Plaintext) != issued.Hash {
		t.Error("HashOf did not reproduce the hash issued with the token")
	}
}

func TestHashOfDiffersForDifferentTokens(t *testing.T) {
	t.Parallel()

	if token.HashOf("ses_aaa") == token.HashOf("ses_bbb") {
		t.Error("different tokens produced the same hash")
	}
}

// A presented token is compared in constant time, so an attacker cannot
// discover a valid hash one byte at a time by measuring the lookup.
func TestEqualsMatchesOnlyTheSameValue(t *testing.T) {
	t.Parallel()

	a := token.HashOf("ses_aaa")

	if !token.Equals(a, token.HashOf("ses_aaa")) {
		t.Error("Equals rejected two hashes of the same token")
	}
	if token.Equals(a, token.HashOf("ses_bbb")) {
		t.Error("Equals accepted hashes of different tokens")
	}
	if token.Equals(a, "") {
		t.Error("Equals accepted an empty hash")
	}
}

func TestNewRejectsAnUnknownPurpose(t *testing.T) {
	t.Parallel()

	if _, err := token.New(token.Purpose("exfiltration")); err == nil {
		t.Error("New accepted an unknown purpose, want it refused")
	}
}

// The plaintext exists for exactly one response and is never persisted, so the
// type must not hand it to a logger by accident.
func TestIssuedTokenDoesNotStringifyThePlaintext(t *testing.T) {
	t.Parallel()

	issued, err := token.New(token.PurposeSession)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if strings.Contains(issued.String(), issued.Plaintext) {
		t.Errorf("String() = %q, want the plaintext redacted", issued.String())
	}
}

// The prefix says what a token claims to be, which is useful for routing a
// request to the right lookup. It is never the decision: the authoritative
// purpose is the one stored beside the hash.
func TestPurposeOfReadsThePrefix(t *testing.T) {
	t.Parallel()

	for _, purpose := range token.Purposes() {
		issued, err := token.New(purpose)
		if err != nil {
			t.Fatalf("New(%q) returned error: %v", purpose, err)
		}

		got, recognised := token.PurposeOf(issued.Plaintext)
		if !recognised {
			t.Errorf("PurposeOf did not recognise a token it issued for %q", purpose)
			continue
		}
		if got != purpose {
			t.Errorf("PurposeOf = %q, want %q", got, purpose)
		}
	}
}

func TestPurposeOfRejectsWhatItDoesNotRecognise(t *testing.T) {
	t.Parallel()

	for name, candidate := range map[string]string{
		"no separator":     "sesabc123",
		"unknown prefix":   "xyz_abc123",
		"empty":            "",
		"separator only":   "_",
		"looks like a JWT": "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, recognised := token.PurposeOf(candidate); recognised {
				t.Errorf("PurposeOf(%q) claimed to recognise it", candidate)
			}
		})
	}
}

// New must fail rather than return a usable token if it cannot say what the
// token is for, since the purpose is what stops a password reset token being
// presented where a session is expected.
func TestNewRejectsAnEmptyPurpose(t *testing.T) {
	t.Parallel()

	if _, err := token.New(""); err == nil {
		t.Error("New accepted an empty purpose")
	}
}

// The PKCE challenge, against RFC 7636's own worked example.
//
// A hand-rolled assertion here would only prove the function agrees with
// itself. The vector in Appendix B is what every provider implements against,
// and getting the encoding wrong is the mistake that produces a challenge
// which looks right and is rejected at the token endpoint.
func TestTheChallengeMatchesTheSpecificationsVector(t *testing.T) {
	const (
		verifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	)

	if got := token.ChallengeFor(verifier); got != challenge {
		t.Fatalf("ChallengeFor = %q, want RFC 7636's %q", got, challenge)
	}
}

// URL-safe and unpadded, which is not cosmetic: '+' and '=' survive a query
// string differently depending on who encodes them, and a provider comparing
// strings will disagree.
func TestTheChallengeIsUrlSafeAndUnpadded(t *testing.T) {
	for range 200 {
		issued, err := token.New(token.PurposeOAuthVerifier)
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		got := token.ChallengeFor(issued.Plaintext)
		if strings.ContainsAny(got, "+/=") {
			t.Fatalf("challenge %q contains a character that does not survive a query string", got)
		}
	}
}

func TestTheChallengeIsNotTheVerifier(t *testing.T) {
	issued, err := token.New(token.PurposeOAuthVerifier)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// The whole point: the challenge goes out in the open and must not be
	// reversible to the verifier that is held back.
	if token.ChallengeFor(issued.Plaintext) == issued.Plaintext {
		t.Fatal("the challenge is the verifier")
	}
}
