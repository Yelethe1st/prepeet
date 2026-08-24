package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/broadcast"
	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// WakeupTopic is the signal that the outbox has rows waiting.
//
// It is a latency optimisation and nothing more. Correctness comes from
// polling: every claim is a query against next_attempt_at, so an event is
// delivered whether or not any signal about it arrived. That separation is what
// makes it safe for the wakeup to travel over a transport which openly loses
// messages.
//
// Getting this the other way round is the classic outbox bug. A dispatcher that
// only ran when notified stops forever the first time a notification is missed,
// and notifications are missed routinely: the listener is reconnecting, the
// process was starting, the notification arrived before the subscription did.
const WakeupTopic = "outbox_pending"

// Defaults for a dispatcher. Each is overridable, and each is chosen rather
// than inherited.
const (
	// defaultBatch is how many events one claim takes.
	//
	// Small enough that a dispatcher dying mid-batch redelivers little, large
	// enough that a backlog drains without a query per event. The claim holds a
	// row lock for the whole batch, so a large batch also means a large lock.
	defaultBatch = 50

	// defaultPollInterval is the longest the dispatcher waits before looking
	// anyway.
	//
	// This is the number that decides how long an event can sit if every signal
	// about it was lost. Five seconds is short enough that nobody notices and
	// long enough that an idle deployment is not running a query per second per
	// task for nothing.
	defaultPollInterval = 5 * time.Second

	// defaultDeliveryTimeout bounds one delivery attempt. Without it a handler
	// that hangs holds its claim until the visibility window expires, and the
	// dispatcher delivers nothing else in the meantime.
	defaultDeliveryTimeout = 30 * time.Second
)

// Handler delivers one event to wherever it is going.
//
// It is defined here, by the consumer, rather than by any of the packages that
// will implement it, per ADR-0005. A webhook sender, a Temporal signal and a
// projection updater are all handlers, and none of them needs to know the
// others exist.
//
// An error means the attempt failed and should be retried. Returning nil for an
// event that did not arrive is the one thing a handler must not do, because the
// dispatcher will then mark it delivered and nothing will ever retry it.
type Handler interface {
	Deliver(ctx context.Context, event Pending) error
}

