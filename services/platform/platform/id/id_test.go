package id_test

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

var canonical = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewProducesCanonicalUUIDv7(t *testing.T) {
	t.Parallel()

	got := id.New().String()

	if !canonical.MatchString(got) {
		t.Errorf("String() = %q, want a canonical UUIDv7 with version 7 and RFC 4122 variant", got)
	}
}

func TestNewIsUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, 10000)
	for range 10000 {
		got := id.New().String()
		if _, duplicate := seen[got]; duplicate {
			t.Fatalf("New() produced a duplicate after %d values: %q", len(seen), got)
		}
		seen[got] = struct{}{}
	}
}

// UUIDv7 is time ordered. Identifiers are used as primary keys, so lexical
// order has to match creation order or every insert fragments the index.
func TestNewIsLexicallyOrderedByCreationTime(t *testing.T) {
	t.Parallel()

	const count = 500
	values := make([]string, count)
	for i := range count {
		values[i] = id.New().String()
	}

	ordered := make([]string, count)
	copy(ordered, values)
	sort.Strings(ordered)

	for i := range values {
		if values[i] != ordered[i] {
			t.Fatalf("values are not lexically ordered by creation time at index %d", i)
		}
	}
}

// Identifiers are opaque to clients but readable to operators. A prefix says
// what kind of thing an identifier refers to without encoding tenant or any
// other authorization input into it.
func TestPrefixedProducesAReadableOpaqueIdentifier(t *testing.T) {
	t.Parallel()

	got := id.Prefixed("ses")

	if !strings.HasPrefix(got, "ses_") {
		t.Errorf("Prefixed(%q) = %q, want it to start with %q", "ses", got, "ses_")
	}
	if strings.Count(got, "_") != 1 {
		t.Errorf("Prefixed() = %q, want exactly one separator", got)
	}
	if suffix := strings.TrimPrefix(got, "ses_"); len(suffix) == 0 {
		t.Errorf("Prefixed() = %q, want a non-empty suffix", got)
	}
}

func TestPrefixedIsUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		got := id.Prefixed("req")
		if _, duplicate := seen[got]; duplicate {
			t.Fatalf("Prefixed() produced a duplicate: %q", got)
		}
		seen[got] = struct{}{}
	}
}

// A prefixed identifier is not a canonical UUID and must not be mistaken for
// one by a caller parsing it.
func TestPrefixedIsNotCanonicalUUID(t *testing.T) {
	t.Parallel()

	if got := id.Prefixed("ses"); canonical.MatchString(got) {
		t.Errorf("Prefixed() = %q, want a prefixed form rather than a bare UUID", got)
	}
}

func TestParseRoundTripsACanonicalUUID(t *testing.T) {
	t.Parallel()

	original := id.New()

	parsed, err := id.Parse(original.String())
	if err != nil {
		t.Fatalf("Parse(%q) returned error: %v", original.String(), err)
	}
	if parsed != original {
		t.Errorf("Parse(String()) = %v, want %v", parsed, original)
	}
}

func TestParseRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"empty":            "",
		"too short":        "0190a1b2-c3d4",
		"not hexadecimal":  "0190a1b2-c3d4-7xyz-8abc-0123456789ab",
		"missing hyphens":  "0190a1b2c3d47abc8abc0123456789ab",
		"wrong version":    "0190a1b2-c3d4-4abc-8abc-0123456789ab",
		"wrong variant":    "0190a1b2-c3d4-7abc-1abc-0123456789ab",
		"trailing garbage": "0190a1b2-c3d4-7abc-8abc-0123456789ab ",
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := id.Parse(input); err == nil {
				t.Errorf("Parse(%q) returned no error, want one", input)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────── suffix

// Suffix disambiguates a human-chosen name that collided, such as two
// organisations both called Acme.
func TestSuffixIsShortAndUrlSafe(t *testing.T) {
	t.Parallel()

	for range 200 {
		suffix := id.Suffix()

		if len(suffix) < 4 || len(suffix) > 8 {
			t.Fatalf("Suffix() = %q, which is %d characters; it appears in URLs beside a name",
				suffix, len(suffix))
		}
		for _, r := range suffix {
			isLower := r >= 'a' && r <= 'z'
			isDigit := r >= '0' && r <= '9'
			if !isLower && !isDigit {
				t.Fatalf("Suffix() = %q, which contains %q; a slug is lower case letters and digits",
					suffix, r)
			}
		}
	}
}

// A counter would make slugs enumerable: acme-2 existing tells anyone that two
// organisations called Acme registered, which is not theirs to learn.
func TestSuffixIsNotSequential(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		seen[id.Suffix()] = struct{}{}
	}

	// Collisions are possible and fine, since the caller retries. What must not
	// happen is a small or predictable range.
	if len(seen) < 900 {
		t.Errorf("1000 suffixes produced only %d distinct values, so they are predictable enough "+
			"to enumerate", len(seen))
	}
}
