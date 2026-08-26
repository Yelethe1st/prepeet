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

// The control event protocol: RTC-02, to realtime-protocol.md.
//
// One authoritative conversation timeline per session, across many
// connection attempts. Each attempt is an epoch; events order by sequence
// within their epoch; event ids deduplicate retries; and the accepted
// cursor - the highest contiguous sequence - is persisted on the session so
// recovery never relies on browser memory. A stale epoch cannot mutate
// anything: takeover supersedes it, and its events are refused by name.
//
// Durability follows the spec's split. Final transcript, lifecycle, turn
// and preference events are durable rows in an append-only log; partial
// captions and input levels are ephemeral, validated and acknowledged but
// never stored, and they carry no sequence because a slot that would
// vanish on restart is a gap manufactured for later.

// The durable control event vocabulary, browser to Go. A type outside this
// map is refused: an unknown event accepted silently is an unchecked path.
var durableEventTypes = map[string]bool{
	"connection.established":       true,
	"connection.degraded":          true,
	"connection.lost":              true,
	"connection.resumed":           true,
	"device.microphone":            true,
	"transcript.segment.final":     true,
	"transcript.segment.corrected": true,
	"turn.boundary":                true,
	"interruption":                 true,
	"preference.captions":          true,
	"preference.push_to_talk":      true,
	"session.leave":                true,
}

// Ephemeral types: acknowledged, never stored, sequence-less.
var ephemeralEventTypes = map[string]bool{
	"transcript.segment.partial": true,
	"device.input_level":         true,
}

// Protocol refusals.
var (
	ErrEpochStale = errors.New("interview: EPOCH_STALE: this connection was superseded; resume to continue")
	ErrNoAttempt  = errors.New("interview: NO_ATTEMPT: the session has no active connection attempt")
)

// ControlEvent is one envelope from the browser.
type ControlEvent struct {
	EventID    string
	Epoch      int
	Sequence   int
	Type       string
	Payload    json.RawMessage
	OccurredAt time.Time
}

// EventOutcome is the per-event verdict inside an acknowledgment.
type EventOutcome struct {
	EventID string
	// Status is accepted, duplicate, or refused.
	Status string
	// Reason names a refusal: EVENT_TYPE_UNKNOWN, SEQUENCE_CONFLICT,
	// SEQUENCE_INVALID.
	Reason string
}

// Acknowledgment is Go's answer: the cursor and the holes.
type Acknowledgment struct {
	Epoch int
	// Accepted is the highest contiguous sequence: everything at or below
	// it is durably held.
	Accepted int
	// Missing lists the gaps below the highest stored sequence, for the
	// client to resend.
	Missing  []SequenceRange
	Outcomes []EventOutcome
}

// SequenceRange is one inclusive gap.
type SequenceRange struct {
	From int
	To   int
}

// Events ingests and replays the control timeline.
type Events struct {
	store *Store
}

// NewEvents wires the protocol over the session store.
func NewEvents(store *Store) *Events {
	return &Events{store: store}
}

// BeginAttempt opens the next epoch for a session: supersedes the previous
// attempt, advances the session's epoch monotonically, resets the cursor.
func (e *Events) BeginAttempt(ctx context.Context, session Session) (int, error) {
	tx, err := e.store.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("interview: beginning attempt: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return 0, err
	}
	q := db.New(tx)

	epoch := session.ConnectionEpoch + 1
	if err := q.SupersedeAttempts(ctx, session.ID); err != nil {
		return 0, fmt.Errorf("interview: superseding attempts: %w", err)
	}
	if err := q.InsertAttempt(ctx, db.InsertAttemptParams{
		ID: id.New().String(), SessionID: session.ID, Mode: session.Mode,
		CandidateID: session.CandidateID, TenantID: session.TenantID,
		ConnectionEpoch: int32(epoch),
	}); err != nil {
		return 0, fmt.Errorf("interview: recording the attempt: %w", err)
	}
	moved, err := q.AdvanceSessionEpoch(ctx, db.AdvanceSessionEpochParams{
		ID: session.ID, Epoch: int32(epoch),
	})
	if err != nil {
		return 0, fmt.Errorf("interview: advancing the epoch: %w", err)
	}
	if moved == 0 {
		// Somebody advanced past us concurrently: their takeover wins.
		return 0, ErrEpochStale
	}
	return epoch, tx.Commit(ctx)
}

