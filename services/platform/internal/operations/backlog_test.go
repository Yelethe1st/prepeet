package operations_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/operations"
)

// The alerting half of OPS-03, which is a decision rather than a query: what
// depth and what age mean a candidate is about to wait longer than the service
// level objective promises.
//
// These run without a database on purpose. The threshold is the thing under
// test, and a threshold asserted through PostgreSQL is a threshold nobody
// re-derives when the objective changes.

// The derivation itself is a test, because the number is only defensible while
// the arithmetic behind it holds. If somebody widens the alert to quieten it,
// this fails and says which promise the new number breaks.
func TestTheBacklogAgeBudgetLeavesTheJourneyBudgetIntact(t *testing.T) {
	t.Parallel()

	// Both hops at the threshold must still leave the journey most of its
	// budget, or the alert fires after the candidate has already waited.
	spentIfBothHopsBreach := operations.QueueHopsPerJourney * operations.PendingAgeBudget
	if spentIfBothHopsBreach > operations.ReviewJourneyBudget/3 {
		t.Errorf("a backlog at the threshold on every hop spends %s of the %s journey budget, which is more than a third",
			spentIfBothHopsBreach, operations.ReviewJourneyBudget)
	}

	// And it must sit above ordinary retrying, or the alert pages for a
	// provider blinking once and operators learn to ignore it.
	if operations.PendingAgeBudget <= operations.FirstRetryCycle {
		t.Errorf("the budget %s is inside the first retry cycle %s, so a single failed attempt pages",
			operations.PendingAgeBudget, operations.FirstRetryCycle)
	}
}

func TestAnEmptyBacklogIsHealthy(t *testing.T) {
	t.Parallel()

	assessment := operations.Assess(operations.Depth{})
	if assessment.Breached() {
		t.Errorf("an empty backlog is breaching: %s", assessment.Summary())
	}
}

func TestADeepButYoungBacklogIsHealthy(t *testing.T) {
	t.Parallel()

	// Depth alone says nothing: ten thousand items delivered in a second is a
	// busy system, and alerting on it would page for success.
	assessment := operations.Assess(operations.Depth{Pending: 10_000, OldestPending: time.Second})
	if assessment.Breached() {
		t.Errorf("a deep but young backlog is breaching: %s", assessment.Summary())
	}
}

func TestWorkOlderThanTheBudgetBreaches(t *testing.T) {
	t.Parallel()

	assessment := operations.Assess(operations.Depth{
		Pending: 1, OldestPending: operations.PendingAgeBudget + time.Second,
	})
	if !assessment.AgeBreached {
		t.Error("work older than the budget is not breaching")
	}
	if !assessment.Breached() {
		t.Error("an age breach does not count as a breach")
	}
	if !strings.Contains(assessment.Summary(), "oldest") {
		t.Errorf("the summary does not say what breached: %q", assessment.Summary())
	}
}

// One item short of the budget must not alert, or every threshold argument
// above is off by one and the alert fires earlier than it was designed to.
func TestWorkInsideTheBudgetDoesNotBreach(t *testing.T) {
	t.Parallel()

	assessment := operations.Assess(operations.Depth{
		Pending: 1, OldestPending: operations.PendingAgeBudget - time.Millisecond,
	})
	if assessment.AgeBreached {
		t.Error("work inside the budget is breaching")
	}
}

// Dead-lettered work is the terminal indicator rather than the early one: by
// the time attempts are exhausted the wait is already hours long, so one item
// is enough.
func TestOneFailedItemBreaches(t *testing.T) {
	t.Parallel()

	assessment := operations.Assess(operations.Depth{Failed: 1})
	if !assessment.FailedBreached {
		t.Error("dead-lettered work is not breaching")
	}
	if !strings.Contains(assessment.Summary(), "failed") {
		t.Errorf("the summary does not name the failed work: %q", assessment.Summary())
	}
}

// fakeQueue is a backlog source that answers whatever the test needs.
type fakeQueue struct {
	depth operations.Depth
	err   error
	calls int
}

func (f *fakeQueue) Depth(context.Context) (operations.Depth, error) {
	f.calls++
	return f.depth, f.err
}

func TestTheMonitorWarnsWhenTheBacklogBreaches(t *testing.T) {
	t.Parallel()

	var log bytes.Buffer
	monitor := operations.NewMonitor(&fakeQueue{depth: operations.Depth{
		Pending: 3, OldestPending: 10 * time.Minute,
	}}, slog.New(slog.NewTextHandler(&log, nil)))

	assessment, err := monitor.Measure(context.Background())
	if err != nil {
		t.Fatalf("measuring: %v", err)
	}
	if !assessment.Breached() {
		t.Fatal("a ten minute old backlog did not breach")
	}
	if !strings.Contains(log.String(), "level=WARN") {
		t.Errorf("the breach was not logged at warning level: %s", log.String())
	}
}

// A backlog that clears has to say so, or the operator watching the incident
// cannot tell recovery from a monitor that stopped running.
func TestTheMonitorReportsRecoveryOnce(t *testing.T) {
	t.Parallel()

	var log bytes.Buffer
	queue := &fakeQueue{depth: operations.Depth{Pending: 3, OldestPending: 10 * time.Minute}}
	monitor := operations.NewMonitor(queue, slog.New(slog.NewTextHandler(&log, nil)))

	if _, err := monitor.Measure(context.Background()); err != nil {
		t.Fatalf("measuring: %v", err)
	}
	queue.depth = operations.Depth{}
	log.Reset()
	if _, err := monitor.Measure(context.Background()); err != nil {
		t.Fatalf("measuring: %v", err)
	}
	if !strings.Contains(log.String(), "recovered") {
		t.Errorf("recovery was not reported: %s", log.String())
	}

	// And only once: a healthy system must not log a recovery every interval.
	log.Reset()
	if _, err := monitor.Measure(context.Background()); err != nil {
		t.Fatalf("measuring: %v", err)
	}
	if log.Len() != 0 {
		t.Errorf("a healthy backlog is still logging: %s", log.String())
	}
}

func TestTheMonitorReportsAnUnreadableBacklog(t *testing.T) {
	t.Parallel()

	broken := errors.New("no database")
	monitor := operations.NewMonitor(&fakeQueue{err: broken}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))

	if _, err := monitor.Measure(context.Background()); !errors.Is(err, broken) {
		t.Errorf("a failed measurement returned %v, want the underlying error", err)
	}
}

// The loop stops when it is asked to. A monitor that outlives its context keeps
// a shutting-down worker alive.
func TestTheMonitorStopsWithItsContext(t *testing.T) {
	t.Parallel()

	queue := &fakeQueue{}
	monitor := operations.NewMonitor(queue, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		monitor.Run(ctx)
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the monitor did not stop when its context was cancelled")
	}
	if queue.calls == 0 {
		t.Error("the monitor never measured, so it would have alerted on nothing")
	}
}
