//go:build integration

package tenantadmin_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin"
)

// TEN-03 against real PostgreSQL. The review is a row that exists and has to
// be answered, not a query somebody could have run: opening it materialises
// one item per member, dormancy is computed from what the workspace has
// actually seen them do, and it cannot be closed with an item still pending.

// roster is the port opening a review reads its people through. A fake here
// rather than identity, because a bounded context never imports another and
// cmd wires the two: what this package is responsible for is what it does
// with a roster, not where the roster comes from.
type roster struct {
	entries []tenantadmin.RosterEntry
	err     error
}

func (r *roster) List(context.Context, string) ([]tenantadmin.RosterEntry, error) {
	return r.entries, r.err
}

// revoker records what the review asked to have removed.
type revoker struct {
	revoked []string
	err     error
}

func (r *revoker) Revoke(_ context.Context, _, _, membershipID string) error {
	if r.err != nil {
		return r.err
	}
	r.revoked = append(r.revoked, membershipID)
	return nil
}

// schedule is the standard these tests review against: quarterly, a fortnight
// to finish, dormant after ninety days.
func schedule() tenantadmin.ReviewSchedule {
	return tenantadmin.ReviewSchedule{EveryDays: 90, CompleteWithinDays: 14, DormantAfterDays: 90}
}

// acted writes an audit row for one person in one workspace at one time,
// which is the signal dormancy is read from.
func acted(t *testing.T, tenantID, userID string, when time.Time) {
	t.Helper()
	if _, err := admin(t).Exec(context.Background(), `
		INSERT INTO audit.events (id, tenant_id, actor_id, actor_type, action, outcome, occurred_at)
		VALUES (gen_random_uuid(), $1, $2, 'user', 'evaluation.reviewed', 'allowed', $3)`,
		tenantID, userID, when); err != nil {
		t.Fatalf("seeding activity: %v", err)
	}
}

// workspace seeds a tenant with two members: one who acted yesterday and one
// who has never been seen doing anything here.
func workspace(t *testing.T, now time.Time) (reviews *tenantadmin.AccessReviews, tenantID, actorID string,
	active, dormant tenantadmin.RosterEntry, removals *revoker) {
	t.Helper()
	tenantID = seedTenant(t)
	actorID = seedUser(t)

	activeMembership, activeUser := seedMember(t, tenantID, "recruiter")
	dormantMembership, dormantUser := seedMember(t, tenantID, "viewer")
	acted(t, tenantID, activeUser, now.Add(-24*time.Hour))

	active = tenantadmin.RosterEntry{
		MembershipID: activeMembership, UserID: activeUser, Role: "recruiter", Status: "active",
	}
	dormant = tenantadmin.RosterEntry{
		MembershipID: dormantMembership, UserID: dormantUser, Role: "viewer", Status: "active",
	}
	removals = &revoker{}
	reviews = tenantadmin.NewAccessReviews(pool,
		&roster{entries: []tenantadmin.RosterEntry{active, dormant}}, removals)
	return reviews, tenantID, actorID, active, dormant, removals
}

// The prompt, materialised. One item per member, so the question is asked of
// each person by name rather than left to whoever reads a list.
func TestOpeningAReviewAsksAboutEveryMember(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, active, dormant, _ := workspace(t, now)

	review, err := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if review.Status != "open" {
		t.Errorf("Status = %q, want open", review.Status)
	}
	if want := now.AddDate(0, 0, 14); review.DueAt.Sub(want).Abs() > time.Minute {
		t.Errorf("DueAt = %v, want about %v", review.DueAt, want)
	}

	items, err := reviews.Items(ctx, tenantID, review.ID)
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Items returned %d, want one per member", len(items))
	}
	seen := map[string]tenantadmin.ReviewItem{}
	for _, item := range items {
		seen[item.MembershipID] = item
		if item.Decision != "pending" {
			t.Errorf("a fresh item is %q, want pending", item.Decision)
		}
	}
	if seen[active.MembershipID].Role != "recruiter" || seen[dormant.MembershipID].Role != "viewer" {
		t.Error("the items did not snapshot the roles the roster reported")
	}
}

