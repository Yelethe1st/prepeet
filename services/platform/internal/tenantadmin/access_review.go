package tenantadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// The periodic access review: TEN-03.
//
// The ticket's first line decides the design. A report is a query somebody
// could run; a review is a row that exists, is due on a date, names every
// person who can reach candidate evidence, and cannot be closed until each of
// them has been confirmed or revoked. Everything here follows from wanting
// the second thing rather than the first.
//
// The items are a snapshot rather than a live view. A completed review is a
// record of decisions taken against stated facts - who held what role, when
// they were last seen here - and recomputing those at read time would mean a
// finished review no longer shows what was confirmed.

// The decisions a reviewer may record.
const (
	// DecisionConfirmed means the access is still needed.
	DecisionConfirmed = "confirmed"
	// DecisionRevoked means it is not, and the review removes it.
	DecisionRevoked = "revoked"
)

// Access review refusals.
var (
	// ErrReviewOpen means one is already open. Two would split the roster
	// between them and neither would answer "has access been reviewed".
	ErrReviewOpen = errors.New("tenantadmin: ACCESS_REVIEW_OPEN: a review is already open for this workspace")
	// ErrReviewNotFound covers absence and another workspace's review alike.
	ErrReviewNotFound = errors.New("tenantadmin: ACCESS_REVIEW_NOT_FOUND: no such review")
	// ErrReviewItemNotFound covers absence and another workspace's item alike.
	ErrReviewItemNotFound = errors.New("tenantadmin: ACCESS_REVIEW_ITEM_NOT_FOUND: no such review item")
	// ErrReviewItemDecided means somebody answered for this person first.
	ErrReviewItemDecided = errors.New("tenantadmin: ACCESS_REVIEW_ITEM_DECIDED: that access has already been reviewed")
	// ErrReviewIncomplete is the difference between a review and a report: it
	// cannot be marked done while somebody in it is unanswered for.
	ErrReviewIncomplete = errors.New("tenantadmin: ACCESS_REVIEW_INCOMPLETE: every person in the review needs a decision")
	// ErrReviewDecisionInvalid rejects anything that is not confirm or revoke.
	ErrReviewDecisionInvalid = errors.New("tenantadmin: ACCESS_REVIEW_DECISION_INVALID: a review decision is confirm or revoke")
)

// RosterEntry is one person's access, as the context that owns memberships
// reports it.
type RosterEntry struct {
	MembershipID string
	UserID       string
	Role         string
	// Status is the membership's own: a revoked membership is not reviewed,
	// because there is nothing left to confirm or remove.
	Status string
}

// Roster is what opening a review needs from the context that owns
// memberships.
//
// Declared here and implemented elsewhere, per ADR-0005: who belongs to a
// workspace is identity's answer, and a review that queried memberships
// directly would make these two one module with a directory between them.
type Roster interface {
	List(ctx context.Context, tenantID string) ([]RosterEntry, error)
}

// Revoker removes a member's access.
//
// The review needs this rather than only a column, because "revoked" written
// beside access that still works is worse than writing nothing: somebody will
// read it and believe the access is gone.
type Revoker interface {
	Revoke(ctx context.Context, tenantID, actorID, membershipID string) error
}

// ReviewSchedule is the standard a review is opened under.
type ReviewSchedule struct {
	// EveryDays is how often access is reviewed.
	EveryDays int
	// CompleteWithinDays is how long the reviewer has, which becomes the
	// review's due date.
	CompleteWithinDays int
	// DormantAfterDays is how long unused access has to sit before the
	// review calls it dormant.
	DormantAfterDays int
}

// DefaultReviewSchedule is quarterly, with a fortnight to finish and ninety
// days of silence counting as dormant.
//
// Quarterly because access review is a control that decays if it is a chore:
// a monthly one is skipped and an annual one is a formality. The numbers are
// a starting point a workspace can be given its own, not a law.
func DefaultReviewSchedule() ReviewSchedule {
	return ReviewSchedule{EveryDays: 90, CompleteWithinDays: 14, DormantAfterDays: 90}
}

