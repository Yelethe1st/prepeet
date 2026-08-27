//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

var (
	pool     *pgxpool.Pool
	registry *content.Store
)

const (
	e2eAuthor   = "00000000-0000-7000-8000-0000000000c1"
	e2eReviewer = "00000000-0000-7000-8000-0000000000c2"
)

// planBody is the plan the registry publishes and Python composes against.
var planBody = json.RawMessage(`{"stages":["intro","core","close"]}`)

// policyBody mirrors the shipped policy artifact: what each stage may
// spend, pinned with the rest.
var policyBody = json.RawMessage(`{"stages":[` +
	`{"id":"evidence","required":true,"budget_units":100},` +
	`{"id":"aggregation","required":true,"budget_units":20},` +
	`{"id":"articulation","required":false,"budget_units":60},` +
	`{"id":"coaching","required":false,"budget_units":40}]}`)

// rubricBody mirrors the shipped practice-default artifact: composition
// pins it, aggregation later judges by it.
var rubricBody = json.RawMessage(`{"sufficiency":{"min_supporting":2},"bands":[` +
	`{"id":"developing","min_ratio":0},{"id":"solid","min_ratio":0.55},{"id":"strong","min_ratio":0.8}],` +
	`"confidence":{"high":{"min_supporting":4,"max_contradictory":0},` +
	`"medium":{"min_supporting":2,"max_contradictory":1}}}`)

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("prepeet"),
		tcpostgres.WithUsername("prepeet_migrator"),
		tcpostgres.WithPassword("migrator-password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting PostgreSQL: %v\n", err)
		os.Exit(1)
	}

	adminURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "connection string: %v\n", err)
		os.Exit(1)
	}
	if err := database.Migrate(ctx, adminURL, database.MigrateOptions{AppPassword: "app-password"}); err != nil {
		fmt.Fprintf(os.Stderr, "migrating: %v\n", err)
		os.Exit(1)
	}

	cfg, err := pgx.ParseConfig(adminURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing: %v\n", err)
		os.Exit(1)
	}
	pool, err = pgxpool.New(ctx, fmt.Sprintf(
		"postgres://prepeet_app:app-password@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "app pool: %v\n", err)
		os.Exit(1)
	}
	registry = content.NewStore(pool)

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed connect: %v\n", err)
		os.Exit(1)
	}
	for _, user := range []struct{ id, email string }{
		{e2eAuthor, "author.e2e@example.com"}, {e2eReviewer, "reviewer.e2e@example.com"},
	} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO identity.users (id, email) VALUES ($1, $2)`, user.id, user.email); err != nil {
			fmt.Fprintf(os.Stderr, "seeding: %v\n", err)
			os.Exit(1)
		}
	}
	_ = conn.Close(ctx)

	// The plan, through the registry's own lifecycle: what production does.
	draft, err := registry.CreateDraft(ctx, content.Draft{
		Type: "plan", Reference: "bp_backend_v1", Version: "1.0.0",
		SchemaVersion: "1.0", Body: planBody, CreatedBy: e2eAuthor,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "drafting the plan: %v\n", err)
		os.Exit(1)
	}
	step := draft
	for _, to := range []content.Status{content.StatusValidating, content.StatusApproved} {
		if step, err = registry.Transition(ctx, step, to); err != nil {
			fmt.Fprintf(os.Stderr, "walking the plan to %s: %v\n", to, err)
			os.Exit(1)
		}
	}
	if _, err := registry.Publish(ctx, step, e2eReviewer); err != nil {
		fmt.Fprintf(os.Stderr, "publishing the plan: %v\n", err)
		os.Exit(1)
	}
	// The rubric too: composition pins it alongside the plan (EVL-02), so
	// the suite publishes one through the same lifecycle production uses.
	rubricDraft, err := registry.CreateDraft(ctx, content.Draft{
		Type: "rubric", Reference: "rubric/practice-default", Version: "1.1.0",
		SchemaVersion: "1.0", Body: rubricBody, CreatedBy: e2eAuthor,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "drafting the rubric: %v\n", err)
		os.Exit(1)
	}
	step = rubricDraft
	for _, to := range []content.Status{content.StatusValidating, content.StatusApproved} {
		if step, err = registry.Transition(ctx, step, to); err != nil {
			fmt.Fprintf(os.Stderr, "walking the rubric to %s: %v\n", to, err)
			os.Exit(1)
		}
	}
	if _, err := registry.Publish(ctx, step, e2eReviewer); err != nil {
		fmt.Fprintf(os.Stderr, "publishing the rubric: %v\n", err)
		os.Exit(1)
	}

	// And the model policy, pinned beside them (EVL-07): what each stage
	// of this session was allowed to spend.
	policyDraft, err := registry.CreateDraft(ctx, content.Draft{
		Type: "model_policy", Reference: "policy/practice-default", Version: "1.0.0",
		SchemaVersion: "1.0", Body: policyBody, CreatedBy: e2eAuthor,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "drafting the policy: %v\n", err)
		os.Exit(1)
	}
	step = policyDraft
	for _, to := range []content.Status{content.StatusValidating, content.StatusApproved} {
		if step, err = registry.Transition(ctx, step, to); err != nil {
			fmt.Fprintf(os.Stderr, "walking the policy to %s: %v\n", to, err)
			os.Exit(1)
		}
	}
	if _, err := registry.Publish(ctx, step, e2eReviewer); err != nil {
		fmt.Fprintf(os.Stderr, "publishing the policy: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating: %v\n", err)
	}
	os.Exit(code)
}

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

	composer, conn, err := newComposer(address, registry)
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

	if result.BundleRef != "sessions/ses_e2e_1/bundle" {
		t.Errorf("bundle ref = %q", result.BundleRef)
	}

	// CAT-02's middle box, across two languages and a database: the bundle
	// records the pin the registry resolved, digest and all.
	published, err := registry.Resolve(ctx, "bp_backend_v1", "")
	if err != nil {
		t.Fatalf("re-resolving the plan: %v", err)
	}
	var bundle struct {
		SchemaVersion string `json:"schema_version"`
		BlueprintID   string `json:"blueprint_id"`
		PinnedInputs  []struct {
			ArtifactType  string `json:"artifact_type"`
			Reference     string `json:"reference"`
			Version       string `json:"version"`
			SchemaVersion string `json:"schema_version"`
			Digest        string `json:"digest"`
		} `json:"pinned_inputs"`
	}
	if err := json.Unmarshal(result.BundleBody, &bundle); err != nil {
		t.Fatalf("the bundle body is not the document the contract promises: %v", err)
	}
	if len(bundle.PinnedInputs) != 3 {
		t.Fatalf("the bundle records %d pins, want the plan, the rubric and the policy", len(bundle.PinnedInputs))
	}
	// Found by type, not by position: the bundle sorts its pins so the
	// same inputs in any order are the same bundle, and an assertion on
	// index would break the moment another artifact is pinned.
	byType := map[string]struct {
		ArtifactType  string `json:"artifact_type"`
		Reference     string `json:"reference"`
		Version       string `json:"version"`
		SchemaVersion string `json:"schema_version"`
		Digest        string `json:"digest"`
	}{}
	for _, pinned := range bundle.PinnedInputs {
		byType[pinned.ArtifactType] = pinned
	}
	for _, expected := range []string{"plan", "rubric", "model_policy"} {
		if _, present := byType[expected]; !present {
			t.Fatalf("the bundle pins no %s: %+v", expected, bundle.PinnedInputs)
		}
	}
	pin := byType["plan"]
	if pin.Reference != "bp_backend_v1" || pin.Version != "1.0.0" {
		t.Errorf("the bundle's plan pin is %+v", pin)
	}
	if pin.Digest != published.Digest {
		t.Errorf("the bundle pins digest %q, the registry published %q", pin.Digest, published.Digest)
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

	composer, conn, err := newComposer(address, registry)
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

func TestAnUnknownBlueprintIsRefusedBeforeTheWire(t *testing.T) {
	// No Python here on purpose: the registry is Go's, and a blueprint it
	// cannot resolve is a request failure the adapter answers locally with
	// the same taxonomy code Python would use.
	composer := &grpcComposer{registry: registry}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := composer.Compose(ctx, interview.ComposeRequest{
		SessionID: "ses_e2e_3", Mode: "practice", CandidateID: "usr_e2e",
		BlueprintID: "bp_that_never_was",
	})

	var failure *interview.ComposeFailure
	if !errors.As(err, &failure) {
		t.Fatalf("the refusal arrived untyped: %v", err)
	}
	if failure.Code != "FAILURE_CODE_ARTIFACT_NOT_FOUND" {
		t.Errorf("code = %q", failure.Code)
	}
	if failure.Retryable {
		t.Error("ARTIFACT_NOT_FOUND arrived retryable; the descriptor says otherwise")
	}
}
