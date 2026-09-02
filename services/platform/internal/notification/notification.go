// Package notification owns transactional email: what is sent, in which
// version of which wording, and whether it arrived.
//
// It does not decide when an email is warranted. The context that owns the
// state change enqueues in its own transaction, and this package carries the
// message out. See README.md for what it must never do.
//
// Implements INT-01.
package notification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/notification/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Queue enqueues and drains emails.
//
// The SQL lives in db/queries.sql per ADR-0010. What stays here is what sqlc
// cannot say: which statements share the caller's transaction, and how a
// failure becomes a retry or a dead letter.
type Queue struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewQueue builds the queue.
func NewQueue(pool *pgxpool.Pool) *Queue {
	return &Queue{pool: pool, q: db.New(pool)}
}

// Enqueue renders the template and stores the email inside the caller's
// transaction.
//
// It takes the transaction for the outbox's reason: an email promised by a
// transaction that rolls back must vanish with it, and a token committed
// without its email is a token nobody can ever use. Rendering happens here,
// against the version recorded on the row, so the send path applies no logic
// and a template edit cannot change a message that was already promised.
func (n *Queue) Enqueue(ctx context.Context, tx pgx.Tx, recipient string, input Input) (string, error) {
	if recipient == "" {
		return "", errors.New("notification: an email needs a recipient")
	}

	rendered, err := Render(input)
	if err != nil {
		return "", err
	}

	mailID := id.New().String()
	if err := db.New(tx).Enqueue(ctx, db.EnqueueParams{
		ID:              mailID,
		Recipient:       recipient,
		Template:        rendered.Template,
		TemplateVersion: rendered.Version,
		Subject:         rendered.Subject,
		Body:            rendered.Body,
	}); err != nil {
		return "", fmt.Errorf("notification: enqueueing %s: %w", rendered.Template, err)
	}
	return mailID, nil
}

// Pending is one claimed email, ready to hand to the transport.
type Pending struct {
	ID        string
	Recipient string
	Subject   string
	Body      string
	Attempts  int
}

// claimVisibility hides a claimed email from other senders while one attempts
// it. Long enough for a slow SMTP conversation, short enough that a sender
// killed mid-send does not strand the email.
const claimVisibility = 2 * time.Minute

