package recruiting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/recruiting/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Re-review: REV-06, to responsible-hiring.md's appeals section.
//
// An appeal is raised against a decision that exists, freezes what that
// decision was informed by at the moment of raising, and is answered by
// somebody other than the person whose decision is under appeal. The
// independence rule lives in the schema as well as here, the resolution is
// write-once by trigger, and the outcome vocabulary is closed: upheld or
// revised, where a revision is a NEW decision recorded through REV-03's
// append-only path rather than an edit of anything.

// ReReviewSLA is how long an appeal may stand unanswered: the platform's
// provisional default, stated in one place so it becomes a versioned
// policy the moment DEC-14's screening policy decision lands, exactly as
// the reconnect grace did.
const ReReviewSLA = 7 * 24 * time.Hour

// The outcome vocabulary.
const (
	ReReviewUpheld  = "upheld"
	ReReviewRevised = "revised"
)

var validOutcomes = map[string]bool{ReReviewUpheld: true, ReReviewRevised: true}

// The refusals an appeal can meet.
var (
	// ErrAppealNoDecision means there is nothing decided to appeal.
	ErrAppealNoDecision = errors.New("recruiting: no decision exists to appeal")
	// ErrAppealReasonRequired is the reason rule, same as every decision's.
	ErrAppealReasonRequired = errors.New("recruiting: an appeal requires a reason")
	// ErrAppealSelfReview means the assignment or resolution names the
	// original reviewer: the one person who cannot answer it.
	ErrAppealSelfReview = errors.New("recruiting: the original reviewer cannot re-review their own decision")
	// ErrAppealUnknown covers an appeal that does not exist and one on
	// another campaign alike.
	ErrAppealUnknown = errors.New("recruiting: no such re-review on this campaign")
	// ErrAppealResolved means the appeal already has its answer.
	ErrAppealResolved = errors.New("recruiting: this re-review is already resolved")
	// ErrAppealResolverNotAssigned means somebody other than the assigned
	// reviewer tried to answer.
	ErrAppealResolverNotAssigned = errors.New("recruiting: only the assigned reviewer resolves a re-review")
	// ErrAppealOutcomeInvalid means the outcome is outside the vocabulary,
	// or the rationale or disclosure is missing: a resolution is whole or
	// it is not recorded.
	ErrAppealOutcomeInvalid = errors.New("recruiting: a resolution needs a known outcome, its rationale and the permitted disclosure")
)

// Resolution is an answered appeal's ending, present whole or not at all.
type Resolution struct {
	Outcome    string
	Rationale  string
	Disclosure string
	ResolvedBy string
	ResolvedAt time.Time
}

// ReReview is one appeal as the queue reads it.
type ReReview struct {
	ID               string
	SessionID        string
	RequestedBy      string
	Reason           string
	AppealedDecision string
	OriginalReviewer string
	Frozen           EvidenceVersion
	BundleDigest     string
	AssignedTo       string
	RaisedAt         time.Time
	DueAt            time.Time
	// Resolution is nil while the appeal is open.
	Resolution *Resolution
}

