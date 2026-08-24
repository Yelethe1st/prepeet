// Package telemetry produces traces, metrics and logs, and makes it structurally
// difficult for restricted content to reach any of them.
//
// docs/security/data-classification.md classifies transcript text, evaluation
// prose and candidate contact details as Restricted. Telemetry leaves this
// system: it goes to a vendor, is retained on somebody else's schedule, and is
// readable by anyone with dashboard access. A transcript fragment in a span
// attribute is therefore a restricted disclosure that no retention policy
// covers and no deletion request can reach.
//
// So the rule is not a convention. Attributes are constructed from an approved
// key or not at all, and free text is scrubbed before it is attached. A
// convention would not survive a hurried debugging session at two in the
// morning, which is exactly when someone adds the attribute that leaks.
//
// Implements part of PLT-08 and the enforcement half of SEC-08.
package telemetry

import (
	"fmt"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

// MaxMessageLength bounds any free text attached to telemetry.
//
// Blunt, and the right instrument here. A dashboard cannot display a paragraph
// anyway, and a paragraph arriving in telemetry is usually content that got
// there by accident.
const MaxMessageLength = 512

// Key is an approved attribute key.
//
// The type exists so a key is a decision rather than a string literal typed
// into a handler. Adding one is a reviewed change to this file.
type Key string

// The approved keys. Every one of these identifies something rather than
// describing it, which is the property that keeps content out: an identifier
// resolves to a record for someone authorised to read it, while a description
// carries the content to whoever holds the dashboard.
const (
	KeyRequestID   Key = "prepeet.request_id"
	KeyTenantID    Key = "prepeet.tenant_id"
	KeyUserID      Key = "prepeet.user_id"
	KeySessionID   Key = "prepeet.session_id"
	KeyEventID     Key = "prepeet.event_id"
	KeyEventType   Key = "prepeet.event_type"
	KeyCapability  Key = "prepeet.capability"
	KeyDecision    Key = "prepeet.decision"
	KeyMode        Key = "prepeet.mode"
	KeyEnvironment Key = "prepeet.environment"
	KeyOutcome     Key = "prepeet.outcome"
	KeyErrorCode   Key = "prepeet.error_code"
	KeyAttempt     Key = "prepeet.attempt"
	KeyArtifactVer Key = "prepeet.artifact_version"
	KeyPolicyVer   Key = "prepeet.policy_version"
	KeyDurationMS  Key = "prepeet.duration_ms"
)

var approved = []Key{
	KeyRequestID, KeyTenantID, KeyUserID, KeySessionID,
	KeyEventID, KeyEventType, KeyCapability, KeyDecision,
	KeyMode, KeyEnvironment, KeyOutcome, KeyErrorCode,
	KeyAttempt, KeyArtifactVer, KeyPolicyVer, KeyDurationMS,
}

var approvedSet = func() map[Key]struct{} {
	set := make(map[Key]struct{}, len(approved))
	for _, key := range approved {
		set[key] = struct{}{}
	}
	return set
}()

// ApprovedKeys returns every key that may appear in telemetry.
func ApprovedKeys() []Key {
	out := make([]Key, len(approved))
	copy(out, approved)
	return out
}

// Attr builds an attribute, and reports whether the key was approved.
//
// The second return is deliberately not an error. A caller cannot usefully
// handle "this attribute was refused" at runtime, and failing the request
// because of a telemetry mistake would be worse than the mistake. The refusal
// is caught by the test that walks recorded spans instead, which is where a
// build should fail.
func Attr(key Key, value string) (attribute.KeyValue, bool) {
	if _, ok := approvedSet[key]; !ok {
		return attribute.KeyValue{}, false
	}
	return attribute.String(string(key), Scrub(value)), true
}

// MustAttr builds an attribute from a key known at compile time.
//
// It panics on an unapproved key, which is safe because every call site passes
// a constant from this file. A panic here is a programming error caught on the
// first run rather than a leak discovered in a dashboard.
func MustAttr(key Key, value string) attribute.KeyValue {
	attr, ok := Attr(key, value)
	if !ok {
		panic(fmt.Sprintf("telemetry: %q is not an approved attribute key", key))
	}
	return attr
}

// Patterns that must never survive into telemetry.
//
// Each one exists because that shape reaches an error message in the ordinary
// course of things, not because someone might be careless.
var (
	// A driver error naming the user it could not find, or a validation error
	// echoing what was typed.
	emailPattern = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	// A rejected token logged so somebody can "check which one it was". The
	// prefixes are the ones platform/token issues.
	tokenPattern = regexp.MustCompile(`\b(ses|ref|vrf|rst|mgc|inv)_[A-Za-z0-9_\-]{16,}`)
	// A connection failure carrying its own credentials, which is the default
	// behaviour of most drivers.
	connectionPattern = regexp.MustCompile(`([a-z][a-z0-9+.\-]*)://[^:/\s]+:[^@/\s]+@`)
	// A stored credential quoted back in an error about it being unusable.
	hashPattern = regexp.MustCompile(`\$argon2(id|i|d)\$[^\s]*`)
)

// Scrub removes restricted content from free text.
//
// It runs on every attribute value and every log message. The order matters:
// connection strings are handled before addresses, because a connection string
// contains something that looks like neither and both.
//
// It is deliberately not clever. A scrubber that tried to understand the text
// would fail differently on each input; one that removes four known shapes and
// truncates the rest fails predictably, which is the property worth having in
// something that runs on every log line.
func Scrub(text string) string {
	if text == "" {
		return text
	}

	scrubbed := connectionPattern.ReplaceAllString(text, "$1://[redacted]@")
	scrubbed = hashPattern.ReplaceAllString(scrubbed, "[redacted hash]")
	scrubbed = tokenPattern.ReplaceAllString(scrubbed, "[redacted token]")
	scrubbed = emailPattern.ReplaceAllString(scrubbed, "[redacted address]")

	if len(scrubbed) > MaxMessageLength {
		// The marker matters as much as the cut. A silently shortened message
		// reads as complete, and somebody will draw a conclusion from the half
		// they can see.
		scrubbed = strings.TrimSpace(scrubbed[:MaxMessageLength]) + " [truncated]"
	}
	return scrubbed
}
