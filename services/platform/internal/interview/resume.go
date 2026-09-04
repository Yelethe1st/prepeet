package interview

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Session resume: SES-06, to realtime-protocol.md's reconnection contract.
//
// A dropped session continues as itself or not at all. Resume opens the
// next connection epoch, which supersedes whatever tab still believes it
// holds the session - the stale epoch's next batch is refused whole, so one
// candidate has one live connection without any lock. The answer carries
// the recovery cursor: what the previous epoch durably holds and the exact
// gaps still owed, because the client rebuilds itself from the server's
// record and resends what is missing rather than trusting its own memory.
//
// Resume authorizes recovery; it does not pretend one happened. A
// reconnecting session stays reconnecting until the new connection's
// established event arrives, exactly as the first attempt's did, and the
// grace window is enforced from the policy stamped at start: expiry is
// SES-06's finalization path, never a restart.

// resumeJoinWindow is how long the fresh grant admits joining, the same
// window a start's grant carries: joining is for now.
const resumeJoinWindow = 2 * time.Minute

// The distinct refusals resume owes the client.
var (
	// ErrResumeNotResumable means no interview is in flight: nothing
	// started, or the record is already frozen. Start and redo are the
	// front doors; resume cannot reopen a sealed conversation.
	ErrResumeNotResumable = errors.New("interview: SESSION_NOT_RESUMABLE: no interview is in flight to resume")
	// ErrResumeGraceExpired means the reconnection window lapsed. What was
	// captured is finalized with the interruption recorded as coverage;
	// answering anything else here would quietly turn a policy decision
	// into a race against the finalizer.
	ErrResumeGraceExpired = errors.New("interview: GRACE_EXPIRED: the reconnection window has lapsed")
)

// Resumed is the resume command's answer: the session on its new epoch, the
// fresh grant, the stamped timing, and the recovery cursor.
type Resumed struct {
	Session Session
	Grant   RoomGrant
	Timing  TimingPolicy
	// PreviousEpoch is the attempt this resume superseded; zero when the
	// first attempt died before it opened.
	PreviousEpoch int
	// AcceptedSequence is the highest contiguous sequence the previous
	// epoch durably holds; Missing lists the gaps under its highest stored
	// slot, for the client to resend.
	AcceptedSequence int
	Missing          []SequenceRange
}

// Resumer runs the resume command.
type Resumer struct {
	store  *Store
	events *Events
	grants RoomGrants
}

// NewResumer wires the command.
func NewResumer(store *Store, grants RoomGrants) *Resumer {
	return &Resumer{store: store, events: NewEvents(store), grants: grants}
}

// Resume opens the next connection attempt for a session the caller lost.
func (r *Resumer) Resume(ctx context.Context, sessionID, mode, candidateID, tenantID string) (Resumed, error) {
	session, err := r.store.Get(ctx, sessionID, mode, candidateID, tenantID)
	if err != nil {
		return Resumed{}, err
	}

	switch session.State {
	case StateConnecting, StateInProgress, StateReconnecting:
		// The states with an interview to continue: an attempt that died
		// before opening, a drop the server has not seen yet, and a drop it
		// has. Everything else has nothing in flight.
	default:
		return Resumed{}, ErrResumeNotResumable
	}

	// The stamped policy governs, never the current one: a policy published
	// mid-session must not move a window the candidate is already inside.
	// A session that crashed before its stamp is healed with the current
	// policy exactly as start would have stamped it, first-write-wins.
	policy, err := r.timingOf(ctx, session)
	if err != nil {
		return Resumed{}, err
	}
	if session.State == StateReconnecting {
		deadline := session.StateChangedAt.Add(time.Duration(policy.ReconnectGraceSeconds) * time.Second)
		if time.Now().After(deadline) {
			return Resumed{}, ErrResumeGraceExpired
		}
	}

	// The recovery cursor is read before the takeover: what the epoch being
	// superseded durably holds, which is what the client must reconcile
	// against.
	previousEpoch := session.ConnectionEpoch
	accepted, missing := 0, []SequenceRange(nil)
	if previousEpoch > 0 {
		accepted, missing, err = r.events.CursorOf(ctx, session, previousEpoch)
		if err != nil {
			return Resumed{}, err
		}
	}

	if _, err := r.events.BeginAttempt(ctx, session); err != nil {
		// A concurrent resume advanced past us: its takeover stands, and
		// the refusal already names it.
		return Resumed{}, err
	}
	current, err := r.store.Get(ctx, sessionID, mode, candidateID, tenantID)
	if err != nil {
		return Resumed{}, err
	}

	grant, err := r.grants.MintJoin(current.ID, candidateID, resumeJoinWindow)
	if err != nil {
		return Resumed{}, fmt.Errorf("interview: minting the resume grant: %w", err)
	}
	return Resumed{
		Session: current, Grant: grant, Timing: policy,
		PreviousEpoch: previousEpoch, AcceptedSequence: accepted, Missing: missing,
	}, nil
}

// timingOf answers the policy stamped on the session, healing a missing
// stamp with the current policy the way start would have.
func (r *Resumer) timingOf(ctx context.Context, session Session) (TimingPolicy, error) {
	if session.TimingPolicyVersion > 0 {
		return r.store.TimingPolicyByVersion(ctx, session.TimingPolicyVersion)
	}
	policy, err := r.store.CurrentTimingPolicy(ctx)
	if err != nil {
		return TimingPolicy{}, err
	}
	if err := r.store.StampTimingPolicy(ctx, session, policy); err != nil {
		return TimingPolicy{}, err
	}
	return policy, nil
}
