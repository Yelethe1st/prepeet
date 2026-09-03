//go:build integration

package recruiting_test

// SCR-06 against real PostgreSQL: the things only the schema can promise.
//
// The unit tests prove the decision rules. These prove that the three
// records genuinely cannot be rewritten, that the fulfilment trigger holds
// even for a caller that bypasses the store, that the vocabulary constraint
// refuses a diagnosis-shaped value at the database too, and that a request
// is invisible outside its tenant. Every cross-tenant attempt is scoped to
// a row that exists under the other tenant, because an unscoped attempt
// returns zero rows whether the policy works or not.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// requestFor writes a fresh request on a fresh campaign and returns both.
func requestFor(t *testing.T, tenantID string, adjustment recruiting.Adjustment) recruiting.AccommodationRequest {
	t.Helper()
	ctx := context.Background()
	store := recruiting.NewStore(pool)

	campaign, err := store.CreateDraft(ctx, draftFor(tenantID, "Accommodations "+string(adjustment)))
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	request, err := recruiting.NewAccommodationRequest(recruiting.AccommodationRequestInput{
		TenantID: tenantID, CampaignID: campaign.ID, CandidateID: id.New().String(),
		Adjustment: adjustment, Phase: recruiting.PhaseNoSession,
	})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	stored, err := store.RequestAccommodation(ctx, request)
	if err != nil {
		t.Fatalf("store request: %v", err)
	}
	return stored
}

// decide records a decision through the store, waiting a beat first so that
// two decisions on one request always carry distinct timestamps and "latest"
// is never a coin toss.
func decide(t *testing.T, tenantID string, request recruiting.AccommodationRequest, granted bool) {
	t.Helper()
	time.Sleep(5 * time.Millisecond)
	decision, err := recruiting.NewAccommodationDecision(request.ID, granted, id.New().String())
	if err != nil {
		t.Fatalf("build decision: %v", err)
	}
	if err := recruiting.NewStore(pool).DecideAccommodation(context.Background(), tenantID, request.CampaignID, decision); err != nil {
		t.Fatalf("record decision: %v", err)
	}
}

func TestAnAccommodationRequestCannotBeRewritten(t *testing.T) {
	ctx := context.Background()
	request := requestFor(t, tenantA, recruiting.AdjustmentCaptions)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("scope: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE recruiting.accommodation_request SET adjustment = 'push_to_talk' WHERE id = $1`,
		request.ID); err == nil {
		t.Fatal("a recorded accommodation request was rewritten")
	}
}

func TestAnAccommodationRequestCannotBeDeleted(t *testing.T) {
	ctx := context.Background()
	request := requestFor(t, tenantA, recruiting.AdjustmentExtraTime)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("scope: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM recruiting.accommodation_request WHERE id = $1`, request.ID); err == nil {
		t.Fatal("a recorded accommodation request was deleted")
	}
}

func TestADecisionCannotBeRewritten(t *testing.T) {
	ctx := context.Background()
	request := requestFor(t, tenantA, recruiting.AdjustmentCaptions)
	decide(t, tenantA, request, false)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("scope: %v", err)
	}

	// Turning a recorded decline into a grant is exactly the rewrite the
	// append-only rule exists to refuse; a change of mind is a new row.
	if _, err := tx.Exec(ctx,
		`UPDATE recruiting.accommodation_decision SET granted = true WHERE request_id = $1`,
		request.ID); err == nil {
		t.Fatal("a recorded decision was rewritten")
	}
}

