package interview

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Session start: SES-02. The command that turns a ready session into a
// connecting one, with quota reserved first and a room grant minted last.
//
// The order is the safety argument. Quota is reserved before anything
// transitions or spends, so the refusal ADR-0014 promises happens before
// recording could begin; the ledger's exactly-once guard turns a retried
// reservation into success already achieved, so a crash between reserve and
// transition converges instead of double-billing; and the grant is minted
// only after the transition owns the start, so a token never exists for a
// session that did not move. Nothing consults quota after this function:
// an interview already running cannot be interrupted by a quota event
// because no code path asks.

// startJoinWindow is how long the minted grant admits joining. Joining is
// for now; reconnection mints a fresh grant through its own flow.
const startJoinWindow = 2 * time.Minute

// StartLedger is what start needs from billing, declared here per ADR-0005
// and wired in cmd. ErrQuotaExhausted and already-metered convergence are
// the contract; interview never sees the ledger itself.
type StartLedger interface {
	// ReserveStart admits one session under the tenant's quota or refuses.
	// Reserving the same session again reports already-metered, which start
	// treats as its own retry.
	ReserveStart(ctx context.Context, tenantID, sessionID, mode string) error
}

// ErrLedgerAlreadyMetered is how the cmd adapter reports the convergence
// case without interview importing billing.
var ErrLedgerAlreadyMetered = errors.New("interview: this session's start is already metered")

// RoomGrants is what start needs from the realtime plane: one join grant,
// scoped to one room and one identity, short-lived.
type RoomGrants interface {
	MintJoin(room, identity string, ttl time.Duration) (RoomGrant, error)
}

// RoomGrant is what the browser needs to join, and nothing more.
type RoomGrant struct {
	URL       string
	Room      string
	Token     string
	ExpiresAt time.Time
}

// The distinct refusals SES-02 requires. Somebody else's session is not
// among them: the store's owner scoping makes it ErrNotFound, because
// existence is not answered across owners, and that refusal is this
// product's unauthorized.
var (
	ErrStartExpired        = errors.New("interview: SESSION_EXPIRED: this session has expired; set up a fresh one")
	ErrStartAlreadyStarted = errors.New("interview: SESSION_ALREADY_STARTED: this session has already started")
	ErrStartNotReady       = errors.New("interview: SESSION_NOT_READY: this session is not ready to start")
	ErrStartQuotaExhausted = errors.New("interview: QUOTA_EXHAUSTED: the workspace is at its session limit")
)

// Started is the start command's answer: the session as it now is, and the
// grant to join it.
type Started struct {
	Session Session
	Grant   RoomGrant
}

// Starter runs the start command.
type Starter struct {
	store  *Store
	events *Events
	ledger StartLedger
	grants RoomGrants
}

// NewStarter wires the command.
func NewStarter(store *Store, ledger StartLedger, grants RoomGrants) *Starter {
	return &Starter{store: store, events: NewEvents(store), ledger: ledger, grants: grants}
}

// Start moves one ready session to connecting and mints its join grant.
func (s *Starter) Start(ctx context.Context, sessionID, mode, candidateID, tenantID string) (Started, error) {
	session, err := s.store.Get(ctx, sessionID, mode, candidateID, tenantID)
	if err != nil {
		return Started{}, err
	}

	switch session.State {
	case StateReady:
		// The one state start applies to.
	case StateExpired:
		return Started{}, ErrStartExpired
	case StateConnecting, StateInProgress, StateReconnecting, StateFinalizing,
		StateEvaluating, StateReviewReady, StateArchived:
		return Started{}, ErrStartAlreadyStarted
	default:
		// Draft, composing, cancelled, the failed states: each has its own
		// screen; none of them is startable.
		return Started{}, ErrStartNotReady
	}

	// Screening spends a tenant's quota; practice has no tenant and no
	// quota in this sense (ADR-0014).
	if session.Mode == "screening" {
		err := s.ledger.ReserveStart(ctx, session.TenantID, session.ID, session.Mode)
		switch {
		case errors.Is(err, ErrLedgerAlreadyMetered):
			// A previous attempt reserved and then died before the
			// transition, or this is a concurrent retry. The reservation
			// stands; proceed to own the start.
		case err != nil:
			return Started{}, translateLedger(err)
		}
	}

	actor := Actor{ID: candidateID, Type: "user"}
	started, err := s.store.Transition(ctx, session, StateConnecting, Effects{}, actor)
	if errors.Is(err, ErrStaleVersion) {
		// Two starts raced; the other one owns it now.
		return Started{}, ErrStartAlreadyStarted
	}
	if err != nil {
		return Started{}, err
	}

	// The start opens epoch one: the protocol's timeline begins here, and
	// every control event names the epoch it belongs to. A crash between
	// the transition and this line leaves a connecting session without an
	// attempt, which the resume flow heals by beginning the next epoch.
	epoch, err := s.events.BeginAttempt(ctx, started)
	if err != nil {
		return Started{}, err
	}
	started.ConnectionEpoch = epoch

	grant, err := s.grants.MintJoin(started.ID, candidateID, startJoinWindow)
	if err != nil {
		return Started{}, fmt.Errorf("interview: minting the room grant: %w", err)
	}
	return Started{Session: started, Grant: grant}, nil
}

// translateLedger keeps the quota refusal's identity without interview
// importing billing's sentinel.
func translateLedger(err error) error {
	if err != nil && errorsIsQuota(err) {
		return ErrStartQuotaExhausted
	}
	return err
}

// errorsIsQuota matches the adapter's wrapped refusal by unwrapping to the
// sentinel the adapter attaches.
func errorsIsQuota(err error) bool {
	return errors.Is(err, ErrStartQuotaExhausted)
}
