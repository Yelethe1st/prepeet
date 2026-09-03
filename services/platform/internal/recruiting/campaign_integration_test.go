//go:build integration

package recruiting_test

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
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// SCR-01 and SCR-02 against real PostgreSQL.
//
// The unit tests prove the opening decision. These prove the things only the
// database can answer: that a campaign is invisible outside its tenant, that a
// recruiter sees only campaigns they are on, that an acceptance cannot be
// edited afterwards, and that a republished disclosure leaves an existing
// acceptance alone.
//
// Every cross-tenant attempt here is scoped to a row that exists under the
// other tenant. An unscoped attempt matches zero rows whether the policy works
// or not, so it would pass against no security at all.

var (
	pool *pgxpool.Pool
	// The migrator's connection, kept because determinations are seeded with
	// it: the application role is granted SELECT only on that table, which is
	// the guarantee rather than an oversight, so a test that needs one has to
	// act as the role that may write it.
	adminURL string
)

const (
	tenantA = "00000000-0000-7000-8000-00000000a001"
	tenantB = "00000000-0000-7000-8000-00000000b001"
)

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
	adminURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "url: %v\n", err)
		os.Exit(1)
	}
	if err := database.Migrate(ctx, adminURL, database.MigrateOptions{AppPassword: "app-password"}); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	cfg, _ := pgx.ParseConfig(adminURL)
	pool, err = pgxpool.New(ctx, fmt.Sprintf(
		"postgres://prepeet_app:app-password@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "pool: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

// seedDetermination writes a determination as the migrator.
//
// Nothing in the application may write this table: it grants SELECT only, and
// that is the guarantee rather than an oversight, since a determination is
// counsel's record and not something a request should be able to mint. A test
// that needs one therefore acts as the role that may.
func seedDetermination(t *testing.T, jurisdiction string) string {
	t.Helper()
	determinationID := id.New().String()
	admin, err := pgxpool.New(context.Background(), adminURL)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer admin.Close()
	_, err = admin.Exec(context.Background(),
		`INSERT INTO recruiting.jurisdiction_determination
		 (id, jurisdiction, version, result_disclosure, appeal_status, approver, approved_at)
		 VALUES ($1, $2, 1, 'evidence_without_band', 'right', 'Test Approver', now())`,
		determinationID, jurisdiction)
	if err != nil {
		t.Fatalf("seeding the determination: %v", err)
	}
	return determinationID
}

// TestTheApplicationCannotMintADetermination is the other half of that grant.
//
// ADR-0020 makes a missing determination the refusal that keeps DEC-11 honest.
// If a request could write one, the refusal would be a formality anybody could
// step around, so the absence of an INSERT grant is load-bearing and is tested
// as such.
func TestTheApplicationCannotMintADetermination(t *testing.T) {
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`INSERT INTO recruiting.jurisdiction_determination
		 (id, jurisdiction, version, result_disclosure, appeal_status, approver, approved_at)
		 VALUES ($1, 'ZZ', 1, 'full_evaluation', 'platform_policy', 'Not Counsel', now())`,
		id.New().String())
	if err == nil {
		t.Fatal("the application role minted its own legal determination")
	}
}

// The whole opening flow against the database: a draft with a determination and
// four published pins becomes open, and its pins are what it opened with.
func TestOpeningWritesThePinsAndTheDeterminationTogether(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	determinationID := seedDetermination(t, "FR")

	campaign, err := store.CreateDraft(ctx, recruiting.Campaign{
		TenantID: tenantA, Name: "Opening flow", RoleReference: "role/backend",
		Jurisdiction: "FR", CreatedBy: id.New().String(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	opening := recruiting.Opening{
		Determination: recruiting.Determination{ID: determinationID, Jurisdiction: "FR"},
		Pins: []recruiting.Pin{
			{Type: "rubric", Reference: "rubric/backend", Version: "3.0.0", Digest: "sha256:" + repeat("a")},
			{Type: "calibration", Reference: "calibration/backend", Version: "1.0.0", Digest: "sha256:" + repeat("b")},
			{Type: "persona", Reference: "persona/neutral", Version: "1.0.0", Digest: "sha256:" + repeat("c")},
			{Type: "plan", Reference: "plan/standard", Version: "2.0.0", Digest: "sha256:" + repeat("d")},
		},
	}

	opened, err := store.Open(ctx, campaign, opening)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if opened.Status != recruiting.StatusOpen {
		t.Fatalf("status is %q", opened.Status)
	}
	if opened.DeterminationID != determinationID {
		t.Fatalf("the campaign did not pin the determination it opened against")
	}

	// Opening twice must not re-resolve anything. The guard is in the query's
	// WHERE, so the second attempt finds no draft rather than overwriting.
	if _, err := store.Open(ctx, campaign, opening); err == nil {
		t.Fatal("a campaign was opened twice")
	}
}

// repeat builds the hex half of a digest.
func repeat(char string) string {
	out := ""
	for i := 0; i < 64; i++ {
		out += char
	}
	return out
}

func draftFor(tenantID, name string) recruiting.Campaign {
	return recruiting.Campaign{
		TenantID: tenantID, Name: name,
		RoleReference: "role/backend", Jurisdiction: "GB",
		CreatedBy: id.New().String(),
	}
}

// Criterion three of SCR-01: recruiter access is scoped per campaign and
// enforced server-side.
func TestARecruiterSeesOnlyTheCampaignsTheyAreOn(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	recruiter := id.New().String()

	mine, err := store.CreateDraft(ctx, draftFor(tenantA, "Mine"))
	if err != nil {
		t.Fatalf("create mine: %v", err)
	}
	theirs, err := store.CreateDraft(ctx, draftFor(tenantA, "Someone else's"))
	if err != nil {
		t.Fatalf("create theirs: %v", err)
	}
	if err := store.GrantAccess(ctx, tenantA, mine.ID, recruiter, id.New().String()); err != nil {
		t.Fatalf("grant: %v", err)
	}

	if _, err := store.CampaignForRecruiter(ctx, tenantA, mine.ID, recruiter); err != nil {
		t.Fatalf("a recruiter could not read their own campaign: %v", err)
	}

	// Scoped at a campaign that exists, in the same tenant, which the recruiter
	// is simply not on. Membership of the workspace is not membership of every
	// campaign in it.
	_, err = store.CampaignForRecruiter(ctx, tenantA, theirs.ID, recruiter)
	if !errors.Is(err, recruiting.ErrNoAccess) {
		t.Fatalf("a recruiter read a campaign they are not on: %v", err)
	}
}

func TestACampaignIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	recruiter := id.New().String()

	theirs, err := store.CreateDraft(ctx, draftFor(tenantB, "Tenant B's campaign"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Granted under B, so the only thing standing between A and this row is the
	// row-level security policy.
	if err := store.GrantAccess(ctx, tenantB, theirs.ID, recruiter, id.New().String()); err != nil {
		t.Fatalf("grant: %v", err)
	}

	// The same recruiter, the same campaign id, read under tenant A. Every
	// identifier here is real, so a policy that did nothing would return it.
	_, err = store.CampaignForRecruiter(ctx, tenantA, theirs.ID, recruiter)
	if !errors.Is(err, recruiting.ErrNoAccess) {
		t.Fatalf("tenant A read tenant B's campaign: %v", err)
	}
}

// The campaign table's own policy, isolated.
//
// TestACampaignIsInvisibleFromAnotherTenant goes through a join with
// campaign_recruiter, so it is defended by that table's policy and passes even
// if campaign's own policy is permissive. Attacking the policy proved exactly
// that: making campaign_tenant USING(true) left every test green. This reads
// the campaign row directly, so nothing but campaign's policy stands in the
// way, and it is the test that fails when that policy is weakened.
func TestTheCampaignTableRefusesACrossTenantReadOnItsOwn(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)

	theirs, err := store.CreateDraft(ctx, draftFor(tenantB, "B's own row"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("scope: %v", err)
	}

	// Scoped at a row that exists, by its real id, under the wrong tenant. An
	// unscoped select would return nothing whether the policy worked or not.
	var name string
	err = tx.QueryRow(ctx,
		`SELECT name FROM recruiting.campaign WHERE id = $1`, theirs.ID).Scan(&name)
	if err == nil {
		t.Fatalf("tenant A read tenant B's campaign row directly: %q", name)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("want the row hidden, got a different failure: %v", err)
	}
}

// The same isolation for the pins, which carry the configuration a competitor
// would most want: which rubric and persona a rival is hiring against.
func TestCampaignPinsAreInvisibleAcrossTenants(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	determinationID := seedDetermination(t, "ES")

	campaign, err := store.CreateDraft(ctx, recruiting.Campaign{
		TenantID: tenantB, Name: "B's pins", RoleReference: "role/backend",
		Jurisdiction: "ES", CreatedBy: id.New().String(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	opening := recruiting.Opening{
		Determination: recruiting.Determination{ID: determinationID, Jurisdiction: "ES"},
		Pins: []recruiting.Pin{
			{Type: "rubric", Reference: "rubric/backend", Version: "3.0.0", Digest: "sha256:" + repeat("e")},
			{Type: "calibration", Reference: "calibration/backend", Version: "1.0.0", Digest: "sha256:" + repeat("f")},
			{Type: "persona", Reference: "persona/neutral", Version: "1.0.0", Digest: "sha256:" + repeat("0")},
			{Type: "plan", Reference: "plan/standard", Version: "2.0.0", Digest: "sha256:" + repeat("1")},
		},
	}
	if _, err := store.Open(ctx, campaign, opening); err != nil {
		t.Fatalf("open: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("scope: %v", err)
	}

	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM recruiting.campaign_pin WHERE campaign_id = $1`,
		campaign.ID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("tenant A can see %d of tenant B's pins", count)
	}
}

// Criterion one of SCR-02: the exact disclosure version is recorded with the
// acceptance.
func TestAnAcceptanceRecordsTheExactVersionAndSurvivesRepublication(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	candidate := id.New().String()

	campaign, err := store.CreateDraft(ctx, draftFor(tenantA, "Disclosure test"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	first := "sha256:" + "1111111111111111111111111111111111111111111111111111111111111111"
	acceptance, err := recruiting.NewAcceptance(recruiting.AcceptanceRequest{
		TenantID: tenantA, CampaignID: campaign.ID, CandidateID: candidate,
		DisclosureDigest: first, DisclosureVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("NewAcceptance: %v", err)
	}
	decisions := []recruiting.ConsentDecision{
		{Purpose: "interview_and_evaluation", Required: true, Granted: true},
		{Purpose: "model_improvement", Required: false, Granted: false},
	}
	if err := store.RecordAcceptance(ctx, acceptance, decisions); err != nil {
		t.Fatalf("RecordAcceptance: %v", err)
	}

	// The disclosure is republished at 2.0.0, which is the ordinary thing that
	// happens to a disclosure. Criterion three: it must not rewrite what
	// somebody already accepted.
	second := "sha256:" + "2222222222222222222222222222222222222222222222222222222222222222"
	later, err := recruiting.NewAcceptance(recruiting.AcceptanceRequest{
		TenantID: tenantA, CampaignID: campaign.ID, CandidateID: candidate,
		DisclosureDigest: second, DisclosureVersion: "2.0.0",
	})
	if err != nil {
		t.Fatalf("NewAcceptance: %v", err)
	}
	if err := store.RecordAcceptance(ctx, later, decisions); err != nil {
		t.Fatalf("RecordAcceptance second: %v", err)
	}

	history, err := store.AcceptancesFor(ctx, tenantA, campaign.ID, candidate)
	if err != nil {
		t.Fatalf("AcceptancesFor: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("want both acceptances kept, got %d", len(history))
	}
	var kept bool
	for _, row := range history {
		if row.DisclosureVersion == "1.0.0" && row.DisclosureDigest == first {
			kept = true
		}
	}
	if !kept {
		t.Fatal("republishing the disclosure rewrote what the candidate had already accepted")
	}
}

// Criterion three of SCR-02, said the other way round: the row itself cannot be
// edited, so no code path can quietly restate what somebody agreed to.
func TestAnAcceptanceCannotBeEditedOrDeleted(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	candidate := id.New().String()

	campaign, err := store.CreateDraft(ctx, draftFor(tenantA, "Immutability test"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	digest := "sha256:" + "3333333333333333333333333333333333333333333333333333333333333333"
	acceptance, err := recruiting.NewAcceptance(recruiting.AcceptanceRequest{
		TenantID: tenantA, CampaignID: campaign.ID, CandidateID: candidate,
		DisclosureDigest: digest, DisclosureVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("NewAcceptance: %v", err)
	}
	if err := store.RecordAcceptance(ctx, acceptance, nil); err != nil {
		t.Fatalf("RecordAcceptance: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("scope: %v", err)
	}

	// Scoped at the row just written, under its own tenant, so the only thing
	// that can refuse this is the trigger.
	_, err = tx.Exec(ctx,
		`UPDATE recruiting.disclosure_acceptance SET disclosure_version = '9.9.9'
		 WHERE campaign_id = $1 AND candidate_id = $2`, campaign.ID, candidate)
	if err == nil {
		t.Fatal("an acceptance was edited after the fact")
	}

	_, err = tx.Exec(ctx,
		`DELETE FROM recruiting.disclosure_acceptance
		 WHERE campaign_id = $1 AND candidate_id = $2`, campaign.ID, candidate)
	if err == nil {
		t.Fatal("an acceptance was deleted")
	}
}

// The standing decision is the latest per purpose, which is how a withdrawal
// will be recorded under SEC-04 without any row being edited.
func TestALaterDecisionSupersedesWithoutEditing(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	candidate := id.New().String()

	campaign, err := store.CreateDraft(ctx, draftFor(tenantA, "Withdrawal shape"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	digest := "sha256:" + "4444444444444444444444444444444444444444444444444444444444444444"
	acceptance, err := recruiting.NewAcceptance(recruiting.AcceptanceRequest{
		TenantID: tenantA, CampaignID: campaign.ID, CandidateID: candidate,
		DisclosureDigest: digest, DisclosureVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("NewAcceptance: %v", err)
	}

	if err := store.RecordAcceptance(ctx, acceptance, []recruiting.ConsentDecision{
		{Purpose: "model_improvement", Required: false, Granted: true},
	}); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Time moves so the ordering is unambiguous rather than resolved by chance.
	time.Sleep(10 * time.Millisecond)
	if err := store.RecordAcceptance(ctx, acceptance, []recruiting.ConsentDecision{
		{Purpose: "model_improvement", Required: false, Granted: false},
	}); err != nil {
		t.Fatalf("second: %v", err)
	}

	standing, err := store.StandingConsent(ctx, tenantA, campaign.ID, candidate)
	if err != nil {
		t.Fatalf("StandingConsent: %v", err)
	}
	if len(standing) != 1 {
		t.Fatalf("want one standing decision per purpose, got %d", len(standing))
	}
	if standing[0].Granted {
		t.Fatal("the withdrawn decision is still standing")
	}
}

// The database refuses what the Go refuses, independently.
func TestTheDatabaseRefusesModelImprovementAsRequired(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	candidate := id.New().String()

	campaign, err := store.CreateDraft(ctx, draftFor(tenantA, "Bundling attempt"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	digest := "sha256:" + "5555555555555555555555555555555555555555555555555555555555555555"
	acceptance, err := recruiting.NewAcceptance(recruiting.AcceptanceRequest{
		TenantID: tenantA, CampaignID: campaign.ID, CandidateID: candidate,
		DisclosureDigest: digest, DisclosureVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("NewAcceptance: %v", err)
	}

	// Straight past ValidatePurposes and into the store, which is what a future
	// caller that forgot to validate would do.
	err = store.RecordAcceptance(ctx, acceptance, []recruiting.ConsentDecision{
		{Purpose: "model_improvement", Required: true, Granted: true},
	})
	if err == nil {
		t.Fatal("the database accepted model improvement as a required consent")
	}
}

// TEN-04's third criterion, from the campaign side: a rubric a running campaign
// is using is not removed even as a draft.
//
// The refusal itself lives in the rubric library. What this proves is the
// answer it depends on, which was an unimplemented port until now: the library
// was refusing on the strength of a question nobody could answer.
func TestAnOpenCampaignIsReportedAsUsingItsPinnedRubric(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	determinationID := seedDetermination(t, "PT")

	campaign, err := store.CreateDraft(ctx, recruiting.Campaign{
		TenantID: tenantA, Name: "Uses the backend rubric", RoleReference: "role/backend",
		Jurisdiction: "PT", CreatedBy: id.New().String(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// A draft pins nothing, so nothing is in use yet.
	using, err := store.CampaignsUsing(ctx, tenantA, "rubric/backend-ten04")
	if err != nil {
		t.Fatalf("CampaignsUsing: %v", err)
	}
	if len(using) != 0 {
		t.Fatalf("a draft campaign was reported as using a rubric: %v", using)
	}

	if _, err := store.Open(ctx, campaign, recruiting.Opening{
		Determination: recruiting.Determination{ID: determinationID, Jurisdiction: "PT"},
		Pins: []recruiting.Pin{
			{Type: "rubric", Reference: "rubric/backend-ten04", Version: "3.0.0", Digest: "sha256:" + repeat("2")},
			{Type: "calibration", Reference: "calibration/backend", Version: "1.0.0", Digest: "sha256:" + repeat("3")},
			{Type: "persona", Reference: "persona/neutral", Version: "1.0.0", Digest: "sha256:" + repeat("4")},
			{Type: "plan", Reference: "plan/standard", Version: "2.0.0", Digest: "sha256:" + repeat("5")},
		},
	}); err != nil {
		t.Fatalf("open: %v", err)
	}

	using, err = store.CampaignsUsing(ctx, tenantA, "rubric/backend-ten04")
	if err != nil {
		t.Fatalf("CampaignsUsing: %v", err)
	}
	if len(using) != 1 || using[0] != "Uses the backend rubric" {
		t.Fatalf("an open campaign is not reported as using its rubric: %v", using)
	}
}

// Another workspace's campaign is not an answer to this workspace's question,
// and leaking the name would say who else is hiring for what.
func TestUsageDoesNotReachAcrossTenants(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	determinationID := seedDetermination(t, "NL")

	campaign, err := store.CreateDraft(ctx, recruiting.Campaign{
		TenantID: tenantB, Name: "B's campaign", RoleReference: "role/backend",
		Jurisdiction: "NL", CreatedBy: id.New().String(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.Open(ctx, campaign, recruiting.Opening{
		Determination: recruiting.Determination{ID: determinationID, Jurisdiction: "NL"},
		Pins: []recruiting.Pin{
			{Type: "rubric", Reference: "rubric/shared-ten04", Version: "1.0.0", Digest: "sha256:" + repeat("6")},
			{Type: "calibration", Reference: "calibration/backend", Version: "1.0.0", Digest: "sha256:" + repeat("7")},
			{Type: "persona", Reference: "persona/neutral", Version: "1.0.0", Digest: "sha256:" + repeat("8")},
			{Type: "plan", Reference: "plan/standard", Version: "2.0.0", Digest: "sha256:" + repeat("9")},
		},
	}); err != nil {
		t.Fatalf("open: %v", err)
	}

	// Asked under A, about the reference B is using. A real campaign exists, so
	// an empty answer here is the policy working rather than nothing to find.
	using, err := store.CampaignsUsing(ctx, tenantA, "rubric/shared-ten04")
	if err != nil {
		t.Fatalf("CampaignsUsing: %v", err)
	}
	if len(using) != 0 {
		t.Fatalf("tenant A was told about tenant B's campaign: %v", using)
	}
}

// DeterminationByID answers the exact version a campaign pinned, not the
// jurisdiction's latest: the disclosure a running campaign's candidates get is
// the one counsel approved when it opened.
func TestDeterminationByIDReadsThePinnedVersion(t *testing.T) {
	store := recruiting.NewStore(pool)
	determinationID := seedDetermination(t, "DE")

	got, err := store.DeterminationByID(context.Background(), determinationID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.ID != determinationID || got.ResultDisclosure != "evidence_without_band" {
		t.Fatalf("read back %+v", got)
	}
	if _, err := store.DeterminationByID(context.Background(), id.New().String()); !errors.Is(err, recruiting.ErrNoDetermination) {
		t.Fatalf("unknown id error = %v, want ErrNoDetermination", err)
	}
}
