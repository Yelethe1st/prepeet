package notification

import (
	"fmt"
	"strings"
	"text/template"
)

// The templates, versioned.
//
// Plain text deliberately. Every mail client renders it, screen readers read
// it in order, and there is no tracking pixel to explain to a privacy review.
// The version bumps when the wording changes meaning, not for a typo fix,
// because its job is to answer "what did we tell this person" after the fact.
//
// A template accepts exactly its declared variable struct and nothing else:
// rendering resolves fields against that struct with missing keys as errors,
// so transcript or evaluation content cannot reach a body without someone
// first adding a field for it to a struct in this file, which is the review
// point INT-01's content rule hangs on.

// Input is a variable struct for exactly one template.
//
// Implementations are the closed set below. The interface is what lets the
// queue accept "an email" without a stringly-typed template name that could
// name a template the variables do not fit.
type Input interface {
	// template names which template these variables belong to.
	template() string
}

// VerifyEmail confirms a new address.
type VerifyEmail struct {
	// Link is the single-use verification URL.
	Link string
	// ExpiresMinutes is how long the link works, stated to the recipient so
	// the expired screen is never a surprise.
	ExpiresMinutes int
}

func (VerifyEmail) template() string { return "verify-email" }

// PasswordReset carries a password recovery link.
type PasswordReset struct {
	// Link is the single-use recovery URL.
	Link string
	// ExpiresMinutes is how long the link works.
	ExpiresMinutes int
}

func (PasswordReset) template() string { return "password-reset" }

// MagicLink signs the recipient in without a password.
type MagicLink struct {
	// Link is the single-use sign-in URL.
	Link string
	// ExpiresMinutes is how long the link works.
	ExpiresMinutes int
}

func (MagicLink) template() string { return "magic-link" }

// ScreeningInvitation invites a candidate to a campaign's interview.
//
// Employer and role are named because a candidate may hold invitations from
// several employers at once, and an email that could be any of them is one
// they cannot safely act on. No transcript or evaluation content appears here,
// nor could: an invitation precedes the interview it invites.
type ScreeningInvitation struct {
	// Employer is the workspace's display name, what the candidate recognises.
	Employer string
	// Role is the human role title the campaign interviews for.
	Role string
	// Link is the single-use acceptance URL.
	Link string
	// ExpiresHours is how long the invitation stands, in hours rather than
	// minutes because an invitation is answered over days, not in one sitting.
	ExpiresHours int
}

func (ScreeningInvitation) template() string { return "screening-invitation" }

// OTP carries a one-time code.
type OTP struct {
	// Code is the short-lived numeric code. A code rather than a link, for the
	// person typing it on a device other than the one that asked.
	Code string
	// ExpiresMinutes is how long the code works.
	ExpiresMinutes int
}

func (OTP) template() string { return "otp" }

// Rendered is a template applied to its variables, ready to enqueue.
type Rendered struct {
	Template string
	Version  string
	Subject  string
	Body     string
}

// definition is one template at one version.
type definition struct {
	version string
	subject string
	body    string
}

// registry is every template this system can send.
//
// The unsolicited-mail line appears in every body, because each of these can
// arrive unrequested (anyone can type any address into a form), and the person
// who did not ask deserves to be told that ignoring it is safe.
var registry = map[string]definition{
	"verify-email": {
		version: "1",
		subject: "Verify your email address",
		body: `Confirm your email address to finish setting up your Prepeet account:

{{.Link}}

The link works once and expires in {{.ExpiresMinutes}} minutes.

If you did not create a Prepeet account, ignore this email and nothing will happen.`,
	},
	"password-reset": {
		version: "1",
		subject: "Reset your password",
		body: `Use this link to choose a new password for your Prepeet account:

{{.Link}}

The link works once and expires in {{.ExpiresMinutes}} minutes.

If you did not ask to reset your password, ignore this email; your password has not changed.`,
	},
	"magic-link": {
		version: "1",
		subject: "Your sign-in link",
		body: `Use this link to sign in to Prepeet:

{{.Link}}

The link works once and expires in {{.ExpiresMinutes}} minutes.

If you did not ask to sign in, ignore this email and nothing will happen.`,
	},
	"otp": {
		version: "1",
		subject: "Your verification code",
		body: `Your Prepeet verification code is:

{{.Code}}

The code works once and expires in {{.ExpiresMinutes}} minutes.

If you did not ask for a code, ignore this email and nothing will happen.`,
	},
	"screening-invitation": {
		version: "1",
		subject: "You have been invited to an interview",
		body: `{{.Employer}} has invited you to a short voice interview for {{.Role}}.

{{.Link}}

The invitation is yours alone and expires in {{.ExpiresHours}} hours. Before you
begin, you will see exactly what is recorded, who can see it, and how the result
is used, and you decide whether to go ahead.

If you were not expecting this, you can ignore it; nothing happens until you open
the link and agree.`,
	},
}

// Templates returns the registered template names and versions.
//
// For the tests that assert coverage; nothing at runtime enumerates templates.
func Templates() map[string]string {
	names := make(map[string]string, len(registry))
	for name, def := range registry {
		names[name] = def.version
	}
	return names
}

// Render applies a template to its variables.
func Render(input Input) (Rendered, error) {
	return renderNamed(input.template(), input)
}

// renderNamed is Render with the template chosen separately, so a test can
// prove that wrong variables fail rather than rendering "<no value>".
func renderNamed(name string, variables any) (Rendered, error) {
	def, ok := registry[name]
	if !ok {
		return Rendered{}, fmt.Errorf("notification: no template named %q", name)
	}

	subject, err := render(name+":subject", def.subject, variables)
	if err != nil {
		return Rendered{}, err
	}
	body, err := render(name+":body", def.body, variables)
	if err != nil {
		return Rendered{}, err
	}

	return Rendered{Template: name, Version: def.version, Subject: subject, Body: body}, nil
}

// render executes one template with missing variables as errors.
func render(name, text string, variables any) (string, error) {
	parsed, err := template.New(name).Option("missingkey=error").Parse(text)
	if err != nil {
		return "", fmt.Errorf("notification: parsing %s: %w", name, err)
	}

	var out strings.Builder
	if err := parsed.Execute(&out, variables); err != nil {
		return "", fmt.Errorf("notification: rendering %s: %w", name, err)
	}
	return out.String(), nil
}
