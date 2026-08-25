//go:build integration

package content_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

// CAT-01 against real PostgreSQL: drafted, reviewed, published, pinned and
// rolled back, with the ADR's promises attacked rather than assumed.

var (
	pool     *pgxpool.Pool
	adminURL string
)

const (
	authorID    = "00000000-0000-7000-8000-0000000000a7"
	reviewerID  = "00000000-0000-7000-8000-0000000000a8"
	tenantAlpha = "00000000-0000-7000-8000-0000000000aa"
	tenantBeta  = "00000000-0000-7000-8000-0000000000ab"
)

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

	adminURL, err = container.ConnectionString(ctx, "sslmode=disable")
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

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed connect: %v\n", err)
		os.Exit(1)
	}
	for _, user := range []struct{ id, email string }{
		{authorID, "author.content@example.com"},
		{reviewerID, "reviewer.content@example.com"},
	} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO identity.users (id, email) VALUES ($1, $2)`, user.id, user.email); err != nil {
			fmt.Fprintf(os.Stderr, "seeding users: %v\n", err)
			os.Exit(1)
		}
	}
	for _, tenant := range []struct{ id, slug string }{
		{tenantAlpha, "alpha-content"}, {tenantBeta, "beta-content"},
	} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO tenancy.tenants (id, name, slug, region) VALUES ($1, $2, $3, 'eu-west-2')`,
			tenant.id, tenant.slug, tenant.slug); err != nil {
			fmt.Fprintf(os.Stderr, "seeding: %v\n", err)
			os.Exit(1)
		}
	}
	_ = conn.Close(ctx)

	code := m.Run()
	pool.Close()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating: %v\n", err)
	}
	os.Exit(code)
}

// draftFor writes a platform draft with a body distinguishing it.
func draftFor(t *testing.T, reference, version, marker string) content.Artifact {
	t.Helper()
	store := content.NewStore(pool)
	artifact, err := store.CreateDraft(context.Background(), content.Draft{
		Type:          "persona",
		Reference:     reference,
		Version:       version,
		SchemaVersion: "1.0",
		Body:          json.RawMessage(fmt.Sprintf(`{"tone":"structured","marker":%q}`, marker)),
		CreatedBy:     authorID,
	})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	return artifact
}

// approve walks a draft to approved, as validation and review would.
func approve(t *testing.T, artifact content.Artifact) content.Artifact {
	t.Helper()
	store := content.NewStore(pool)
	validating, err := store.Transition(context.Background(), artifact, content.StatusValidating)
	if err != nil {
		t.Fatalf("to validating: %v", err)
	}
	approved, err := store.Transition(context.Background(), validating, content.StatusApproved)
	if err != nil {
		t.Fatalf("to approved: %v", err)
	}
	return approved
}

// reference gives each test its own name in the shared catalogue.
func reference(t *testing.T) string {
	t.Helper()
	return "persona/" + t.Name()
}

