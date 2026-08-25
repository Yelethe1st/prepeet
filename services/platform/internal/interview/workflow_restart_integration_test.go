//go:build integration

package interview_test

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	sdkclient "go.temporal.io/sdk/client"
	sdkworker "go.temporal.io/sdk/worker"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/temporal"
)

// PLT-06's open box: a worker killed mid-workflow replays without duplicating
// state, usage or notification.
//
// The kill is real. A composer blocks inside the Compose activity while the
// worker that runs it is stopped; a second worker picks the retry up, this
// time composition answers, and then everything is counted: composition ran
// more than once - that is the at-least-once truth being exercised, not a
// flaw - while the session advanced exactly once, the catalogue event was
// published exactly once, and the audit trail holds exactly one ready row.
//
// Needs the local stack's Temporal (make local-up), like the client's own
// integration tests, and the suite's PostgreSQL container for the store.

// gateComposer blocks until released, and counts.
type gateComposer struct {
	calls   atomic.Int32
	release chan struct{}
	result  interview.ComposeResult
}

func (g *gateComposer) Compose(ctx context.Context, _ interview.ComposeRequest) (interview.ComposeResult, error) {
	g.calls.Add(1)
	select {
	case <-g.release:
		return g.result, nil
	case <-ctx.Done():
		// The worker died or the attempt timed out: exactly the interruption
		// under test.
		return interview.ComposeResult{}, ctx.Err()
	}
}

func temporalAddress() string {
	if configured := os.Getenv("PREPEET_TEMPORAL_ADDRESS"); configured != "" {
		return configured
	}
	return "localhost:7233"
}

func TestWorkerRestartMidWorkflowDuplicatesNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := temporal.Dial(ctx, temporal.Config{
		Address: temporalAddress(), Namespace: "prepeet-local",
	})
	if err != nil {
		t.Fatalf("Dial(%s): %v\n  Is the local stack running? make local-up", temporalAddress(), err)
	}
	defer client.Close()

	// The session, taken to composing exactly as the API's compose command
	// will: the workflow's job starts there.
	store := interview.NewStore(pool)
	session := interview.Session{
		ID: id.New().String(), Mode: "practice",
		CandidateID: candidateID, BlueprintID: "bp_backend_v1",
	}
	if err := store.Create(ctx, session, candidate); err != nil {
		t.Fatalf("create: %v", err)
	}
	created, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	composing, err := store.Transition(ctx, created, interview.StateComposing, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("to composing: %v", err)
	}

	composer := &gateComposer{
		release: make(chan struct{}),
		result: interview.ComposeResult{
			BundleRef: "bundles/" + session.ID, BundleDigest: "sha256:restart", BundleRevision: 1,
		},
	}
	activities := interview.NewActivities(store, composer)

	// A queue unique to this run, so a worker left over from an earlier test
	// cannot steal the task and turn this into a test of nothing.
	queue := "restart-test-" + session.ID
	newWorker := func() sdkworker.Worker {
		w := sdkworker.New(client, queue, sdkworker.Options{})
		w.RegisterWorkflow(interview.CompositionWorkflow)
		w.RegisterActivity(activities.Compose)
		w.RegisterActivity(activities.MarkReady)
		w.RegisterActivity(activities.MarkFailed)
		return w
	}

	first := newWorker()
	if err := first.Start(); err != nil {
		t.Fatalf("first worker: %v", err)
	}

	input := interview.CompositionInput{
		SessionID: session.ID, Mode: "practice",
		CandidateID: candidateID, BlueprintID: "bp_backend_v1", ActorID: candidateID,
	}
	run, err := client.ExecuteWorkflow(ctx, temporalWorkflowOptions(session.ID, queue),
		interview.CompositionWorkflow, input)
	if err != nil {
		t.Fatalf("starting the workflow: %v", err)
	}

	// Wait until composition is genuinely in flight on the first worker, then
	// kill it with the activity still blocked. This is the "mid-workflow".
	deadline := time.Now().Add(20 * time.Second)
	for composer.calls.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("composition never started on the first worker")
		}
		time.Sleep(50 * time.Millisecond)
	}
	first.Stop()

	// The replacement worker, and this time composition may answer.
	close(composer.release)
	second := newWorker()
	if err := second.Start(); err != nil {
		t.Fatalf("second worker: %v", err)
	}
	defer second.Stop()

	if err := run.Get(ctx, nil); err != nil {
		t.Fatalf("the workflow did not survive the restart: %v", err)
	}

	// The counting. Composition ran at least twice - the interrupted attempt
	// and the successful one - which is what makes the exactly-onces below
	// worth asserting rather than trivially true.
	if composer.calls.Load() < 2 {
		t.Fatalf("Compose ran %d time(s); the restart was never exercised", composer.calls.Load())
	}

	final, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("final read: %v", err)
	}
	if final.State != interview.StateReady {
		t.Fatalf("final state = %s, want ready", final.State)
	}
	if final.Version != composing.Version+1 {
		t.Fatalf("version = %d, want %d: the transition happened more than once",
			final.Version, composing.Version+1)
	}
	if final.BundleDigest != "sha256:restart" {
		t.Fatalf("bundle digest = %q", final.BundleDigest)
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var events int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM integration.outbox
		WHERE event_type = 'interview.session_ready.v1'
		  AND payload->>'session_id' = $1`, session.ID).Scan(&events); err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if events != 1 {
		t.Fatalf("%d session_ready events were published; a consumer would notify twice", events)
	}

	var audits int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM audit.events
		WHERE action = 'interview.session_ready' AND subject_id = $1`, session.ID).Scan(&audits); err != nil {
		t.Fatalf("counting audit: %v", err)
	}
	if audits != 1 {
		t.Fatalf("%d ready audit rows; the trail claims it happened %d times", audits, audits)
	}
}

// temporalWorkflowOptions is the production identity rule, in the test: the
// session id IS the workflow id, so a duplicate compose command joins the
// running workflow instead of starting a second composition.
func temporalWorkflowOptions(sessionID, queue string) sdkclient.StartWorkflowOptions {
	return sdkclient.StartWorkflowOptions{
		ID:        "compose-" + sessionID,
		TaskQueue: queue,
	}
}
