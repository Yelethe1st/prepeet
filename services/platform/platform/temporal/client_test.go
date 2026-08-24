package temporal_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/temporal"
)

// "This process does not use Temporal" and "Temporal is unreachable" need
// opposite responses: the first is how cmd/api starts, the second is an
// incident. Telling them apart by reading an error string is how they end up
// being confused.
func TestDialWithNoAddressIsADistinctError(t *testing.T) {
	t.Parallel()

	_, err := temporal.Dial(context.Background(), temporal.Config{Namespace: "prepeet-local"})
	if !errors.Is(err, temporal.ErrNotConfigured) {
		t.Errorf("Dial with no address returned %v, want ErrNotConfigured", err)
	}
}

// A namespace is what separates environments, so dialling without one is a
// mistake worth catching before the connection rather than after.
func TestDialWithoutANamespaceIsRefused(t *testing.T) {
	t.Parallel()

	_, err := temporal.Dial(context.Background(), temporal.Config{Address: "localhost:7233"})
	if err == nil {
		t.Fatal("Dial without a namespace was accepted")
	}
	if errors.Is(err, temporal.ErrNotConfigured) {
		t.Error("a missing namespace was reported as a missing address")
	}
}

// A certificate that will not load must name the paths and nothing else. The
// contents of a key file have no business in a log line, including when the
// reason for the log line is that the key file is malformed.
func TestAnUnreadableCertificateNamesThePathsAndNotTheMaterial(t *testing.T) {
	t.Parallel()

	_, err := temporal.Dial(context.Background(), temporal.Config{
		Address:   "localhost:7233",
		Namespace: "prepeet-local",
		CertFile:  "/no/such/client.pem",
		KeyFile:   "/no/such/client.key",
	})
	if err == nil {
		t.Fatal("a missing certificate pair was accepted")
	}
	if !strings.Contains(err.Error(), "/no/such/client.pem") {
		t.Errorf("the error does not say which file could not be read: %v", err)
	}
}

// Health output is public, so it must not describe the failure. platform/health
// enforces this at its own boundary; this keeps the habit where the check is
// written.
func TestTheHealthCheckDoesNotDescribeTheFailure(t *testing.T) {
	t.Parallel()

	err := temporal.Check(nil)(context.Background())
	if err == nil {
		t.Fatal("a nil client reported healthy")
	}
	for _, leak := range []string{"7233", "localhost", "connection refused"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("the health error carries deployment detail: %v", err)
		}
	}
}

// A failed dial reports the address it could not reach, which is useful and is
// also how a credential gets into a log line if the address ever carries one.
//
// This replaced a test that dialled a bad address and asserted the SDK logged
// nothing sensitive. That test passed because the SDK logged nothing at all,
// which is the shape of assertion this codebase keeps finding and removing. It
// also hid a real defect, which is what this asserts instead: the error itself
// carried the credential, and every caller was relying on remembering to scrub
// it.
func TestAFailedDialDoesNotCarryACredentialInItsError(t *testing.T) {
	t.Parallel()

	_, err := temporal.Dial(context.Background(), temporal.Config{
		Address:   "postgres://prepeet:hunter2@db.internal:5432/prepeet",
		Namespace: "prepeet-local",
	})
	if err == nil {
		t.Fatal("that address was accepted")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("the dial error carries a credential: %v", err)
	}
	if !strings.Contains(err.Error(), "dialling") {
		t.Errorf("the error no longer says what failed: %v", err)
	}
}

// The adapter must scrub on its own, against a logger that does not.
//
// An earlier version of this test passed the adapter a telemetry.NewLogger,
// which scrubs already, so removing the adapter's scrubbing entirely left the
// test green. It was verifying the wrong component. A plain handler is what
// makes the assertion about the adapter.
func TestTheLogAdapterScrubsBothMessageAndValues(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	plain := slog.New(slog.NewJSONHandler(&out, nil))

	temporal.LogForTest(plain, "dial failed for daniel.okonkwo@example.com",
		"dsn", "postgres://prepeet:hunter2@db.internal:5432/prepeet")

	written := out.String()
	for _, secret := range []string{"hunter2", "daniel.okonkwo"} {
		if strings.Contains(written, secret) {
			t.Errorf("the adapter let %q through: %s", secret, written)
		}
	}
	if !strings.Contains(written, "dial failed") {
		t.Errorf("the adapter removed the part an operator needs: %s", written)
	}
}

// Sanity: the logger it wraps is a real one, so a nil logger cannot make the
// worker panic on its first SDK log line.
func TestANilLoggerDoesNotPanic(t *testing.T) {
	t.Parallel()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Errorf("a nil logger panicked: %v", recovered)
		}
	}()

	temporal.LogForTest(nil, "message", "key", "value")
	_ = slog.Default()
}