// Review is one periodic review of who can reach candidate evidence.
type Review struct {
	ID       string
	Status   string
	OpenedAt time.Time
	OpenedBy string
	DueAt    time.Time
	// DormantAfterDays is the standard this review applied, carried on the
	// review because "nobody was dormant" means different things against
	// different standards.
	DormantAfterDays int
	CompletedAt      *time.Time
	CompletedBy      string
}

// ReviewItem is one person's access as it stood when the review opened, and
// what the reviewer decided about it.
type ReviewItem struct {
	ID           string
	ReviewID     string
	MembershipID string
	UserID       string
	Role         string
	// LastActiveAt is nil for somebody the workspace has never recorded
	// doing anything, which is the strongest form of dormant.
	LastActiveAt *time.Time
	Dormant      bool
	Decision     string
	DecidedAt    *time.Time
	DecidedBy    string
	Note         string
}

// AccessReviews opens, answers and closes a workspace's access reviews.
type AccessReviews struct {
	pool    *pgxpool.Pool
	roster  Roster
	revoker Revoker
}

// NewAccessReviews wires the review over its two ports.
func NewAccessReviews(pool *pgxpool.Pool, roster Roster, revoker Revoker) *AccessReviews {
	return &AccessReviews{pool: pool, roster: roster, revoker: revoker}
}

