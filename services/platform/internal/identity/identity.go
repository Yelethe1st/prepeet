// Package identity owns users, credentials and sessions.
//
// ADR-0003 decided to build authentication rather than buy it, which makes
// everything here a standing obligation rather than a one-off implementation.
// Three properties are load bearing and are asserted by tests rather than
// assumed:
//
// Nothing reveals whether an address exists. Registration answers identically
// for a new and an existing address, login answers identically for a wrong
// password and an unknown user, and the unknown path spends comparable time so
// the clock does not say what the body will not.
//
// Sessions are revocable. They are rows rather than self-describing tokens,
// because a recruiter removed from a tenant must stop being able to read
// candidate evidence within seconds.
//
// A reused refresh token is treated as theft. It revokes the whole family
// descended from that login, because being logged out is a cheap failure and an
// attacker keeping a foothold is not.
//
// A user is not owned by a tenant. The same person practises privately and may
// screen for several employers, and their practice history is never reachable
// from any employer authority.
//
// Implements part of IAM-01.
package identity

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Length bounds.
//
// The password minimum is a real floor. The maximum is not a strength policy:
// argon2 hashes whatever it is given, so an unbounded input is a denial of
// service rather than a strong password.
const (
	maxEmailLength    = 320
	minPasswordLength = 12
	maxPasswordLength = 1024
	maxOrgNameLength  = 200
)

// AccountType distinguishes the two registration paths.
type AccountType string

const (
	// AccountCandidate registers a person for themselves.
	AccountCandidate AccountType = "candidate"
	// AccountOrganisation additionally creates a tenant and an owning
	// membership.
	AccountOrganisation AccountType = "organisation"
)

// Validation errors. These are shown to the person typing, so each says what to
// fix and none echoes what was typed.
var (
	ErrEmailInvalid     = errors.New("identity: that does not look like an email address we can deliver to")
	ErrPasswordTooShort = fmt.Errorf("identity: a password needs at least %d characters", minPasswordLength)
	ErrPasswordTooLong  = fmt.Errorf("identity: a password cannot be longer than %d characters", maxPasswordLength)
	ErrAccountType      = errors.New("identity: account type must be candidate or organisation")
	ErrOrganisationName = errors.New("identity: an organisation registration needs an organisation name")
)

// NormaliseEmail prepares an address for storage and comparison.
//
// Case folding and trimming only. Treating dots or plus addressing as
// equivalent is a provider-specific rule, and applying it here would merge two
// addresses the provider considers different, which silently hands one person's
// account to another.
func NormaliseEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// ValidateEmail checks an address is one we could deliver to.
//
// This is deliberately not a full RFC 5322 parse. The useful checks are that it
// has exactly one at sign with something either side, that it fits, and that it
// carries no control characters. The last one matters most: a newline in an
// address is a header injection into the verification email we are about to
// send.
func ValidateEmail(raw string) error {
	address := NormaliseEmail(raw)

	if address == "" || len(address) > maxEmailLength {
		return ErrEmailInvalid
	}
	for _, r := range address {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return ErrEmailInvalid
		}
	}
	// A percent-encoded newline is the same injection wearing a hat.
	lowered := strings.ToLower(address)
	if strings.Contains(lowered, "%0a") || strings.Contains(lowered, "%0d") {
		return ErrEmailInvalid
	}

	local, domain, found := strings.Cut(address, "@")
	if !found || local == "" || domain == "" {
		return ErrEmailInvalid
	}
	if strings.Contains(domain, "@") {
		return ErrEmailInvalid
	}
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return ErrEmailInvalid
	}
	return nil
}

// ValidatePassword checks length only.
//
// Composition rules, meaning a symbol and a digit and a capital, push people
// towards predictable substitutions and away from length, which is the property
// that actually matters. Strength beyond length belongs in a breached-password
// check, which arrives before the practice release gate.
func ValidatePassword(plaintext string) error {
	switch {
	case len(plaintext) < minPasswordLength:
		return ErrPasswordTooShort
	case len(plaintext) > maxPasswordLength:
		return ErrPasswordTooLong
	default:
		return nil
	}
}

// ValidateAccountType checks the registration path and what it requires.
func ValidateAccountType(accountType AccountType, organisationName string) error {
	switch accountType {
	case AccountCandidate:
		return nil
	case AccountOrganisation:
		name := strings.TrimSpace(organisationName)
		if name == "" || len(name) > maxOrgNameLength {
			return ErrOrganisationName
		}
		return nil
	default:
		return ErrAccountType
	}
}
