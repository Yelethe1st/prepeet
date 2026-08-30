// Package operations is the platform's own view of the work it owes: how much
// durable work is waiting, how old it is, what has failed for good, and the two
// things an operator may do about it.
//
// It exists as a bounded context rather than as a handful of queries because
// the interesting part is not the counting. It is the judgement: at what age a
// backlog stops being throughput and becomes a candidate waiting, and what a
// retry is allowed to do to work that has already had side effects. Both are
// decisions, both belong somewhere reviewable, and neither belongs in a
// dashboard query somebody edits during an incident.
//
// It owns no tables. The work it reports on belongs to the outbox, reached
// through the port below and wired in cmd per ADR-0005, and the record of what
// an operator did belongs to audit.events, which is the one table shared by
// every context precisely so that the trail can be read in one query.
//
// Implements OPS-03.
package operations

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// Depth is the backlog at one moment.
//
// Age is carried alongside the counts because a count on its own cannot be
// alerted on. Ten thousand items delivered within a second is a busy system;
// three items that have sat for ten minutes is an incident, and only the age
// tells the two apart.
type Depth struct {
	// Pending is undelivered work that is still being attempted.
	Pending int
	// Failed is work that has exhausted its attempts and now needs a person.
	Failed int
	// OldestPending is how long the oldest undelivered item has existed,
	// measured from when the fact occurred rather than from the last attempt,
	// because that is the wait the candidate experiences.
	OldestPending time.Duration
}

// The numbers the alert is derived from, named rather than folded into the
// threshold, so the derivation can be re-run when a promise changes.
const (
	// ReviewJourneyBudget is the tighter of the two completion-to-review
	// objectives in docs/operations/service-level-objectives.md: practice at
	// p95 under three minutes, screening at five. The alert is designed
	// against the tighter one, because an alert tuned to the looser promise
	// would let practice breach silently.
	ReviewJourneyBudget = 3 * time.Minute

	// QueueHopsPerJourney is how many times a completed session crosses the
	// outbox before its result is reviewable: once to start the evaluation
	// workflow from interview.session_completed.v1, and once more to move the
	// session to review_ready from evaluation.completed.v1. Both hops spend
	// the same budget, so the threshold has to be a per-hop share of it rather
	// than the whole thing.
	QueueHopsPerJourney = 2

	// FirstRetryCycle is what an ordinary, self-healing retry costs: up to one
	// dispatcher poll interval to be claimed, the first backoff after a failed
	// attempt, and one more poll interval to be claimed again. Roughly twenty
	// seconds with the dispatcher's current settings.
	//
	// The threshold has to sit above this, or a provider blinking once pages
	// somebody, and an alert that pages for a blink is an alert operators
	// learn to close without reading. The numbers are restated here rather
	// than imported from the outbox: this context does not depend on the
	// infrastructure it reports on, and a change to the dispatcher's backoff
	// therefore has to be reflected here deliberately.
	FirstRetryCycle = 20 * time.Second
)

// PendingAgeBudget is how old undelivered work may get before somebody is told.
//
// Thirty seconds, derived rather than chosen. A completed session crosses the
// queue twice inside a three minute promise, so if both hops sat at this
// threshold the queue alone would have spent sixty seconds, a third of the
// budget, leaving the evaluation itself the two minutes it actually needs. It
// is also above the twenty seconds an ordinary retry costs, so a single failed
// attempt does not page: what does page is a second consecutive failure or a
// dispatcher that has stopped draining, which are both conditions no amount of
// waiting fixes.
//
// The point of the derivation is the direction of the error. The alert fires
// while two thirds of the candidate's budget is still unspent, which is what
// "before candidates notice a delay" has to mean to be checkable.
const PendingAgeBudget = 30 * time.Second

// Assessment is a measured backlog judged against the budgets above.
//
// The two indicators are kept apart because they mean different things to
// whoever is woken up. An age breach is predictive: the work is still moving
// and there is time to act. A failed-work breach is terminal: attempts are
// exhausted, nothing will retry on its own, and somebody has to decide.
type Assessment struct {
	Depth          Depth
	AgeBreached    bool
	FailedBreached bool
}

// Breached reports whether anything needs an operator.
func (a Assessment) Breached() bool { return a.AgeBreached || a.FailedBreached }

// Summary is one line naming what breached and by how much.
//
// It exists so the alert carries its own diagnosis. An alert that says only
// that a threshold was crossed sends somebody to a dashboard to find out which
// one, and that trip is minutes long at three in the morning.
func (a Assessment) Summary() string {
	if !a.Breached() {
		return fmt.Sprintf("backlog healthy: %d pending, oldest %s", a.Depth.Pending, a.Depth.OldestPending)
	}
	var reasons []string
	if a.AgeBreached {
		reasons = append(reasons, fmt.Sprintf("oldest pending work is %s, over the %s budget",
			a.Depth.OldestPending.Round(time.Second), PendingAgeBudget))
	}
	if a.FailedBreached {
		reasons = append(reasons, fmt.Sprintf("%d items have failed for good and need an operator", a.Depth.Failed))
	}
	return strings.Join(reasons, "; ")
}

// Assess judges a measurement against the budgets.
//
// A function rather than a method on Depth, so the judgement can be applied to
// a depth that came from anywhere: a monitor, a console request, or a test that
// wants to know what the system would have done.
func Assess(depth Depth) Assessment {
	return Assessment{
		Depth:       depth,
		AgeBreached: depth.OldestPending >= PendingAgeBudget,
		// Any dead-lettered item at all. By the time attempts are exhausted
		// the work has been undeliverable for the better part of a day, which
		// is far past every journey budget in the objectives: there is no
		// count of these that is acceptable and no waiting that improves it.
		FailedBreached: depth.Failed > 0,
	}
}