// EventStore is what a dispatcher needs from the outbox.
//
// *Store satisfies it. The interface exists so the delivery loop can be tested
// without a database: the interesting behaviour here is retry, panic
// containment and when to wait, none of which is about SQL, and requiring
// PostgreSQL to assert any of it would mean asserting less of it.
type EventStore interface {
	Claim(ctx context.Context, limit int) ([]Pending, error)
	MarkDelivered(ctx context.Context, eventID string) error
	MarkFailed(ctx context.Context, eventID, reason string) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(ctx context.Context, event Pending) error

func (f HandlerFunc) Deliver(ctx context.Context, event Pending) error { return f(ctx, event) }

// DispatcherOptions configures a dispatcher. The zero value is usable.
type DispatcherOptions struct {
	Batch           int
	PollInterval    time.Duration
	DeliveryTimeout time.Duration
	Logger          *slog.Logger
}

func (o DispatcherOptions) withDefaults() DispatcherOptions {
	if o.Batch <= 0 {
		o.Batch = defaultBatch
	}
	if o.PollInterval <= 0 {
		o.PollInterval = defaultPollInterval
	}
	if o.DeliveryTimeout <= 0 {
		o.DeliveryTimeout = defaultDeliveryTimeout
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

// Dispatcher carries events from the outbox to their consumers.
//
// Without one, Publish writes rows that accumulate forever: the outbox is a
// table plus a promise, and this is the half that keeps the promise.
//
// Several may run at once. Claim uses FOR UPDATE SKIP LOCKED, so a second
// dispatcher steps over what the first is holding and adds throughput rather
// than duplicating work. That is the property ADR-0006 chose over a lock
// service, and the integration tests exercise it directly.
type Dispatcher struct {
	store   EventStore
	handler Handler
	wakeups broadcast.Broadcaster
	opts    DispatcherOptions
}

// NewDispatcher builds a dispatcher.
//
// wakeups may be nil, in which case the dispatcher polls only. That is a
// supported configuration rather than a degraded one: it costs latency and
// nothing else, which is precisely the claim the design rests on.
func NewDispatcher(store EventStore, handler Handler, wakeups broadcast.Broadcaster, opts DispatcherOptions) *Dispatcher {
	return &Dispatcher{
		store:   store,
		handler: handler,
		wakeups: wakeups,
		opts:    opts.withDefaults(),
	}
}

// Run delivers events until ctx is cancelled.
//
// It returns nil on cancellation, because a dispatcher stopping because it was
// asked to is not a failure and should not make a process exit non-zero.
func (d *Dispatcher) Run(ctx context.Context) error {
	log := d.opts.Logger

	// The subscription is opened before the first drain, so an event published
	// during startup is signalled rather than waiting for the first poll.
	var wakeups <-chan broadcast.Message
	if d.wakeups != nil {
		subscription, err := d.wakeups.Subscribe(ctx, WakeupTopic)
		if err != nil {
			// Not fatal. Losing the signal costs latency, and polling still
			// delivers everything. Refusing to start over it would trade a
			// working dispatcher for a fast one.
			log.Warn("outbox dispatcher could not subscribe to wakeups and will poll only",
				slog.String("error", err.Error()))
		} else {
			defer func() { _ = subscription.Close() }()
			wakeups = subscription.Messages()
		}
	}

	timer := time.NewTimer(d.opts.PollInterval)
	defer timer.Stop()

	for {
		// Drain before waiting, so a dispatcher starting up with a backlog
		// clears it rather than sleeping first.
		drained, err := d.drain(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// A failed claim is usually the database being briefly unavailable.
			// The dispatcher waits and tries again rather than exiting, because
			// a process that exits here would be restarted into the same
			// condition by the orchestrator.
			log.Error("outbox dispatcher could not claim events",
				slog.String("error", telemetry.Scrub(err.Error())))
		}

		// A full batch means there is probably more, so the next claim happens
		// immediately rather than after a wait. Without this a backlog drains at
		// one batch per poll interval, which for a large backlog is hours.
		if drained == d.opts.Batch {
			continue
		}

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(d.opts.PollInterval)

		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		case _, open := <-wakeups:
			if !open {
				// The subscription ended. Polling continues, so this is a
				// latency change rather than a failure.
				wakeups = nil
				log.Warn("outbox dispatcher lost its wakeup subscription and is polling only")
			}
		}
	}
}

// drain claims one batch and attempts delivery of each event in it. It returns
// how many were claimed, which is what tells Run whether to go straight round
// again.
func (d *Dispatcher) drain(ctx context.Context) (int, error) {
	claimed, err := d.store.Claim(ctx, d.opts.Batch)
	if err != nil {
		return 0, err
	}

	for _, event := range claimed {
		if ctx.Err() != nil {
			// Stopping mid-batch is safe and is the right thing during
			// shutdown. The remaining events keep their claim until the
			// visibility window expires, then another dispatcher takes them.
			return len(claimed), nil
		}
		d.deliver(ctx, event)
	}
	return len(claimed), nil
}

// deliver attempts one event and records the outcome.
func (d *Dispatcher) deliver(ctx context.Context, event Pending) {
	log := d.opts.Logger

	ctx, span := telemetry.Tracer("platform/outbox").Start(ctx, "outbox.deliver")
	defer span.End()
	span.SetAttributes(
		telemetry.MustAttr(telemetry.KeyEventID, event.ID),
		telemetry.MustAttr(telemetry.KeyEventType, event.Type),
	)

	attemptCtx, cancel := context.WithTimeout(ctx, d.opts.DeliveryTimeout)
	deliveryErr := d.safeDeliver(attemptCtx, event)
	cancel()

	// Recording the outcome uses the parent context rather than the attempt
	// one. Using the attempt context would mean a delivery that timed out could
	// not record that it timed out, and the event would look untried.
	if deliveryErr != nil {
		span.SetAttributes(telemetry.MustAttr(telemetry.KeyOutcome, "failed"))

		if err := d.store.MarkFailed(ctx, event.ID, deliveryErr.Error()); err != nil {
			// The event keeps its claim and is retried when the visibility
			// window expires, so this is a lost record rather than a lost
			// event.
			log.Error("outbox dispatcher could not record a failed delivery",
				slog.String("error", telemetry.Scrub(err.Error())))
		}
		return
	}

	span.SetAttributes(telemetry.MustAttr(telemetry.KeyOutcome, "delivered"))

	if err := d.store.MarkDelivered(ctx, event.ID); err != nil {
		// The event was delivered but not marked, so it will be delivered
		// again. That is the at-least-once guarantee showing its edges, and it
		// is why consumers deduplicate by event id.
		log.Error("outbox dispatcher delivered an event but could not mark it",
			slog.String("error", telemetry.Scrub(err.Error())))
	}
}

// safeDeliver runs a handler and converts a panic into a failed attempt.
//
// A handler is written by another context and reaches a provider, a webhook
// endpoint or a workflow client. A panic in one must not take down the
// dispatcher, because that would stop delivery of every other event including
// the ones that would have succeeded.
func (d *Dispatcher) safeDeliver(ctx context.Context, event Pending) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("handler panicked: %s", telemetry.Scrub(fmt.Sprint(recovered)))
		}
	}()

	if deliveryErr := d.handler.Deliver(ctx, event); deliveryErr != nil {
		return errors.New(telemetry.Scrub(deliveryErr.Error()))
	}
	return nil
}

// Compile-time proof that the real store satisfies what the dispatcher needs.
var _ EventStore = (*Store)(nil)
