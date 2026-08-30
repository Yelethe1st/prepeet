// Package wiring holds the adapters that join a bounded context to the
// infrastructure it declared a port for.
//
// It lives under cmd because that is the only place allowed to see both sides,
// per ADR-0005: a context declares the interface it needs, the infrastructure
// knows nothing about the context, and the translation happens where the
// program is composed. It is a package rather than code inside one main only
// because two commands compose the same port - the worker, which measures the
// backlog, and opsctl, through which a person acts on it - and two copies of a
// translation are two places for it to drift.
//
// Nothing here decides anything. A decision made in wiring is a decision nobody
// reviews.
package wiring

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/operations"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// Backlog presents the outbox as the work queue the operations context asked
// for.
type Backlog struct{ events *outbox.Store }

// NewBacklog adapts an outbox store.
func NewBacklog(events *outbox.Store) Backlog { return Backlog{events: events} }

// Depth measures the queue.
func (b Backlog) Depth(ctx context.Context) (operations.Depth, error) {
	backlog, err := b.events.Backlog(ctx)
	if err != nil {
		return operations.Depth{}, err
	}
	return operations.Depth{
		Pending:       backlog.Pending,
		Failed:        backlog.Failed,
		OldestPending: backlog.OldestPending,
	}, nil
}

// Failed lists work waiting for an operator.
func (b Backlog) Failed(ctx context.Context, limit int) ([]operations.Item, error) {
	failed, err := b.events.FailedEvents(ctx, limit)
	if err != nil {
		return nil, err
	}

	items := make([]operations.Item, 0, len(failed))
	for _, event := range failed {
		items = append(items, operations.Item{
			ID:         event.ID,
			Kind:       event.Type,
			TenantID:   event.TenantID,
			OccurredAt: event.OccurredAt,
			FailedAt:   event.DeadAt,
			Attempts:   event.Attempts,
			LastError:  event.LastError,
		})
	}
	return items, nil
}

// Recover returns one failed event to the dispatcher, inside the console's
// transaction so the retry and the audit row saying who ordered it commit
// together.
func (b Backlog) Recover(ctx context.Context, tx pgx.Tx, itemID string) (bool, error) {
	return b.events.Recover(ctx, tx, itemID)
}

// Discard marks one failed event as never to be delivered, in the same
// transaction as its audit row, for the same reason.
func (b Backlog) Discard(ctx context.Context, tx pgx.Tx, itemID, reason string) (bool, error) {
	return b.events.Discard(ctx, tx, itemID, reason)
}

// Compile-time proof that the outbox satisfies what operations asked for. A
// port whose implementation is only checked when the program is assembled is a
// port that breaks in main.
var _ operations.WorkQueue = Backlog{}
