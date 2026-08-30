package outbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/platform/outbox/db"
)

// The operator's view of this table, and the two transitions they may make.
//
// It lives in the outbox rather than in whatever context serves the console,
// because this is the outbox's own state machine. A second package writing
// dead_at and discarded_at would be a second place that has to agree with
// Claim about what "waiting" means, and the two would agree until one of them
// changed. What the console owns is the decision and the audit; what happens to
// the row is here.
//
// Implements part of OPS-03.

// Backlog is how much undelivered work exists and how long the oldest of it has
// been waiting.
//
// The age is part of the measurement rather than a separate query because the
// two are only meaningful together. Depth without age cannot distinguish a busy
// minute from an outage, and alerting on depth alone pages for success.
type Backlog struct {
	// Pending is undelivered work that is still being attempted.
	Pending int
	// Failed is work whose attempts are exhausted, waiting for a person.
	Failed int
	// OldestPending is measured from when the fact occurred, not from the last
	// attempt: that is the wait somebody downstream is experiencing.
	OldestPending time.Duration
}

// Failed is one dead-lettered event as an operator needs to see it.
//
// It carries no payload. An operator deciding whether to retry needs to know
// what kind of work it is, whose it is, how long it has been failing and why;
// none of those is in the payload, and the payload is the one field that could
// carry something an operations screen has no business displaying.
type Failed struct {
	ID       string
	Type     string
	TenantID string
	// OccurredAt is when the fact happened, which with DeadAt is what says
	// whether this is a fresh incident or a week-old row nobody noticed.
	OccurredAt time.Time
	DeadAt     time.Time
	Attempts   int
	// LastError is the delivery failure, already scrubbed and truncated on the
	// way in by MarkFailed.
	LastError string
}

// Backlog measures the queue.
//
// Two aggregates rather than one query with filters, because each matches a
// partial index exactly and a single scan with FILTER clauses would match
// neither. This runs every few seconds forever, against a table that keeps
// delivered rows.
func (s *Store) Backlog(ctx context.Context) (Backlog, error) {
	pending, err := s.q.PendingBacklog(ctx)
	if err != nil {
		return Backlog{}, fmt.Errorf("outbox: measuring pending work: %w", err)
	}
	failed, err := s.q.FailedBacklog(ctx)
	if err != nil {
		return Backlog{}, fmt.Errorf("outbox: measuring failed work: %w", err)
	}

	return Backlog{
		Pending: int(pending.Depth),
		Failed:  int(failed),
		// Seconds arrive as a float from PostgreSQL and become a Duration here,
		// so no caller has to remember which unit this number was in.
		OldestPending: time.Duration(pending.OldestSeconds * float64(time.Second)),
	}, nil
}

// FailedEvents lists dead-lettered work, newest failure first.
func (s *Store) FailedEvents(ctx context.Context, limit int) ([]Failed, error) {
	rows, err := s.q.ListFailed(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("outbox: listing failed events: %w", err)
	}

	failed := make([]Failed, 0, len(rows))
	for _, row := range rows {
		item := Failed{
			ID: row.ID, Type: row.EventType, TenantID: row.TenantID,
			OccurredAt: row.OccurredAt, Attempts: int(row.Attempts),
			LastError: row.LastError,
		}
		// dead_at is nullable on the column even though this query selects only
		// rows where it is set, so the generated type is a pointer and the
		// dereference has to be guarded rather than assumed.
		if row.DeadAt != nil {
			item.DeadAt = *row.DeadAt
		}
		failed = append(failed, item)
	}
	return failed, nil
}

// Recover returns one dead-lettered event to the queue, inside the caller's
// transaction.
//
// It takes the transaction for the same reason Publish does: the caller has
// something to write alongside it - the audit row saying who decided this - and
// a retry recorded without its audit row, or audited without happening, is
// worse than either alone.
//
// The boolean is the guarantee. Only work that is still failed, still
// undelivered and not discarded can be revived, and false means the row did not
// match: already retried by somebody else, already delivered, already
// discarded, or never there. A caller must not treat false as success, and this
// is what makes two operators retrying the same item deliver it once.
func (s *Store) Recover(ctx context.Context, tx pgx.Tx, eventID string) (bool, error) {
	_, err := db.New(tx).RecoverFailed(ctx, eventID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("outbox: recovering %s: %w", eventID, err)
	}
	return true, nil
}

// Discard marks one dead-lettered event as never to be delivered, inside the
// caller's transaction.
//
// A state transition rather than a delete, deliberately. The row is the only
// remaining answer to what happened to that work, and a DELETE against a table
// under forced row-level security removes nothing and raises nothing when no
// policy matches, which is a silent failure this project has already met.
//
// The boolean means the same as Recover's: false is a transition that did not
// happen, and the caller has to say so rather than report a discard it did not
// make.
func (s *Store) Discard(ctx context.Context, tx pgx.Tx, eventID, reason string) (bool, error) {
	// The same truncation MarkFailed applies, for the same reason: this is read
	// in a list, and an operator pasting a stack trace should not make the list
	// unreadable.
	if len(reason) > 1000 {
		reason = reason[:1000]
	}

	_, err := db.New(tx).DiscardFailed(ctx, db.DiscardFailedParams{ID: eventID, Reason: reason})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("outbox: discarding %s: %w", eventID, err)
	}
	return true, nil
}