// BacklogSource is what the monitor needs in order to measure.
//
// Declared here, by the consumer, rather than by the outbox that implements it,
// per ADR-0005. It is deliberately the narrowest of this package's two ports:
// measuring must not require the ability to retry or discard anything, so a
// monitor wired by mistake into the console's dependencies still cannot mutate
// work.
type BacklogSource interface {
	Depth(ctx context.Context) (Depth, error)
}

// MonitorInterval is how often the backlog is measured.
//
// A quarter of the age budget, so a breach is seen within a quarter of the
// window it describes rather than being discovered a whole budget late. Cheap
// enough to run against an indexed count: this is two aggregate queries, not a
// scan of the table.
const MonitorInterval = PendingAgeBudget / 4

// Monitor measures the backlog on a schedule and says when it breaches.
//
// It is the half of OPS-03 that has to run whether or not anybody is looking.
// The console below answers questions somebody asked; this asks the question
// nobody thought to.
type Monitor struct {
	source BacklogSource
	log    *slog.Logger

	pending   metric.Int64Gauge
	failed    metric.Int64Gauge
	oldestAge metric.Float64Gauge

	// breaching remembers the last verdict so recovery can be reported once
	// rather than every interval. An operator watching an incident needs to
	// see it end; they do not need to be told every interval that it has not
	// started again.
	breaching bool
}

// NewMonitor builds a monitor.
//
// The instruments are created here rather than at package initialisation
// because an instrument binds to whichever meter provider is installed when it
// is built, and one built during initialisation binds to the noop provider that
// exists before telemetry.Setup runs. It would then record nothing, silently,
// for the life of the process.
func NewMonitor(source BacklogSource, log *slog.Logger) *Monitor {
	if log == nil {
		log = slog.Default()
	}
	meter := telemetry.Meter("internal/operations")
	return &Monitor{
		source:    source,
		log:       log,
		pending:   mustInt64Gauge(meter, "prepeet.outbox.pending", "Undelivered durable work waiting in the outbox."),
		failed:    mustInt64Gauge(meter, "prepeet.outbox.failed", "Durable work that has exhausted its delivery attempts."),
		oldestAge: mustFloat64Gauge(meter, "prepeet.outbox.oldest_pending_age", "Age of the oldest undelivered item, in seconds."),
	}
}

// mustInt64Gauge builds a gauge or panics.
//
// Only a malformed instrument definition can fail here, which is a programming
// error in the lines above rather than a runtime condition, and a monitor that
// started without its instruments would report a healthy dashboard because
// nothing was being recorded.
func mustInt64Gauge(meter metric.Meter, name, description string) metric.Int64Gauge {
	instrument, err := meter.Int64Gauge(name, metric.WithDescription(description))
	if err != nil {
		panic("operations: building " + name + ": " + err.Error())
	}
	return instrument
}

// mustFloat64Gauge builds a gauge or panics, for the same reason as above.
func mustFloat64Gauge(meter metric.Meter, name, description string) metric.Float64Gauge {
	instrument, err := meter.Float64Gauge(name, metric.WithDescription(description), metric.WithUnit("s"))
	if err != nil {
		panic("operations: building " + name + ": " + err.Error())
	}
	return instrument
}

// Measure takes one reading, records it and reports what it means.
//
// Separate from Run so the decision can be exercised without a clock, and so a
// console or a health endpoint can take the same reading the alert is based on
// rather than a second one that might disagree.
func (m *Monitor) Measure(ctx context.Context) (Assessment, error) {
	depth, err := m.source.Depth(ctx)
	if err != nil {
		return Assessment{}, fmt.Errorf("operations: measuring the backlog: %w", err)
	}

	m.pending.Record(ctx, int64(depth.Pending))
	m.failed.Record(ctx, int64(depth.Failed))
	m.oldestAge.Record(ctx, depth.OldestPending.Seconds())

	assessment := Assess(depth)
	switch {
	case assessment.Breached():
		m.breaching = true
		// Warning rather than error: the system is still working, and this is
		// the alert that is supposed to arrive before anything is broken.
		m.log.WarnContext(ctx, "the work backlog is over its budget",
			slog.String("summary", assessment.Summary()),
			slog.Int("pending", depth.Pending),
			slog.Int("failed", depth.Failed),
			slog.Float64("oldest_pending_seconds", depth.OldestPending.Seconds()))
	case m.breaching:
		m.breaching = false
		m.log.InfoContext(ctx, "the work backlog has recovered",
			slog.Int("pending", depth.Pending))
	}
	return assessment, nil
}

// Run measures until ctx is cancelled.
//
// It returns nothing. There is no failure here a caller could act on: an
// unreadable backlog is logged and retried on the next tick, because a monitor
// that exited on a transient database error would take the alerting with it at
// exactly the moment alerting matters.
func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(MonitorInterval)
	defer ticker.Stop()

	for {
		// Measured before waiting, so a process that starts into an existing
		// backlog says so immediately rather than one interval later.
		if _, err := m.Measure(ctx); err != nil && ctx.Err() == nil {
			m.log.ErrorContext(ctx, "the work backlog could not be measured",
				slog.String("error", telemetry.Scrub(err.Error())))
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
