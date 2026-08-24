//go:build integration

// Temporal tests against a real server.
//
// The converter and the client's refusals are asserted without a server in the
// other files. What is here is what only a running Temporal can show: that the
// namespace decision holds, that the payload rule survives an actual round trip
// through history rather than only through the converter, and that a workflow
// argument carrying restricted content is refused at the point of starting the
// workflow rather than after it is stored.
package temporal_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	sdkclient "go.temporal.io/sdk/client"

	"github.com/Yelethe1st/prepeet/services/platform/platform/temporal"
)

// address is the local stack from infrastructure/local/docker-compose.yml.
// Overridable so this can run against something else in CI.
func address() string {
	if configured := os.Getenv("PREPEET_TEMPORAL_ADDRESS"); configured != "" {
		return configured
	}
	return "localhost:7233"
}

const namespace = "prepeet-local"

func connect(t *testing.T) sdkclient.Client {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := temporal.Dial(ctx, temporal.Config{Address: address(), Namespace: namespace})
	if err != nil {
		t.Fatalf("Dial(%s): %v\n  Is the local stack running? make local-up", address(), err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestTheClientReachesTheConfiguredNamespace(t *testing.T) {
	client := connect(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.CheckHealth(ctx, &sdkclient.CheckHealthRequest{}); err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
}

// ADR-0007 separates environments by namespace. A namespace that does not exist
// must fail rather than falling back to default, or the separation is a naming
// convention rather than a boundary.
func TestAnUnknownNamespaceIsRefusedRatherThanFallingBack(t *testing.T) {
	client := connect(t)
	_ = client

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	other, err := temporal.Dial(ctx, temporal.Config{
		Address:   address(),
		Namespace: "prepeet-production",
	})
	if err != nil {
		return // refused at dial, which is the strongest form of the answer
	}
	t.Cleanup(other.Close)

	// Dial can succeed lazily, so the assertion is that using it fails.
	_, err = other.ExecuteWorkflow(ctx,
		sdkclient.StartWorkflowOptions{ID: "should-not-start", TaskQueue: "none"},
		"NoSuchWorkflow")
	if err == nil {
		t.Error("a workflow started against a namespace that does not exist, " +
			"so environments are not actually separated")
	}
}

// The payload rule, asserted where it matters: at the point of starting a
// workflow, against a real server. The converter tests prove the converter
// refuses; this proves the converter is on the path the client actually uses.
func TestARestrictedWorkflowArgumentIsRefusedBeforeItIsStored(t *testing.T) {
	client := connect(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	transcript := strings.Repeat("The candidate described their approach at length. ", 200)

	_, err := client.ExecuteWorkflow(ctx,
		sdkclient.StartWorkflowOptions{
			ID:        "restricted-argument-" + time.Now().Format("20060102150405.000"),
			TaskQueue: "prepeet-test",
		},
		"EvaluateSession", transcript)

	if err == nil {
		t.Fatal("a workflow carrying a transcript was accepted, so the payload rule " +
			"is not on the path the client uses")
	}
	if !strings.Contains(err.Error(), "ADR-0007") {
		t.Errorf("the failure is not the payload rule, so this test may be passing for "+
			"another reason: %v", err)
	}
}

// And the converse, so the test above cannot pass because everything fails.
func TestAnIdentifierWorkflowArgumentIsAccepted(t *testing.T) {
	client := connect(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	run, err := client.ExecuteWorkflow(ctx,
		sdkclient.StartWorkflowOptions{
			ID:        "identifier-argument-" + time.Now().Format("20060102150405.000"),
			TaskQueue: "prepeet-test",
		},
		"EvaluateSession", map[string]any{"session_id": "ses_7Kq2XA", "rubric_version": 4})

	if err != nil {
		t.Fatalf("a workflow carrying identifiers was refused: %v", err)
	}

	// No worker polls prepeet-test, so this stays queued. Terminating it keeps
	// the local namespace tidy for the next run.
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer terminateCancel()
		_ = client.TerminateWorkflow(terminateCtx, run.GetID(), run.GetRunID(), "test cleanup")
	})
}

// ErrNotConfigured must stay distinguishable with a real server present, since
// that is the case where confusing it with a connection failure is easiest.
func TestNotConfiguredStaysDistinctFromUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, notConfigured := temporal.Dial(ctx, temporal.Config{Namespace: namespace})
	if !errors.Is(notConfigured, temporal.ErrNotConfigured) {
		t.Errorf("no address gave %v, want ErrNotConfigured", notConfigured)
	}

	_, unreachable := temporal.Dial(ctx, temporal.Config{
		Address: "127.0.0.1:1", Namespace: namespace,
	})
	if errors.Is(unreachable, temporal.ErrNotConfigured) {
		t.Error("an unreachable server was reported as not configured, and those need opposite responses")
	}
}
