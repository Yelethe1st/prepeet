package recruiting

// The accommodation half of the store: persistence for SCR-06's three
// append-only records. Every method scopes its transaction to the tenant
// first, because that is what every row-level security policy on these
// tables reads, and an unscoped caller sees nothing rather than everything.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/recruiting/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// RequestAccommodation writes a candidate's request.
//
// The caller builds the request through NewAccommodationRequest, which is
// where the phase rule and the named-adjustment rule live; this method only
// persists what that gate admitted. The schema repeats the vocabulary check,
// so a caller that skips the constructor meets the CHECK constraint instead.
func (s *Store) RequestAccommodation(ctx context.Context, request AccommodationRequest) (AccommodationRequest, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AccommodationRequest{}, fmt.Errorf("recruiting: beginning the request: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, request.TenantID); err != nil {
		return AccommodationRequest{}, err
	}

	row, err := db.New(tx).RequestAccommodation(ctx, db.RequestAccommodationParams{
		ID: id.New().String(), TenantID: request.TenantID,
		CampaignID: request.CampaignID, CandidateID: request.CandidateID,
		Adjustment: string(request.Adjustment),
	})
	if err != nil {
		return AccommodationRequest{}, fmt.Errorf("recruiting: recording the request: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return AccommodationRequest{}, fmt.Errorf("recruiting: committing the request: %w", err)
	}
	return requestFrom(row), nil
}

// DecideAccommodation records one human's answer to one request.
//
// Append-only by schema: a later change of mind is a later call to this
// method, and both rows remain, so "what had been decided when the interview
// ran" stays answerable.
func (s *Store) DecideAccommodation(ctx context.Context, tenantID string, decision AccommodationDecision) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("recruiting: beginning the decision: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return err
	}

	if err := db.New(tx).RecordAccommodationDecision(ctx, db.RecordAccommodationDecisionParams{
		ID: id.New().String(), TenantID: tenantID,
		RequestID: decision.RequestID, Granted: decision.Granted, DecidedBy: decision.DecidedBy,
	}); err != nil {
		return fmt.Errorf("recruiting: recording the decision: %w", err)
	}
	return tx.Commit(ctx)
}

// FulfilAccommodation records a granted adjustment being applied to a session.
//
// The standing decision is read and the fulfilment written in one
// transaction, so a withdrawal cannot land between the check and the write.
// The rule is enforced twice on purpose: NewFulfilment refuses here with an
// error a caller can act on, and the fulfilment table's trigger refuses again
// in the database for whatever future path skips this method.
func (s *Store) FulfilAccommodation(ctx context.Context, tenantID, requestID, sessionID string) (Fulfilment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Fulfilment{}, fmt.Errorf("recruiting: beginning the fulfilment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return Fulfilment{}, err
	}
	q := db.New(tx)

	standing, err := standingDecision(ctx, q, requestID)
	if err != nil {
		return Fulfilment{}, err
	}
	fulfilment, err := NewFulfilment(requestID, sessionID, standing)
	if err != nil {
		return Fulfilment{}, err
	}

	if err := q.RecordAccommodationFulfilment(ctx, db.RecordAccommodationFulfilmentParams{
		ID: id.New().String(), TenantID: tenantID,
		RequestID: requestID, SessionID: sessionID,
	}); err != nil {
		return Fulfilment{}, fmt.Errorf("recruiting: recording the fulfilment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Fulfilment{}, fmt.Errorf("recruiting: committing the fulfilment: %w", err)
	}
	return fulfilment, nil
}

// AccommodationsFor answers every request this candidate made on this
// campaign, each with its derived state. This is the read behind "a candidate
// must be able to see the state of their request".
func (s *Store) AccommodationsFor(ctx context.Context, tenantID, campaignID, candidateID string) ([]AccommodationView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recruiting: beginning the accommodation read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	q := db.New(tx)

	rows, err := q.AccommodationRequestsFor(ctx, db.AccommodationRequestsForParams{
		CampaignID: campaignID, CandidateID: candidateID,
	})
	if err != nil {
		return nil, fmt.Errorf("recruiting: reading requests: %w", err)
	}

	views := make([]AccommodationView, 0, len(rows))
	for _, row := range rows {
		standing, err := standingDecision(ctx, q, row.ID)
		if err != nil {
			return nil, err
		}
		view := AccommodationView{
			Request: AccommodationRequest{
				ID: row.ID, TenantID: tenantID, CampaignID: row.CampaignID,
				CandidateID: row.CandidateID, Adjustment: Adjustment(row.Adjustment),
				RequestedAt: row.RequestedAt,
			},
			State: StateOf(standing),
		}
		if standing != nil {
			view.DecidedBy = standing.DecidedBy
			decidedAt := standing.DecidedAt
			view.DecidedAt = &decidedAt
		}
		views = append(views, view)
	}
	return views, nil
}

// SessionAccommodation is one adjustment as it was applied to one session:
// what the interview runner's port will serve when the composition root
// wires it, and the shape SCR-08's incident view reads coverage against.
type SessionAccommodation struct {
	RequestID   string
	SessionID   string
	Adjustment  Adjustment
	FulfilledAt time.Time
}

// AccommodationsForSession answers what was actually applied to one session.
func (s *Store) AccommodationsForSession(ctx context.Context, tenantID, sessionID string) ([]SessionAccommodation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recruiting: beginning the session read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := db.New(tx).AccommodationsForSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("recruiting: reading session accommodations: %w", err)
	}
	applied := make([]SessionAccommodation, 0, len(rows))
	for _, row := range rows {
		applied = append(applied, SessionAccommodation{
			RequestID: row.RequestID, SessionID: row.SessionID,
			Adjustment: Adjustment(row.Adjustment), FulfilledAt: row.FulfilledAt,
		})
	}
	return applied, nil
}

// standingDecision reads the latest decision for a request, nil when nobody
// has answered. Absence is a real state here rather than an error, because
// "requested" is the first thing a candidate sees.
func standingDecision(ctx context.Context, q *db.Queries, requestID string) (*AccommodationDecision, error) {
	row, err := q.StandingAccommodationDecision(ctx, requestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("recruiting: reading the standing decision: %w", err)
	}
	return &AccommodationDecision{
		RequestID: row.RequestID, Granted: row.Granted,
		DecidedBy: row.DecidedBy, DecidedAt: row.DecidedAt,
	}, nil
}

// requestFrom maps a generated row onto the domain type.
func requestFrom(row db.RecruitingAccommodationRequest) AccommodationRequest {
	return AccommodationRequest{
		ID: row.ID, TenantID: row.TenantID, CampaignID: row.CampaignID,
		CandidateID: row.CandidateID, Adjustment: Adjustment(row.Adjustment),
		RequestedAt: row.RequestedAt,
	}
}
