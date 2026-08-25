//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
)

// CTR-02 across the wire for real: Go's adapter against the Python server,
// both built from the same contract. This is the walking skeleton's last
// segment - the two planes exchanging a typed result and a typed refusal -
// and the cross-language fixture the contract's governance asked for.
//
// Python runs as a subprocess through uv, exactly as `make dev` runs it. When
// uv is absent the test skips loudly rather than failing: the Go CI job has
// no Python toolchain, and the Python job proves the same server against the
// same contract from its side.

func startIntelligence(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv is not installed; the cross-language test runs where Python's toolchain exists")
	}

	// A port chosen by binding, then released for Python to take: the small
	// race is acceptable in a test that owns the machine.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatalf("locating the repository: %v", err)
	}

	// The server's stderr goes to a file the test owns, not to the test
	// binary's own stderr: a leaked child holding that pipe makes `go test`
	// report the whole package failed on I/O thirty seconds after every test
	// passed, which is exactly how this arrangement was discovered.
	stderr, err := os.CreateTemp(t.TempDir(), "intelligence-*.log")
	if err != nil {
		t.Fatalf("stderr file: %v", err)
	}

	command := exec.Command("uv", "run", "python", "-m",
		"prepeet_ai.transport.server", "--port", fmt.Sprint(port))
	command.Dir = filepath.Join(root, "services/intelligence")
	command.Stderr = stderr
	command.Stdout = stderr
	// Its own process group, so cleanup can kill the whole tree. uv runs
	// python as a child, and killing only uv leaves python alive - serving,
	// holding pipes, and outliving the test run.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatalf("starting the intelligence plane: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = command.Wait()
		if t.Failed() {
			log, _ := os.ReadFile(stderr.Name())
			t.Logf("intelligence plane output:\n%s", log)
		}
	})

	address := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(30 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			_ = conn.Close()
			return address
		}
		if time.Now().After(deadline) {
			t.Fatalf("the intelligence plane never started listening on %s", address)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestGoComposesThroughPython(t *testing.T) {
	address := startIntelligence(t)

	composer, conn, err := newComposer(address)
	if err != nil {
		t.Fatalf("newComposer: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := composer.Compose(ctx, interview.ComposeRequest{
		SessionID:   "ses_e2e_1",
		Mode:        "practice",
		CandidateID: "usr_e2e",
		BlueprintID: "bp_backend_v1",
	})
	if err != nil {
		t.Fatalf("Compose across the wire: %v", err)
	}

	if result.BundleRef != "bundles/ses_e2e_1" {
		t.Errorf("bundle ref = %q", result.BundleRef)
	}
	if len(result.BundleDigest) < 10 || result.BundleDigest[:7] != "sha256:" {
		t.Errorf("bundle digest = %q", result.BundleDigest)
	}
	if result.BundleRevision != 1 {
		t.Errorf("revision = %d", result.BundleRevision)
	}

	// Determinism across the wire: the workflow's convergence property, with
	// the real server on the other end.
	again, err := composer.Compose(ctx, interview.ComposeRequest{
		SessionID: "ses_e2e_1", Mode: "practice",
		CandidateID: "usr_e2e", BlueprintID: "bp_backend_v1",
	})
	if err != nil {
		t.Fatalf("second compose: %v", err)
	}
	if again.BundleDigest != result.BundleDigest {
		t.Fatal("the same request produced a different digest across calls")
	}
}

func TestARefusalCrossesTheWireTyped(t *testing.T) {
	// The taxonomy's round trip: Python raises INVALID_INPUT, packs it as the
	// contract's Failure detail, and Go's adapter reads it back with the retry
	// decision from the descriptor - not from anything either side wrote by
	// hand. This failing would mean the two planes disagree about the one
	// vocabulary they share.
	address := startIntelligence(t)

	composer, conn, err := newComposer(address)
	if err != nil {
		t.Fatalf("newComposer: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err = composer.Compose(ctx, interview.ComposeRequest{
		SessionID: "ses_e2e_2", Mode: "practice", CandidateID: "usr_e2e",
		BlueprintID: "", // the refusal under test
	})

	var failure *interview.ComposeFailure
	if !errors.As(err, &failure) {
		t.Fatalf("the refusal arrived untyped: %v", err)
	}
	if failure.Code != "FAILURE_CODE_INVALID_INPUT" {
		t.Errorf("code = %q", failure.Code)
	}
	if failure.Retryable {
		t.Error("INVALID_INPUT arrived retryable; the descriptor says otherwise")
	}
}