// RaiseReReview opens an appeal against the session's latest decision,
// freezing that decision's evidence version and the bundle the session
// ran. The bundle digest comes from the caller because the session is the
// interview context's; everything else is this context's own record.
func (s *Store) RaiseReReview(ctx context.Context, tenantID, campaignID, sessionID, requestedBy, reason, bundleDigest string) (ReReview, error) {
	if strings.TrimSpace(reason) == "" {
		return ReReview{}, ErrAppealReasonRequired
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReReview{}, fmt.Errorf("recruiting: beginning re-review: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return ReReview{}, err
	}
	queries := db.New(tx)

	decision, err := queries.LatestReviewDecision(ctx, db.LatestReviewDecisionParams{
		SessionID: sessionID, CampaignID: campaignID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReReview{}, ErrAppealNoDecision
	}
	if err != nil {
		return ReReview{}, fmt.Errorf("recruiting: reading the decision under appeal: %w", err)
	}

	row, err := queries.RaiseReReview(ctx, db.RaiseReReviewParams{
		ID: id.New().String(), CampaignID: campaignID, TenantID: tenantID,
		SessionID: sessionID, RequestedBy: requestedBy, Reason: reason,
		AppealedDecision: decision.ID, OriginalReviewer: decision.DecidedBy,
		EvaluationID: decision.EvaluationID, ResultDigest: decision.ResultDigest,
		RubricDigest: decision.RubricDigest, BundleDigest: bundleDigest,
		DueAt: time.Now().Add(ReReviewSLA),
	})
	if err != nil {
		return ReReview{}, fmt.Errorf("recruiting: raising the re-review: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReReview{}, fmt.Errorf("recruiting: committing the re-review: %w", err)
	}
	return reReviewFrom(reReviewRow(row)), nil
}

// AssignReReview names who answers an open appeal. The original reviewer
// is refused here and by the schema's own CHECK, so no code path can seat
// them.
func (s *Store) AssignReReview(ctx context.Context, tenantID, campaignID, appealID, assignee string) (ReReview, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReReview{}, fmt.Errorf("recruiting: beginning assignment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return ReReview{}, err
	}
	queries := db.New(tx)

	current, err := queries.ReReviewByID(ctx, db.ReReviewByIDParams{ID: appealID, CampaignID: campaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReReview{}, ErrAppealUnknown
	}
	if err != nil {
		return ReReview{}, fmt.Errorf("recruiting: reading the re-review: %w", err)
	}
	if current.Outcome.Valid {
		return ReReview{}, ErrAppealResolved
	}
	if current.OriginalReviewer == assignee {
		return ReReview{}, ErrAppealSelfReview
	}

	moved, err := queries.AssignReReview(ctx, db.AssignReReviewParams{
		ID: appealID, CampaignID: campaignID, AssignedTo: assignee,
	})
	if err != nil {
		return ReReview{}, fmt.Errorf("recruiting: assigning the re-review: %w", err)
	}
	if moved == 0 {
		return ReReview{}, ErrAppealResolved
	}
	assigned, err := queries.ReReviewByID(ctx, db.ReReviewByIDParams{ID: appealID, CampaignID: campaignID})
	if err != nil {
		return ReReview{}, fmt.Errorf("recruiting: reading back the assignment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReReview{}, fmt.Errorf("recruiting: committing the assignment: %w", err)
	}
	return reReviewFrom(assigned), nil
}

// ResolveReReview answers an open appeal, once, as the assigned reviewer.
// The resolution is whole - outcome, rationale and the disclosure the
// candidate is permitted - or it is not recorded.
func (s *Store) ResolveReReview(ctx context.Context, tenantID, campaignID, appealID, resolvedBy, outcome, rationale, disclosure string) (ReReview, error) {
	if !validOutcomes[outcome] || strings.TrimSpace(rationale) == "" || strings.TrimSpace(disclosure) == "" {
		return ReReview{}, ErrAppealOutcomeInvalid
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReReview{}, fmt.Errorf("recruiting: beginning resolution: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return ReReview{}, err
	}
	queries := db.New(tx)

	current, err := queries.ReReviewByID(ctx, db.ReReviewByIDParams{ID: appealID, CampaignID: campaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReReview{}, ErrAppealUnknown
	}
	if err != nil {
		return ReReview{}, fmt.Errorf("recruiting: reading the re-review: %w", err)
	}
	if current.Outcome.Valid {
		return ReReview{}, ErrAppealResolved
	}
	if current.OriginalReviewer == resolvedBy {
		return ReReview{}, ErrAppealSelfReview
	}
	if current.AssignedTo == nil || *current.AssignedTo != resolvedBy {
		return ReReview{}, ErrAppealResolverNotAssigned
	}

	moved, err := queries.ResolveReReview(ctx, db.ResolveReReviewParams{
		ID: appealID, CampaignID: campaignID, Outcome: outcome,
		Rationale: rationale, Disclosure: disclosure, ResolvedBy: resolvedBy,
	})
	if err != nil {
		return ReReview{}, fmt.Errorf("recruiting: resolving the re-review: %w", err)
	}
	if moved == 0 {
		return ReReview{}, ErrAppealResolved
	}
	resolved, err := queries.ReReviewByID(ctx, db.ReReviewByIDParams{ID: appealID, CampaignID: campaignID})
	if err != nil {
		return ReReview{}, fmt.Errorf("recruiting: reading back the resolution: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReReview{}, fmt.Errorf("recruiting: committing the resolution: %w", err)
	}
	return reReviewFrom(resolved), nil
}

// ReReviewsForSession answers every appeal on the session, oldest first.
func (s *Store) ReReviewsForSession(ctx context.Context, tenantID, campaignID, sessionID string) ([]ReReview, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recruiting: beginning re-review list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).ReReviewsForSession(ctx, db.ReReviewsForSessionParams{
		SessionID: sessionID, CampaignID: campaignID,
	})
	if err != nil {
		return nil, fmt.Errorf("recruiting: listing re-reviews: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("recruiting: committing the re-review list: %w", err)
	}
	out := make([]ReReview, 0, len(rows))
	for _, row := range rows {
		out = append(out, reReviewFrom(db.ReReviewByIDRow(row)))
	}
	return out, nil
}

// reReviewRow converts the insert's returning row onto the read shape.
func reReviewRow(row db.RaiseReReviewRow) db.ReReviewByIDRow {
	return db.ReReviewByIDRow(row)
}

func reReviewFrom(row db.ReReviewByIDRow) ReReview {
	appeal := ReReview{
		ID: row.ID, SessionID: row.SessionID, RequestedBy: row.RequestedBy,
		Reason: row.Reason, AppealedDecision: row.AppealedDecision,
		OriginalReviewer: row.OriginalReviewer,
		Frozen: EvidenceVersion{
			EvaluationID: row.EvaluationID,
			ResultDigest: row.ResultDigest,
			RubricDigest: row.RubricDigest,
		},
		BundleDigest: row.BundleDigest,
		AssignedTo:   orBlank(row.AssignedTo),
		RaisedAt:     row.RaisedAt, DueAt: row.DueAt,
	}
	if row.Outcome.Valid {
		resolution := Resolution{
			Outcome:    row.Outcome.String,
			Rationale:  row.OutcomeRationale.String,
			Disclosure: row.CandidateDisclosure.String,
			ResolvedBy: orBlank(row.ResolvedBy),
		}
		if row.ResolvedAt != nil {
			resolution.ResolvedAt = *row.ResolvedAt
		}
		appeal.Resolution = &resolution
	}
	return appeal
}

// orBlank renders a nullable column as its value, empty when null.
func orBlank(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
