package interview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/interview/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// The store: SES-01's durable half.
//
// Every transition is one transaction holding four things that must not
// separate: the version-guarded state change, the effects the transition
// carries (a bundle at readiness, a failure code at failure), the outbox
// event where the catalogue defines one, and the audit row naming who did it.
// A transition that commits without its audit row is an authorisation
// decision nobody can review; an event without its state change announces
// something that did not happen.
//
// The SQL lives in db/queries.sql per ADR-0010. What stays here is the
// transaction shape and how a zero-row update becomes the right refusal.

// Stable refusals beyond the machine's own. Callers branch on these; the API
// layer maps them to their own error codes rather than to a 500.
var (
	// ErrStaleVersion means somebody else transitioned the session after the
	// caller read it. The caller re-reads and decides again; overwriting
	// silently is SES-01's named failure.
	ErrStaleVersion = errors.New("interview: SESSION_VERSION_STALE: the session changed after it was read")
	// ErrNotFound means no such session is visible in this scope, which
	// deliberately covers both absence and somebody else's session.
	ErrNotFound = errors.New("interview: SESSION_NOT_FOUND: no such session")
)

// Session is the aggregate as read.
type Session struct {
	ID          string
	Mode        string
	CandidateID string
	// TenantID is empty for practice, by the schema's CHECK rather than by
	// convention.
	TenantID string
	// CampaignID is set for screening and empty for practice, by the schema's
	// CHECK: a screening session runs for exactly one campaign.
	CampaignID  string
	BlueprintID string
	// Config is the validated catalogue selection the session was created
	// from, immutable by trigger from the moment it is written. The bundle,
	// not this, records what actually ran.
	Config json.RawMessage
	// RecordingPreference is what this session keeps - audio_and_transcript
	// or transcript_only - chosen at composition, honoured at capture,
	// immutable by the same trigger as config.
	RecordingPreference string
	// ConsentVersion is the published version of the consent text the
	// preference was chosen against, so "what did this person agree to"
	// resolves to exact words forever.
	ConsentVersion string
	// ConnectionEpoch is the current attempt's epoch; zero before any
	// attempt. AcceptedSequence is the highest contiguous accepted control
	// event in that epoch: the cursor recovery proves itself against.
	ConnectionEpoch  int
	AcceptedSequence int
	State            State
	Version          int
	BundleRef        string
	BundleDigest     string
	BundleRevision   int
	FailureCode      string
	// TimingPolicyVersion is the timing policy stamped at start; zero
	// before the session has started.
	TimingPolicyVersion int
	CreatedAt           time.Time
	StateChangedAt      time.Time
}

// Actor is who a command runs as, recorded on every transition's audit row.
type Actor struct {
	// ID is the person whose authority the command carries. For an automated
	// transition this is still the person the workflow acts for, because the
	// audit policy binds untenanted rows to the acting user and because "the
	// system did it" answers no question about whose session it was.
	ID string
	// Type distinguishes the person acting from automation acting for them:
	// "user" or "service".
	Type string
}

// Effects are what a transition writes beyond the state itself.
type Effects struct {
	// The bundle, on the transition to ready. Set exactly once; the store
	// never overwrites a bundle field that is already set, which is the
	// immutability half of ADR's session bundle rule at the row level.
	BundleRef      string
	BundleDigest   string
	BundleRevision int
	// BundleBody is the composed bundle document, persisted in the ready
	// transition's transaction. Empty for transitions that carry no bundle.
	BundleBody []byte
	// FailureCode, on a transition into a *_failed state: the stable code an
	// operator reads before deciding whether retry is worth it.
	FailureCode string
	// Event, when the catalogue defines one for this transition. Published
	// through the outbox inside the same transaction.
	Event *outbox.Event
}

// Store persists sessions.
type Store struct {
	pool   *pgxpool.Pool
	events *outbox.Store
	// claimStale overrides how long a media claim with no egress id may stand
	// before another delivery may adopt it. Empty means the production window;
	// only a test that must prove the takeover path sets it, because waiting
	// out the real one would make that test useless to run.
	claimStale string
}

