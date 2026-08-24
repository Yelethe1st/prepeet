package identity

import (
	"strings"
	"unicode"
)

// maxSlugLength bounds a tenant slug.
//
// A slug appears in URLs and in operator tooling, so an unbounded one is a
// display problem everywhere it is shown. Sixty-three matches the DNS label
// limit, which is the shape a slug is most likely to end up in.
const maxSlugLength = 63

// Slugify turns an organisation name into a URL-safe identifier.
//
// It is deliberately lossy and deliberately not unique. Two organisations may
// legitimately be called Acme, and refusing the second signup because of the
// first would be absurd, so uniqueness is the repository's problem and this
// only has to produce a reasonable starting point.
//
// Non-ASCII letters are dropped rather than transliterated. Transliteration
// needs a table per script and gets names wrong in ways their owners find
// insulting, so a name that reduces to nothing falls back to a generic slug and
// the tenant is addressed by its identifier instead. The display name, which is
// what people actually see, keeps every character exactly as given.
func Slugify(name string) string {
	var builder strings.Builder
	lastWasSeparator := true // leading separators are dropped

	for _, r := range strings.ToLower(name) {
		switch {
		case r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			builder.WriteRune(r)
			lastWasSeparator = false
		case !lastWasSeparator:
			// Any run of anything else becomes one hyphen, so "Acme  &  Co"
			// does not become "acme----co".
			builder.WriteByte('-')
			lastWasSeparator = true
		}
	}

	slug := strings.Trim(builder.String(), "-")
	if len(slug) > maxSlugLength {
		slug = strings.Trim(slug[:maxSlugLength], "-")
	}
	if slug == "" {
		// A name made entirely of characters this drops. The tenant still needs
		// a slug, and the identifier suffix the repository adds makes it usable.
		return "org"
	}
	return slug
}
