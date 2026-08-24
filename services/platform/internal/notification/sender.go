package notification

import (
	"context"
	"log/slog"
	"time"
)

// Transport carries one rendered email to the outside world.
//
// Declared here rather than imported, per ADR-0005's direction: this package
// consumes the capability, so this package says how narrow it is. Plain
// strings so that platform/email satisfies it structurally without either
// package importing the other.
type Transport interface {
	// Send delivers one message, or returns why it could not. Every error is
	// treated as retryable up to MaxAttempts, because SMTP's permanent-failure
	// signals are unreliable enough that distinguishing them is the provider
	// feedback problem INT-01 records as outstanding.
	Send(ctx context.Context, recipient, subject, body string) error
}

// Sender drains the queue through a transport.
type Sender struct {
	queue     *Queue
	transport Transport
	log       *slog.Logger

	// interval is how often the queue is polled. A poll, not a push: at email
	// volumes a cheap indexed query every few seconds is simpler than a wakeup
	// channel, and a verification email arriving two seconds later is
	// invisible next to mailbox latency.
	interval time.Duration
}

// batchSize bounds one claim, so a backlog drains in visible increments and a
// crash loses at most one batch of claims to the visibility window.
const batchSize = 20

// NewSender builds the sender.
func NewSender(queue *Queue, transport Transport, log *slog.Logger) *Sender {
	return &Sender{queue: queue, transport: transport, log: log, interval: 3 * time.Second}
}

// Run drains until the context ends. It returns nil on cancellation, because
// shutdown is the one way it is supposed to stop.
func (s *Sender) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		s.drain(ctx)

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// drain claims and sends until the queue is empty or the context ends.
func (s *Sender) drain(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		claimed, err := s.queue.Claim(ctx, batchSize)
		if err != nil {
			// A claim failure is a database problem the next tick retries;
			// logging is all there is to do that stopping would not make worse.
			s.log.Error("claiming emails", slog.String("error", err.Error()))
			return
		}
		if len(claimed) == 0 {
			return
		}

		for _, email := range claimed {
			s.deliver(ctx, email)
		}
	}
}

// deliver sends one email and records the outcome.
func (s *Sender) deliver(ctx context.Context, email Pending) {
	if err := s.transport.Send(ctx, email.Recipient, email.Subject, email.Body); err != nil {
		// The recipient address is deliberately absent from the log: it is
		// personal data, and the mail id is enough to find the row.
		s.log.Warn("email delivery failed",
			slog.String("mail_id", email.ID),
			slog.Int("attempts", email.Attempts+1),
			slog.String("error", err.Error()))
		if err := s.queue.MarkFailed(ctx, email.ID, err.Error()); err != nil {
			s.log.Error("recording delivery failure", slog.String("error", err.Error()))
		}
		return
	}

	if err := s.queue.MarkSent(ctx, email.ID); err != nil {
		// The email went out and the record failed, so the visibility window
		// will offer it to a sender again: at-least-once, exactly like the
		// outbox, and why every message says its link works once.
		s.log.Error("recording delivery", slog.String("mail_id", email.ID),
			slog.String("error", err.Error()))
	}
}
