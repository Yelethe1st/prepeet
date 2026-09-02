//go:build integration

package interview_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// SES-02 against real PostgreSQL: the distinct refusals, the ordering that
// keeps quota ahead of spend, the convergence of a retried start, and the
// property that no quota event can reach an interview already running.
//
// The ledger here is a scripted fake implementing the port interview
// declares: the real boundary arithmetic is billing's own proven suite, and
// what this file owns is the start command's control flow around it.

// scriptedLedger implements interview.StartLedger.
type scriptedLedger struct {
	mu       sync.Mutex
	reserved map[string]bool
	limit    int
	err      error
}

func newScriptedLedger(limit int) *scriptedLedger {
	return &scriptedLedger{reserved: map[string]bool{}, limit: limit}
}

func (l *scriptedLedger) ReserveStart(_ context.Context, _, sessionID, _ string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return l.err
	}
	if l.reserved[sessionID] {
		return interview.ErrLedgerAlreadyMetered
	}
	if len(l.reserved) >= l.limit {
		return interview.ErrStartQuotaExhausted
	}
	l.reserved[sessionID] = true
	return nil
}

// grantRecorder implements interview.RoomGrants.
type grantRecorder struct {
	mu     sync.Mutex
	minted []string
	fail   bool
}

func (g *grantRecorder) MintJoin(room, identity string, ttl time.Duration) (interview.RoomGrant, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fail {
		return interview.RoomGrant{}, errors.New("the signer is misconfigured")
	}
	g.minted = append(g.minted, room+":"+identity)
	return interview.RoomGrant{
		URL: "wss://rtc.test", Room: room, Token: "tok-" + room,
		ExpiresAt: time.Now().Add(ttl),
	}, nil
}

// readySession composes a practice session up to ready.
func readySession(t *testing.T) interview.Session {
	t.Helper()
	ctx := context.Background()
	store := interview.NewStore(pool)
	session := createPractice(t)
	composing, err := store.Transition(ctx, session, interview.StateComposing, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("to composing: %v", err)
	}
	effects := interview.Effects{
		BundleRef: "bundles/" + session.ID, BundleDigest: "sha256:d", BundleRevision: 1,
	}
	event, err := interview.ReadyEvent(composing, effects, candidate)
	if err != nil {
		t.Fatalf("ready event: %v", err)
	}
	effects.Event = event
	ready, err := store.Transition(ctx, composing, interview.StateReady, effects, candidate)
	if err != nil {
		t.Fatalf("to ready: %v", err)
	}
	return ready
}

