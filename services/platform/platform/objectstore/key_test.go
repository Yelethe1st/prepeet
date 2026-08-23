package objectstore_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
)

func TestKeyPlacesTenantAndSessionInThePath(t *testing.T) {
	t.Parallel()

	key, err := objectstore.NewKey(objectstore.KeyParts{
		TenantID:  "tn_northwind",
		SessionID: "ses_7Kq2XA",
		Purpose:   objectstore.PurposeMedia,
		Name:      "candidate-audio.opus",
	})
	if err != nil {
		t.Fatalf("NewKey returned error: %v", err)
	}

	for _, want := range []string{"tn_northwind", "ses_7Kq2XA", "media"} {
		if !strings.Contains(key.String(), want) {
			t.Errorf("key = %q, want it to contain %q", key, want)
		}
	}
}

// The path exists for lifecycle rules and for reconciliation, not for
// authorization. Parsing a tenant out of a key and trusting it would make the
// key an authorization input, which docs/contracts/public-api.md forbids.
func TestKeyIsNotAnAuthorizationInput(t *testing.T) {
	t.Parallel()

	key, err := objectstore.NewKey(objectstore.KeyParts{
		TenantID:  "tn_northwind",
		SessionID: "ses_7Kq2XA",
		Purpose:   objectstore.PurposeMedia,
		Name:      "audio.opus",
	})
	if err != nil {
		t.Fatalf("NewKey returned error: %v", err)
	}

	// The type exposes no accessor that hands back a tenant, because a caller
	// that could read one would eventually authorize against it.
	if _, hasTenant := any(key).(interface{ TenantID() string }); hasTenant {
		t.Error("Key exposes TenantID(), which invites authorizing against the key rather than the request")
	}
}

// A caller-supplied name reaching the key unescaped would let one tenant write
// into another tenant's prefix.
func TestKeyRejectsTraversalAndSeparators(t *testing.T) {
	t.Parallel()

	for name, candidate := range map[string]string{
		"parent traversal": "../../tn_other/session/x",
		"absolute path":    "/etc/passwd",
		"embedded slash":   "nested/audio.opus",
		"empty":            "",
		"only dots":        "..",
		"null byte":        "audio\x00.opus",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := objectstore.NewKey(objectstore.KeyParts{
				TenantID:  "tn_northwind",
				SessionID: "ses_7Kq2XA",
				Purpose:   objectstore.PurposeMedia,
				Name:      candidate,
			})
			if err == nil {
				t.Errorf("NewKey accepted name %q, want it rejected", candidate)
			}
		})
	}
}

func TestKeyRejectsAnEmptyTenantOrSession(t *testing.T) {
	t.Parallel()

	cases := map[string]objectstore.KeyParts{
		"no tenant":  {SessionID: "ses_7Kq2XA", Purpose: objectstore.PurposeMedia, Name: "a.opus"},
		"no session": {TenantID: "tn_northwind", Purpose: objectstore.PurposeMedia, Name: "a.opus"},
		"no purpose": {TenantID: "tn_northwind", SessionID: "ses_7Kq2XA", Name: "a.opus"},
	}

	for name, parts := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := objectstore.NewKey(parts); err == nil {
				t.Error("NewKey returned no error, want one")
			}
		})
	}
}

// Purposes are separated in the path so a retention rule can expire recordings
// on one schedule and exports on another, which DEC-15 requires.
func TestKeySeparatesPurposes(t *testing.T) {
	t.Parallel()

	seen := make(map[string]objectstore.Purpose)
	for _, purpose := range []objectstore.Purpose{
		objectstore.PurposeMedia, objectstore.PurposeDocument, objectstore.PurposeExport,
	} {
		key, err := objectstore.NewKey(objectstore.KeyParts{
			TenantID:  "tn_northwind",
			SessionID: "ses_7Kq2XA",
			Purpose:   purpose,
			Name:      "a.bin",
		})
		if err != nil {
			t.Fatalf("NewKey(%q) returned error: %v", purpose, err)
		}
		if previous, clash := seen[key.String()]; clash {
			t.Errorf("purposes %q and %q produce the same key %q", previous, purpose, key)
		}
		seen[key.String()] = purpose
	}
}

func TestKeyRejectsAnUnknownPurpose(t *testing.T) {
	t.Parallel()

	_, err := objectstore.NewKey(objectstore.KeyParts{
		TenantID:  "tn_northwind",
		SessionID: "ses_7Kq2XA",
		Purpose:   objectstore.Purpose("exfiltration"),
		Name:      "a.bin",
	})
	if err == nil {
		t.Error("NewKey accepted an unknown purpose, want it rejected")
	}
}

// Media authorization is short lived by rule, not by convention. A long lived
// URL is a bearer token for a candidate's recording that outlives the session.
func TestPresignTTLIsClampedToTheMaximum(t *testing.T) {
	t.Parallel()

	got := objectstore.ClampTTL(24 * time.Hour)

	if got > objectstore.MaxPresignTTL {
		t.Errorf("ClampTTL(24h) = %s, want no more than %s", got, objectstore.MaxPresignTTL)
	}
}

func TestPresignTTLIsClampedToTheMinimum(t *testing.T) {
	t.Parallel()

	if got := objectstore.ClampTTL(0); got < objectstore.MinPresignTTL {
		t.Errorf("ClampTTL(0) = %s, want at least %s", got, objectstore.MinPresignTTL)
	}
	if got := objectstore.ClampTTL(-time.Hour); got < objectstore.MinPresignTTL {
		t.Errorf("ClampTTL(-1h) = %s, want at least %s", got, objectstore.MinPresignTTL)
	}
}

func TestPresignTTLPassesThroughAReasonableValue(t *testing.T) {
	t.Parallel()

	want := 5 * time.Minute

	if got := objectstore.ClampTTL(want); got != want {
		t.Errorf("ClampTTL(%s) = %s, want it unchanged", want, got)
	}
}

func TestMaxPresignTTLIsShortEnoughToBeDefensible(t *testing.T) {
	t.Parallel()

	if objectstore.MaxPresignTTL > time.Hour {
		t.Errorf("MaxPresignTTL = %s, want an hour or less: a presigned URL is a bearer token for a recording",
			objectstore.MaxPresignTTL)
	}
}
