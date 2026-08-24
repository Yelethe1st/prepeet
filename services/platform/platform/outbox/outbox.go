// Package outbox publishes durable events in the same transaction as the state
// change they describe.
//
// ADR-0005 makes this load bearing rather than convenient. No context imports
// another, so a state change one context needs to tell others about has exactly
// one route out: a row written here, alongside the change itself.
//
// The shared transaction is the entire point. An event published outside it is
// a fact that may not have happened: a process can die between committing state
// and reaching a broker, and then the world believes something the database
// does not. Writing both together makes that impossible, and the price is
// needing a dispatcher to carry rows onward afterwards.
//
// Claiming with FOR UPDATE SKIP LOCKED rather than a lock service is a
// correctness choice rather than a cost one, and ADR-0006 records why: the lock
// and the work share a transactional scope so they cannot disagree, there is no
// lease to tune, and each additional dispatcher adds throughput instead of
// idling behind a leader.
//
// This package is infrastructure and knows nothing about any bounded context.
// It moves envelopes, per docs/contracts/event-catalog.md, and never inspects a
// payload.
//
// Implements part of INT-02.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Yelethe1st/prepeet/services/platform/platform/broadcast"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// MaxAttempts is how many times delivery is tried before the event is dead
// lettered.
//
// Ten, with the backoff below, spans roughly a day. An endpoint down for longer
// than that is not having a blip, and continuing to retry would turn somebody
// else's outage into our load problem while hiding the failure from whoever
// needs to act on it.
const MaxAttempts = 10

// versionedType matches an event type ending in a contract version, such as
// evaluation.completed.v1. The version in the type is what consumers subscribe
// against, per ADR-0004.
var versionedType = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+\.v[0-9]+$`)

// ErrInvalidEvent means the envelope is incomplete.
var ErrInvalidEvent = errors.New("outbox: event envelope is incomplete")

// Actor is who or what caused the event.
type Actor struct {
	Type string // "user" or "service"
	ID   string
}

// Event is one durable fact, in the envelope from
// docs/contracts/event-catalog.md.
type Event struct {
	// Type carries its contract version, such as evaluation.completed.v1. A new
	// version is a new type rather than a field change, because that is what
	// lets a consumer keep working while a migration runs.
	Type          string
	SchemaVersion string
	// TenantID is empty for events that belong to a person rather than a
	// tenant, since the same person may belong to several. See ADR-0002.
	TenantID      string
	OccurredAt    time.Time
	Producer      string
	Actor         Actor
	Purpose       string
	CorrelationID string
	CausationID   string
	// Payload carries identifiers and the minimum needed to act, never a row
	// dump and never restricted content: events reach integrations and
	// analytics, where the retention and access rules belong to somebody else.
	Payload json.RawMessage
}

func (e Event) validate() error {
	switch {
	case e.Type == "":
		return fmt.Errorf("%w: type is required", ErrInvalidEvent)
	case !versionedType.MatchString(e.Type):
		return fmt.Errorf("%w: type %q must end in a contract version, such as .v1", ErrInvalidEvent, e.Type)
	case e.SchemaVersion == "":
		return fmt.Errorf("%w: schema version is required", ErrInvalidEvent)
	case e.Producer == "":
		return fmt.Errorf("%w: producer is required", ErrInvalidEvent)
	case e.Actor.Type == "" || e.Actor.ID == "":
		return fmt.Errorf("%w: actor type and id are required", ErrInvalidEvent)
	default:
		return nil
	}
}

// Store reads and writes the outbox.
type Store struct {
	pool *pgxpool.Pool
}

// New builds a store.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Publish writes an event inside the caller's transaction.
//
// It takes the transaction rather than opening its own, which is the whole
// contract of this package: the event and the state change commit together or
// neither does. A caller that passes a fresh transaction has not published an
// event, it has written a row that might describe nothing.
func (s *Store) Publish(ctx context.Context, tx pgx.Tx, event Event) (string, error) {
	if err := event.validate(); err != nil {
		return "", err
	}

	occurred := event.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	}
	payload := event.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	eventID := id.New().String()
	if _, err := tx.Exec(ctx, `
		INSERT INTO integration.outbox
			(id, event_type, schema_version, tenant_id, occurred_at, producer,
			 actor_type, actor_id, purpose, correlation_id, causation_id, payload)
		VALUES ($1, $2, $3, nullif($4, '')::uuid, $5, $6, $7, $8,
		        nullif($9, ''), nullif($10, ''), nullif($11, ''), $12)`,
		eventID, event.Type, event.SchemaVersion, event.TenantID, occurred, event.Producer,
		event.Actor.Type, event.Actor.ID, event.Purpose, event.CorrelationID, event.CausationID,
		payload); err != nil {
		return "", fmt.Errorf("outbox: inserting event: %w", err)
	}

	// The wakeup is emitted inside the caller's transaction, deliberately, and
	// deliberately not through platform/broadcast.
	//
	// PostgreSQL holds a notification until the transaction commits and drops it
	// if the transaction rolls back, so the signal becomes visible exactly when
	// the row does. No external transport can do that: publishing before the
	// commit announces an event that may never exist, and publishing after
	// leaves a window where a crash loses the signal entirely.
	//
	// Losing it would only cost latency, since the dispatcher polls regardless.
	// But a guarantee available for free is worth taking, and this is the one
	// place where being tied to PostgreSQL is an advantage rather than a debt.
	// The payload is empty on purpose. The dispatcher needs to know only that
	// there is work; it reads what the work is from the table, under the same
	// claim that makes concurrent dispatchers safe. Putting the event in the
	// signal would mean delivering from a message that may have been superseded.
	channel, body, err := broadcast.NotifyArguments(WakeupTopic, nil)
	if err != nil {
		return "", fmt.Errorf("outbox: preparing the dispatcher signal: %w", err)
	}
	if _, err := tx.Exec(ctx, "SELECT pg_notify($1, $2)", channel, body); err != nil {
		return "", fmt.Errorf("outbox: signalling the dispatcher: %w", err)
	}

	return eventID, nil
}

// Pending is an event waiting to be delivered.
type Pending struct {
	ID            string
	Type          string
	SchemaVersion string
	TenantID      string
	OccurredAt    time.Time
	Producer      string
	Actor         Actor
	Purpose       string
	CorrelationID string
	CausationID   string
	Payload       json.RawMessage
	Attempts      int
}

// Claim takes up to limit events for delivery.
//
// FOR UPDATE SKIP LOCKED is what makes more than one dispatcher safe. Without
// it two dispatchers reading the same rows would both deliver them, and a
// tenant's ATS would see one candidate submitted twice. With it, the second
// dispatcher steps over rows the first is holding and takes the next ones
// instead, so adding dispatchers adds throughput rather than duplicates.
//
// The rows stay claimed only for the life of this transaction. A dispatcher
// that dies mid-delivery releases them, and they are redelivered, which is the
// at-least-once guarantee showing its edges: consumers deduplicate by event id.
func (s *Store) Claim(ctx context.Context, limit int) ([]Pending, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("outbox: beginning claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id::text, event_type, schema_version, coalesce(tenant_id::text, ''),
		       occurred_at, producer, actor_type, actor_id,
		       coalesce(purpose, ''), coalesce(correlation_id, ''), coalesce(causation_id, ''),
		       payload, attempts
		FROM integration.outbox
		WHERE published_at IS NULL
		  AND dead_at IS NULL
		  AND next_attempt_at <= now()
		ORDER BY next_attempt_at, id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("outbox: claiming events: %w", err)
	}

	var claimed []Pending
	for rows.Next() {
		var p Pending
		if err := rows.Scan(&p.ID, &p.Type, &p.SchemaVersion, &p.TenantID,
			&p.OccurredAt, &p.Producer, &p.Actor.Type, &p.Actor.ID,
			&p.Purpose, &p.CorrelationID, &p.CausationID, &p.Payload, &p.Attempts); err != nil {
			rows.Close()
			return nil, fmt.Errorf("outbox: reading claimed event: %w", err)
		}
		claimed = append(claimed, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox: reading claimed events: %w", err)
	}

	// The claim is marked inside the same transaction that locked the rows, so
	// releasing the lock and recording the attempt happen together.
	if len(claimed) > 0 {
		ids := make([]string, 0, len(claimed))
		for _, p := range claimed {
			ids = append(ids, p.ID)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE integration.outbox
			SET next_attempt_at = now() + $2::interval
			WHERE id = ANY($1::uuid[])`, ids, claimVisibility.String()); err != nil {
			return nil, fmt.Errorf("outbox: marking claim: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("outbox: committing claim: %w", err)
	}
	return claimed, nil
}

