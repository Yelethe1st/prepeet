package interview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
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

// Conversational types end at the seal: once completion freezes the
// record, these are refused, because a transcript that can still grow was
// never sealed.
var conversationalEventTypes = map[string]bool{
	"transcript.segment.partial":   true,
	"transcript.segment.final":     true,
	"transcript.segment.corrected": true,
	"turn.boundary":                true,
	"interruption":                 true,
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
	// SEQUENCE_INVALID, INTERRUPTION_INVALID, INTERRUPTION_NOT_IN_FLIGHT.
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
	return e.ingest(ctx, sessionID, mode, candidateID, tenantID, epoch, events, false)
}

// IngestAsService lands the agent's events (RTC-05, ADR-0019): the server
// stamps the current epoch and assigns the next sequences itself, so the
// agent and the browser never share a numbering and the agent never has
// to know which attempt it is speaking into. Ephemeral types take no
// slot, exactly as on the browser path.
func (e *Events) IngestAsService(ctx context.Context, sessionID, mode, candidateID, tenantID string, events []ControlEvent) (Acknowledgment, error) {
	return e.ingest(ctx, sessionID, mode, candidateID, tenantID, 0, events, true)
}

func (e *Events) ingest(ctx context.Context, sessionID, mode, candidateID, tenantID string, epoch int, events []ControlEvent, assign bool) (Acknowledgment, error) {
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
	if assign {
		epoch = session.ConnectionEpoch
		stored, err := q.StoredSequences(ctx, db.StoredSequencesParams{
			SessionID: sessionID, Epoch: int32(epoch),
		})
		if err != nil {
			return Acknowledgment{}, fmt.Errorf("interview: reading sequences: %w", err)
		}
		next := 1
		if len(stored) > 0 {
			next = int(stored[len(stored)-1]) + 1
		}
		for i := range events {
			events[i].Epoch = epoch
			if ephemeralEventTypes[events[i].Type] {
				events[i].Sequence = 0
				continue
			}
			events[i].Sequence = next
			next++
		}
	} else if epoch != session.ConnectionEpoch {
		return Acknowledgment{}, ErrEpochStale
	}

	// A sealed transcript takes no more conversation. Non-conversational
	// events (leaving, connection state) still land, because a goodbye is
	// not testimony.
	sealed := false
	if _, err := db.New(tx).GetSeal(ctx, sessionID); err == nil {
		sealed = true
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Acknowledgment{}, fmt.Errorf("interview: checking the seal: %w", err)
	}

	// The actor every effect of this batch is recorded under: the browser
	// acts as the candidate, the agent path as automation acting for them.
	actor := Actor{ID: candidateID, Type: "user"}
	if assign {
		actor.Type = "service"
	}

	// The accepted connection lifecycle, in batch order. The fold into the
	// state machine happens after the events commit, because the events are
	// the record and the machine follows the record, never the reverse.
	var lifecycle []string
	outcomes := make([]EventOutcome, 0, len(events))
	for _, event := range events {
		if sealed && conversationalEventTypes[event.Type] {
			outcomes = append(outcomes, EventOutcome{
				EventID: event.EventID, Status: "refused", Reason: "EVENT_AFTER_SEAL",
			})
			continue
		}
		outcome := e.apply(ctx, tx, session, event, actor)
		if outcome.Status == "accepted" {
			switch event.Type {
			case "connection.established", "connection.resumed", "connection.lost":
				lifecycle = append(lifecycle, event.Type)
			}
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

	// Fold the accepted lifecycle into the machine, in the order the batch
	// spoke it, each transition its own transaction after the events
	// committed. The first established connection moves connecting to
	// in_progress - the interview is genuinely happening from here - and the
	// same signal returns a reconnecting session to progress, because a
	// recovered interview resumed rather than started twice. A loss while in
	// progress moves the session to reconnecting and announces it, so the
	// drop is the server's state, never only the tab's memory. A stale
	// version means a concurrent ingest folded first; its fold stands.
	current := session
	for _, kind := range lifecycle {
		var to State
		effects := Effects{}
		switch {
		case (kind == "connection.established" || kind == "connection.resumed") &&
			(current.State == StateConnecting || current.State == StateReconnecting):
			to = StateInProgress
			if current.State == StateConnecting {
				// Only the first arrival is the start; a recovery announces
				// nothing new, the session already started.
				event, err := startedEvent(current)
				if err != nil {
					return Acknowledgment{}, err
				}
				effects.Event = event
			}
		case kind == "connection.lost" && current.State == StateInProgress:
			to = StateReconnecting
			event, err := interruptedEvent(current)
			if err != nil {
				return Acknowledgment{}, err
			}
			effects.Event = event
		default:
			continue
		}
		next, err := e.store.Transition(ctx, current, to, effects, actor)
		if errors.Is(err, ErrStaleVersion) {
			break
		}
		if err != nil {
			return Acknowledgment{}, err
		}
		current = next
	}
	return Acknowledgment{Epoch: epoch, Accepted: accepted, Missing: missing, Outcomes: outcomes}, nil
}

// apply lands one event: stored if durable, acknowledged if ephemeral,
// refused by name otherwise.
//
// The insert runs inside a savepoint, because a unique violation poisons
// the enclosing PostgreSQL transaction and one duplicate must not abort
// the rest of the batch.
func (e *Events) apply(ctx context.Context, tx pgx.Tx, session Session, event ControlEvent, actor Actor) EventOutcome {
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
	if event.Type == "transcript.segment.final" || event.Type == "transcript.segment.corrected" {
		// A transcript row that cannot serve as evidence is refused at the
		// door, not discovered by evaluation later.
		if err := validateTranscriptPayload(event.Type, event.Payload); err != nil {
			return EventOutcome{EventID: event.EventID, Status: "refused", Reason: "TRANSCRIPT_INVALID"}
		}
	}
	var interruption *interruptionPayload
	if event.Type == "interruption" {
		// The interruption fact rides the timeline's own event (SES-06): the
		// realtime layer that saw the drop is the only party that knows its
		// cause and duration, and the event's dedup identity is what keeps a
		// resent report from inventing a second fact. Refused before it can
		// take a slot when it could not serve the human decision it exists
		// for, exactly as a malformed transcript segment is.
		parsed, err := parseInterruptionPayload(event.Payload)
		if err != nil {
			return EventOutcome{EventID: event.EventID, Status: "refused", Reason: "INTERRUPTION_INVALID"}
		}
		switch session.State {
		case StateInProgress, StateReconnecting:
			// The states an interruption can befall: an interview in flight.
		default:
			return EventOutcome{EventID: event.EventID, Status: "refused", Reason: "INTERRUPTION_NOT_IN_FLIGHT"}
		}
		interruption = &parsed
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
		if interruption != nil {
			// Inside the same savepoint as the event, so the fact and the
			// record that reported it commit together or neither does: a
			// duplicate event never reaches here, which is what keeps one
			// drop from becoming two facts.
			if _, err := e.store.recordInterruptionIn(ctx, savepoint, session,
				interruption.Cause, interruption.DurationSeconds, actor); err != nil {
				_ = savepoint.Rollback(ctx)
				return EventOutcome{EventID: event.EventID, Status: "refused", Reason: "EVENT_STORE_FAILED"}
			}
		}
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

// interruptedEvent is the catalogue's session_interrupted notification,
// published atomically with the transition to reconnecting. Reason "network"
// is the closed set's name for a lost connection; resumable is true because
// the grace window opens with this very transition, and SES-06's expiry is
// what later says otherwise; attempt names which interruption this is, and
// the connection epoch is exactly that ordinal - each resume opens the next
// epoch, so the pattern operations aggregates on needs no second counter.
func interruptedEvent(session Session) (*outbox.Event, error) {
	payload, err := json.Marshal(map[string]any{
		"session_id": session.ID,
		"reason":     "network",
		"resumable":  true,
		"attempt":    session.ConnectionEpoch,
	})
	if err != nil {
		return nil, fmt.Errorf("interview: encoding the interrupted event: %w", err)
	}
	return &outbox.Event{
		Type:          "interview.session_interrupted.v1",
		SchemaVersion: "1.0",
		TenantID:      session.TenantID,
		Producer:      "interview",
		Actor:         outbox.Actor{Type: "user", ID: session.CandidateID},
		Purpose:       session.Mode,
		Payload:       payload,
	}, nil
}

// startedEvent is the catalogue's session_started notification, published
// atomically with the in_progress transition, carrying exactly what its
// contract names. The media consumer reads the recording preference from
// the session row it fetches anyway (RTC-05).
func startedEvent(session Session) (*outbox.Event, error) {
	payload, err := json.Marshal(map[string]any{
		"session_id":    session.ID,
		"bundle_digest": session.BundleDigest,
	})
	if err != nil {
		return nil, fmt.Errorf("interview: encoding the started event: %w", err)
	}
	return &outbox.Event{
		Type:          "interview.session_started.v1",
		SchemaVersion: "1.0",
		TenantID:      session.TenantID,
		Producer:      "interview",
		Actor:         outbox.Actor{Type: "user", ID: session.CandidateID},
		Purpose:       session.Mode,
		Payload:       payload,
	}, nil
}
