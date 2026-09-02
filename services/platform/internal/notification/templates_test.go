package notification

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// update rewrites the golden files instead of comparing against them.
//
// The goldens are the preview INT-01 requires: what each template version
// renders is committed, so a template change is a readable diff in review
// rather than a surprise in somebody's inbox.
var update = flag.Bool("update", false, "rewrite the template golden files")

// inputs is one rendering of every template, with unmistakably fake values.
//
// Every template must appear here: the coverage test below fails on one that
// does not, because a template nobody rendered in a test is a template first
// rendered in production.
var inputs = map[string]Input{
	"verify-email":   VerifyEmail{Link: "https://app.example.test/verify?token=TOKEN", ExpiresMinutes: 30},
	"password-reset": PasswordReset{Link: "https://app.example.test/reset?token=TOKEN", ExpiresMinutes: 30},
	"magic-link":     MagicLink{Link: "https://app.example.test/magic?token=TOKEN", ExpiresMinutes: 15},
	"otp":            OTP{Code: "000000", ExpiresMinutes: 10},
	"screening-invitation": ScreeningInvitation{
		Employer: "Northwind Health", Role: "Registered Nurse, Intensive Care",
		Link: "https://app.example.test/invitations/accept?token=TOKEN", ExpiresHours: 72,
	},
}

func TestEveryTemplateRendersItsGolden(t *testing.T) {
	for name, input := range inputs {
		rendered, err := Render(input)
		if err != nil {
			t.Fatalf("rendering %s: %v", name, err)
		}

		if rendered.Template != name {
			t.Fatalf("input for %s rendered template %s", name, rendered.Template)
		}
		if rendered.Version == "" {
			t.Errorf("%s carries no version, so a sent email could not say which wording it used", name)
		}
		if rendered.Subject == "" || rendered.Body == "" {
			t.Errorf("%s rendered an empty subject or body", name)
		}

		golden := filepath.Join("testdata", name+".golden.txt")
		content := "Subject: " + rendered.Subject + "\n\n" + rendered.Body
		if *update {
			if err := os.WriteFile(golden, []byte(content), 0o644); err != nil {
				t.Fatalf("writing %s: %v", golden, err)
			}
			continue
		}

		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("reading %s (run with -update to create it): %v", golden, err)
		}
		if string(want) != content {
			t.Errorf("%s renders differently from its golden.\n"+
				"If the change is intended, rerun with -update and review the diff.\ngot:\n%s", name, content)
		}
	}
}

func TestEveryTemplateIsRenderedByATest(t *testing.T) {
	// The registry is the source; the inputs map above must cover it.
	for name := range Templates() {
		if _, covered := inputs[name]; !covered {
			t.Errorf("%s has no test input, so nothing previews it before it is sent", name)
		}
	}
	for name := range inputs {
		if _, exists := Templates()[name]; !exists {
			t.Errorf("the test renders %s, which is not a registered template", name)
		}
	}
}

func TestAnUndeclaredVariableIsARenderError(t *testing.T) {
	// text/template silently prints "<no value>" for a missing key unless told
	// otherwise, which would put that string in front of a person. This pins
	// the option that turns it into an error instead.
	type wrong struct{ Nonexistent string }
	_, err := renderNamed("verify-email", wrong{Nonexistent: "x"})
	if err == nil {
		t.Fatal("rendering with the wrong variables succeeded; missing variables would reach recipients as <no value>")
	}
}

func TestEveryExpiryIsStatedToTheRecipient(t *testing.T) {
	// The prototype distinguishes expired-token outcomes, and the email is
	// where the person learns the deadline exists. A token email that does not
	// state its expiry turns the expired screen into a surprise.
	//
	// Any unit, not only minutes: a sign-in link lives for minutes and an
	// invitation for days, and pinning the assertion to "minutes" would force
	// an invitation to lie about its own lifetime to satisfy a test.
	for name, input := range inputs {
		rendered, err := Render(input)
		if err != nil {
			t.Fatalf("rendering %s: %v", name, err)
		}
		if !strings.Contains(rendered.Body, "minutes") &&
			!strings.Contains(rendered.Body, "hours") {
			t.Errorf("%s does not tell the recipient how long they have", name)
		}
	}
}

func TestNoTemplateAsksForCredentials(t *testing.T) {
	// A legitimate email that asks someone to reply with a password trains
	// people to answer the phishing email that imitates it.
	for name, input := range inputs {
		rendered, _ := Render(input)
		lowered := strings.ToLower(rendered.Subject + " " + rendered.Body)
		if strings.Contains(lowered, "reply with") {
			t.Errorf("%s asks the recipient to reply with something", name)
		}
	}
}
