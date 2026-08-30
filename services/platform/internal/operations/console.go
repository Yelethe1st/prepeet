package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/operations/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// The operator's half of OPS-03: what failed, and the two things a person may
// do about it under pressure.
//
// The design constraint is that both actions are safe to take while an incident
// is running, by somebody who has not read this file. That is why there are two
// of them and not more, why each is a state transition the queue either makes
// or reports it did not make, and why neither can happen without an audit row
// in the same transaction.

// Item is one piece of failed work as an operator sees it.
//
// Kind rather than Type, because the console shows work from a queue whose
// units happen to be events today; nothing here reads a payload, and nothing
// here should start to. An operator decides from what kind of work it is, whose
// it is, how long it has been failing and what the failure said.
type Item struct {
	ID       string
	Kind     string
	TenantID string
	// OccurredAt is when the underlying fact happened and FailedAt when the
	// queue gave up on it. Both, because together they say whether this is a
	// fresh incident or a week-old row nobody was watching.
	OccurredAt time.Time
	FailedAt   time.Time
	Attempts   int
	// LastError is the delivery failure, already scrubbed by whatever recorded
	// it. It is shown so an operator can tell "the endpoint was down" from "the
	// endpoint rejected this specific item", which decide differently.
	LastError string
}

// WorkQueue is everything the console needs from whatever holds the work.
//
// Declared here, by the consumer, and implemented by the outbox with an adapter
// in cmd, per ADR-0005. The two mutating methods take the caller's transaction
// deliberately: this package writes the audit row, the queue makes the
// transition, and the requirement is that neither happens without the other.
//
// Both return a boolean rather than an error for "nothing changed", because
// nothing changing is an ordinary outcome here and not a fault. Two operators
// working the same queue will race, and the loser has to be told they lost
// rather than shown a failure that looks like a bug.
type WorkQueue interface {
	BacklogSource

	Failed(ctx context.Context, limit int) ([]Item, error)
	Recover(ctx context.Context, tx pgx.Tx, itemID string) (changed bool, err error)
	Discard(ctx context.Context, tx pgx.Tx, itemID, reason string) (changed bool, err error)
}

// The refusals. Each is a sentinel because each has a different answer, and an
// operator reading a string is an operator guessing.
var (
	// ErrNotRecoverable means the item is not failed work any more: somebody
	// else retried it, it was discarded, it was delivered, or it never existed.
	// One error for all of them, because the operator's next move is the same:
	// look at the list again.
	ErrNotRecoverable = errors.New("operations: that work is no longer waiting for an operator")

	// ErrReasonRequired refuses an action that could not be reviewed later. A
	// blank reason in an audit trail is worse than no row, because it looks
	// like diligence.
	ErrReasonRequired = errors.New("operations: an operator action needs a reason")

	// ErrOperatorRequired refuses an anonymous action. The audit row is bound
	// to the acting user by the table's own policy, so an action with no actor
	// is one that cannot be recorded, and an action that cannot be recorded
	// must not happen.
	ErrOperatorRequired = errors.New("operations: an operator action needs an actor")
)

// The audit actions, as they appear in the trail. Constants rather than
// literals at the call site, because these strings are what somebody greps for
// during a review a year from now.
const (
	actionRetried   = "operations.work_retried"
	actionDiscarded = "operations.work_discarded"
)

// Operator is who is acting and under which request.
//
// RequestID is carried into the audit row so the record and the trace describe
// the same moment, per the telemetry conventions. It is optional; the actor is
// not.
type Operator struct {
	UserID    string
	RequestID string
}

// Console is the operations surface over failed work.
//
// It holds the pool as well as the queue because the transaction is its own to
// open: the audit row and the queue's transition belong to the same commit, and
// only the party that writes both can guarantee that.
type Console struct {
	pool  *pgxpool.Pool
	queue WorkQueue
	q     *db.Queries
}

// NewConsole builds the console over a queue.
func NewConsole(pool *pgxpool.Pool, queue WorkQueue) *Console {
	return &Console{pool: pool, queue: queue, q: db.New(pool)}
}

// Backlog reports the current depth already judged against the budgets.
//
// It returns the assessment rather than the raw depth so that a console and the
// alert cannot disagree about what is healthy. Two thresholds, one in the
// monitor and one on a screen, is how an operator ends up looking at a green
// dashboard while a pager goes off.
func (c *Console) Backlog(ctx context.Context) (Assessment, error) {
	depth, err := c.queue.Depth(ctx)
	if err != nil {
		return Assessment{}, fmt.Errorf("operations: reading the backlog: %w", err)
	}
	return Assess(depth), nil
}

// DefaultFailedLimit and MaxFailedLimit bound the failed-work listing.
//
// A page rather than everything, because the one time this list is opened in
// anger is when something has failed thousands of times, and that is the worst
// possible moment to send an unbounded query and a screen full of rows.
const (
	DefaultFailedLimit = 50
	MaxFailedLimit     = 200
)