func TestTheWholeLifecycleDraftedThroughRolledBack(t *testing.T) {
	// CAT-01's first criterion in one journey: drafted, reviewed, published,
	// pinned, rolled back.
	ctx := context.Background()
	store := content.NewStore(pool)
	ref := reference(t)

	one := approve(t, draftFor(t, ref, "1.0.0", "first"))
	published, err := store.Publish(ctx, one, reviewerID)
	if err != nil {
		t.Fatalf("publish v1: %v", err)
	}
	if published.Status != content.StatusPublished || published.PublishedBy != reviewerID {
		t.Fatalf("v1 after publish: %s by %q", published.Status, published.PublishedBy)
	}

	current, err := store.Resolve(ctx, ref, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if current.Version != "1.0.0" {
		t.Fatalf("current = %s, want 1.0.0", current.Version)
	}

	two := approve(t, draftFor(t, ref, "1.1.0", "second"))
	if _, err := store.Publish(ctx, two, reviewerID); err != nil {
		t.Fatalf("publish v2: %v", err)
	}
	current, err = store.Resolve(ctx, ref, "")
	if err != nil {
		t.Fatalf("resolve after v2: %v", err)
	}
	if current.Version != "1.1.0" {
		t.Fatalf("current = %s, want 1.1.0", current.Version)
	}

	// The bad publication: roll back to 1.0.0.
	if _, err := store.Rollback(ctx, ref, "1.0.0", "", reviewerID); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	current, err = store.Resolve(ctx, ref, "")
	if err != nil {
		t.Fatalf("resolve after rollback: %v", err)
	}
	if current.Version != "1.0.0" {
		t.Fatalf("current after rollback = %s, want 1.0.0", current.Version)
	}

	// The rolled-away version is deprecated and standing, not gone.
	rolledAway, err := store.Get(ctx, two.ID, "")
	if err != nil {
		t.Fatalf("reading the rolled-away version: %v", err)
	}
	if rolledAway.Status != content.StatusDeprecated {
		t.Fatalf("rolled-away version is %s, want deprecated", rolledAway.Status)
	}
}

func TestPublicationNeverMutatesAPinnedDigest(t *testing.T) {
	// The ADR's central promise and DEC-09's second box: a session pinned
	// v1's digest, v2 publishes, and the pin still resolves to byte-identical
	// content.
	ctx := context.Background()
	store := content.NewStore(pool)
	ref := reference(t)

	one := approve(t, draftFor(t, ref, "1.0.0", "pinned"))
	published, err := store.Publish(ctx, one, reviewerID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// The pin, as composition would take it.
	pinnedDigest := published.Digest
	pinnedBody := string(published.Body)

	two := approve(t, draftFor(t, ref, "2.0.0", "replacement"))
	if _, err := store.Publish(ctx, two, reviewerID); err != nil {
		t.Fatalf("publish v2: %v", err)
	}

	resolved, err := store.GetByDigest(ctx, pinnedDigest, "")
	if err != nil {
		t.Fatalf("the pinned digest no longer resolves: %v", err)
	}
	if string(resolved.Body) != pinnedBody {
		t.Fatal("the pinned digest resolves to different content after a publication")
	}
	if resolved.Version != "1.0.0" {
		t.Fatalf("the pin drifted to version %s", resolved.Version)
	}
}

func TestPublishedRowsAreImmutableByTrigger(t *testing.T) {
	// Even the migrator - the table's owner, with FORCE keeping it honest -
	// cannot edit published content. A change is a new version or nothing.
	ctx := context.Background()
	store := content.NewStore(pool)
	ref := reference(t)

	published, err := store.Publish(ctx, approve(t, draftFor(t, ref, "1.0.0", "frozen")), reviewerID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx,
		`UPDATE content.artifacts SET body = '{"tone":"edited"}' WHERE id = $1`, published.ID)
	if err == nil {
		t.Fatal("a published body was edited; the immutability trigger is gone")
	}

	_, err = conn.Exec(ctx, `DELETE FROM content.artifacts WHERE id = $1`, published.ID)
	if err == nil {
		t.Fatal("a published row was deleted; history left with it")
	}
}

func TestThePublisherMustNotBeTheDrafter(t *testing.T) {
	// ADR-0011's separation of duties, refused by the aggregate rather than
	// by review vigilance: the author holding every capability still cannot
	// ship their own artifact.
	ctx := context.Background()
	store := content.NewStore(pool)

	approved := approve(t, draftFor(t, reference(t), "1.0.0", "self"))
	_, err := store.Publish(ctx, approved, authorID)
	if !errors.Is(err, content.ErrSelfPublish) {
		t.Fatalf("self-publication = %v, want ErrSelfPublish", err)
	}

	// And nothing moved: the version is still approved, the pointer unset.
	still, err := store.Get(ctx, approved.ID, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if still.Status != content.StatusApproved {
		t.Fatalf("the refused publish moved the artifact to %s", still.Status)
	}
}

func TestRollbackRefusesAVersionThatNeverPublished(t *testing.T) {
	// Rollback replays the past; pointing the catalogue at an unreviewed
	// draft would be publication without its duties, through the back door.
	ctx := context.Background()
	store := content.NewStore(pool)
	ref := reference(t)

	if _, err := store.Publish(ctx, approve(t, draftFor(t, ref, "1.0.0", "real")), reviewerID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	draftFor(t, ref, "1.1.0", "never-reviewed")

	_, err := store.Rollback(ctx, ref, "1.1.0", "", reviewerID)
	if !errors.Is(err, content.ErrNotPublished) {
		t.Fatalf("rollback to a draft = %v, want ErrNotPublished", err)
	}
}

func TestTenantsReadTheirOwnAndThePlatformsAndNobodyElses(t *testing.T) {
	// The one-registry-two-scopes rule: alpha's rubric is invisible to beta,
	// the platform catalogue is visible to both.
	ctx := context.Background()
	store := content.NewStore(pool)
	ref := reference(t)

	platform := approve(t, draftFor(t, ref, "1.0.0", "platform"))
	if _, err := store.Publish(ctx, platform, reviewerID); err != nil {
		t.Fatalf("publish platform: %v", err)
	}

	alphaDraft, err := store.CreateDraft(ctx, content.Draft{
		Type: "rubric", Reference: "rubric/" + t.Name(), Version: "1.0.0",
		SchemaVersion: "1.0", Body: json.RawMessage(`{"bands":3}`),
		TenantID: tenantAlpha, CreatedBy: authorID,
	})
	if err != nil {
		t.Fatalf("alpha draft: %v", err)
	}

	// Beta cannot see alpha's artifact even by id.
	if _, err := store.Get(ctx, alphaDraft.ID, tenantBeta); !errors.Is(err, content.ErrNotFound) {
		t.Fatalf("beta read alpha's artifact: %v", err)
	}
	// Alpha sees its own.
	if _, err := store.Get(ctx, alphaDraft.ID, tenantAlpha); err != nil {
		t.Fatalf("alpha cannot read its own artifact: %v", err)
	}
	// Both resolve the platform catalogue.
	for _, tenant := range []string{tenantAlpha, tenantBeta} {
		if _, err := store.Resolve(ctx, ref, tenant); err != nil {
			t.Fatalf("tenant %s cannot resolve the platform catalogue: %v", tenant, err)
		}
	}
}

func TestTheDigestIsCanonicalNotTextual(t *testing.T) {
	// Two spellings of the same document must pin identically, or a
	// reformatted artifact would look like a content change to every consumer.
	a, err := content.DigestOf(json.RawMessage(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatalf("digest a: %v", err)
	}
	b, err := content.DigestOf(json.RawMessage("{\n  \"a\": 1,\n  \"b\": 2\n}"))
	if err != nil {
		t.Fatalf("digest b: %v", err)
	}
	if a != b {
		t.Fatal("key order and whitespace changed the digest")
	}

	c, err := content.DigestOf(json.RawMessage(`{"a":1,"b":3}`))
	if err != nil {
		t.Fatalf("digest c: %v", err)
	}
	if a == c {
		t.Fatal("different content produced the same digest")
	}
}
