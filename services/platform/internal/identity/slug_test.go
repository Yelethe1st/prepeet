package identity_test

import (
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
)

func TestSlugifyProducesAUsableIdentifier(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"Northwind Recruiting":     "northwind-recruiting",
		"Acme":                     "acme",
		"ACME":                     "acme",
		"Acme  &  Co":              "acme-co",
		"  Leading and trailing  ": "leading-and-trailing",
		"Hyphen-Already":           "hyphen-already",
		"Numbers 123 Ltd":          "numbers-123-ltd",
		"...":                      "org",
		"":                         "org",
	}

	for name, want := range cases {
		if got := identity.Slugify(name); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", name, got, want)
		}
	}
}

// A slug reaches URLs and operator tooling, so anything that could be read as
// path structure, a query, or markup must not survive.
func TestSlugifyRemovesAnythingStructural(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"Acme/../etc/passwd",
		"Acme?admin=true",
		"<script>alert(1)</script>",
		"Acme#fragment",
		"Acme%20Ltd",
		"Acme\x00Ltd",
	} {
		slug := identity.Slugify(name)
		for _, forbidden := range []string{"/", "?", "#", "%", "<", ">", "\x00", " ", "="} {
			if strings.Contains(slug, forbidden) {
				t.Errorf("Slugify(%q) = %q, which contains %q", name, slug, forbidden)
			}
		}
	}
}

// A slug that is only hyphens is not a slug. This is the case a naive
// implementation produces from a name with no ASCII letters at all.
func TestSlugifyNeverReturnsSomethingEmptyOrPunctuation(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"---", "   ", "!!!", "日本語", "Ω"} {
		slug := identity.Slugify(name)
		if slug == "" {
			t.Errorf("Slugify(%q) returned an empty slug", name)
		}
		if strings.Trim(slug, "-") == "" {
			t.Errorf("Slugify(%q) = %q, which is only separators", name, slug)
		}
	}
}

// A slug appears in URLs, so an unbounded one is a display problem everywhere
// it is shown.
func TestSlugifyIsBounded(t *testing.T) {
	t.Parallel()

	slug := identity.Slugify(strings.Repeat("Northwind Recruiting ", 20))

	if len(slug) > 63 {
		t.Errorf("Slugify produced %d characters, want at most 63", len(slug))
	}
	if strings.HasSuffix(slug, "-") {
		t.Errorf("truncation left a trailing separator: %q", slug)
	}
}

// The display name is not the slug. A name is how an organisation writes
// itself, and reducing it is this function's job only for the identifier.
func TestSlugifyDoesNotClaimToPreserveTheName(t *testing.T) {
	t.Parallel()

	if got := identity.Slugify("Ünïcôdé Ltd"); strings.ContainsAny(got, "Üïôé") {
		t.Errorf("Slugify(%q) = %q, and non-ASCII is dropped rather than kept", "Ünïcôdé Ltd", got)
	}
}