// readyScreening seeds a screening session at ready, directly.
func readyScreening(t *testing.T) interview.Session {
	t.Helper()
	ctx := context.Background()
	store := interview.NewStore(pool)
	session := interview.Session{
		ID: id.New().String(), Mode: "screening", CandidateID: candidateID,
		TenantID: tenantID, CampaignID: seedCampaign(t), BlueprintID: "bp_screen",
	}
	if err := store.Create(ctx, session, candidate); err != nil {
		t.Fatalf("create screening: %v", err)
	}
	created, err := store.Get(ctx, session.ID, "screening", candidateID, tenantID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	composing, err := store.Transition(ctx, created, interview.StateComposing, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("to composing: %v", err)
	}
	ready, err := store.Transition(ctx, composing, interview.StateReady, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("to ready: %v", err)
	}
	return ready
}

func TestStartMovesAReadySessionAndMintsItsGrant(t *testing.T) {
	ctx := context.Background()
	ledger := newScriptedLedger(10)
	grants := &grantRecorder{}
	starter := interview.NewStarter(interview.NewStore(pool), ledger, grants)
	session := readySession(t)

	started, err := starter.Start(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.Session.State != interview.StateConnecting {
		t.Fatalf("state = %s, want connecting", started.Session.State)
	}
	// The grant is scoped to exactly this session and this person.
	if started.Grant.Room != session.ID || started.Grant.Token == "" || started.Grant.URL == "" {
		t.Fatalf("grant = %+v", started.Grant)
	}
	if grants.minted[0] != session.ID+":"+candidateID {
		t.Fatalf("minted %v", grants.minted)
	}
	// Practice touches no quota: no tenant, no reservation (ADR-0014).
	if len(ledger.reserved) != 0 {
		t.Fatal("a practice start reserved tenant quota")
	}

	// SES-05: the start stamped the timing policy in force and answered
	// its values, so the client compiles in no grace constant and the
	// session stays reconstructable after the policy moves on.
	if started.Timing.Version != 1 || started.Timing.ReconnectGraceSeconds != 120 || started.Timing.MaxOverrunSeconds != 300 {
		t.Fatalf("timing = %+v, want the seeded v1 policy", started.Timing)
	}
	stamped, err := interview.NewStore(pool).Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("reread: %v", err)
	}
	if stamped.TimingPolicyVersion != 1 {
		t.Fatalf("the session's stamp is %d, want 1", stamped.TimingPolicyVersion)
	}
}

func TestEachRefusalIsItsOwn(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	starter := interview.NewStarter(store, newScriptedLedger(10), &grantRecorder{})

	// Not ready: a composing session has no start to offer.
	composing, err := store.Transition(ctx, createPractice(t), interview.StateComposing, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("to composing: %v", err)
	}
	if _, err := starter.Start(ctx, composing.ID, "practice", candidateID, ""); !errors.Is(err, interview.ErrStartNotReady) {
		t.Fatalf("composing = %v, want ErrStartNotReady", err)
	}

	// Already started: the second start finds connecting, not ready.
	session := readySession(t)
	if _, err := starter.Start(ctx, session.ID, "practice", candidateID, ""); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, err := starter.Start(ctx, session.ID, "practice", candidateID, ""); !errors.Is(err, interview.ErrStartAlreadyStarted) {
		t.Fatalf("second start = %v, want ErrStartAlreadyStarted", err)
	}

	// Expired: its own words, not a generic not-ready.
	expired := readySession(t)
	if _, err := store.Transition(ctx, expired, interview.StateExpired, interview.Effects{}, candidate); err != nil {
		t.Fatalf("to expired: %v", err)
	}
	if _, err := starter.Start(ctx, expired.ID, "practice", candidateID, ""); !errors.Is(err, interview.ErrStartExpired) {
		t.Fatalf("expired = %v, want ErrStartExpired", err)
	}

	// Unauthorized is this product's not-found: somebody else's session
	// does not exist for this caller, and existence is not answered.
	other := readySession(t)
	if _, err := starter.Start(ctx, other.ID, "practice", recruiterID, ""); !errors.Is(err, interview.ErrNotFound) {
		t.Fatalf("someone else's session = %v, want ErrNotFound", err)
	}
}

func TestScreeningReservesBeforeStartingAndRefusesAtTheLimit(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	ledger := newScriptedLedger(1)
	starter := interview.NewStarter(store, ledger, &grantRecorder{})

	first := readyScreening(t)
	second := readyScreening(t)

	if _, err := starter.Start(ctx, first.ID, "screening", candidateID, tenantID); err != nil {
		t.Fatalf("first screening start: %v", err)
	}
	if !ledger.reserved[first.ID] {
		t.Fatal("the start did not reserve")
	}

	// Over quota: refused distinctly, and refused BEFORE any transition -
	// the session is still ready, so quota arriving later makes it
	// startable without repair.
	_, err := starter.Start(ctx, second.ID, "screening", candidateID, tenantID)
	if !errors.Is(err, interview.ErrStartQuotaExhausted) {
		t.Fatalf("over quota = %v, want ErrStartQuotaExhausted", err)
	}
	current, err := store.Get(ctx, second.ID, "screening", candidateID, tenantID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if current.State != interview.StateReady {
		t.Fatalf("the refused session moved to %s; the refusal was not before the spend", current.State)
	}
}

func TestARetriedStartConvergesOnTheExistingReservation(t *testing.T) {
	// The crash window: a previous attempt reserved, then died before the
	// transition. The retry meets already-metered and proceeds to own the
	// start rather than refusing or double-billing.
	ctx := context.Background()
	ledger := newScriptedLedger(5)
	starter := interview.NewStarter(interview.NewStore(pool), ledger, &grantRecorder{})
	session := readyScreening(t)

	if err := ledger.ReserveStart(ctx, tenantID, session.ID, "screening"); err != nil {
		t.Fatalf("pre-reserving: %v", err)
	}

	started, err := starter.Start(ctx, session.ID, "screening", candidateID, tenantID)
	if err != nil {
		t.Fatalf("retried start: %v", err)
	}
	if started.Session.State != interview.StateConnecting {
		t.Fatalf("state = %s", started.Session.State)
	}
}

func TestAQuotaEventAfterStartTouchesNothing(t *testing.T) {
	// The third box. The started interview keeps running - its transitions
	// keep working - whatever the ledger says afterwards, because nothing
	// after start consults it.
	ctx := context.Background()
	store := interview.NewStore(pool)
	ledger := newScriptedLedger(1)
	starter := interview.NewStarter(store, ledger, &grantRecorder{})

	session := readyScreening(t)
	started, err := starter.Start(ctx, session.ID, "screening", candidateID, tenantID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// The quota collapses to zero behind them.
	ledger.err = interview.ErrStartQuotaExhausted

	inProgress, err := store.Transition(ctx, started.Session, interview.StateInProgress, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("the running interview was refused a transition after a quota event: %v", err)
	}
	if inProgress.State != interview.StateInProgress {
		t.Fatalf("state = %s", inProgress.State)
	}
}

func TestConcurrentStartsAdmitExactlyOne(t *testing.T) {
	ctx := context.Background()
	starter := interview.NewStarter(interview.NewStore(pool), newScriptedLedger(10), &grantRecorder{})
	session := readySession(t)

	const racers = 6
	var wg sync.WaitGroup
	results := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			_, results[slot] = starter.Start(ctx, session.ID, "practice", candidateID, "")
		}(i)
	}
	wg.Wait()

	won := 0
	for _, err := range results {
		switch {
		case err == nil:
			won++
		case errors.Is(err, interview.ErrStartAlreadyStarted):
		default:
			t.Fatalf("a racer failed with %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d racers won the same session", won)
	}
}

func TestAFailedGrantLeavesAStartableWorld(t *testing.T) {
	// If the signer is down the session has moved to connecting but nobody
	// can join. The failure is loud, and connecting returns to ready on
	// abort per the machine, so the recovery path exists; what this pins is
	// that the error names the grant, not a mystery.
	ctx := context.Background()
	starter := interview.NewStarter(interview.NewStore(pool), newScriptedLedger(10), &grantRecorder{fail: true})
	session := readySession(t)

	_, err := starter.Start(ctx, session.ID, "practice", candidateID, "")
	if err == nil || !strings.Contains(err.Error(), "room grant") {
		t.Fatalf("a failed mint = %v, want a named grant failure", err)
	}
}