// claimVisibility is how long a claimed event is hidden from other dispatchers
// while this one attempts delivery. Long enough for a slow endpoint, short
// enough that a dispatcher killed mid-delivery does not strand the event.
const claimVisibility = 5 * time.Minute

// MarkDelivered records that an event reached its consumers.
func (s *Store) MarkDelivered(ctx context.Context, eventID string) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE integration.outbox SET published_at = now() WHERE id = $1 AND published_at IS NULL`,
		eventID); err != nil {
		return fmt.Errorf("outbox: marking delivered: %w", err)
	}
	return nil
}

// MarkFailed records a failed attempt and schedules the next one.
//
// The event returns to pending with a later attempt time, so a provider that is
// down for a minute does not cost the event. After MaxAttempts it is dead
// lettered instead, because an event nobody can deliver is an operational fact
// somebody needs to see rather than a row retried silently forever.
func (s *Store) MarkFailed(ctx context.Context, eventID, reason string) error {
	// The reason is stored for an operator and is truncated, because a provider
	// error can be a full HTML page and this column is read in a list.
	if len(reason) > 1000 {
		reason = reason[:1000]
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("outbox: beginning failure record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The attempt count is read and the wait computed in Go, so the backoff
	// curve lives in exactly one place. Computing it in SQL as well would mean
	// two formulas that agree until somebody changes one.
	var attempts int
	if err := tx.QueryRow(ctx,
		`SELECT attempts FROM integration.outbox WHERE id = $1 FOR UPDATE`,
		eventID).Scan(&attempts); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // already gone; nothing to record against
		}
		return fmt.Errorf("outbox: reading attempts: %w", err)
	}
	attempts++

	var deadAt *time.Time
	if attempts >= MaxAttempts {
		now := time.Now().UTC()
		deadAt = &now
	}

	if _, err := tx.Exec(ctx, `
		UPDATE integration.outbox
		SET attempts = $2,
		    last_error = $3,
		    next_attempt_at = now() + make_interval(secs => $4),
		    dead_at = $5
		WHERE id = $1`,
		eventID, attempts, reason, Backoff(attempts).Seconds(), deadAt); err != nil {
		return fmt.Errorf("outbox: marking failed: %w", err)
	}

	return tx.Commit(ctx)
}

// Backoff returns how long to wait before attempt number n.
//
// Exponential with a cap. The cap matters more than the curve: without it, a
// tenth attempt would be days away and an endpoint that came back an hour ago
// would still be waiting.
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	const (
		base = 10 * time.Second
		cap  = 30 * time.Minute
	)
	wait := time.Duration(math.Pow(2, float64(attempt-1))) * base
	if wait > cap || wait <= 0 {
		return cap
	}
	return wait
}
