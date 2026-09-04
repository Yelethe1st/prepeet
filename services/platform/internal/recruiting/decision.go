package recruiting

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"context"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/recruiting/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// The hiring decision: REV-03, to responsible-hiring.md.
//
// Nothing in this product advances or rejects a candidate automatically. A
// decision is a named human's, with a required reason, recorded against
// the evidence version it was informed by, and the history is append-only:
// changing a decision is a new row, and every earlier one keeps its true
// actor. Where the reviewer disagreed with an assessed band, the band they
// disagreed with and their rationale travel on the same row, because a
// disagreement without its reasoning is indistinguishable from a whim.

// The decision vocabulary is the event catalogue's, verbatim.
const (
	DecisionAdvance = "advance"
	DecisionReject  = "reject"
	DecisionHold    = "hold"
)

var validDecisions = map[string]bool{
	DecisionAdvance: true, DecisionReject: true, DecisionHold: true,
}

// The refusals a decision can meet, each its own sentinel because the
// reviewer's next step differs.
var (
	ErrDecisionUnknown        = errors.New("recruiting: not a known decision")
	ErrDecisionReasonRequired = errors.New("recruiting: a decision requires a reason")
	ErrDecisionActorRequired  = errors.New("recruiting: a decision requires the person who made it")
	// ErrOverrideIncomplete means an override is missing its rationale, the
	// band it sets, or the competency it concerns: a disagreement that
	// cannot say what it disagrees with, or why, is refused whole.
	ErrOverrideIncomplete = errors.New("recruiting: an override requires its competency, band and rationale")
)

// BandOverride is one recorded disagreement with an assessed band.
type BandOverride struct {
	CompetencyID string `json:"competency_id"`
	// RecordedBand is the band the aggregation assessed - captured by the
	// composition from the stored result, never read from a request.
	RecordedBand string `json:"recorded_band"`
	// OverrideBand is the reviewer's own reading.
	OverrideBand string `json:"override_band"`
	// Rationale is why they disagree. Required: REV-03's second box.
	Rationale string `json:"rationale"`
}

// EvidenceVersion names what the decision was informed by, captured
// server-side at decision time.
type EvidenceVersion struct {
	EvaluationID string
	ResultDigest string
	RubricDigest string
}

// ReviewDecision is one recorded decision, as history reads it.
type ReviewDecision struct {
	ID        string
	SessionID string
	DecidedBy string
	Decision  string
	Reason    string
	Evidence  EvidenceVersion
	Overrides []BandOverride
	CreatedAt time.Time
}

// RecordReviewDecision appends one decision and publishes the catalogued
// event in the same transaction: the fact and its announcement commit
// together or neither does.
func (s *Store) RecordReviewDecision(
	ctx context.Context,
	tenantID, campaignID, sessionID, decidedBy, decision, reason string,
	evidence EvidenceVersion,
	overrides []BandOverride,
) (ReviewDecision, error) {
	if !validDecisions[decision] {
		return ReviewDecision{}, fmt.Errorf("%w: %q", ErrDecisionUnknown, decision)
	}
	if strings.TrimSpace(reason) == "" {
		return ReviewDecision{}, ErrDecisionReasonRequired
	}
	if strings.TrimSpace(decidedBy) == "" {
		return ReviewDecision{}, ErrDecisionActorRequired
	}
	for _, override := range overrides {
		if strings.TrimSpace(override.CompetencyID) == "" ||
			strings.TrimSpace(override.RecordedBand) == "" ||
			strings.TrimSpace(override.OverrideBand) == "" ||
			strings.TrimSpace(override.Rationale) == "" {
			return ReviewDecision{}, ErrOverrideIncomplete
		}
	}
	if overrides == nil {
		overrides = []BandOverride{}
	}
	encodedOverrides, err := json.Marshal(overrides)
	if err != nil {
		return ReviewDecision{}, fmt.Errorf("recruiting: encoding overrides: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReviewDecision{}, fmt.Errorf("recruiting: beginning decision: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return ReviewDecision{}, err
	}

	row, err := db.New(tx).RecordReviewDecision(ctx, db.RecordReviewDecisionParams{
		ID: id.New().String(), CampaignID: campaignID, TenantID: tenantID,
		SessionID: sessionID, DecidedBy: decidedBy, Decision: decision,
		Reason: reason, EvaluationID: evidence.EvaluationID,
		ResultDigest: evidence.ResultDigest, RubricDigest: evidence.RubricDigest,
		Overrides: encodedOverrides,
	})
	if err != nil {
		return ReviewDecision{}, fmt.Errorf("recruiting: recording the decision: %w", err)
	}

	// The catalogued announcement: who decided, on what, informed by which
	// evaluation. No reasoning text - that is restricted content, read from
	// the record by somebody authorised.
	payload, err := json.Marshal(map[string]string{
		"review_id":      row.ID,
		"application_id": sessionID,
		"decision":       decision,
		"decided_by":     decidedBy,
		"evaluation_id":  evidence.EvaluationID,
	})
	if err != nil {
		return ReviewDecision{}, fmt.Errorf("recruiting: encoding the decision event: %w", err)
	}
	if _, err := outbox.New(s.pool).Publish(ctx, tx, outbox.Event{
		Type:          "review.decision_recorded.v1",
		SchemaVersion: "1.0",
		TenantID:      tenantID,
		Producer:      "recruiting",
		Actor:         outbox.Actor{Type: "user", ID: decidedBy},
		Purpose:       "screening",
		Payload:       payload,
	}); err != nil {
		return ReviewDecision{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ReviewDecision{}, fmt.Errorf("recruiting: committing the decision: %w", err)
	}
	return decisionFrom(row.ID, row.SessionID, row.DecidedBy, row.Decision,
		row.Reason, row.EvaluationID, row.ResultDigest, row.RubricDigest,
		row.Overrides, row.CreatedAt)
}

// ReviewDecisionsForSession answers the whole history, oldest first: every
// decision ever recorded, each with its true actor and evidence version.
func (s *Store) ReviewDecisionsForSession(ctx context.Context, tenantID, campaignID, sessionID string) ([]ReviewDecision, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recruiting: beginning decision history: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).ReviewDecisionsForSession(ctx, db.ReviewDecisionsForSessionParams{
		SessionID: sessionID, CampaignID: campaignID,
	})
	if err != nil {
		return nil, fmt.Errorf("recruiting: reading decision history: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("recruiting: committing decision history: %w", err)
	}
	out := make([]ReviewDecision, 0, len(rows))
	for _, row := range rows {
		decision, err := decisionFrom(row.ID, row.SessionID, row.DecidedBy,
			row.Decision, row.Reason, row.EvaluationID, row.ResultDigest,
			row.RubricDigest, row.Overrides, row.CreatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, decision)
	}
	return out, nil
}

func decisionFrom(rowID, sessionID, decidedBy, decision, reason, evaluationID,
	resultDigest, rubricDigest string, overrides []byte, createdAt time.Time,
) (ReviewDecision, error) {
	var decoded []BandOverride
	if err := json.Unmarshal(overrides, &decoded); err != nil {
		return ReviewDecision{}, fmt.Errorf("recruiting: decoding overrides: %w", err)
	}
	return ReviewDecision{
		ID: rowID, SessionID: sessionID, DecidedBy: decidedBy,
		Decision: decision, Reason: reason,
		Evidence: EvidenceVersion{
			EvaluationID: evaluationID, ResultDigest: resultDigest,
			RubricDigest: rubricDigest,
		},
		Overrides: decoded, CreatedAt: createdAt,
	}, nil
}