// The second box, and the reason the review is worth opening: access nobody
// has used is surfaced without anybody having to ask for it.
func TestDormantAccessIsSurfacedWithoutBeingAskedFor(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, active, dormant, _ := workspace(t, now)

	review, err := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	items, err := reviews.Items(ctx, tenantID, review.ID)
	if err != nil {
		t.Fatalf("Items: %v", err)
	}

	for _, item := range items {
		switch item.MembershipID {
		case active.MembershipID:
			if item.Dormant {
				t.Error("somebody who acted yesterday was marked dormant")
			}
			if item.LastActiveAt == nil {
				t.Error("somebody who acted yesterday has no last-active time")
			}
		case dormant.MembershipID:
			if !item.Dormant {
				t.Error("somebody who has never acted here was not marked dormant")
			}
			if item.LastActiveAt != nil {
				t.Error("somebody who has never acted here has a last-active time")
			}
		}
	}
	// And dormant access is listed first, because the prompt exists to put it
	// in front of the person who can remove it.
	if !items[0].Dormant {
		t.Error("the first item is not the dormant one")
	}
}

// Activity older than the standard is dormancy, which is the case a threshold
// exists for at all.
func TestActivityOlderThanTheStandardCountsAsDormant(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, active, _, _ := workspace(t, now)
	acted(t, tenantID, active.UserID, now.AddDate(0, 0, -200))

	// Reviewed against a standard the yesterday-activity still clears, and
	// then against one it does not, so the difference is the standard rather
	// than the data.
	strict := tenantadmin.ReviewSchedule{EveryDays: 90, CompleteWithinDays: 14, DormantAfterDays: 1}
	review, err := reviews.Open(ctx, tenantID, actorID, strict, now.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	items, err := reviews.Items(ctx, tenantID, review.ID)
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	for _, item := range items {
		if !item.Dormant {
			t.Errorf("membership %s was not dormant under a one-day standard", item.MembershipID)
		}
	}
	if review.DormantAfterDays != 1 {
		t.Errorf("DormantAfterDays = %d, want the standard the review applied recorded on it",
			review.DormantAfterDays)
	}
}

// Two open reviews would split the roster between them and neither would be
// the answer to "has access been reviewed".
func TestAWorkspaceCanHaveOnlyOneOpenReview(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, _, _, _ := workspace(t, now)

	if _, err := reviews.Open(ctx, tenantID, actorID, schedule(), now); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := reviews.Open(ctx, tenantID, actorID, schedule(), now); !errors.Is(err, tenantadmin.ErrReviewOpen) {
		t.Fatalf("second Open = %v, want ErrReviewOpen", err)
	}
}

func TestConfirmingRecordsWhoDecidedAndWhy(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, _, _, _ := workspace(t, now)

	review, err := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	items, _ := reviews.Items(ctx, tenantID, review.ID)

	decided, err := reviews.Decide(ctx, tenantID, actorID, items[0].ID,
		tenantadmin.DecisionConfirmed, "still covering the ICU panel")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if decided.Decision != tenantadmin.DecisionConfirmed {
		t.Errorf("Decision = %q, want confirmed", decided.Decision)
	}
	if decided.DecidedBy != actorID {
		t.Errorf("DecidedBy = %s, want the reviewer", decided.DecidedBy)
	}
	if decided.DecidedAt == nil {
		t.Error("a decided item has no decision time")
	}
	if decided.Note != "still covering the ICU panel" {
		t.Errorf("Note = %q, want the reviewer's words", decided.Note)
	}
}

// The prompt's teeth: revoking through the review actually removes access
// rather than only recording an intention to.
func TestRevokingThroughTheReviewRemovesTheAccess(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, _, dormant, removals := workspace(t, now)

	review, err := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	items, _ := reviews.Items(ctx, tenantID, review.ID)

	var target string
	for _, item := range items {
		if item.MembershipID == dormant.MembershipID {
			target = item.ID
		}
	}
	if _, err := reviews.Decide(ctx, tenantID, actorID, target,
		tenantadmin.DecisionRevoked, "left the team in March"); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if len(removals.revoked) != 1 || removals.revoked[0] != dormant.MembershipID {
		t.Errorf("revoked = %v, want the dormant membership", removals.revoked)
	}
}

// If the revocation fails, the item must stay pending: a review that says
// "revoked" beside access that still works is worse than one that says
// nothing, because somebody will believe it.
func TestAFailedRevocationLeavesTheItemPending(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	tenantID := seedTenant(t)
	actorID := seedUser(t)
	membershipID, userID := seedMember(t, tenantID, "viewer")

	removals := &revoker{err: errors.New("membership service unavailable")}
	reviews := tenantadmin.NewAccessReviews(pool, &roster{entries: []tenantadmin.RosterEntry{{
		MembershipID: membershipID, UserID: userID, Role: "viewer", Status: "active",
	}}}, removals)

	review, err := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	items, _ := reviews.Items(ctx, tenantID, review.ID)

	if _, err := reviews.Decide(ctx, tenantID, actorID, items[0].ID,
		tenantadmin.DecisionRevoked, "left"); err == nil {
		t.Fatal("Decide succeeded although the revocation failed")
	}

	after, _ := reviews.Items(ctx, tenantID, review.ID)
	if after[0].Decision != "pending" {
		t.Errorf("Decision = %q, want pending after a failed revocation", after[0].Decision)
	}
}

func TestAnItemCanOnlyBeDecidedOnce(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, _, _, _ := workspace(t, now)

	review, _ := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	items, _ := reviews.Items(ctx, tenantID, review.ID)

	if _, err := reviews.Decide(ctx, tenantID, actorID, items[0].ID,
		tenantadmin.DecisionConfirmed, "keep"); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if _, err := reviews.Decide(ctx, tenantID, actorID, items[0].ID,
		tenantadmin.DecisionConfirmed, "keep again"); !errors.Is(err, tenantadmin.ErrReviewItemDecided) {
		t.Fatalf("second Decide = %v, want ErrReviewItemDecided", err)
	}
}

// The difference between a review and a report: it cannot be marked done
// while somebody in it has not been answered for.
func TestAReviewCannotBeCompletedWithAnItemStillPending(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, _, _, _ := workspace(t, now)

	review, _ := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	items, _ := reviews.Items(ctx, tenantID, review.ID)

	if _, err := reviews.Complete(ctx, tenantID, actorID, review.ID); !errors.Is(err, tenantadmin.ErrReviewIncomplete) {
		t.Fatalf("Complete = %v, want ErrReviewIncomplete", err)
	}
	for _, item := range items {
		if _, err := reviews.Decide(ctx, tenantID, actorID, item.ID,
			tenantadmin.DecisionConfirmed, "confirmed"); err != nil {
			t.Fatalf("Decide: %v", err)
		}
	}
	completed, err := reviews.Complete(ctx, tenantID, actorID, review.ID)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if completed.Status != "completed" || completed.CompletedAt == nil {
		t.Errorf("Status = %q, CompletedAt = %v; want a completed review", completed.Status, completed.CompletedAt)
	}
	if completed.CompletedBy != actorID {
		t.Errorf("CompletedBy = %s, want the reviewer", completed.CompletedBy)
	}
}

// The third box.
func TestCompletionIsAuditedWithWhatWasDecided(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, _, dormant, _ := workspace(t, now)

	review, _ := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	items, _ := reviews.Items(ctx, tenantID, review.ID)
	for _, item := range items {
		decision := tenantadmin.DecisionConfirmed
		if item.MembershipID == dormant.MembershipID {
			decision = tenantadmin.DecisionRevoked
		}
		if _, err := reviews.Decide(ctx, tenantID, actorID, item.ID, decision, "reviewed"); err != nil {
			t.Fatalf("Decide: %v", err)
		}
	}
	if _, err := reviews.Complete(ctx, tenantID, actorID, review.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	var auditActor, subject, detail string
	if err := admin(t).QueryRow(ctx, `
		SELECT actor_id::text, subject_id, detail::text FROM audit.events
		WHERE tenant_id = $1 AND action = 'tenant.access_review_completed'
		ORDER BY occurred_at DESC LIMIT 1`, tenantID).Scan(&auditActor, &subject, &detail); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if auditActor != actorID {
		t.Errorf("audit actor = %s, want the reviewer", auditActor)
	}
	if subject != review.ID {
		t.Errorf("audit subject = %s, want the review", subject)
	}
	for _, want := range []string{`"confirmed":1`, `"revoked":1`} {
		if !strings.Contains(strings.ReplaceAll(detail, " ", ""), want) {
			t.Errorf("the completion audit does not carry %s: %s", want, detail)
		}
	}
}

// Due-ness is the scheduled half. A workspace that has never reviewed is due
// now; one that just finished is not; one whose last review is older than the
// cadence is due again.
func TestAWorkspaceThatHasNeverReviewedIsDue(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, _, _, _, _ := workspace(t, now)

	due, since, err := reviews.Due(ctx, tenantID, schedule(), now)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if !due {
		t.Error("a workspace that has never reviewed access is not due for one")
	}
	if !since.IsZero() {
		t.Errorf("last reviewed = %v, want the zero time for never", since)
	}
}

func TestAWorkspaceThatJustReviewedIsNotDueUntilTheCadenceElapses(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, _, _, _ := workspace(t, now)

	review, _ := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	items, _ := reviews.Items(ctx, tenantID, review.ID)
	for _, item := range items {
		if _, err := reviews.Decide(ctx, tenantID, actorID, item.ID,
			tenantadmin.DecisionConfirmed, "confirmed"); err != nil {
			t.Fatalf("Decide: %v", err)
		}
	}
	if _, err := reviews.Complete(ctx, tenantID, actorID, review.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	due, _, err := reviews.Due(ctx, tenantID, schedule(), time.Now())
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if due {
		t.Error("a workspace that has just reviewed access is due for another already")
	}

	due, since, err := reviews.Due(ctx, tenantID, schedule(), time.Now().AddDate(0, 0, 91))
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if !due {
		t.Error("a workspace whose review is older than the cadence is not due for another")
	}
	if since.IsZero() {
		t.Error("a workspace that has reviewed reports no last review")
	}
}

// The attack that means something: items that genuinely exist under another
// workspace, named directly from inside this one's scope.
func TestOneWorkspaceCannotSeeAnothersAccessReview(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, victimTenant, actorID, _, _, _ := workspace(t, now)
	attackerTenant := seedTenant(t)

	review, err := reviews.Open(ctx, victimTenant, actorID, schedule(), now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	victimItems, err := reviews.Items(ctx, victimTenant, review.ID)
	if err != nil || len(victimItems) == 0 {
		t.Fatalf("the rows under attack must exist: %d items, err %v", len(victimItems), err)
	}

	// Through the store, naming the victim's review by id.
	stolen, err := reviews.Items(ctx, attackerTenant, review.ID)
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(stolen) != 0 {
		t.Errorf("an administrator of one workspace read %d of another's review items", len(stolen))
	}

	// And directly, so a future change to the query cannot make this pass by
	// filtering in Go what the policy was supposed to filter in the database.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, attackerTenant); err != nil {
		t.Fatalf("scoping: %v", err)
	}
	var reviewsSeen, itemsSeen int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM tenancy.access_reviews WHERE tenant_id = $1`, victimTenant).Scan(&reviewsSeen); err != nil {
		t.Fatalf("counting reviews: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM tenancy.access_review_items WHERE review_id = $1`, review.ID).Scan(&itemsSeen); err != nil {
		t.Fatalf("counting items: %v", err)
	}
	if reviewsSeen != 0 || itemsSeen != 0 {
		t.Errorf("saw %d reviews and %d items belonging to another workspace", reviewsSeen, itemsSeen)
	}
}

// Deciding another workspace's item must change nothing, which is a different
// failure from not being able to see it.
func TestOneWorkspaceCannotDecideAnothersReviewItem(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, victimTenant, actorID, _, _, _ := workspace(t, now)
	attackerTenant := seedTenant(t)

	review, _ := reviews.Open(ctx, victimTenant, actorID, schedule(), now)
	items, _ := reviews.Items(ctx, victimTenant, review.ID)

	if _, err := reviews.Decide(ctx, attackerTenant, actorID, items[0].ID,
		tenantadmin.DecisionConfirmed, "not mine"); !errors.Is(err, tenantadmin.ErrReviewItemNotFound) {
		t.Fatalf("cross-tenant Decide = %v, want ErrReviewItemNotFound", err)
	}

	after, _ := reviews.Items(ctx, victimTenant, review.ID)
	for _, item := range after {
		if item.Decision != "pending" {
			t.Errorf("another workspace decided item %s", item.ID)
		}
	}
}

// A review that can be removed answers "has access been reviewed" with
// whatever the last person to look wanted it to say.
func TestTheApplicationRoleCannotDeleteAReview(t *testing.T) {
	ctx := context.Background()
	for _, table := range []string{"tenancy.access_reviews", "tenancy.access_review_items"} {
		var granted bool
		if err := admin(t).QueryRow(ctx,
			`SELECT has_table_privilege('prepeet_app', $1, 'DELETE')`, table).Scan(&granted); err != nil {
			t.Fatalf("checking %s: %v", table, err)
		}
		if granted {
			t.Errorf("prepeet_app holds DELETE on %s", table)
		}
	}
}

// The prompt an administrator opening the workspace is shown: whichever
// review is outstanding, or nothing when there is none.
func TestCurrentAnswersTheOpenReviewAndNothingWhenThereIsNone(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, _, _, _ := workspace(t, now)

	if _, err := reviews.Current(ctx, tenantID); !errors.Is(err, tenantadmin.ErrReviewNotFound) {
		t.Fatalf("Current before any review = %v, want ErrReviewNotFound", err)
	}

	opened, err := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	current, err := reviews.Current(ctx, tenantID)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if current.ID != opened.ID {
		t.Errorf("Current = %s, want the review that was opened (%s)", current.ID, opened.ID)
	}

	// And once it is answered and closed, there is nothing outstanding again.
	items, _ := reviews.Items(ctx, tenantID, opened.ID)
	for _, item := range items {
		if _, err := reviews.Decide(ctx, tenantID, actorID, item.ID,
			tenantadmin.DecisionConfirmed, "confirmed"); err != nil {
			t.Fatalf("Decide: %v", err)
		}
	}
	if _, err := reviews.Complete(ctx, tenantID, actorID, opened.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, err := reviews.Current(ctx, tenantID); !errors.Is(err, tenantadmin.ErrReviewNotFound) {
		t.Fatalf("Current after completion = %v, want ErrReviewNotFound", err)
	}
}

// Reading another workspace's review by id must be absence rather than
// refusal, which is what the policy actually makes true.
func TestGettingAnotherWorkspacesReviewIsAbsence(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, victimTenant, actorID, _, _, _ := workspace(t, now)
	attackerTenant := seedTenant(t)

	review, err := reviews.Open(ctx, victimTenant, actorID, schedule(), now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := reviews.Get(ctx, victimTenant, review.ID); err != nil {
		t.Fatalf("the review under attack must exist: %v", err)
	}
	if _, err := reviews.Get(ctx, attackerTenant, review.ID); !errors.Is(err, tenantadmin.ErrReviewNotFound) {
		t.Fatalf("cross-tenant Get = %v, want ErrReviewNotFound", err)
	}
}

// A decision that is neither confirm nor revoke is refused before anything is
// asked of the membership side.
func TestAReviewDecisionMustBeConfirmOrRevoke(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, _, _, removals := workspace(t, now)

	review, _ := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	items, _ := reviews.Items(ctx, tenantID, review.ID)

	if _, err := reviews.Decide(ctx, tenantID, actorID, items[0].ID, "maybe", ""); !errors.Is(
		err, tenantadmin.ErrReviewDecisionInvalid) {
		t.Fatalf("Decide = %v, want ErrReviewDecisionInvalid", err)
	}
	if len(removals.revoked) != 0 {
		t.Error("an invalid decision reached the membership side")
	}
}

// A revoked membership is not reviewed: there is nothing left to confirm or
// remove, and listing it would make every review longer than it needs to be.
func TestARevokedMembershipIsNotPutInTheReview(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	tenantID := seedTenant(t)
	actorID := seedUser(t)
	activeMembership, activeUser := seedMember(t, tenantID, "recruiter")
	goneMembership, goneUser := seedMember(t, tenantID, "viewer")

	reviews := tenantadmin.NewAccessReviews(pool, &roster{entries: []tenantadmin.RosterEntry{
		{MembershipID: activeMembership, UserID: activeUser, Role: "recruiter", Status: "active"},
		{MembershipID: goneMembership, UserID: goneUser, Role: "viewer", Status: "revoked"},
	}}, &revoker{})

	review, err := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	items, err := reviews.Items(ctx, tenantID, review.ID)
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(items) != 1 || items[0].MembershipID != activeMembership {
		t.Errorf("the review holds %d items, want only the membership that still has access", len(items))
	}
}

// Completing a review that was already completed changes nothing, which is
// the shape a retry needs.
func TestCompletingAClosedReviewIsRefused(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	reviews, tenantID, actorID, _, _, _ := workspace(t, now)

	review, _ := reviews.Open(ctx, tenantID, actorID, schedule(), now)
	items, _ := reviews.Items(ctx, tenantID, review.ID)
	for _, item := range items {
		if _, err := reviews.Decide(ctx, tenantID, actorID, item.ID,
			tenantadmin.DecisionConfirmed, "confirmed"); err != nil {
			t.Fatalf("Decide: %v", err)
		}
	}
	if _, err := reviews.Complete(ctx, tenantID, actorID, review.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, err := reviews.Complete(ctx, tenantID, actorID, review.ID); !errors.Is(
		err, tenantadmin.ErrReviewNotFound) {
		t.Fatalf("second Complete = %v, want a refusal", err)
	}
}

// Settings and reviews are read through a workspace, so asking without one
// has to fail closed rather than answer for everybody.
func TestReadingWithNoWorkspaceIsRefused(t *testing.T) {
	ctx := context.Background()
	store := tenantadmin.NewSettingsStore(pool)
	if _, err := store.Current(ctx, ""); err == nil {
		t.Error("reading settings with no workspace succeeded")
	}
	reviews := tenantadmin.NewAccessReviews(pool, &roster{}, &revoker{})
	if _, err := reviews.Current(ctx, ""); err == nil {
		t.Error("reading a review with no workspace succeeded")
	}
}