// Due reports whether a workspace is due an access review, and when it last
// completed one.
//
// A workspace that has never completed one is due immediately, which is the
// absence of a row rather than a special case: the first review is the one
// most worth having.
func (a *AccessReviews) Due(ctx context.Context, tenantID string, schedule ReviewSchedule, now time.Time) (bool, time.Time, error) {
	tx, err := a.begin(ctx, tenantID)
	if err != nil {
		return false, time.Time{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := db.New(tx).LatestCompletedAccessReview(ctx, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, fmt.Errorf("tenantadmin: reading the last review: %w", err)
	}
	if row.CompletedAt == nil {
		return true, time.Time{}, nil
	}
	last := *row.CompletedAt
	return !now.Before(last.AddDate(0, 0, schedule.EveryDays)), last, nil
}

// Open starts a review and materialises one item per member.
//
// The roster arrives through the port; the dormancy standard is applied here
// and recorded on the review. Opening is what turns "somebody should check
// this" into a row with a due date that a workspace can be shown it is late
// on.
func (a *AccessReviews) Open(ctx context.Context, tenantID, actorID string, schedule ReviewSchedule, now time.Time) (Review, error) {
	people, err := a.roster.List(ctx, tenantID)
	if err != nil {
		return Review{}, fmt.Errorf("tenantadmin: reading the roster: %w", err)
	}

	tx, err := a.begin(ctx, tenantID)
	if err != nil {
		return Review{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	activity, err := q.LastActivityByActor(ctx, tenantID)
	if err != nil {
		return Review{}, fmt.Errorf("tenantadmin: reading activity: %w", err)
	}
	lastSeen := make(map[string]time.Time, len(activity))
	for _, row := range activity {
		lastSeen[row.ActorID] = row.LastActiveAt
	}

	reviewID := id.New().String()
	if err := q.InsertAccessReview(ctx, db.InsertAccessReviewParams{
		ID: reviewID, TenantID: tenantID, OpenedBy: actorID,
		DueAt:            now.AddDate(0, 0, schedule.CompleteWithinDays),
		DormantAfterDays: int32(schedule.DormantAfterDays),
	}); err != nil {
		if isUniqueViolation(err) {
			return Review{}, ErrReviewOpen
		}
		return Review{}, fmt.Errorf("tenantadmin: opening the review: %w", err)
	}

	dormantBefore := now.AddDate(0, 0, -schedule.DormantAfterDays)
	for _, person := range people {
		if person.Status == "revoked" {
			continue
		}
		var lastActive *time.Time
		dormant := true
		if seen, ok := lastSeen[person.UserID]; ok {
			lastActive = &seen
			dormant = seen.Before(dormantBefore)
		}
		if err := q.InsertAccessReviewItem(ctx, db.InsertAccessReviewItemParams{
			ID: id.New().String(), TenantID: tenantID, ReviewID: reviewID,
			MembershipID: person.MembershipID, UserID: person.UserID, Role: person.Role,
			LastActiveAt: lastActive, Dormant: dormant,
		}); err != nil {
			return Review{}, fmt.Errorf("tenantadmin: adding %s to the review: %w", person.MembershipID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Review{}, err
	}
	return a.Get(ctx, tenantID, reviewID)
}

// Get reads one review.
func (a *AccessReviews) Get(ctx context.Context, tenantID, reviewID string) (Review, error) {
	tx, err := a.begin(ctx, tenantID)
	if err != nil {
		return Review{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := db.New(tx).GetAccessReview(ctx, db.GetAccessReviewParams{
		TenantID: tenantID, ID: reviewID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Review{}, ErrReviewNotFound
	}
	if err != nil {
		return Review{}, fmt.Errorf("tenantadmin: reading the review: %w", err)
	}
	return Review{
		ID: row.ID, Status: row.Status, OpenedAt: row.OpenedAt, OpenedBy: row.OpenedBy,
		DueAt: row.DueAt, DormantAfterDays: int(row.DormantAfterDays),
		CompletedAt: row.CompletedAt, CompletedBy: row.CompletedBy,
	}, nil
}

// Current answers the open review, if there is one.
func (a *AccessReviews) Current(ctx context.Context, tenantID string) (Review, error) {
	tx, err := a.begin(ctx, tenantID)
	if err != nil {
		return Review{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := db.New(tx).FindOpenAccessReview(ctx, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Review{}, ErrReviewNotFound
	}
	if err != nil {
		return Review{}, fmt.Errorf("tenantadmin: reading the open review: %w", err)
	}
	return Review{
		ID: row.ID, Status: row.Status, OpenedAt: row.OpenedAt, OpenedBy: row.OpenedBy,
		DueAt: row.DueAt, DormantAfterDays: int(row.DormantAfterDays),
		CompletedAt: row.CompletedAt, CompletedBy: row.CompletedBy,
	}, nil
}

// Items answers everyone in a review, dormant access first.
func (a *AccessReviews) Items(ctx context.Context, tenantID, reviewID string) ([]ReviewItem, error) {
	tx, err := a.begin(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := db.New(tx).ListAccessReviewItems(ctx, db.ListAccessReviewItemsParams{
		TenantID: tenantID, ReviewID: reviewID,
	})
	if err != nil {
		return nil, fmt.Errorf("tenantadmin: reading review items: %w", err)
	}
	items := make([]ReviewItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, ReviewItem{
			ID: row.ID, ReviewID: row.ReviewID, MembershipID: row.MembershipID,
			UserID: row.UserID, Role: row.Role, LastActiveAt: row.LastActiveAt,
			Dormant: row.Dormant, Decision: row.Decision, DecidedAt: row.DecidedAt,
			DecidedBy: row.DecidedBy, Note: row.Note,
		})
	}
	return items, nil
}

// Decide records the outcome for one person's access.
//
// A revocation removes the access before the row is written, and the row is
// only written if the removal succeeded. The order is deliberate: a review
// that says "revoked" beside access that still works is a lie somebody will
// act on, whereas an item left pending is a question still to answer.
func (a *AccessReviews) Decide(ctx context.Context, tenantID, actorID, itemID, decision, note string) (ReviewItem, error) {
	if decision != DecisionConfirmed && decision != DecisionRevoked {
		return ReviewItem{}, ErrReviewDecisionInvalid
	}

	item, err := a.item(ctx, tenantID, itemID)
	if err != nil {
		return ReviewItem{}, err
	}
	if item.Decision != "pending" {
		return ReviewItem{}, ErrReviewItemDecided
	}

	if decision == DecisionRevoked {
		if err := a.revoker.Revoke(ctx, tenantID, actorID, item.MembershipID); err != nil {
			return ReviewItem{}, fmt.Errorf("tenantadmin: revoking %s: %w", item.MembershipID, err)
		}
	}

	tx, err := a.begin(ctx, tenantID)
	if err != nil {
		return ReviewItem{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	decided, err := db.New(tx).DecideAccessReviewItem(ctx, db.DecideAccessReviewItemParams{
		TenantID: tenantID, ID: itemID, Decision: decision, DecidedBy: actorID, Note: note,
	})
	if err != nil {
		return ReviewItem{}, fmt.Errorf("tenantadmin: recording the decision: %w", err)
	}
	// Guarded on still being pending, so a second reviewer answering the same
	// person loses on rows affected rather than overwriting the first answer.
	if decided == 0 {
		return ReviewItem{}, ErrReviewItemDecided
	}
	if err := tx.Commit(ctx); err != nil {
		return ReviewItem{}, err
	}
	return a.item(ctx, tenantID, itemID)
}

// Complete closes a review, and refuses while anybody in it is unanswered.
//
// The audit row carries the counts rather than the names: who was confirmed
// and who was revoked is in the items, which are still there, and the audit
// trail's job here is to record that the control ran and who ran it.
func (a *AccessReviews) Complete(ctx context.Context, tenantID, actorID, reviewID string) (Review, error) {
	tx, err := a.begin(ctx, tenantID)
	if err != nil {
		return Review{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	pending, err := q.CountPendingAccessReviewItems(ctx, db.CountPendingAccessReviewItemsParams{
		TenantID: tenantID, ReviewID: reviewID,
	})
	if err != nil {
		return Review{}, fmt.Errorf("tenantadmin: counting what is unanswered: %w", err)
	}
	if pending > 0 {
		return Review{}, ErrReviewIncomplete
	}

	items, err := q.ListAccessReviewItems(ctx, db.ListAccessReviewItemsParams{
		TenantID: tenantID, ReviewID: reviewID,
	})
	if err != nil {
		return Review{}, fmt.Errorf("tenantadmin: reading what was decided: %w", err)
	}
	counts := map[string]int{DecisionConfirmed: 0, DecisionRevoked: 0}
	for _, item := range items {
		counts[item.Decision]++
	}

	closed, err := q.CompleteAccessReview(ctx, db.CompleteAccessReviewParams{
		TenantID: tenantID, ID: reviewID, CompletedBy: actorID,
	})
	if err != nil {
		return Review{}, fmt.Errorf("tenantadmin: completing the review: %w", err)
	}
	if closed == 0 {
		return Review{}, ErrReviewNotFound
	}

	detail, err := json.Marshal(counts)
	if err != nil {
		return Review{}, fmt.Errorf("tenantadmin: encoding the audit detail: %w", err)
	}
	if err := q.InsertTenantAuditEvent(ctx, db.InsertTenantAuditEventParams{
		ID: id.New().String(), TenantID: tenantID, ActorID: actorID,
		Action: "tenant.access_review_completed", SubjectType: "access_review",
		SubjectID: reviewID, Detail: detail,
	}); err != nil {
		return Review{}, fmt.Errorf("tenantadmin: auditing the completion: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Review{}, err
	}
	return a.Get(ctx, tenantID, reviewID)
}

// item reads one review item by id, inside the workspace's scope. Another
// workspace's item is absent rather than forbidden, which is what the policy
// makes true and what a caller can safely be told.
func (a *AccessReviews) item(ctx context.Context, tenantID, itemID string) (ReviewItem, error) {
	tx, err := a.begin(ctx, tenantID)
	if err != nil {
		return ReviewItem{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := db.New(tx).GetAccessReviewItem(ctx, db.GetAccessReviewItemParams{
		TenantID: tenantID, ID: itemID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewItem{}, ErrReviewItemNotFound
	}
	if err != nil {
		return ReviewItem{}, fmt.Errorf("tenantadmin: reading the review item: %w", err)
	}
	return ReviewItem{
		ID: row.ID, ReviewID: row.ReviewID, MembershipID: row.MembershipID,
		UserID: row.UserID, Role: row.Role, LastActiveAt: row.LastActiveAt,
		Dormant: row.Dormant, Decision: row.Decision, DecidedAt: row.DecidedAt,
		DecidedBy: row.DecidedBy, Note: row.Note,
	}, nil
}

func (a *AccessReviews) begin(ctx context.Context, tenantID string) (pgx.Tx, error) {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("tenantadmin: beginning: %w", err)
	}
	if err := database.SetTenant(ctx, tx, tenantID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