// Ingest accepts one batch of events for a session's current epoch.
//
// Duplicates converge, out-of-order events land in their slots, a stale
// epoch is refused whole, and the acknowledgment carries the new contiguous
// cursor with the exact gaps still owed. The whole batch is one
// transaction, so the persisted cursor never claims events that did not
// commit.
func (e *Events) Ingest(ctx context.Context, sessionID, mode, candidateID, tenantID string, epoch int, events []ControlEvent) (Acknowledgment, error) {
	tx, err := e.store.pool.Begin(ctx)
	if err != nil {
		return Acknowledgment{}, fmt.Errorf("interview: beginning ingest: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, mode, candidateID, tenantID); err != nil {
		return Acknowledgment{}, err
	}
	q := db.New(tx)

	session, err := e.store.get(ctx, tx, sessionID)
	if err != nil {
		return Acknowledgment{}, err
	}
	if session.ConnectionEpoch == 0 {
		return Acknowledgment{}, ErrNoAttempt
	}
	// The one epoch rule: only the current attempt speaks. A batch from a
	// superseded connection is refused whole, because a stale tab must not
	// write history into a session that has moved on.
	if epoch != session.ConnectionEpoch {
		return Acknowledgment{}, ErrEpochStale
	}

	sawEstablished := false
	outcomes := make([]EventOutcome, 0, len(events))
	for _, event := range events {
		outcome := e.apply(ctx, tx, session, event)
		if outcome.Status == "accepted" && event.Type == "connection.established" {
			sawEstablished = true
		}
		outcomes = append(outcomes, outcome)
	}

	accepted, missing, err := e.cursor(ctx, q, session.ID, epoch)
	if err != nil {
		return Acknowledgment{}, err
	}
	if accepted > session.AcceptedSequence {
		if _, err := q.PersistCursor(ctx, db.PersistCursorParams{
			ID: session.ID, Epoch: int32(epoch), Accepted: int32(accepted),
		}); err != nil {
			return Acknowledgment{}, fmt.Errorf("interview: persisting the cursor: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Acknowledgment{}, err
	}

	// The first established connection is what moves connecting to
	// in_progress: the interview is genuinely happening from here. Its own
	// transaction, after the events committed, and idempotent by the
	// machine's own guard: a session already in progress stays there.
	if sawEstablished && session.State == StateConnecting {
		actor := Actor{ID: candidateID, Type: "user"}
		if _, err := e.store.Transition(ctx, session, StateInProgress, Effects{}, actor); err != nil &&
			!errors.Is(err, ErrStaleVersion) {
			return Acknowledgment{}, err
		}
	}
	return Acknowledgment{Epoch: epoch, Accepted: accepted, Missing: missing, Outcomes: outcomes}, nil
}

// apply lands one event: stored if durable, acknowledged if ephemeral,
// refused by name otherwise.
//
// The insert runs inside a savepoint, because a unique violation poisons
// the enclosing PostgreSQL transaction and one duplicate must not abort
// the rest of the batch.
func (e *Events) apply(ctx context.Context, tx pgx.Tx, session Session, event ControlEvent) EventOutcome {
	if ephemeralEventTypes[event.Type] {
		// Validated and waved through: never stored, never a sequence slot,
		// because a slot that vanishes on restart is a manufactured gap.
		return EventOutcome{EventID: event.EventID, Status: "accepted"}
	}
	if !durableEventTypes[event.Type] {
		return EventOutcome{EventID: event.EventID, Status: "refused", Reason: "EVENT_TYPE_UNKNOWN"}
	}
	if event.Sequence < 1 {
		return EventOutcome{EventID: event.EventID, Status: "refused", Reason: "SEQUENCE_INVALID"}
	}

	payload := event.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return EventOutcome{EventID: event.EventID, Status: "refused", Reason: "EVENT_STORE_FAILED"}
	}
	err = db.New(savepoint).InsertControlEvent(ctx, db.InsertControlEventParams{
		EventID: event.EventID, SessionID: session.ID, Mode: session.Mode,
		CandidateID: session.CandidateID, TenantID: session.TenantID,
		ConnectionEpoch: int32(session.ConnectionEpoch), Sequence: int32(event.Sequence),
		EventType: event.Type, Payload: payload, OccurredAt: event.OccurredAt,
	})
	if err == nil {
		if err := savepoint.Commit(ctx); err != nil {
			return EventOutcome{EventID: event.EventID, Status: "refused", Reason: "EVENT_STORE_FAILED"}
		}
		return EventOutcome{EventID: event.EventID, Status: "accepted"}
	}
	_ = savepoint.Rollback(ctx)

	if isUniqueViolation(err) {
		// Which uniqueness? The same event retried converges; a different
		// event claiming an occupied slot is corruption and is refused.
		present, existsErr := db.New(tx).ControlEventExists(ctx, event.EventID)
		if existsErr == nil && present {
			return EventOutcome{EventID: event.EventID, Status: "duplicate"}
		}
		return EventOutcome{EventID: event.EventID, Status: "refused", Reason: "SEQUENCE_CONFLICT"}
	}
	return EventOutcome{EventID: event.EventID, Status: "refused", Reason: "EVENT_STORE_FAILED"}
}

// cursor computes the highest contiguous sequence and the gaps under the
// highest stored one.
func (e *Events) cursor(ctx context.Context, q *db.Queries, sessionID string, epoch int) (int, []SequenceRange, error) {
	stored, err := q.StoredSequences(ctx, db.StoredSequencesParams{
		SessionID: sessionID, Epoch: int32(epoch),
	})
	if err != nil {
		return 0, nil, fmt.Errorf("interview: reading sequences: %w", err)
	}

	accepted := 0
	var missing []SequenceRange
	expected := 1
	for _, sequence := range stored {
		s := int(sequence)
		if s > expected {
			missing = append(missing, SequenceRange{From: expected, To: s - 1})
		}
		if s == accepted+1 && len(missing) == 0 {
			accepted = s
		}
		expected = s + 1
	}
	return accepted, missing, nil
}

// Replay answers everything after a cursor, in the one authoritative order.
// Replaying twice from the same cursor answers identically; that property
// is what the client rebuilds itself on after a reconnect.
func (e *Events) Replay(ctx context.Context, sessionID, mode, candidateID, tenantID string, afterEpoch, afterSequence int) ([]ControlEvent, error) {
	tx, err := e.store.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("interview: beginning replay: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, mode, candidateID, tenantID); err != nil {
		return nil, err
	}

	if _, err := e.store.get(ctx, tx, sessionID); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).ReplayControlEvents(ctx, db.ReplayControlEventsParams{
		SessionID: sessionID, AfterEpoch: int32(afterEpoch), AfterSequence: int32(afterSequence),
	})
	if err != nil {
		return nil, fmt.Errorf("interview: replaying: %w", err)
	}
	events := make([]ControlEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, ControlEvent{
			EventID: row.EventID, Epoch: int(row.ConnectionEpoch), Sequence: int(row.Sequence),
			Type: row.EventType, Payload: json.RawMessage(row.Payload), OccurredAt: row.OccurredAt,
		})
	}
	return events, nil
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return errors.Is(err, pgx.ErrTxCommitRollback)
}
