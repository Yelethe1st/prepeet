package identity_test

import (
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
)

// Addresses are normalised before they are stored or compared, or the same
// person registers twice by capitalising differently and then cannot log in.
func TestNormaliseEmail(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ in, want string }{
		"lowercases":        {"Daniel.Okonkwo@Example.COM", "daniel.okonkwo@example.com"},
		"trims surrounding": {"  daniel@example.com  ", "daniel@example.com"},
		"already normal":    {"daniel@example.com", "daniel@example.com"},
		"trims a tab":       {"\tdaniel@example.com\n", "daniel@example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := identity.NormaliseEmail(tc.in); got != tc.want {
				t.Errorf("NormaliseEmail(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The local part is left alone. Treating dots or plus addressing as equivalent
// is a provider-specific rule, and applying it would merge two addresses that
// the provider considers different.
func TestNormaliseEmailDoesNotRewriteTheLocalPart(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"daniel.okonkwo@example.com",
		"daniel+prepeet@example.com",
		"d.a.n.i.e.l@example.com",
	} {
		if got := identity.NormaliseEmail(address); got != address {
			t.Errorf("NormaliseEmail(%q) = %q, want the local part untouched", address, got)
		}
	}
}

func TestValidateEmailAcceptsOrdinaryAddresses(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"daniel@example.com",
		"daniel.okonkwo@sub.example.co.uk",
		"d+tag@example.io",
	} {
		if err := identity.ValidateEmail(address); err != nil {
			t.Errorf("ValidateEmail(%q) returned %v, want nil", address, err)
		}
	}
}

func TestValidateEmailRejectsWhatCannotBeDelivered(t *testing.T) {
	t.Parallel()

	for name, address := range map[string]string{
		"empty":        "",
		"no at":        "danielexample.com",
		"no domain":    "daniel@",
		"no local":     "@example.com",
		"space inside": "daniel okonkwo@example.com",
		"newline":      "daniel@example.com\nBcc: someone@else.com",
		"too long":     strings.Repeat("a", 310) + "@example.com",
		"two at signs": "daniel@example@com",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := identity.ValidateEmail(address); err == nil {
				t.Errorf("ValidateEmail(%q) returned nil, want an error", address)
			}
		})
	}
}

// A header injection through an address would let someone add recipients to the
// verification email we send.
func TestValidateEmailRejectsHeaderInjection(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"daniel@example.com\r\nBcc: attacker@example.com",
		"daniel@example.com%0aBcc:attacker@example.com",
		"daniel@example.com\x00",
	} {
		if err := identity.ValidateEmail(address); err == nil {
			t.Errorf("ValidateEmail(%q) accepted an address carrying control characters", address)
		}
	}
}

// The minimum is a real floor rather than a gesture. The maximum exists because
// argon2 hashes whatever it is given, so an unbounded input is a denial of
// service rather than a strong password.
func TestValidatePasswordBounds(t *testing.T) {
	t.Parallel()

	if err := identity.ValidatePassword(strings.Repeat("a", 11)); err == nil {
		t.Error("ValidatePassword accepted an 11 character password")
	}
	if err := identity.ValidatePassword(strings.Repeat("a", 12)); err != nil {
		t.Errorf("ValidatePassword rejected a 12 character password: %v", err)
	}
	if err := identity.ValidatePassword(strings.Repeat("a", 2048)); err == nil {
		t.Error("ValidatePassword accepted a 2048 byte password")
	}
}

// A validation error is shown to the person typing, so it must say what to fix
// and must never echo what they typed.
func TestValidatePasswordErrorDoesNotEchoThePassword(t *testing.T) {
	t.Parallel()

	const secret = "shortsecret"

	err := identity.ValidatePassword(secret)
	if err == nil {
		t.Fatal("ValidatePassword accepted a short password")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error contains the password: %v", err)
	}
}

func TestValidateAccountType(t *testing.T) {
	t.Parallel()

	if err := identity.ValidateAccountType(identity.AccountCandidate, ""); err != nil {
		t.Errorf("a candidate registration was rejected: %v", err)
	}
	if err := identity.ValidateAccountType(identity.AccountOrganisation, "Northwind Health"); err != nil {
		t.Errorf("an organisation registration with a name was rejected: %v", err)
	}
	if err := identity.ValidateAccountType(identity.AccountOrganisation, ""); err == nil {
		t.Error("an organisation registration with no name was accepted")
	}
	if err := identity.ValidateAccountType(identity.AccountType("platform"), ""); err == nil {
		t.Error("an unknown account type was accepted")
	}
}
