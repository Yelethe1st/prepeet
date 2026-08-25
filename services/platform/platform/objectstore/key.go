// Package objectstore stores and serves the candidate media, documents and
// exports the product produces.
//
// Everything here is Restricted content under docs/security/data-classification.md:
// a recording of a named person taking part in a hiring process. Two rules
// follow, and this package enforces both rather than trusting callers.
//
// Object keys are derived, never accepted. A caller supplying a raw key could
// write into another tenant's prefix, so the key is built from validated parts.
//
// Access is short lived. A presigned URL is a bearer token for a candidate's
// recording, so its lifetime is clamped here rather than left to the caller.
//
// The key path carries tenant and session so that lifecycle rules and
// reconciliation can work, but it is never an authorization input. Authorization
// is decided against the active tenant on the request, exactly as it is for
// resource identifiers. See docs/contracts/public-api.md.
//
// Implements part of PLT-05.
package objectstore

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

// Presigned URL lifetimes.
//
// The maximum is deliberately short. Anyone holding the URL can fetch the
// object, and unlike a session it cannot be revoked once issued, so the only
// control available is that it expires quickly.
const (
	MinPresignTTL = 30 * time.Second
	MaxPresignTTL = 15 * time.Minute
)

// ErrInvalidKey is returned when key parts would produce an unsafe key.
var ErrInvalidKey = errors.New("objectstore: invalid key parts")

// Purpose separates objects that are retained on different schedules. Media,
// documents and exports each expire under their own rule, which DEC-15 sets, so
// they must be separable by prefix for a lifecycle policy to act on them.
type Purpose string

const (
	PurposeMedia    Purpose = "media"
	PurposeDocument Purpose = "document"
	PurposeExport   Purpose = "export"
)

// valid reports whether the purpose is one this package knows. An unknown
// purpose is rejected rather than passed through, because an object stored
// under an unrecognised prefix would never be caught by a lifecycle rule and
// would be retained indefinitely.
func (p Purpose) valid() bool {
	switch p {
	case PurposeMedia, PurposeDocument, PurposeExport:
		return true
	default:
		return false
	}
}

// KeyParts are the validated inputs a key is built from.
type KeyParts struct {
	TenantID  string
	SessionID string
	Purpose   Purpose
	// Name is the object's own name within its session, such as
	// "candidate-audio.opus". It is caller supplied and therefore untrusted.
	Name string
}

// Key is a validated object key.
//
// It deliberately exposes only String(). There is no TenantID accessor: a
// caller that could read a tenant back out of a key would eventually authorize
// against it, and the key is not evidence of anything. It is a storage address.
type Key struct {
	value string
}

// String returns the key as stored.
func (k Key) String() string { return k.value }

// NewKey derives a safe object key from validated parts.
//
// The name is checked rather than escaped. Escaping would let a hostile name
// through in a mangled form, and there is no legitimate reason for a name to
// contain a separator, a traversal sequence or a control character.
func NewKey(parts KeyParts) (Key, error) {
	if parts.TenantID == "" {
		return Key{}, fmt.Errorf("%w: tenant is required", ErrInvalidKey)
	}
	if parts.SessionID == "" {
		return Key{}, fmt.Errorf("%w: session is required", ErrInvalidKey)
	}
	if !parts.Purpose.valid() {
		return Key{}, fmt.Errorf("%w: purpose %q is not one this package stores", ErrInvalidKey, parts.Purpose)
	}
	if err := validateName(parts.Name); err != nil {
		return Key{}, err
	}

	return Key{value: path.Join(
		"tenant", parts.TenantID,
		"session", parts.SessionID,
		string(parts.Purpose),
		parts.Name,
	)}, nil
}

// NewCandidateKey derives the key for a candidate-owned document.
//
// Candidate documents belong to a person, not to a tenant or a session, so
// they live under their own root: a CV keyed by tenant would be exactly the
// linkage IAM-06 forbids, recorded in storage layout. The purpose is fixed to
// document because that is the only thing a candidate stores directly; the
// prefix still separates by purpose so DEC-15's lifecycle rules can act.
func NewCandidateKey(userID, name string) (Key, error) {
	if userID == "" {
		return Key{}, fmt.Errorf("%w: user is required", ErrInvalidKey)
	}
	if err := validateName(name); err != nil {
		return Key{}, err
	}
	return Key{value: path.Join(
		"candidate", userID, string(PurposeDocument), name,
	)}, nil
}

// Prefix returns the key prefix for one session's objects of one purpose. It is
// what reconciliation and lifecycle rules operate on.
func Prefix(tenantID, sessionID string, purpose Purpose) (string, error) {
	if tenantID == "" || sessionID == "" || !purpose.valid() {
		return "", fmt.Errorf("%w: tenant, session and a known purpose are required", ErrInvalidKey)
	}
	return path.Join("tenant", tenantID, "session", sessionID, string(purpose)) + "/", nil
}

// validateName rejects any name that could escape its prefix or confuse a
// downstream consumer.
func validateName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("%w: name is required", ErrInvalidKey)
	case len(name) > 255:
		return fmt.Errorf("%w: name is longer than 255 bytes", ErrInvalidKey)
	case strings.ContainsAny(name, `/\`):
		return fmt.Errorf("%w: name %q contains a path separator", ErrInvalidKey, name)
	case name == "." || name == "..":
		return fmt.Errorf("%w: name %q is a path element", ErrInvalidKey, name)
	case strings.Contains(name, ".."):
		return fmt.Errorf("%w: name %q contains a traversal sequence", ErrInvalidKey, name)
	}

	for _, r := range name {
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("%w: name contains a control character", ErrInvalidKey)
		}
	}
	return nil
}

// ClampTTL bounds a presigned URL lifetime.
//
// Clamping rather than erroring is deliberate: a caller asking for too long has
// made a judgement error, not a security decision, and the safe response is to
// issue a shorter URL rather than fail the candidate's playback.
func ClampTTL(requested time.Duration) time.Duration {
	switch {
	case requested < MinPresignTTL:
		return MinPresignTTL
	case requested > MaxPresignTTL:
		return MaxPresignTTL
	default:
		return requested
	}
}