// NewStore builds the store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, events: outbox.New(pool)}
}

// scope sets the transaction's row-level security context for one session.
//
// Practice acts as the candidate in an untenanted transaction; screening acts
// under the tenant. This is where the activity that transitions a session
// inherits exactly the authority of the session's owner and no more - there
// is no service scope that sees everything, on purpose.
func scope(ctx context.Context, tx pgx.Tx, mode, candidateID, tenantID string) error {
	if mode == "practice" {
		return database.SetUser(ctx, tx, candidateID)
	}
	return database.SetTenant(ctx, tx, tenantID)
}

// Create writes a draft session, its catalogue event and its audit row.
func (s *Store) Create(ctx context.Context, session Session, actor Actor) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("interview: beginning create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return err
	}

	config := session.Config
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	preference := session.RecordingPreference
	if preference == "" {
		// The data-minimising reading is the only defensible default: a
		// session that never stated a preference keeps the least.
		preference = RecordingTranscriptOnly
	}
	if err := db.New(tx).InsertSession(ctx, db.InsertSessionParams{
		ID:                  session.ID,
		Mode:                session.Mode,
		CandidateID:         session.CandidateID,
		TenantID:            session.TenantID,
		CampaignID:          session.CampaignID,
		BlueprintID:         session.BlueprintID,
		Config:              config,
		RecordingPreference: preference,
		ConsentVersion:      session.ConsentVersion,
	}); err != nil {
		return fmt.Errorf("interview: inserting session: %w", err)
	}

	payload, err := json.Marshal(map[string]string{
		"session_id":   session.ID,
		"candidate_id": session.CandidateID,
		"mode":         session.Mode,
		"blueprint_id": session.BlueprintID,
	})
	if err != nil {
		return fmt.Errorf("interview: encoding the created event: %w", err)
	}
	if _, err := s.events.Publish(ctx, tx, outbox.Event{
		Type:          "interview.session_created.v1",
		SchemaVersion: "1.0",
		TenantID:      session.TenantID,
		Producer:      "interview",
		Actor:         outbox.Actor{Type: actor.Type, ID: actor.ID},
		Purpose:       session.Mode,
		Payload:       payload,
	}); err != nil {
		return err
	}

	if err := s.audit(ctx, tx, session, actor, "interview.session_created", "allowed"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Get reads one session under its own scope.
//
// Mode and identity come from the caller because they are what decide the
// scope, and the scope is what decides whether the row is visible at all: a
// caller that lies about the mode sees nothing rather than someone else's row.
func (s *Store) Get(ctx context.Context, sessionID, mode, candidateID, tenantID string) (Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("interview: beginning read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := scope(ctx, tx, mode, candidateID, tenantID); err != nil {
		return Session{}, err
	}
	return s.get(ctx, tx, sessionID)
}

// GetScreeningForCandidate reads a screening session as the candidate who sits
// it: an untenanted transaction acting as themselves, which is the only scope
// the candidate can offer, since they belong to no tenant. The tenant policy
// scopes screening sessions to the recruiters who run them; this is the owner
// side, the screening analogue of how Get reads a practice session, and it is
// what lets a candidate see their own interview at all.
//
// A session that is not this candidate's, or is practice rather than screening,
// is ErrNotFound: the owner policy yields nothing rather than someone else's
// row, so existence is not answered across candidates.
func (s *Store) GetScreeningForCandidate(ctx context.Context, sessionID, candidateID string) (Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("interview: beginning candidate read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := database.SetUser(ctx, tx, candidateID); err != nil {
		return Session{}, err
	}
	session, err := s.get(ctx, tx, sessionID)
	if err != nil {
		return Session{}, err
	}
	// The owner scope also admits this candidate's practice sessions, through
	// the practice-owner policy beside the screening one. This method answers
	// only for screening, so a practice session read by its id here is not
	// found rather than returned: the candidate has other routes to their
	// practice sessions, and mixing the two here would let a screening-only
	// caller act on a practice session by holding its id.
	if session.Mode != "screening" {
		return Session{}, ErrNotFound
	}
	return session, nil
}

func (s *Store) get(ctx context.Context, tx pgx.Tx, sessionID string) (Session, error) {
	row, err := db.New(tx).GetSession(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("interview: reading session: %w", err)
	}
	return Session{
		ID: row.ID, Mode: row.Mode, CandidateID: row.CandidateID, TenantID: row.TenantID,
		CampaignID:  row.CampaignID,
		BlueprintID: row.BlueprintID, Config: json.RawMessage(row.Config),
		RecordingPreference: row.RecordingPreference, ConsentVersion: row.ConsentVersion,
		ConnectionEpoch: int(row.ConnectionEpoch), AcceptedSequence: int(row.AcceptedSequence),
		State: State(row.State), Version: int(row.Version),
		BundleRef: row.BundleRef, BundleDigest: row.BundleDigest,
		BundleRevision: int(row.BundleRevision), FailureCode: row.FailureCode,
		TimingPolicyVersion: int(row.TimingPolicyVersion),
		CreatedAt:           row.CreatedAt, StateChangedAt: row.StateChangedAt,
	}, nil
}

// Transition moves a session from one state to another, or says exactly why
// not: the machine refuses an illegal edge, a zero-row update becomes stale
// or not-found by re-reading, and everything the transition carries commits
// with it or not at all.
func (s *Store) Transition(ctx context.Context, session Session, to State, effects Effects, actor Actor) (Session, error) {
	if err := CanTransition(session.State, to); err != nil {
		return Session{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("interview: beginning transition: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return Session{}, err
	}

	moved, err := db.New(tx).TransitionSession(ctx, db.TransitionSessionParams{
		ID:              session.ID,
		FromState:       string(session.State),
		ToState:         string(to),
		ExpectedVersion: int32(session.Version),
		BundleRef:       effects.BundleRef,
		BundleDigest:    effects.BundleDigest,
		BundleRevision:  int32(effects.BundleRevision),
		FailureCode:     effects.FailureCode,
	})
	if err != nil {
		return Session{}, fmt.Errorf("interview: transitioning: %w", err)
	}
	if moved == 0 {
		// The guard refused. Re-read inside the same scope to say which
		// refusal it was: gone, or moved on since the caller looked.
		if _, err := s.get(ctx, tx, session.ID); errors.Is(err, ErrNotFound) {
			return Session{}, ErrNotFound
		}
		return Session{}, ErrStaleVersion
	}

	if len(effects.BundleBody) > 0 {
		// The bundle and the ready state commit together: a session marked
		// ready whose bundle vanished would pin a digest nothing can resolve.
		if err := db.New(tx).InsertSessionBundle(ctx, db.InsertSessionBundleParams{
			SessionID: session.ID,
			Digest:    effects.BundleDigest,
			Body:      effects.BundleBody,
		}); err != nil {
			return Session{}, fmt.Errorf("interview: persisting the bundle: %w", err)
		}
	}

	if effects.Event != nil {
		if _, err := s.events.Publish(ctx, tx, *effects.Event); err != nil {
			return Session{}, err
		}
	}

	if err := s.audit(ctx, tx, session, actor,
		fmt.Sprintf("interview.session_%s", to), "allowed"); err != nil {
		return Session{}, err
	}

	updated, err := s.get(ctx, tx, session.ID)
	if err != nil {
		return Session{}, err
	}
	return updated, tx.Commit(ctx)
}

// audit appends the transition to the audit trail inside the transaction.
func (s *Store) audit(ctx context.Context, tx pgx.Tx, session Session, actor Actor, action, outcome string) error {
	if err := db.New(tx).InsertAuditEvent(ctx, db.InsertAuditEventParams{
		ID:        id.New().String(),
		TenantID:  session.TenantID,
		ActorID:   actor.ID,
		ActorType: actor.Type,
		Action:    action,
		SessionID: session.ID,
		Outcome:   outcome,
	}); err != nil {
		return fmt.Errorf("interview: writing audit row: %w", err)
	}
	return nil
}

// ReadyEvent builds the catalogue's session_ready event for a transition to
// ready. In this package rather than at call sites, because the workflow and
// any future retry path must publish exactly the same shape, and the payload's
// required fields are the catalogue's to define, not each caller's to recall.
func ReadyEvent(session Session, effects Effects, actor Actor) (*outbox.Event, error) {
	payload, err := json.Marshal(map[string]any{
		"session_id":      session.ID,
		"bundle_id":       session.ID,
		"bundle_digest":   effects.BundleDigest,
		"bundle_revision": effects.BundleRevision,
	})
	if err != nil {
		return nil, fmt.Errorf("interview: encoding the ready event: %w", err)
	}
	return &outbox.Event{
		Type:          "interview.session_ready.v1",
		SchemaVersion: "1.0",
		TenantID:      session.TenantID,
		Producer:      "interview",
		Actor:         outbox.Actor{Type: actor.Type, ID: actor.ID},
		Purpose:       session.Mode,
		Payload:       payload,
	}, nil
}

// Bundle answers a session's frozen bundle document under its own scope.
func (s *Store) Bundle(ctx context.Context, sessionID, mode, candidateID, tenantID string) ([]byte, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("interview: beginning bundle read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, mode, candidateID, tenantID); err != nil {
		return nil, err
	}
	row, err := db.New(tx).GetSessionBundle(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("interview: reading bundle: %w", err)
	}
	return row.Body, nil
}

// CurrentTimingPolicy answers the timing rules in force now.
func (s *Store) CurrentTimingPolicy(ctx context.Context) (TimingPolicy, error) {
	row, err := db.New(s.pool).CurrentTimingPolicy(ctx)
	if err != nil {
		return TimingPolicy{}, fmt.Errorf("interview: reading the timing policy: %w", err)
	}
	return TimingPolicy{
		Version:               int(row.Version),
		ReconnectGraceSeconds: int(row.ReconnectGraceSeconds),
		MaxOverrunSeconds:     int(row.MaxOverrunSeconds),
	}, nil
}

// StampTimingPolicy records which policy governs a session, once: the
// stamp is first-write-wins so a later policy publish never rewrites a
// session already running.
func (s *Store) StampTimingPolicy(ctx context.Context, session Session, policy TimingPolicy) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("interview: beginning stamp: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return err
	}
	if err := db.New(tx).StampTimingPolicy(ctx, db.StampTimingPolicyParams{
		ID: session.ID, Version: int32(policy.Version),
	}); err != nil {
		return fmt.Errorf("interview: stamping the timing policy: %w", err)
	}
	return tx.Commit(ctx)
}

// ListMine answers every session the scope can see, newest first: the
// owner's whole history, every lifecycle state included - a failed or
// expired session is history too, never a row to hide.
func (s *Store) ListMine(ctx context.Context, mode, candidateID, tenantID string) ([]Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("interview: beginning list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, mode, candidateID, tenantID); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).ListSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("interview: listing sessions: %w", err)
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, Session{
			ID: row.ID, Mode: row.Mode, CandidateID: row.CandidateID, TenantID: row.TenantID,
			CampaignID:  row.CampaignID,
			BlueprintID: row.BlueprintID, Config: json.RawMessage(row.Config),
			RecordingPreference: row.RecordingPreference, ConsentVersion: row.ConsentVersion,
			ConnectionEpoch: int(row.ConnectionEpoch), AcceptedSequence: int(row.AcceptedSequence),
			State: State(row.State), Version: int(row.Version),
			BundleRef: row.BundleRef, BundleDigest: row.BundleDigest,
			BundleRevision: int(row.BundleRevision), FailureCode: row.FailureCode,
			TimingPolicyVersion: int(row.TimingPolicyVersion),
			CreatedAt:           row.CreatedAt, StateChangedAt: row.StateChangedAt,
		})
	}
	return sessions, nil
}
