package interview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/interview/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// InterruptionCause is why an interview stopped. A closed vocabulary, because
// the difference between a device failing and a candidate leaving is exactly
// what a human deciding on re-invitation needs.
type InterruptionCause string

const (
	// CauseConnectionLost is the network dropping.
	CauseConnectionLost InterruptionCause = "connection_lost"
	// CauseDeviceFailure is the candidate's device failing.
	CauseDeviceFailure InterruptionCause = "device_failure"
	// CauseCandidateLeft is the candidate leaving the interview.
	CauseCandidateLeft InterruptionCause = "candidate_left"
	// CauseGraceExpired is the reconnection window lapsing without a resume.
	CauseGraceExpired InterruptionCause = "grace_expired"
)

var validCauses = map[InterruptionCause]bool{
	CauseConnectionLost: true, CauseDeviceFailure: true,
	CauseCandidateLeft: true, CauseGraceExpired: true,
}

// ErrUnknownCause means an interruption named a cause outside the vocabulary.
var ErrUnknownCause = errors.New("interview: not a known interruption cause")

// ErrNotInterruptible means an interruption was recorded against a session that
// is not running, which would be inventing an event.
var ErrNotInterruptible = errors.New("interview: the session is not in an interruptible state")

// Interruption is one recorded stop in an interview.
type Interruption struct {
	ID              string
	Cause           InterruptionCause
	OccurredAt      time.Time
	DurationSeconds int
	ConnectionEpoch int
}

// interruptionPayload is the durable interruption event's body: what the
// realtime layer that saw the drop reports about it.
type interruptionPayload struct {
	Cause           InterruptionCause `json:"cause"`
	DurationSeconds int               `json:"duration_seconds"`
}

// parseInterruptionPayload validates the report at the door, the same way a
// transcript segment is: a cause outside the vocabulary or a nonsensical
// duration is refused before it can take a slot, never discovered later by
// the human deciding on re-invitation.
func parseInterruptionPayload(raw json.RawMessage) (interruptionPayload, error) {
	var payload interruptionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return interruptionPayload{}, fmt.Errorf("interview: decoding the interruption: %w", err)
	}
	if !validCauses[payload.Cause] {
		return interruptionPayload{}, fmt.Errorf("%w: %q", ErrUnknownCause, payload.Cause)
	}
	if payload.DurationSeconds < 0 {
		return interruptionPayload{}, errors.New("interview: an interruption cannot have negative duration")
	}
	return payload, nil
}

// RecordInterruption records that an interview stopped, as a fact independent
// of the session's state.
//
// The state machine already routes a dropped session: reconnecting resumes or,
// when grace expires, finalizes, and the spec makes interrupted a terminal
// state a session is not driven into by this. What was missing is the record of
// what happened and for how long, which re-invitation is a human decision
// about; this is that record. It is append-only, so a session interrupted more
// than once carries every stop.
//
// The session must be running or reconnecting: an interruption is a thing that
// happens to an interview in flight, and recording one against a session that
// is not running would be inventing an event. The duration is the caller's,
// measured from when the connection dropped, because only the realtime layer
// that saw the drop knows when it began.
func (s *Store) RecordInterruption(ctx context.Context, session Session, cause InterruptionCause, durationSeconds int, actor Actor) (Interruption, error) {
	if !validCauses[cause] {
		return Interruption{}, fmt.Errorf("%w: %q", ErrUnknownCause, cause)
	}
	switch session.State {
	case StateInProgress, StateReconnecting:
		// The states an interruption can befall.
	default:
		return Interruption{}, fmt.Errorf("%w: the session is %s", ErrNotInterruptible, session.State)
	}
	if durationSeconds < 0 {
		durationSeconds = 0
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Interruption{}, fmt.Errorf("interview: beginning interrupt: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return Interruption{}, err
	}

	interruptionID, err := s.recordInterruptionIn(ctx, tx, session, cause, durationSeconds, actor)
	if err != nil {
		return Interruption{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Interruption{}, fmt.Errorf("interview: committing the interruption: %w", err)
	}

	return Interruption{
		ID: interruptionID, Cause: cause, OccurredAt: time.Now(),
		DurationSeconds: durationSeconds, ConnectionEpoch: session.ConnectionEpoch,
	}, nil
}

// recordInterruptionIn writes the fact and its audit row inside the caller's
// transaction, so an interruption reported through the control timeline
// commits with the event that reported it. The caller owns the scope and the
// state guard; this owns only what every path must write identically.
func (s *Store) recordInterruptionIn(ctx context.Context, tx pgx.Tx, session Session, cause InterruptionCause, durationSeconds int, actor Actor) (string, error) {
	if durationSeconds < 0 {
		durationSeconds = 0
	}
	interruptionID := id.New().String()
	if err := db.New(tx).InsertInterruption(ctx, db.InsertInterruptionParams{
		ID: interruptionID, SessionID: session.ID, CandidateID: session.CandidateID,
		TenantID: session.TenantID, Cause: string(cause),
		DurationSeconds: int32(durationSeconds), ConnectionEpoch: int32(session.ConnectionEpoch),
	}); err != nil {
		return "", fmt.Errorf("interview: recording the interruption: %w", err)
	}
	return interruptionID, s.audit(ctx, tx, session, actor, "interview.session_interrupted", "allowed")
}

// Interruptions reads what interrupted a session, for whoever may read it.
func (s *Store) Interruptions(ctx context.Context, sessionID, mode, candidateID, tenantID string) ([]Interruption, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("interview: beginning interruptions read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, mode, candidateID, tenantID); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).InterruptionsForSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("interview: reading interruptions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("interview: committing the interruptions read: %w", err)
	}
	out := make([]Interruption, 0, len(rows))
	for _, row := range rows {
		out = append(out, Interruption{
			ID: row.ID, Cause: InterruptionCause(row.Cause), OccurredAt: row.OccurredAt,
			DurationSeconds: int(row.DurationSeconds), ConnectionEpoch: int(row.ConnectionEpoch),
		})
	}
	return out, nil
}
