package telemetry_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// The rule this package exists to make mechanical: no restricted content ever
// reaches telemetry.
//
// docs/security/data-classification.md classifies transcript text, evaluation
// prose and candidate contact details as Restricted. Telemetry goes to a vendor,
// is retained on somebody else's schedule and is readable by anyone with
// dashboard access, so a transcript fragment in a span attribute is a
// restricted disclosure that no retention policy covers.
//
// A convention would not survive contact with a hurried debugging session at
// two in the morning. An allowlist does.

// Words that must not appear in an approved attribute key, because a key named
// after one of them is a key that will eventually carry one.
var forbiddenInKey = []string{
	"email", "transcript", "password", "token", "secret", "credential",
	"answer", "content", "payload", "prose", "name", "phone", "address",
}

func TestNoApprovedKeyInvitesRestrictedContent(t *testing.T) {
	t.Parallel()

	for _, key := range telemetry.ApprovedKeys() {
		lowered := strings.ToLower(string(key))
		for _, forbidden := range forbiddenInKey {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("approved key %q contains %q, and a key named that will eventually carry it",
					key, forbidden)
			}
		}
	}
}

func TestApprovedKeysAreNamespaced(t *testing.T) {
	t.Parallel()

	for _, key := range telemetry.ApprovedKeys() {
		if !strings.HasPrefix(string(key), "prepeet.") {
			t.Errorf("approved key %q is not namespaced, so it can collide with a library's attribute", key)
		}
	}
}

func TestApprovedKeysAreUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[telemetry.Key]struct{})
	for _, key := range telemetry.ApprovedKeys() {
		if _, duplicate := seen[key]; duplicate {
			t.Errorf("%q appears twice in the approved list", key)
		}
		seen[key] = struct{}{}
	}
}

// An attribute is constructed from an approved key or not at all. That is what
// turns the rule from a convention into something the compiler and this test
// enforce together.
func TestAttrRefusesAnUnapprovedKey(t *testing.T) {
	t.Parallel()

	if _, ok := telemetry.Attr(telemetry.Key("prepeet.candidate_answer"), "..."); ok {
		t.Error("Attr accepted a key that is not on the approved list")
	}
}

func TestAttrAcceptsAnApprovedKey(t *testing.T) {
	t.Parallel()

	attr, ok := telemetry.Attr(telemetry.KeyRequestID, "req_01a03")
	if !ok {
		t.Fatal("Attr refused an approved key")
	}
	if string(attr.Key) != string(telemetry.KeyRequestID) {
		t.Errorf("Key = %q, want %q", attr.Key, telemetry.KeyRequestID)
	}
}

// ─────────────────────────────────────────────────────────────── scrubbing

// Free text reaches telemetry through error messages, which nobody writes with
// a classification in mind. A driver error carries a connection string, a
// validation error can carry the value that failed, and a provider error can
// carry a prompt.

var emailPattern = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

func TestScrubRemovesEmailAddresses(t *testing.T) {
	t.Parallel()

	scrubbed := telemetry.Scrub("no user found for daniel.okonkwo@example.com in tenant northwind")

	if emailPattern.MatchString(scrubbed) {
		t.Errorf("Scrub left an address in place: %q", scrubbed)
	}
	if !strings.Contains(scrubbed, "tenant northwind") {
		t.Errorf("Scrub removed more than it needed to: %q", scrubbed)
	}
}

func TestScrubRemovesBearerTokens(t *testing.T) {
	t.Parallel()

	for _, secret := range []string{
		"ses_AbCdEf0123456789AbCdEf0123456789AbCdEf01234",
		"ref_ZzYyXx9876543210ZzYyXx9876543210ZzYyXx98765",
		"rst_QqWwEe1122334455QqWwEe1122334455QqWwEe11223",
	} {
		scrubbed := telemetry.Scrub("rejected token " + secret)
		if strings.Contains(scrubbed, secret) {
			t.Errorf("Scrub left a token in place: %q", scrubbed)
		}
	}
}

func TestScrubRemovesConnectionStrings(t *testing.T) {
	t.Parallel()

	scrubbed := telemetry.Scrub("dial failed: postgres://prepeet:hunter2@db.internal:5432/prepeet")

	for _, secret := range []string{"hunter2", "prepeet:hunter2"} {
		if strings.Contains(scrubbed, secret) {
			t.Errorf("Scrub left a credential in place: %q", scrubbed)
		}
	}
}

