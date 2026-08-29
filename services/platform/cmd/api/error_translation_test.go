package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
)

// Every named refusal reaches the client as itself.
//
// ADR-0005 puts a translation between identity and the API, and a translation
// is a place things fall through. An identity error nobody mapped does not
// fail loudly: it reaches the response writer as an unrecognised error and
// becomes a 500 with a generic message, for a condition the domain took the
// trouble to name. The person is told the product is broken when the truth was
// "that code is wrong" or "that workspace is not yours".
//
// The sentinels are read from the source rather than listed here, so the check
// covers the one somebody adds next. That is the whole point: a list would be
// as easy to forget as the mapping.

// deliberatelyUnmapped are the errors that must not become a client-facing
// answer, each with the reason it is here.
//
// A declaration, so that leaving an error out is a decision in a diff rather
// than an omission nobody sees.
var deliberatelyUnmapped = map[string]string{
	// Elevation is an operator concern reached through OPS-07's console, not
	// through this API. There is no candidate-facing surface to answer on.
	"ErrElevationGone":   "platform elevation is an operator flow, not an API one",
	"ErrElevationReason": "platform elevation is an operator flow, not an API one",
	"ErrElevationTicket": "platform elevation is an operator flow, not an API one",
}

// identityErrors reads every exported error sentinel out of internal/identity.
func identityErrors(t *testing.T) []string {
	t.Helper()

	found := []string{}
	entries, err := os.ReadDir(filepath.Join("..", "..", "internal", "identity"))
	if err != nil {
		t.Fatalf("reading internal/identity: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join("..", "..", "internal", "identity", entry.Name())
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", path, parseErr)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			declaration, ok := node.(*ast.GenDecl)
			if !ok || declaration.Tok != token.VAR {
				return true
			}
			for _, spec := range declaration.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range value.Names {
					if strings.HasPrefix(name.Name, "Err") && name.IsExported() {
						found = append(found, name.Name)
					}
				}
			}
			return true
		})
	}

	sort.Strings(found)
	return found
}

// byName resolves a sentinel to the value, because reflection over a package's
// variables is not possible and the alternative is a list that goes stale.
//
// It is exhaustive by construction: the test below fails on any name it cannot
// resolve, so adding a sentinel to identity and not to this map is caught here
// rather than by the mapping being silently skipped.
func byName() map[string]error {
	return map[string]error{
		"ErrAccountType":          identity.ErrAccountType,
		"ErrCodeIncorrect":        identity.ErrCodeIncorrect,
		"ErrCredentialsInvalid":   identity.ErrCredentialsInvalid,
		"ErrElevationGone":        identity.ErrElevationGone,
		"ErrElevationReason":      identity.ErrElevationReason,
		"ErrElevationTicket":      identity.ErrElevationTicket,
		"ErrEmailInvalid":         identity.ErrEmailInvalid,
		"ErrMemberExists":         identity.ErrMemberExists,
		"ErrMemberNotFound":       identity.ErrMemberNotFound,
		"ErrMemberOwner":          identity.ErrMemberOwner,
		"ErrMemberRoleInvalid":    identity.ErrMemberRoleInvalid,
		"ErrMemberStale":          identity.ErrMemberStale,
		"ErrMemberUnknownEmail":   identity.ErrMemberUnknownEmail,
		"ErrNoMembership":         identity.ErrNoMembership,
		"ErrNotFound":             identity.ErrNotFound,
		"ErrOAuthEmailUnverified": identity.ErrOAuthEmailUnverified,
		"ErrOAuthProviderUnknown": identity.ErrOAuthProviderUnknown,
		"ErrOAuthStateExpired":    identity.ErrOAuthStateExpired,
		"ErrOAuthStateInvalid":    identity.ErrOAuthStateInvalid,
		"ErrOrganisationName":     identity.ErrOrganisationName,
		"ErrPasswordTooLong":      identity.ErrPasswordTooLong,
		"ErrPasswordTooShort":     identity.ErrPasswordTooShort,
		"ErrSessionInvalid":       identity.ErrSessionInvalid,
		"ErrTokenExpired":         identity.ErrTokenExpired,
		"ErrTokenInvalid":         identity.ErrTokenInvalid,
		"ErrTokenSuperseded":      identity.ErrTokenSuperseded,
		"ErrTokenUsed":            identity.ErrTokenUsed,
		"ErrTooManyAttempts":      identity.ErrTooManyAttempts,
	}
}

// translators are every place an identity error becomes an API one.
func translators() []func(error) error {
	adapter := identityAdapter{}
	return []func(error) error{
		adapter.translate,
		adapter.translateOAuth,
		translateMemberError,
	}
}

func TestEveryIdentityErrorIsResolvable(t *testing.T) {
	known := byName()

	missing := []string{}
	for _, name := range identityErrors(t) {
		if _, ok := known[name]; !ok {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("these sentinels exist in internal/identity and are unknown to this test,\n"+
			"so nothing here checks whether they reach a client as themselves:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

func TestEveryIdentityErrorBecomesAnApiAnswer(t *testing.T) {
	unmapped := []string{}

	for name, sentinel := range byName() {
		if _, allowed := deliberatelyUnmapped[name]; allowed {
			continue
		}

		translated := false
		for _, translate := range translators() {
			if got := translate(sentinel); got != nil && !errors.Is(got, sentinel) {
				translated = true
				break
			}
		}
		if !translated {
			unmapped = append(unmapped, name)
		}
	}

	sort.Strings(unmapped)
	if len(unmapped) > 0 {
		t.Fatalf("these refusals reach the client as a 500 rather than as themselves:\n  %s\n\n"+
			"Map each one, or declare it in deliberatelyUnmapped with the reason.",
			strings.Join(unmapped, "\n  "))
	}
}

// The declaration cannot outlive the errors it names, or it becomes a list of
// exemptions for things that no longer exist and quietly covers a real gap the
// day one of those names is reused.
func TestNothingIsExemptedThatDoesNotExist(t *testing.T) {
	known := byName()

	stale := []string{}
	for name := range deliberatelyUnmapped {
		if _, exists := known[name]; !exists {
			stale = append(stale, name)
		}
	}

	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("these are exempted and no longer exist:\n  %s", strings.Join(stale, "\n  "))
	}
}

// A translation that returns nil would be worse than one that returns the
// original: the handler would treat the call as having succeeded.
func TestNoTranslationTurnsAFailureIntoASuccess(t *testing.T) {
	for name, sentinel := range byName() {
		for index, translate := range translators() {
			if translate(sentinel) == nil {
				t.Fatalf("translator %d turned %s into a success", index, name)
			}
		}
	}

	// And nil stays nil, or every successful call would start failing.
	for index, translate := range translators() {
		if got := translate(nil); got != nil {
			t.Fatalf("translator %d turned success into %v", index, got)
		}
	}
}

// A sanity check that the mapping is real rather than everything collapsing
// into one answer, which would pass the test above and tell a person nothing.
func TestDistinctRefusalsStayDistinct(t *testing.T) {
	adapter := identityAdapter{}

	if errors.Is(adapter.translate(identity.ErrTokenExpired), api.ErrTokenUsed) {
		t.Fatal("an expired token and a used one are the same answer")
	}
	if errors.Is(adapter.translateOAuth(identity.ErrOAuthStateExpired), api.ErrOAuthStateRejected) {
		t.Fatal("an expired sign-in and a rejected one are the same answer")
	}
}