// The database's own vocabulary check: the store never sends this, so it is
// aimed straight at the constraint. A column that accepted free text here is
// where a condition or a diagnosis would end up being stored.
func TestTheSchemaRefusesADiagnosisShapedAdjustment(t *testing.T) {
	ctx := context.Background()
	campaign, err := recruiting.NewStore(pool).CreateDraft(ctx, draftFor(tenantA, "Vocabulary"))
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("scope: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO recruiting.accommodation_request
		 (id, tenant_id, campaign_id, candidate_id, adjustment)
		 VALUES ($1, $2, $3, $4, 'generalised anxiety, see attached note')`,
		id.New().String(), tenantA, campaign.ID, id.New().String()); err == nil {
		t.Fatal("the schema accepted free text where a named adjustment belongs")
	}
}

func TestFulfilmentWithoutAGrantIsRefusedInGo(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	request := requestFor(t, tenantA, recruiting.AdjustmentPushToTalk)

	// Nobody has decided. The store must refuse with the error a caller can
	// act on, before the database ever sees the insert.
	if _, err := store.FulfilAccommodation(ctx, tenantA, request.ID, id.New().String()); !errors.Is(err, recruiting.ErrNotGranted) {
		t.Fatalf("undecided: want ErrNotGranted, got %v", err)
	}

	decide(t, tenantA, request, false)
	if _, err := store.FulfilAccommodation(ctx, tenantA, request.ID, id.New().String()); !errors.Is(err, recruiting.ErrNotGranted) {
		t.Fatalf("declined: want ErrNotGranted, got %v", err)
	}
}

// The same rule, enforced by the trigger for a caller that skips the store.
// The insert here is deliberately raw SQL: it is the future code path that
// forgot the rule, and the database is the guard that catches it.
func TestTheDatabaseRefusesAFulfilmentWithoutAGrant(t *testing.T) {
	ctx := context.Background()
	request := requestFor(t, tenantA, recruiting.AdjustmentCaptions)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("scope: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO recruiting.accommodation_fulfilment (id, tenant_id, request_id, session_id)
		 VALUES ($1, $2, $3, $4)`,
		id.New().String(), tenantA, request.ID, id.New().String()); err == nil {
		t.Fatal("the database recorded a fulfilment nobody granted")
	}
}

// The control for both refusals above, and the first criterion's record: a
// granted adjustment is applied to a named session and is readable back by
// that session.
func TestAGrantedAccommodationIsRecordedOnTheSession(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	request := requestFor(t, tenantA, recruiting.AdjustmentCaptions)
	decide(t, tenantA, request, true)

	sessionID := id.New().String()
	fulfilment, err := store.FulfilAccommodation(ctx, tenantA, request.ID, sessionID)
	if err != nil {
		t.Fatalf("a granted fulfilment was refused: %v", err)
	}
	if fulfilment.SessionID != sessionID {
		t.Fatalf("the fulfilment names session %q, want %q", fulfilment.SessionID, sessionID)
	}

	applied, err := store.AccommodationsForSession(ctx, tenantA, sessionID)
	if err != nil {
		t.Fatalf("reading the session's accommodations: %v", err)
	}
	if len(applied) != 1 || applied[0].Adjustment != recruiting.AdjustmentCaptions {
		t.Fatalf("the session does not carry the adjustment: %+v", applied)
	}
}

// The recruiting half of the third criterion: the alternative assessment
// path is a first-class adjustment that can be requested, granted by a named
// human and recorded against the session that ran it. Conducting that
// session is the interview context's and waits on the contract.
func TestTheAlternativePathIsGrantedAndRecordedLikeAnyAdjustment(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	request := requestFor(t, tenantA, recruiting.AdjustmentAlternativePath)
	decide(t, tenantA, request, true)

	sessionID := id.New().String()
	if _, err := store.FulfilAccommodation(ctx, tenantA, request.ID, sessionID); err != nil {
		t.Fatalf("the alternative path could not be fulfilled: %v", err)
	}
	applied, err := store.AccommodationsForSession(ctx, tenantA, sessionID)
	if err != nil {
		t.Fatalf("reading the session: %v", err)
	}
	if len(applied) != 1 || applied[0].Adjustment != recruiting.AdjustmentAlternativePath {
		t.Fatalf("the alternative path was not recorded on the session: %+v", applied)
	}
}

// A grant is the standing decision only until a later row says otherwise.
// Append-only does not mean decisions are frozen; it means changing one
// leaves the history intact.
func TestAWithdrawnGrantStopsFurtherFulfilment(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	request := requestFor(t, tenantA, recruiting.AdjustmentExtraTime)

	decide(t, tenantA, request, true)
	decide(t, tenantA, request, false)

	if _, err := store.FulfilAccommodation(ctx, tenantA, request.ID, id.New().String()); !errors.Is(err, recruiting.ErrNotGranted) {
		t.Fatalf("a withdrawn grant still fulfilled: %v", err)
	}
}

