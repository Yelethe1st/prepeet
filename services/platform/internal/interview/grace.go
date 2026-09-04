package interview

import (
	"context"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// Grace expiry: SES-06's finalization half, to session-lifecycle.md.
//
// When a reconnection window lapses without a resume, the platform finalizes
// what was captured rather than leaving the session parked in reconnecting
// forever: the interruption is recorded with grace_expired as its cause, and
// the same completion path a candidate's own complete runs through seals the
// record at the cursor it actually holds. The announcement says the session
// expired rather than completed, and coverage is the evaluation's to state
// from the sealed record - never a number invented here.
//
// The workflow starts from the session_interrupted announcement in the
// worker, one per drop, its identity naming the session and the attempt. It
// re-checks before acting: a session that resumed stands the timer down, and
// a window restarted by a later drop belongs to that drop's own timer.

// GraceInput names the session one drop's timer watches: identifiers only,
// per ADR-0007's payload rule.
type GraceInput struct {
	SessionID   string
	Mode        string
	CandidateID string
	TenantID    string
	// ActorID is whose authority the expiry acts under, recorded with Type
	// "service" because the automation acts for the candidate, not as them.
	ActorID string
}

// GraceCheck is what the timer learns before sleeping.
type GraceCheck struct {
	// Reconnecting reports whether there is still a window to wait out. A
	// session that resumed, completed, or expired already has none.
	Reconnecting bool
	// RemainingSeconds until the stamped window lapses; zero when due.
	RemainingSeconds int
}

// GraceActivities is the timer's side of the world, one instance per worker.
type GraceActivities struct {
	store     *Store
	completer *Completer
}

// NewGraceActivities wires the store and the completion path for a worker.
func NewGraceActivities(store *Store, completer *Completer) *GraceActivities {
	return &GraceActivities{store: store, completer: completer}
}

// RemainingGrace answers whether the session still waits on a recovery and
// for how much longer the stamped policy holds the window open.
func (a *GraceActivities) RemainingGrace(ctx context.Context, input GraceInput) (GraceCheck, error) {
	session, err := a.store.Get(ctx, input.SessionID, input.Mode, input.CandidateID, input.TenantID)
	if err != nil {
		return GraceCheck{}, err
	}
	if session.State != StateReconnecting {
		return GraceCheck{}, nil
	}
	deadline, err := a.deadlineOf(ctx, session)
	if err != nil {
		return GraceCheck{}, err
	}
	remaining := int(time.Until(deadline).Seconds())
	if remaining < 0 {
		remaining = 0
	}
	return GraceCheck{Reconnecting: true, RemainingSeconds: remaining}, nil
}

// ExpireGrace finalizes a session whose window has lapsed, and only such a
// session: one that resumed is left alone, and one whose window was
// restarted by a later drop belongs to that drop's own timer. Safe to
// re-run after a worker death - the fact converges on the one already
// recorded and the completion path converges on its own seal.
func (a *GraceActivities) ExpireGrace(ctx context.Context, input GraceInput) error {
	session, err := a.store.Get(ctx, input.SessionID, input.Mode, input.CandidateID, input.TenantID)
	if err != nil {
		return err
	}
	if session.State != StateReconnecting {
		return nil
	}
	deadline, err := a.deadlineOf(ctx, session)
	if err != nil {
		return err
	}
	if time.Now().Before(deadline) {
		return nil
	}

	actor := Actor{ID: input.ActorID, Type: "service"}

	// The fact first, converging on a previous attempt's: a retry that died
	// between the fact and the seal must not turn one expiry into two
	// interruptions, because the append-only table would keep both.
	recorded, err := a.store.Interruptions(ctx, input.SessionID, input.Mode, input.CandidateID, input.TenantID)
	if err != nil {
		return err
	}
	already := false
	for _, fact := range recorded {
		if fact.Cause == CauseGraceExpired && fact.ConnectionEpoch == session.ConnectionEpoch {
			already = true
			break
		}
	}
	if !already {
		// The duration is how long the candidate had been gone when the
		// window closed, measured from the drop the state row remembers.
		gone := int(time.Since(session.StateChangedAt).Seconds())
		if _, err := a.store.RecordInterruption(ctx, session, CauseGraceExpired, gone, actor); err != nil {
			return err
		}
	}

	// The same sealing path a candidate's own complete runs, at the cursor
	// the record actually holds, announced as expired rather than completed.
	_, err = a.completer.complete(ctx, input.SessionID, input.Mode, input.CandidateID, input.TenantID,
		session.ConnectionEpoch, session.AcceptedSequence, actor, "expired")
	return err
}

// deadlineOf computes when the session's window lapses, from the policy
// stamped at start: a policy published mid-session must not move a window
// the candidate is already inside. A session that somehow reached
// reconnecting without a stamp is judged by the current policy, read-only.
func (a *GraceActivities) deadlineOf(ctx context.Context, session Session) (time.Time, error) {
	var policy TimingPolicy
	var err error
	if session.TimingPolicyVersion > 0 {
		policy, err = a.store.TimingPolicyByVersion(ctx, session.TimingPolicyVersion)
	} else {
		policy, err = a.store.CurrentTimingPolicy(ctx)
	}
	if err != nil {
		return time.Time{}, err
	}
	return session.StateChangedAt.Add(time.Duration(policy.ReconnectGraceSeconds) * time.Second), nil
}

// GraceWorkflow waits out one drop's reconnection window and expires it.
//
// Durable where a process timer is not: a worker killed mid-wait resumes the
// same timer, and the identity ("grace-" + session + attempt) makes a
// redelivered announcement join the running timer instead of starting a
// second. The check runs before the sleep so a session that already
// recovered costs nothing, and again inside ExpireGrace because the world
// may have moved while the timer slept.
func GraceWorkflow(ctx workflow.Context, input GraceInput) error {
	options := workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    5,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, options)

	var activities *GraceActivities

	var check GraceCheck
	if err := workflow.ExecuteActivity(ctx, activities.RemainingGrace, input).Get(ctx, &check); err != nil {
		return err
	}
	if !check.Reconnecting {
		return nil
	}
	if check.RemainingSeconds > 0 {
		if err := workflow.Sleep(ctx, time.Duration(check.RemainingSeconds)*time.Second); err != nil {
			return err
		}
	}
	return workflow.ExecuteActivity(ctx, activities.ExpireGrace, input).Get(ctx, nil)
}
