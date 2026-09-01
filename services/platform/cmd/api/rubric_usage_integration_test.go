//go:build integration

package main

import (
	"context"
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

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// TEN-04's third criterion, proven where the two contexts meet.
//
// The library refuses to discard a draft a running campaign is using, and the
// campaign side answers which campaigns those are. Neither half is evidence on
// its own: the library's own tests answer with a stub, and recruiting's answer
// which campaigns use a reference without knowing what the answer is for. cmd
// is the only place allowed to see both, so it is the only place the criterion
// can actually be shown.

var usagePool *pgxpool.Pool

const usageTenant = "00000000-0000-7000-8000-00000000c001"

func TestMain(m *testing.M) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("prepeet"),
		tcpostgres.WithUsername("prepeet_migrator"),
		tcpostgres.WithPassword("migrator-password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(2*time.Minute)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres: %v\n", err)
		os.Exit(1)
	}
	adminURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "url: %v\n", err)
		os.Exit(1)
	}
	if err := database.Migrate(ctx, adminURL, database.MigrateOptions{AppPassword: "app-password"}); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	cfg, _ := pgx.ParseConfig(adminURL)
	usagePool, err = pgxpool.New(ctx, fmt.Sprintf(
		"postgres://prepeet_app:app-password@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "pool: %v\n", err)
		os.Exit(1)
	}
	usageAdminURL = adminURL

	code := m.Run()
	usagePool.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

var usageAdminURL string

func TestARunningCampaignBlocksDiscardingTheRubricItUses(t *testing.T) {
	ctx := context.Background()
	campaigns := recruiting.NewStore(usagePool)
	usage := newRubricUsage(campaigns)

	const reference = "rubric/blocked-by-a-campaign"

	// Nothing uses it yet, so the library would be free to discard.
	blocking, err := usage.InUse(ctx, usageTenant, reference)
	if err != nil {
		t.Fatalf("InUse: %v", err)
	}
	if len(blocking) != 0 {
		t.Fatalf("a reference nobody pinned is reported as in use: %v", blocking)
	}

	openCampaign(t, campaigns, reference, "Backend hiring, Q3")

	blocking, err = usage.InUse(ctx, usageTenant, reference)
	if err != nil {
		t.Fatalf("InUse: %v", err)
	}
	if len(blocking) != 1 || blocking[0] != "Backend hiring, Q3" {
		t.Fatalf("the campaign using the rubric was not reported: %v", blocking)
	}

	// The refusal a person actually meets, built from that answer. It names the
	// campaign, because "it is in use" that cannot say by what leaves an author
	// with nowhere to go.
	refusal := &tenantadmin.RubricInUseError{Reference: reference, Campaigns: blocking}
	if !errors.Is(error(refusal), error(refusal)) {
		t.Fatal("the refusal does not compare to itself")
	}
	if !contains(refusal.Error(), "Backend hiring, Q3") {
		t.Fatalf("the refusal does not name what is blocking: %s", refusal.Error())
	}
}

func TestAClosedCampaignDoesNotBlockDiscarding(t *testing.T) {
	ctx := context.Background()
	campaigns := recruiting.NewStore(usagePool)
	usage := newRubricUsage(campaigns)

	const reference = "rubric/closed-campaign-only"
	campaign := openCampaign(t, campaigns, reference, "Finished hiring round")
	closeCampaign(t, campaign)

	// A closed campaign runs nothing and issues nothing, so it does not stand
	// between an author and tidying up. What it already evaluated is pinned by
	// digest and stays resolvable whatever happens to the draft.
	blocking, err := usage.InUse(ctx, usageTenant, reference)
	if err != nil {
		t.Fatalf("InUse: %v", err)
	}
	if len(blocking) != 0 {
		t.Fatalf("a closed campaign blocked a discard: %v", blocking)
	}
}

func openCampaign(t *testing.T, campaigns *recruiting.Store, reference, name string) recruiting.Campaign {
	t.Helper()
	ctx := context.Background()

	determination := seedUsageDetermination(t)
	draft, err := campaigns.CreateDraft(ctx, recruiting.Campaign{
		TenantID: usageTenant, Name: name, RoleReference: "role/backend",
		Jurisdiction: "GB", CreatedBy: id.New().String(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	opened, err := campaigns.Open(ctx, draft, recruiting.Opening{
		Determination: recruiting.Determination{ID: determination, Jurisdiction: "GB"},
		Pins: []recruiting.Pin{
			{Type: "rubric", Reference: reference, Version: "1.0.0", Digest: digestOf("a")},
			{Type: "calibration", Reference: "calibration/x", Version: "1.0.0", Digest: digestOf("b")},
			{Type: "persona", Reference: "persona/x", Version: "1.0.0", Digest: digestOf("c")},
			{Type: "plan", Reference: "plan/x", Version: "1.0.0", Digest: digestOf("d")},
		},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return opened
}

func closeCampaign(t *testing.T, campaign recruiting.Campaign) {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, usageAdminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()

	if _, err := admin.Exec(ctx,
		`UPDATE recruiting.campaign SET status = 'closed', closed_at = now() WHERE id = $1`,
		campaign.ID); err != nil {
		t.Fatalf("closing the campaign: %v", err)
	}
}

func seedUsageDetermination(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	determinationID := id.New().String()
	admin, err := pgx.Connect(ctx, usageAdminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()

	if _, err := admin.Exec(ctx,
		`INSERT INTO recruiting.jurisdiction_determination
		 (id, jurisdiction, version, result_disclosure, appeal_status, approver, approved_at)
		 VALUES ($1, 'GB', (SELECT coalesce(max(version), 0) + 1
		                    FROM recruiting.jurisdiction_determination WHERE jurisdiction = 'GB'),
		         'evidence_without_band', 'right', 'Test Approver', now())`,
		determinationID); err != nil {
		t.Fatalf("seeding the determination: %v", err)
	}
	return determinationID
}

func digestOf(char string) string {
	out := "sha256:"
	for i := 0; i < 64; i++ {
		out += char
	}
	return out
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