func TestACandidateSeesTheStateOfTheirRequest(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)

	campaign, err := store.CreateDraft(ctx, draftFor(tenantA, "Candidate view"))
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	candidateID := id.New().String()

	ask := func(adjustment recruiting.Adjustment) recruiting.AccommodationRequest {
		request, err := recruiting.NewAccommodationRequest(recruiting.AccommodationRequestInput{
			TenantID: tenantA, CampaignID: campaign.ID, CandidateID: candidateID,
			Adjustment: adjustment, Phase: recruiting.PhasePreparation,
		})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		stored, err := store.RequestAccommodation(ctx, request)
		if err != nil {
			t.Fatalf("store: %v", err)
		}
		return stored
	}

	pending := ask(recruiting.AdjustmentCaptions)
	granted := ask(recruiting.AdjustmentPushToTalk)
	declined := ask(recruiting.AdjustmentExtraTime)
	decide(t, tenantA, granted, true)
	decide(t, tenantA, declined, false)

	views, err := store.AccommodationsFor(ctx, tenantA, campaign.ID, candidateID)
	if err != nil {
		t.Fatalf("reading the candidate's requests: %v", err)
	}
	states := map[string]recruiting.RequestState{}
	for _, view := range views {
		states[view.Request.ID] = view.State
		if view.State != recruiting.RequestStateRequested && view.DecidedBy == "" {
			t.Fatalf("request %s was decided by nobody", view.Request.ID)
		}
	}
	want := map[string]recruiting.RequestState{
		pending.ID:  recruiting.RequestStateRequested,
		granted.ID:  recruiting.RequestStateGranted,
		declined.ID: recruiting.RequestStateDeclined,
	}
	for requestID, state := range want {
		if states[requestID] != state {
			t.Fatalf("request %s is %q, want %q", requestID, states[requestID], state)
		}
	}
}

// Scoped at a row that exists, by its real id, under the wrong tenant. An
// accommodation request reveals that a person is interviewing somewhere and
// that they asked for an adjustment, which is nobody else's to read.
func TestAnAccommodationIsInvisibleAcrossTenants(t *testing.T) {
	ctx := context.Background()
	request := requestFor(t, tenantB, recruiting.AdjustmentCaptions)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("scope: %v", err)
	}

	var adjustment string
	err = tx.QueryRow(ctx,
		`SELECT adjustment FROM recruiting.accommodation_request WHERE id = $1`,
		request.ID).Scan(&adjustment)
	if err == nil {
		t.Fatalf("tenant A read tenant B's accommodation request: %q", adjustment)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("want the row hidden, got a different failure: %v", err)
	}
}

// A decision lands only on a request that belongs to the named campaign: a
// recruiter cannot answer another campaign's request in the same tenant by its
// id. The request is real under one campaign; deciding it against a different
// campaign finds nothing.
func TestDecideIsScopedToTheRequestsCampaign(t *testing.T) {
	request := requestFor(t, tenantA, recruiting.AdjustmentCaptions)
	otherCampaign := requestFor(t, tenantA, recruiting.AdjustmentExtraTime).CampaignID

	decision, err := recruiting.NewAccommodationDecision(request.ID, true, id.New().String())
	if err != nil {
		t.Fatalf("build decision: %v", err)
	}
	if err := recruiting.NewStore(pool).DecideAccommodation(context.Background(), tenantA, otherCampaign, decision); !errors.Is(err, recruiting.ErrRequestNotFound) {
		t.Fatalf("cross-campaign decide error = %v, want ErrRequestNotFound", err)
	}

	// Its own campaign decides it.
	if err := recruiting.NewStore(pool).DecideAccommodation(context.Background(), tenantA, request.CampaignID, decision); err != nil {
		t.Fatalf("same-campaign decide: %v", err)
	}
}