// Claim takes up to limit emails for sending, in the outbox's claim shape.
func (n *Queue) Claim(ctx context.Context, limit int) ([]Pending, error) {
	tx, err := n.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("notification: beginning claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)
	rows, err := q.Claim(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("notification: claiming emails: %w", err)
	}

	claimed := make([]Pending, 0, len(rows))
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		claimed = append(claimed, Pending{
			ID:        row.ID,
			Recipient: row.Recipient,
			Subject:   row.Subject,
			Body:      row.Body,
			Attempts:  int(row.Attempts),
		})
		ids = append(ids, row.ID)
	}

	if len(ids) > 0 {
		if err := q.HideClaimed(ctx, db.HideClaimedParams{
			Ids: ids, Visibility: claimVisibility.String(),
		}); err != nil {
			return nil, fmt.Errorf("notification: marking claim: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("notification: committing claim: %w", err)
	}
	return claimed, nil
}

// MarkSent records delivery and erases the content in the same statement.
func (n *Queue) MarkSent(ctx context.Context, mailID string) error {
	if err := n.q.MarkSent(ctx, mailID); err != nil {
		return fmt.Errorf("notification: marking sent: %w", err)
	}
	return nil
}

// MaxAttempts is how many failures dead-letter an email.
//
// Fewer than the outbox allows its events, because a token email that cannot
// be delivered inside its own expiry window is not worth delivering: the link
// inside it will refuse anyway, and the person has long since asked again.
const MaxAttempts = 5

// MarkFailed records a failed attempt and schedules the next, or dead-letters
// after MaxAttempts.
func (n *Queue) MarkFailed(ctx context.Context, mailID, reason string) error {
	if len(reason) > 1000 {
		reason = reason[:1000]
	}

	tx, err := n.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("notification: beginning failure record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)
	locked, err := q.LockAttempts(ctx, mailID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // already gone; nothing to record against
		}
		return fmt.Errorf("notification: reading attempts: %w", err)
	}
	attempts := int(locked) + 1

	var deadAt *time.Time
	if attempts >= MaxAttempts {
		now := time.Now().UTC()
		deadAt = &now
	}

	if err := q.RecordFailure(ctx, db.RecordFailureParams{
		ID:             mailID,
		Attempts:       int32(attempts),
		LastError:      reason,
		BackoffSeconds: backoff(attempts).Seconds(),
		DeadAt:         deadAt,
	}); err != nil {
		return fmt.Errorf("notification: marking failed: %w", err)
	}
	return tx.Commit(ctx)
}

// backoff is how long to wait before attempt n: exponential from ten seconds,
// capped at five minutes. The cap matters more than the curve, and it is far
// tighter than the outbox's because the content expires.
func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	wait := 10 * time.Second << (attempt - 1)
	if wait > 5*time.Minute {
		return 5 * time.Minute
	}
	return wait
}

// Delivery is what became of one enqueued email, for the context that sent it.
//
// It carries the state a recruiter needs to decide whether to resend and
// nothing a recruiter has no business seeing: never the body, which is nulled
// at send, and never the recipient's provider, only the outcome.
type Delivery struct {
	// Status is one of the DeliveryStatus* values below.
	Status string
	// Attempts is how many times the sender has tried, for a pending email
	// that is taking a while versus one stuck behind repeated failures.
	Attempts int
	// LastError is the most recent transport failure, empty when there is
	// none. Present so a recruiter is told why, not only that.
	LastError string
}

// The delivery states a caller reads. Ordered from the terminal bad outcomes
// down to the ordinary ones, because that is the order that decides what a
// recruiter should do: a bounce or a complaint needs a new address, a dead
// letter needs a resend, a sent email needs nothing, and a pending one needs
// only patience.
const (
	// DeliveryUnknown is an id with no row: an email that a transaction
	// promised and then rolled back, or one never enqueued. Not "delivered"
	// and not "failed", because neither would be honest about a row that is
	// not there.
	DeliveryUnknown = "unknown"
	// DeliveryBounced means the provider rejected it at the recipient: a wrong
	// or dead address, which a resend to the same address will not fix.
	DeliveryBounced = "bounced"
	// DeliveryComplained means the recipient marked it spam. Sending more is
	// the one response that makes it worse.
	DeliveryComplained = "complained"
	// DeliveryFailed means the sender gave up after its retries: a dead
	// letter, which a resend may still clear if the cause was transient.
	DeliveryFailed = "failed"
	// DeliverySent means it was handed to the transport without error. Not a
	// read receipt: it is the furthest this side can honestly claim.
	DeliverySent = "sent"
	// DeliveryPending means it is still queued or mid-retry.
	DeliveryPending = "pending"
)

// DeliveryStatus answers what became of one email by its id.
//
// The precedence is deliberate: a bounce or a complaint is a fact about the
// address that outlives a later send attempt, so it wins over sent; a dead
// letter is terminal for this send; sent beats pending. An id with no row is
// unknown rather than guessed, because the caller may hold an id whose
// transaction rolled back and a confident answer there would be a lie.
func (n *Queue) DeliveryStatus(ctx context.Context, emailID string) (Delivery, error) {
	row, err := n.q.DeliveryStatusByID(ctx, emailID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{Status: DeliveryUnknown}, nil
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("notification: reading delivery status: %w", err)
	}

	delivery := Delivery{Attempts: int(row.Attempts)}
	if row.LastError.Valid {
		delivery.LastError = row.LastError.String
	}
	switch {
	case row.BouncedAt != nil:
		delivery.Status = DeliveryBounced
	case row.ComplainedAt != nil:
		delivery.Status = DeliveryComplained
	case row.DeadAt != nil:
		delivery.Status = DeliveryFailed
	case row.SentAt != nil:
		delivery.Status = DeliverySent
	default:
		delivery.Status = DeliveryPending
	}
	return delivery, nil
}