// Failed lists work waiting for a decision, newest failure first.
//
// limit of zero means the default rather than nothing, because "show me the
// failures" is the request being made and an empty answer to it would be read
// as good news.
func (c *Console) Failed(ctx context.Context, limit int) ([]Item, error) {
	switch {
	case limit <= 0:
		limit = DefaultFailedLimit
	case limit > MaxFailedLimit:
		limit = MaxFailedLimit
	}

	items, err := c.queue.Failed(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("operations: listing failed work: %w", err)
	}
	return items, nil
}

// Retry returns one failed item to the queue.
//
// It does not deliver anything itself, and that is the design. The item goes
// back to pending and the ordinary dispatcher picks it up, so a retry travels
// the same path as a first attempt: the same handler, the same workflow
// identity, the same duplicate rejection. A retry that delivered directly would
// be a second delivery path, and a second path is a second set of guarantees to
// keep in step.
//
// Duplicate work is prevented twice over, in two independent places. Here, by
// the transition: only work that is still dead-lettered can be revived, so a
// second retry of the same item changes nothing and is refused. And downstream,
// by workflow identity: the handler starts a workflow whose id is derived from
// the entity, with the duplicate-rejection reuse policy, so even a redelivery
// that does happen starts nothing twice. The first stops the operator making
// the mistake; the second means it would not matter if they did.
func (c *Console) Retry(ctx context.Context, operator Operator, itemID, reason string) error {
	return c.act(ctx, operator, itemID, reason, actionRetried,
		func(tx pgx.Tx) (bool, error) { return c.queue.Recover(ctx, tx, itemID) })
}

// Discard marks one failed item as never to be delivered.
//
// The destructive one, and the reason both actions demand a reason. Discarding
// is a decision that some downstream system will never learn about something
// that happened, and the only thing that makes it reviewable afterwards is the
// sentence the operator typed at the time.
func (c *Console) Discard(ctx context.Context, operator Operator, itemID, reason string) error {
	return c.act(ctx, operator, itemID, reason, actionDiscarded,
		func(tx pgx.Tx) (bool, error) { return c.queue.Discard(ctx, tx, itemID, reason) })
}

// act performs one operator transition and records it.
//
// The shape is the point, and it is the same for both actions:
//
//   - Refusals that can be decided without touching anything are decided first,
//     so a missing reason never becomes a half-done action.
//   - The transition and the audit row share one transaction. If the audit
//     write fails, the transition is rolled back with it, which is what makes
//     "every operator action here is audited" a property of the code rather
//     than a habit.
//   - A transition that did not happen is still audited, in a transaction of
//     its own. It has to be separate: there is no effect to be atomic with, and
//     rolling the refusal record back with the empty transition would lose
//     exactly the evidence that two people are working the same queue.
func (c *Console) act(ctx context.Context, operator Operator, itemID, reason, action string,
	transition func(pgx.Tx) (bool, error)) error {
	if strings.TrimSpace(operator.UserID) == "" {
		return ErrOperatorRequired
	}
	if strings.TrimSpace(reason) == "" {
		return ErrReasonRequired
	}

	detail, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return fmt.Errorf("operations: encoding the audit detail: %w", err)
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("operations: beginning %s: %w", action, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The audit table's untenanted policy binds a row to the acting user, so
	// this is what makes the insert below legal as well as attributable.
	if err := database.SetUser(ctx, tx, operator.UserID); err != nil {
		return err
	}

	changed, err := transition(tx)
	if err != nil {
		return fmt.Errorf("operations: %s: %w", action, err)
	}
	if !changed {
		// Rolled back by the deferred call: there is nothing to keep, and the
		// refusal is recorded on its own connection below.
		_ = tx.Rollback(ctx)
		return errors.Join(ErrNotRecoverable, c.audit(ctx, operator, itemID, action, "denied", detail))
	}

	if err := c.auditWithin(ctx, tx, operator, itemID, action, "allowed", detail); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// audit records an action in a transaction of its own.
func (c *Console) audit(ctx context.Context, operator Operator, itemID, action, outcome string, detail []byte) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("operations: beginning the audit record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := database.SetUser(ctx, tx, operator.UserID); err != nil {
		return err
	}
	if err := c.auditWithin(ctx, tx, operator, itemID, action, outcome, detail); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// auditWithin writes the audit row inside a transaction the caller owns.
func (c *Console) auditWithin(ctx context.Context, tx pgx.Tx, operator Operator,
	itemID, action, outcome string, detail []byte) error {
	if err := db.New(tx).InsertRecoveryAudit(ctx, db.InsertRecoveryAuditParams{
		ID:        id.New().String(),
		ActorID:   operator.UserID,
		Action:    action,
		SubjectID: itemID,
		Outcome:   outcome,
		Detail:    detail,
		RequestID: operator.RequestID,
	}); err != nil {
		return fmt.Errorf("operations: auditing %s: %w", action, err)
	}
	return nil
}
