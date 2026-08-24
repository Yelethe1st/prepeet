package api_test

import (
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
)

// A post-login destination is attacker controlled: it arrives in a query
// parameter on a page anyone can link to. Sending someone to it after they
// authenticate is exactly the moment they trust what they see, which is what
// makes an open redirect worth exploiting.
func TestSafeRedirectAcceptsInternalPaths(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/candidate/dashboard",
		"/candidate/session/ses_7Kq2XA/results",
		"/admin/recruiter?status=pending",
		"/candidate/skills#ownership",
		"/",
	} {
		got, ok := api.SafeRedirect(path)
		if !ok {
			t.Errorf("SafeRedirect(%q) refused an internal path", path)
			continue
		}
		if got != path {
			t.Errorf("SafeRedirect(%q) = %q, want it unchanged", path, got)
		}
	}
}

func TestSafeRedirectRefusesAnythingOffSite(t *testing.T) {
	t.Parallel()

	for name, candidate := range map[string]string{
		"absolute http":               "http://evil.example.com/",
		"absolute https":              "https://evil.example.com/",
		"protocol relative":           "//evil.example.com/",
		"protocol relative backslash": `\\evil.example.com/`,
		"javascript":                  "javascript:alert(1)",
		"data":                        "data:text/html,<script>alert(1)</script>",
		"mailto":                      "mailto:someone@example.com",
		"backslash trick":             `/\evil.example.com`,
		"encoded scheme":              "%68ttp://evil.example.com",
		"whitespace prefix":           "  https://evil.example.com",
		"tab inside scheme":           "ht\ttps://evil.example.com",
		"newline":                     "/dashboard\nLocation: https://evil.example.com",
		"empty":                       "",
		"not a path":                  "candidate/dashboard",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, ok := api.SafeRedirect(candidate); ok {
				t.Errorf("SafeRedirect(%q) accepted it and returned %q", candidate, got)
			}
		})
	}
}

// Refusing is not enough on its own: the caller needs somewhere to send the
// person, and it must be a destination that is always safe.
func TestSafeRedirectFallbackIsUsable(t *testing.T) {
	t.Parallel()

	if _, ok := api.SafeRedirect(api.DefaultRedirect); !ok {
		t.Errorf("the default redirect %q is not itself accepted", api.DefaultRedirect)
	}
}
