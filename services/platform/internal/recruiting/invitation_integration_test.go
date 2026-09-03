//go:build integration

package recruiting_test

// SCR-04 against real PostgreSQL: the things only the schema and the policy
// can promise about an invitation.
//
// The unit tests prove the status rules. These prove that only the hash is
// ever stored, that a resend genuinely retires the links it replaces, that
// revocation stops a live link and deletes nothing, and that one tenant's
// invitations are invisible to another. Every cross-tenant attempt names a
// row that exists under the other tenant, because an unscoped read returns
// zero rows whether the policy holds or not and would pass against nothing.

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/token"
)

// jurisdictionSeq gives each opened campaign a distinct jurisdiction, so the
// per-jurisdiction determination seed never collides across tests.
var jurisdictionSeq int64

// openCampaignFor creates and opens a campaign under one tenant, so an
// invitation has an open campaign to be issued against.
//
// The jurisdiction is unique per campaign because a determination is unique on
// (jurisdiction, version) and seedDetermination always writes version one:
// two campaigns sharing a jurisdiction would collide on the second seed, which
// is a property of the determination table and not of anything under test.
func openCampaignFor(t *testing.T, tenantID string) recruiting.Campaign {
	t.Helper()
	ctx := context.Background()
	store := recruiting.NewStore(pool)

	jurisdiction := fmt.Sprintf("GB-%d", atomic.AddInt64(&jurisdictionSeq, 1))
	determinationID := seedDetermination(t, jurisdiction)
	campaign, err := store.CreateDraft(ctx, recruiting.Campaign{
		TenantID: tenantID, Name: "Invitations " + id.New().String(),
		RoleReference: "role/backend", Jurisdiction: jurisdiction, CreatedBy: id.New().String(),
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	opened, err := store.Open(ctx, campaign, recruiting.Opening{
		Determination: recruiting.Determination{ID: determinationID, Jurisdiction: jurisdiction},
		Pins: []recruiting.Pin{
			{Type: "rubric", Reference: "rubric/backend", Version: "3.0.0", Digest: "sha256:" + repeat("a")},
			{Type: "calibration", Reference: "calibration/backend", Version: "1.0.0", Digest: "sha256:" + repeat("b")},
			{Type: "persona", Reference: "persona/neutral", Version: "1.0.0", Digest: "sha256:" + repeat("c")},
			{Type: "plan", Reference: "plan/standard", Version: "2.0.0", Digest: "sha256:" + repeat("d")},
		},
	})
	if err != nil {
		t.Fatalf("open campaign: %v", err)
	}
	return opened
}

// issue puts one invitation on a campaign, minting a real token and recording
// the plaintext it returns so a test can present or search for it. The enqueue
// closure returns a synthetic email id rather than touching notification: this
// package does not read that schema, and the id is a soft reference with no
// foreign key, so any uuid stands in for the delivery record cmd would write.
func issue(t *testing.T, campaign recruiting.Campaign, recipient string) (recruiting.Invitation, string, string) {
	t.Helper()
	minted, err := token.New(token.PurposeInvitation)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	emailID := id.New().String()
	inv, err := recruiting.NewStore(pool).IssueInvitation(context.Background(),
		recruiting.IssueInvitationInput{
			Campaign: campaign, Recipient: recipient, IssuedBy: id.New().String(),
			ExpiresAt: time.Now().Add(recruiting.InvitationExpiry), TokenHash: minted.Hash,
		},
		func(pgx.Tx) (string, error) { return emailID, nil })
	if err != nil {
		t.Fatalf("issue invitation: %v", err)
	}
	return inv, minted.Plaintext, emailID
}

// The credential is never stored. Only its hash reaches the row, and the
// plaintext appears in no column: a database that leaks hands out no working
// links, which is the whole reason the token is hashed at rest.
func TestAnIssuedInvitationStoresOnlyTheHash(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	inv, plaintext, _ := issue(t, campaign, "candidate@example.com")

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("scope: %v", err)
	}

	var storedHash, storedRecipient string
	if err := tx.QueryRow(ctx,
		`SELECT token_hash, recipient::text FROM recruiting.invitation WHERE id = $1`,
		inv.ID).Scan(&storedHash, &storedRecipient); err != nil {
		t.Fatalf("read row: %v", err)
	}

	if storedHash == plaintext {
		t.Fatal("the plaintext token was stored where the hash belongs")
	}
	if storedHash != token.HashOf(plaintext) {
		t.Fatalf("stored hash %q is not the hash of the issued token", storedHash)
	}

	// The plaintext must not have leaked into any text column of the row.
	var hit int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM recruiting.invitation
		 WHERE id = $1 AND (token_hash = $2 OR recipient::text = $2)`,
		inv.ID, plaintext).Scan(&hit); err != nil {
		t.Fatalf("search for plaintext: %v", err)
	}
	if hit != 0 {
		t.Fatal("the plaintext token appears in the stored row")
	}
}

// A resend retires the live links it replaces, so a recipient forwarded two
// emails cannot accept twice: the earlier invitation reads superseded and only
// the latest is live.
func TestResendSupersedesTheLivePriorInvitation(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	first, _, _ := issue(t, campaign, "resend@example.com")
	second, _, _ := issue(t, campaign, "resend@example.com")

	list, err := recruiting.NewStore(pool).InvitationsForCampaign(context.Background(), tenantA, campaign.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	byID := map[string]recruiting.Invitation{}
	for _, inv := range list {
		byID[inv.ID] = inv
	}
	if got := byID[first.ID].Outcome; got != recruiting.InvitationSuperseded {
		t.Fatalf("first invitation outcome = %q, want superseded", got)
	}
	if got := byID[second.ID].Outcome; got != recruiting.InvitationLive {
		t.Fatalf("second invitation outcome = %q, want live", got)
	}
	if !byID[second.ID].Live(time.Now()) {
		t.Fatal("the resent invitation is not live")
	}
}

// Revocation stops a live link and deletes nothing: the row is still there,
// reads revoked, and a second revoke finds nothing live to stop.
func TestRevokeStopsALiveInvitationAndDeletesNothing(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	inv, _, _ := issue(t, campaign, "revoke@example.com")
	store := recruiting.NewStore(pool)
	ctx := context.Background()

	revoked, err := store.RevokeInvitation(ctx, tenantA, campaign.ID, inv.ID)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked.Outcome != recruiting.InvitationRevoked {
		t.Fatalf("outcome = %q, want revoked", revoked.Outcome)
	}

	// The row survives revocation: it appears in the campaign's list, revoked.
	list, err := store.InvitationsForCampaign(ctx, tenantA, campaign.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, got := range list {
		if got.ID == inv.ID {
			found = true
			if got.Outcome != recruiting.InvitationRevoked {
				t.Fatalf("listed outcome = %q, want revoked", got.Outcome)
			}
		}
	}
	if !found {
		t.Fatal("revocation deleted the invitation row")
	}

	// Revoking again is a no-op refusal, not a second ending.
	if _, err := store.RevokeInvitation(ctx, tenantA, campaign.ID, inv.ID); !errors.Is(err, recruiting.ErrInvitationNotFound) {
		t.Fatalf("second revoke error = %v, want ErrInvitationNotFound", err)
	}
}

// One tenant cannot revoke another's invitation. The id names a real row under
// tenantA, so a pass here is the policy refusing the write, not an id that
// happens to match nothing.
func TestRevokeCannotReachAnotherTenantsInvitation(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	inv, _, _ := issue(t, campaign, "cross@example.com")

	if _, err := recruiting.NewStore(pool).RevokeInvitation(context.Background(), tenantB, campaign.ID, inv.ID); !errors.Is(err, recruiting.ErrInvitationNotFound) {
		t.Fatalf("cross-tenant revoke error = %v, want ErrInvitationNotFound", err)
	}

	// And the invitation under tenantA is untouched: still live.
	list, err := recruiting.NewStore(pool).InvitationsForCampaign(context.Background(), tenantA, campaign.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, got := range list {
		if got.ID == inv.ID && got.Outcome != recruiting.InvitationLive {
			t.Fatalf("the invitation was changed across tenants: outcome %q", got.Outcome)
		}
	}
}

// A recruiter cannot revoke an invitation of a campaign other than the one the
// path names, even in the same tenant. The invitation is real; the campaign in
// the guard is a different real campaign, so the revoke matches nothing. This
// is the per-campaign scope the campaign_id guard exists for.
func TestRevokeIsScopedToTheNamedCampaign(t *testing.T) {
	mine := openCampaignFor(t, tenantA)
	other := openCampaignFor(t, tenantA)
	inv, _, _ := issue(t, mine, "scoped-revoke@example.com")

	if _, err := recruiting.NewStore(pool).RevokeInvitation(
		context.Background(), tenantA, other.ID, inv.ID); !errors.Is(err, recruiting.ErrInvitationNotFound) {
		t.Fatalf("revoke against the wrong campaign error = %v, want ErrInvitationNotFound", err)
	}

	// The invitation on its own campaign is untouched.
	list, err := recruiting.NewStore(pool).InvitationsForCampaign(context.Background(), tenantA, mine.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, got := range list {
		if got.ID == inv.ID && got.Outcome != recruiting.InvitationLive {
			t.Fatalf("the invitation was revoked through the wrong campaign: outcome %q", got.Outcome)
		}
	}
}

// One tenant's roster is invisible to another. The campaign id is real under
// tenantA; listing it as tenantB returns nothing rather than another
// workspace's invitations.
func TestInvitationListIsTenantScoped(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	issue(t, campaign, "scoped@example.com")

	list, err := recruiting.NewStore(pool).InvitationsForCampaign(context.Background(), tenantB, campaign.ID)
	if err != nil {
		t.Fatalf("list as other tenant: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("tenantB read %d of tenantA's invitations", len(list))
	}
}

// Issuing against a campaign that is not open is refused, and the email is
// never enqueued: a link to a campaign whose configuration is not fixed would
// be exactly the promise opening exists to prevent.
func TestIssuingRefusesACampaignThatIsNotOpen(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	draft, err := store.CreateDraft(ctx, draftFor(tenantA, "Still a draft"))
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	enqueued := false
	_, err = store.IssueInvitation(ctx, recruiting.IssueInvitationInput{
		Campaign: draft, Recipient: "draft@example.com", IssuedBy: id.New().String(),
		ExpiresAt: time.Now().Add(recruiting.InvitationExpiry), TokenHash: token.HashOf("whatever"),
	}, func(pgx.Tx) (string, error) { enqueued = true; return "", nil })

	if !errors.Is(err, recruiting.ErrCampaignNotOpen) {
		t.Fatalf("error = %v, want ErrCampaignNotOpen", err)
	}
	if enqueued {
		t.Fatal("an email was enqueued for an invitation that was refused")
	}
}

// InvitationByID reads one invitation on its campaign, refuses an unknown id,
// and refuses one that belongs to a different campaign, which is the read the
// resend path depends on for the recipient and the outcome.
func TestInvitationByIDReadsAndScopes(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	other := openCampaignFor(t, tenantA)
	inv, _, emailID := issue(t, campaign, "byid@example.com")
	store := recruiting.NewStore(pool)
	ctx := context.Background()

	got, err := store.InvitationByID(ctx, tenantA, campaign.ID, inv.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Recipient != "byid@example.com" || got.EmailID != emailID {
		t.Fatalf("read back the wrong invitation: %+v", got)
	}
	if got.Outcome != recruiting.InvitationLive {
		t.Fatalf("outcome = %q, want live", got.Outcome)
	}

	// An unknown id is not found.
	if _, err := store.InvitationByID(ctx, tenantA, campaign.ID, id.New().String()); !errors.Is(err, recruiting.ErrInvitationNotFound) {
		t.Fatalf("unknown id error = %v, want ErrInvitationNotFound", err)
	}

	// The invitation read through the wrong campaign is not found, the same
	// per-campaign scope revoke enforces.
	if _, err := store.InvitationByID(ctx, tenantA, other.ID, inv.ID); !errors.Is(err, recruiting.ErrInvitationNotFound) {
		t.Fatalf("wrong-campaign read error = %v, want ErrInvitationNotFound", err)
	}
}

// A candidate holding the token resolves exactly the invitation it names, and
// an unknown token resolves to nothing. This is the token-scoped policy: the
// row is reachable only with the hash that produced it.
func TestResolveInvitationByToken(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	inv, plaintext, _ := issue(t, campaign, "resolve@example.com")
	store := recruiting.NewStore(pool)
	ctx := context.Background()

	got, err := store.ResolveInvitationByToken(ctx, token.HashOf(plaintext))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.ID != inv.ID || got.CampaignID != campaign.ID {
		t.Fatalf("resolved the wrong invitation: %+v", got)
	}

	// A token nobody issued resolves to nothing, the same answer a spent link
	// would give, so a guess cannot be told from a real dead token.
	if _, err := store.ResolveInvitationByToken(ctx, token.HashOf("inv_not-a-real-token")); !errors.Is(err, recruiting.ErrInvitationUnknownToken) {
		t.Fatalf("unknown token error = %v, want ErrInvitationUnknownToken", err)
	}
}

// Holding one campaign's token gives no reach to another invitation. The token
// scope is one row, so resolving with the wrong hash returns nothing even for a
// real invitation that exists under the same tenant.
func TestATokenReachesOnlyItsOwnInvitation(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	mine, mineToken, _ := issue(t, campaign, "mine@example.com")
	_, _, _ = issue(t, campaign, "theirs@example.com")
	store := recruiting.NewStore(pool)
	ctx := context.Background()

	// My token resolves my invitation.
	got, err := store.ResolveInvitationByToken(ctx, token.HashOf(mineToken))
	if err != nil || got.ID != mine.ID {
		t.Fatalf("my token did not resolve my invitation: %v %+v", err, got)
	}
	// A hash I do not hold resolves nothing, even though a real invitation with
	// that recipient exists: the policy admits only the row whose hash I set.
	if _, err := store.ResolveInvitationByToken(ctx, token.HashOf("inv_someone-elses")); !errors.Is(err, recruiting.ErrInvitationUnknownToken) {
		t.Fatalf("a hash I do not hold resolved something: %v", err)
	}
}

// Accepting is single-use: the first accept wins, a second finds nothing live.
func TestAcceptInvitationByTokenIsSingleUse(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	_, plaintext, _ := issue(t, campaign, "accept@example.com")
	store := recruiting.NewStore(pool)
	ctx := context.Background()
	hash := token.HashOf(plaintext)
	candidateID := seedCandidate(t)

	accepted, err := store.AcceptInvitationByToken(ctx, hash, candidateID)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if accepted.Outcome != recruiting.InvitationAccepted {
		t.Fatalf("outcome = %q, want accepted", accepted.Outcome)
	}
	if _, err := store.AcceptInvitationByToken(ctx, hash, candidateID); !errors.Is(err, recruiting.ErrInvitationNotLive) {
		t.Fatalf("second accept error = %v, want ErrInvitationNotLive", err)
	}
}

// An expired invitation cannot be accepted, even though its outcome is still
// null: expiry is a guard in the update, not a stored flag.
func TestAcceptRefusesAnExpiredInvitation(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	inv, plaintext, _ := issue(t, campaign, "expired@example.com")
	expireInvitation(t, inv.ID)

	if _, err := recruiting.NewStore(pool).AcceptInvitationByToken(context.Background(), token.HashOf(plaintext), seedCandidate(t)); !errors.Is(err, recruiting.ErrInvitationNotLive) {
		t.Fatalf("accept of expired error = %v, want ErrInvitationNotLive", err)
	}
}

// Declining is a first-class, single-use outcome: it records declined and a
// second decline finds nothing live.
func TestDeclineInvitationByToken(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	_, plaintext, _ := issue(t, campaign, "decline@example.com")
	store := recruiting.NewStore(pool)
	ctx := context.Background()
	hash := token.HashOf(plaintext)

	declined, err := store.DeclineInvitationByToken(ctx, hash)
	if err != nil {
		t.Fatalf("decline: %v", err)
	}
	if declined.Outcome != recruiting.InvitationDeclined {
		t.Fatalf("outcome = %q, want declined", declined.Outcome)
	}
	if _, err := store.DeclineInvitationByToken(ctx, hash); !errors.Is(err, recruiting.ErrInvitationNotLive) {
		t.Fatalf("second decline error = %v, want ErrInvitationNotLive", err)
	}
}

// A revoked invitation cannot be accepted: the recruiter's stop holds against
// the candidate's click.
func TestAcceptRefusesARevokedInvitation(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	inv, plaintext, _ := issue(t, campaign, "revoked-then-accept@example.com")
	store := recruiting.NewStore(pool)
	ctx := context.Background()

	if _, err := store.RevokeInvitation(ctx, tenantA, campaign.ID, inv.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := store.AcceptInvitationByToken(ctx, token.HashOf(plaintext), seedCandidate(t)); !errors.Is(err, recruiting.ErrInvitationNotLive) {
		t.Fatalf("accept of revoked error = %v, want ErrInvitationNotLive", err)
	}
}

// CampaignByID reads the campaign an invitation points to, tenant-scoped.
func TestCampaignByIDReadsWithinTheTenant(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	store := recruiting.NewStore(pool)
	ctx := context.Background()

	got, err := store.CampaignByID(ctx, tenantA, campaign.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.ID != campaign.ID || got.RoleReference != campaign.RoleReference {
		t.Fatalf("read back the wrong campaign: %+v", got)
	}
	// Another tenant cannot read it, the same tenant policy every campaign read
	// carries.
	if _, err := store.CampaignByID(ctx, tenantB, campaign.ID); !errors.Is(err, recruiting.ErrNoAccess) {
		t.Fatalf("cross-tenant campaign read error = %v, want ErrNoAccess", err)
	}
}

// An accepted invitation is readable by the candidate who accepted it, and by
// no other candidate: the authority the screening session creation checks.
func TestAcceptedInvitationIsReadableByItsCandidate(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	_, plaintext, _ := issue(t, campaign, "accepted-read@example.com")
	store := recruiting.NewStore(pool)
	ctx := context.Background()
	candidateID := seedCandidate(t)

	if _, err := store.AcceptInvitationByToken(ctx, token.HashOf(plaintext), candidateID); err != nil {
		t.Fatalf("accept: %v", err)
	}

	got, err := store.AcceptedInvitationForCandidate(ctx, campaign.ID, candidateID)
	if err != nil {
		t.Fatalf("accepted read: %v", err)
	}
	if got.CampaignID != campaign.ID || got.Outcome != recruiting.InvitationAccepted {
		t.Fatalf("read back %+v", got)
	}

	// A candidate who accepted nothing here finds nothing, so they cannot
	// start a session against a campaign they were never invited to.
	if _, err := store.AcceptedInvitationForCandidate(ctx, campaign.ID, seedCandidate(t)); !errors.Is(err, recruiting.ErrInvitationNotFound) {
		t.Fatalf("another candidate read the acceptance: %v", err)
	}
}

// seedCandidate inserts a user so an accepted invitation's candidate FK resolves.
func seedCandidate(t *testing.T) string {
	t.Helper()
	candidateID := id.New().String()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO identity.users (id, email) VALUES ($1, $2)`,
		candidateID, "cand-"+candidateID+"@example.com"); err != nil {
		t.Fatalf("seeding a candidate: %v", err)
	}
	return candidateID
}

// expireInvitation moves an invitation's expiry into the past, as the migrator,
// so an accept can be tested against a lapsed link without waiting a week.
func expireInvitation(t *testing.T, invitationID string) {
	t.Helper()
	admin, err := pgxpool.New(context.Background(), adminURL)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer admin.Close()
	if _, err := admin.Exec(context.Background(),
		`UPDATE recruiting.invitation SET expires_at = now() - interval '1 hour' WHERE id = $1`,
		invitationID); err != nil {
		t.Fatalf("expiring the invitation: %v", err)
	}
}

// The email id is recorded so cmd can join delivery status: the invitation
// points at the notification row it was carried by.
func TestTheEmailIdIsRecordedForDeliveryStatus(t *testing.T) {
	campaign := openCampaignFor(t, tenantA)
	inv, _, emailID := issue(t, campaign, "delivery@example.com")
	if inv.EmailID != emailID {
		t.Fatalf("invitation email id = %q, want %q", inv.EmailID, emailID)
	}
}