// A password hash is not a password, and it is still not something to publish
// to a dashboard: it is the input to an offline attack.
func TestScrubRemovesPasswordHashes(t *testing.T) {
	t.Parallel()

	hash := "$argon2id$v=19$m=65536,t=2,p=1$c29tZXNhbHQ$aGFzaHZhbHVlaGVyZQ"

	if scrubbed := telemetry.Scrub("stored credential unusable: " + hash); strings.Contains(scrubbed, hash) {
		t.Errorf("Scrub left a hash in place: %q", scrubbed)
	}
}

// Scrubbing must not make a message useless. An operator reading it needs to
// know what failed.
func TestScrubKeepsTheOperationalPart(t *testing.T) {
	t.Parallel()

	scrubbed := telemetry.Scrub("evaluation failed for session ses_7Kq2XA: provider timed out after 30s")

	for _, wanted := range []string{"evaluation failed", "provider timed out", "30s"} {
		if !strings.Contains(scrubbed, wanted) {
			t.Errorf("Scrub removed %q, leaving a message nobody can act on: %q", wanted, scrubbed)
		}
	}
}

// Anything long and unstructured is more likely to be content than a message.
// Truncation is a blunt instrument and the right one here: a dashboard cannot
// display a paragraph anyway, and a paragraph in telemetry is usually a
// transcript that arrived by accident.
func TestScrubTruncatesSomethingUnreasonablyLong(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("the candidate said something at length. ", 100)

	scrubbed := telemetry.Scrub(long)

	if len(scrubbed) > telemetry.MaxMessageLength+32 {
		t.Errorf("Scrub returned %d characters, want it truncated near %d",
			len(scrubbed), telemetry.MaxMessageLength)
	}
	if !strings.Contains(scrubbed, "truncated") {
		t.Error("truncation is silent, so a reader cannot tell the message is incomplete")
	}
}

func TestScrubLeavesAnOrdinaryMessageAlone(t *testing.T) {
	t.Parallel()

	const message = "session ses_7Kq2XA moved from composing to ready"

	if scrubbed := telemetry.Scrub(message); scrubbed != message {
		t.Errorf("Scrub changed an ordinary message:\n  before: %q\n  after:  %q", message, scrubbed)
	}
}

// ─────────────────────────────────────────────────────── detecting, not just removing

// Scrub redacts, because a log message with a credential in it is still worth
// having without the credential. Other callers need the opposite response: a
// workflow argument carrying restricted content must be refused outright, since
// there is no useful version of it with the content removed.
//
// Both answer the same question about the same four shapes, so the shapes are
// defined once and the two responses are built on it. Two copies of the
// patterns would drift, and the copy that was not updated would be the one
// deciding what reaches durable storage.

func TestFindRestrictedNamesTheShapeItFound(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"no user found for daniel.okonkwo@example.com":        "address",
		"rejected token ses_AbCdEf0123456789AbCdEf0123456789": "token",
		"dial postgres://prepeet:hunter2@db.internal:5432/x":  "credential",
		"stored $argon2id$v=19$m=65536,t=2,p=1$c2E$aGE":       "hash",
	}

	for text, wantShape := range cases {
		shape, found := telemetry.FindRestricted(text)
		if !found {
			t.Errorf("FindRestricted(%q) found nothing", text)
			continue
		}
		if !strings.Contains(shape, wantShape) {
			t.Errorf("FindRestricted(%q) said %q, want it to mention %q", text, shape, wantShape)
		}
	}
}

func TestFindRestrictedLeavesOrdinaryTextAlone(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"session ses_7Kq2XA moved from composing to ready",
		"01a0301d-aa10-7000-8f3e-1234567890ab",
		`{"session_id":"ses_7Kq2XA","rubric_version":4}`,
		"",
	} {
		if shape, found := telemetry.FindRestricted(text); found {
			t.Errorf("FindRestricted(%q) reported %q in text that carries nothing restricted", text, shape)
		}
	}
}

// The two must agree. Anything Scrub changes is something FindRestricted finds,
// or one of them is working from a different idea of what is restricted.
func TestScrubAndFindRestrictedAgree(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"no user found for daniel.okonkwo@example.com",
		"rejected token ses_AbCdEf0123456789AbCdEf0123456789",
		"dial postgres://prepeet:hunter2@db.internal:5432/x",
		"stored $argon2id$v=19$m=65536,t=2,p=1$c2E$aGE",
		"session ses_7Kq2XA moved from composing to ready",
		"nothing interesting here",
	} {
		_, found := telemetry.FindRestricted(text)
		changed := telemetry.Scrub(text) != text

		if found != changed {
			t.Errorf("for %q: FindRestricted says %v but Scrub changed it %v", text, found, changed)
		}
	}
}
