//go:build integration

package main

// SCR/EVL-02 for screening: a screening session is composed against exactly the
// configuration its campaign froze, resolved by digest, not against the
// platform default a practice session gets.
//
// The registry publishes the platform rubric, plan and policy in TestMain. Here
// a campaign pins the rubric and plan by their published digests, and the
// composer's screening path is asked what it would send Python. It must be the
// campaign's pinned artifacts, by the digests the campaign chose, plus the
// model policy the platform sets, and nothing from the practice default path.

import (
	"context"
	"errors"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

const screeningTenant = "00000000-0000-7000-8000-0000000000f1"

// pinPublished inserts a campaign and pins the named published artifacts to it
// by their real digests, under the tenant, so the composer can resolve them.
func pinPublished(t *testing.T, references ...string) string {
	t.Helper()
	ctx := context.Background()
	campaignID := id.New().String()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, screeningTenant); err != nil {
		t.Fatalf("scope: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO recruiting.campaign (id, tenant_id, name, role_reference, jurisdiction, created_by)
		 VALUES ($1, $2, 'Composition run', 'role/backend', 'GB', $3)`,
		campaignID, screeningTenant, id.New().String()); err != nil {
		t.Fatalf("campaign: %v", err)
	}
	for _, reference := range references {
		artifact, err := registry.Resolve(ctx, reference, "")
		if err != nil {
			t.Fatalf("resolving %s: %v", reference, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO recruiting.campaign_pin
			   (campaign_id, tenant_id, artifact_type, artifact_id, digest, reference, version)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			campaignID, screeningTenant, artifact.Type, id.New().String(),
			artifact.Digest, artifact.Reference, artifact.Version); err != nil {
			t.Fatalf("pinning %s: %v", reference, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return campaignID
}

func TestScreeningComposesAgainstTheCampaignsPins(t *testing.T) {
	ctx := context.Background()
	campaignID := pinPublished(t, "rubric/practice-default", "bp_backend_v1")

	composer := &grpcComposer{registry: registry, campaigns: recruiting.NewStore(pool)}
	pins, err := composer.resolvePins(ctx, interview.ComposeRequest{
		SessionID: id.New().String(), Mode: "screening",
		CandidateID: id.New().String(), TenantID: screeningTenant, CampaignID: campaignID,
		// A blueprint is deliberately set and deliberately ignored: the campaign's
		// pinned plan is the plan, not this.
		BlueprintID: "bp_backend_v1",
	})
	if err != nil {
		t.Fatalf("resolvePins: %v", err)
	}

	byDigest := map[string]bool{}
	types := map[string]bool{}
	for _, pin := range pins {
		byDigest[pin.Digest] = true
		types[pin.ArtifactType] = true
	}

	// The campaign's two pinned artifacts, by the digests it froze.
	for _, reference := range []string{"rubric/practice-default", "bp_backend_v1"} {
		artifact, err := registry.Resolve(ctx, reference, "")
		if err != nil {
			t.Fatalf("resolving %s: %v", reference, err)
		}
		if !byDigest[artifact.Digest] {
			t.Fatalf("the campaign's pinned %s (%s) was not composed in", reference, artifact.Digest)
		}
	}
	// The model policy is pinned too.
	if !types["model_policy"] {
		t.Fatal("screening composition did not pin the model policy")
	}
}

// A screening request whose campaign pinned nothing is refused, not composed
// into an empty bundle.
func TestScreeningWithNoCampaignPinsIsRefused(t *testing.T) {
	ctx := context.Background()
	empty := pinPublished(t) // a campaign with no pins

	composer := &grpcComposer{registry: registry, campaigns: recruiting.NewStore(pool)}
	_, err := composer.resolvePins(ctx, interview.ComposeRequest{
		SessionID: id.New().String(), Mode: "screening",
		CandidateID: id.New().String(), TenantID: screeningTenant, CampaignID: empty,
		BlueprintID: "bp_backend_v1",
	})
	var failure *interview.ComposeFailure
	if err == nil {
		t.Fatal("a campaign with no pins composed anyway")
	}
	if !errors.As(err, &failure) || failure.Code != "FAILURE_CODE_ARTIFACT_NOT_FOUND" {
		t.Fatalf("error = %v, want an ARTIFACT_NOT_FOUND ComposeFailure", err)
	}
}
