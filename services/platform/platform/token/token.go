// Package token issues and verifies the opaque bearer tokens this product
// hands out: sessions, refresh tokens, email verification links, password
// resets, magic links and invitations.
//
// ADR-0003 chooses opaque tokens over JWTs because revocation is a requirement
// rather than a nicety: a recruiter removed from a tenant must stop being able
// to read candidate evidence within seconds, and a stateless token cannot be
// withdrawn without rebuilding the server-side lookup that stateless tokens
// exist to avoid.
//
// Two rules hold throughout. The plaintext exists for exactly one response and
// is never written down. The database stores only a hash, so reading the
// session table yields nothing that can be presented as a credential.
//
// The hash is SHA-256 rather than argon2id, deliberately. A password is a low
// entropy secret a person chose and needs an expensive hash to survive an
// offline attack. These tokens carry 256 bits of entropy from crypto/rand, so
// guessing one is not a strategy and a slow hash would only add latency to
// every authenticated request.
//
// Implements part of IAM-01.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// secretBytes is the entropy in every token. 256 bits, from crypto/rand.
const secretBytes = 32

// Purpose says what a token is for.
//
// Each purpose has its own prefix so a token appearing in a support ticket or a
// log announces what it is, and so a token issued for one purpose cannot be
// presented for another without the mismatch being visible.
type Purpose string

const (
	PurposeSession       Purpose = "session"
	PurposeRefresh       Purpose = "refresh"
	PurposeEmailVerify   Purpose = "email_verify"
	PurposePasswordReset Purpose = "password_reset"
	PurposeMagicLink     Purpose = "magic_link"
	PurposeOTP           Purpose = "otp"
	PurposeInvitation    Purpose = "invitation"
	PurposeOAuthState    Purpose = "oauth_state"
	PurposeOAuthVerifier Purpose = "oauth_verifier"
)

// prefixes maps each purpose to its wire prefix.
var prefixes = map[Purpose]string{
	PurposeSession:       "ses",
	PurposeRefresh:       "ref",
	PurposeEmailVerify:   "vrf",
	PurposePasswordReset: "rst",
	PurposeMagicLink:     "mgc",
	PurposeOTP:           "otp",
	PurposeInvitation:    "inv",
	PurposeOAuthState:    "oas",
	PurposeOAuthVerifier: "oav",
}

// Purposes returns every known purpose, in a stable order.
func Purposes() []Purpose {
	return []Purpose{
		PurposeSession, PurposeRefresh, PurposeEmailVerify,
		PurposePasswordReset, PurposeMagicLink, PurposeOTP, PurposeInvitation,
	}
}

// Issued is a freshly minted token.
//
// Plaintext goes to the user once and is then unrecoverable. Hash goes to the
// database. Nothing else holds either.
type Issued struct {
	Purpose   Purpose
	Plaintext string
	Hash      string
}

// String redacts the plaintext.
//
// The plaintext is a live credential, and a struct printed into a log with %v
// is the ordinary way one escapes. Implementing String here means that cannot
// happen by accident.
func (i Issued) String() string {
	return fmt.Sprintf("token.Issued{Purpose:%s Plaintext:[redacted] Hash:%s}", i.Purpose, i.Hash)
}

// New mints a token for a purpose.
func New(purpose Purpose) (Issued, error) {
	prefix, known := prefixes[purpose]
	if !known {
		return Issued{}, fmt.Errorf("token: %q is not a known purpose", purpose)
	}

	secret := make([]byte, secretBytes)
	if _, err := rand.Read(secret); err != nil {
		// A token without entropy is guessable, and a guessable session token
		// is an account takeover. Failing is the only safe response.
		return Issued{}, fmt.Errorf("token: reading entropy: %w", err)
	}

	plaintext := prefix + "_" + base64.RawURLEncoding.EncodeToString(secret)
	return Issued{Purpose: purpose, Plaintext: plaintext, Hash: HashOf(plaintext)}, nil
}

// OTPDigits is how long a one-time code is.
//
// Six digits is what people expect from every other service, short enough to
// type from a phone screen. It survives being guessable only because the
// store caps wrong attempts per token; the length is usability, the cap is
// the security.
const OTPDigits = 6

// NewOTP mints a one-time code.
//
// A code rather than a link, for the person reading it on one device and
// typing it on another. It hashes into the same store as every other token;
// the code carries no prefix because a person types it.
func NewOTP() (Issued, error) {
	code := make([]byte, 0, OTPDigits)
	for range OTPDigits {
		digit, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return Issued{}, fmt.Errorf("token: reading entropy: %w", err)
		}
		code = append(code, byte('0')+byte(digit.Int64()))
	}

	plaintext := string(code)
	return Issued{Purpose: PurposeOTP, Plaintext: plaintext, Hash: HashOf(plaintext)}, nil
}

// HashOf returns the stored form of a token.
//
// A lookup hashes what the client presented and searches for that, so the
// database never holds a usable credential and an index on the hash still
// works.
// ChallengeFor is the PKCE S256 challenge for a verifier.
//
// RFC 7636: the challenge is the base64url of the SHA-256 of the verifier,
// unpadded. It is sent to the authorisation endpoint in the open; the
// verifier is held back and presented at the token endpoint, so an
// authorisation code intercepted on the way back cannot be exchanged by
// whoever intercepted it.
//
// Unpadded and URL-safe are not cosmetic. Providers compare the string they
// were sent against the one they derive, and '+' or '=' in a query string
// survives a round trip differently depending on who encodes it.
func ChallengeFor(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func HashOf(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// Equals compares two stored hashes in constant time.
//
// A variable-time comparison leaks a valid hash one byte at a time to anyone
// who can measure the response, which is slow but real.
func Equals(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// PurposeOf reports which purpose a presented token claims, from its prefix.
//
// The prefix is a hint for routing and for humans, never a decision: the
// authoritative purpose is the one recorded in the database alongside the hash.
// Trusting the prefix would let a caller present a password reset token where a
// session is expected.
func PurposeOf(plaintext string) (Purpose, bool) {
	prefix, _, found := strings.Cut(plaintext, "_")
	if !found {
		return "", false
	}
	for purpose, candidate := range prefixes {
		if candidate == prefix {
			return purpose, true
		}
	}
	return "", false
}
