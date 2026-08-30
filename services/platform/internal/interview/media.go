package interview

// The recording's platform half: RTC-05 per ADR-0013.
//
// Recording is server-side SFU egress, so the platform's job is not to
// move bytes - it is to remember which egress is writing where, and at
// completion to prove the artifact actually landed before the seal says
// anything about it. The recorder and the prober are consumer-defined
// ports; cmd wires LiveKit and the object store behind them, and the
// reconciliation semantics are proven against fakes.

import (
	"context"
	"errors"
	"fmt"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/interview/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
)

// Recorder starts and stops one egress. Wired to LiveKit in cmd.
type Recorder interface {
	// StartTrack begins egress of one participant's audio into the given
	// storage key, answering the egress id. The participant is named by
	// room identity: the candidate joins as their user id, the agent as
	// "interviewer". Idempotency across retries is the store's (unique
	// per session and track), not the recorder's.
	StartTrack(ctx context.Context, roomName, participantIdentity, storageKey string) (string, error)
	// StopTrack ends one egress. Stopping an already-stopped egress is
	// not an error worth failing completion over.
	StopTrack(ctx context.Context, egressID string) error
}

// Prober answers what is actually in the object store: size and digest
// from reading the object back, never from anyone's claim about it.
type Prober interface {
	Stat(ctx context.Context, storageKey string) (size int64, digest string, err error)
}

// Tracks recorded per session when recording at all: the candidate's audio
// and the interviewer's synthesized audio, separately, never a mix
// (ADR-0013: a mix would put the interviewer's voice inside the
// candidate's acoustic features).
var recordedTracks = []string{"candidate", "interviewer"}

// MediaTrack is one track's durable record.
type MediaTrack struct {
	Track      string
	StorageKey string
	EgressID   string
	State      string
	Digest     string
	SizeBytes  int64
}

// trackKey derives the one storage key a track may live under.
func trackKey(session Session, track string) (objectstore.Key, error) {
	return objectstore.SealedInputSiblingKey(session.Mode, session.TenantID,
		session.CandidateID, session.ID, objectstore.PurposeMedia, track+".webm")
}

// StartRecording begins egress for a session that records audio, once.
//
// Transcript-only sessions are a structural no-op: egress is never
// started, so durable audio never exists to discard (ADR-0013's strongest
// honouring). A retry or a reconnection converges on the unique
// (session, track) row: the second start finds the row and starts
// nothing, which is the "reconnection does not restart recording" half of
// RTC-05's first box.
func (s *Store) StartRecording(ctx context.Context, recorder Recorder, session Session) error {
	if session.RecordingPreference != "audio_and_transcript" {
		return nil
	}

	for _, track := range recordedTracks {
		key, err := trackKey(session, track)
		if err != nil {
			return err
		}

		// Claim the slot before starting anything, and only while the session
		// is still live.
		//
		// Both halves are one transaction because both were wrong. The state
		// was never checked at all, so a delayed or retried
		// session_started.v1 could begin egress after the candidate had
		// finished and sealed: capture continuing past the consent it was
		// given, with no later lifecycle step to stop it or pay for it. And
		// the claim used to come after the egress, so two deliveries could
		// both call the provider and only one keep its id, leaving a job
		// nobody would ever stop recording the same participant to the same
		// key.
		claimed, err := s.claimTrack(ctx, session, track, key.String())
		if err != nil {
			return err
		}
		if !claimed {
			// Somebody else owns this track, or the session is no longer one
			// that may be recorded. Either way there is nothing to start.
			continue
		}

		identity := track
		if track == "candidate" {
			identity = session.CandidateID
		}
		egressID, err := recorder.StartTrack(ctx, session.ID, identity, key.String())
		if err != nil {
			// Release the claim this attempt took and could not use, so the
			// retry adopts it at once rather than waiting out the staleness
			// window. The window is what tells an abandoned claim from one
			// still in flight, and only the owner of a failed attempt knows
			// immediately which this is, so only the owner short-circuits it.
			//
			// A release that itself fails is not worth losing the real error
			// over: the claim ages out on its own and the retry is delayed
			// rather than lost.
			if releaseErr := s.releaseClaim(ctx, session, track); releaseErr != nil {
				return errors.Join(
					fmt.Errorf("interview: starting %s egress: %w", track, err),
					fmt.Errorf("interview: releasing the %s claim: %w", track, releaseErr))
			}
			return fmt.Errorf("interview: starting %s egress: %w", track, err)
		}

		if err := s.recordEgress(ctx, session, track, egressID); err != nil {
			return err
		}
	}
	return nil
}

// claimTrack takes the (session, track) slot, refusing once the session has
// been sealed or has left the states that may record.
//
// The state check and the claim are in one transaction so a completion
// committing between them cannot be missed.
func (s *Store) claimTrack(ctx context.Context, session Session, track, key string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("interview: beginning track claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return false, err
	}

	// A sealed session is finished. Nothing may begin capturing after the
	// transcript it belongs to has been closed.
	if _, err := db.New(tx).GetSeal(ctx, session.ID); err == nil {
		return false, nil
	}

	current, err := db.New(tx).GetSession(ctx, session.ID)
	if err != nil {
		return false, fmt.Errorf("interview: reading the session to record: %w", err)
	}
	if !recordableStates[current.State] {
		return false, nil
	}

	rows, err := db.New(tx).ClaimMediaTrack(ctx, db.ClaimMediaTrackParams{
		ID: id.New().String(), SessionID: session.ID, Mode: session.Mode,
		CandidateID: session.CandidateID, TenantID: session.TenantID,
		Track: track, StorageKey: key, StaleAfter: s.claimStaleAfter(),
	})
	if err != nil {
		return false, fmt.Errorf("interview: claiming %s: %w", track, err)
	}
	if rows == 0 {
		return false, nil
	}
	return true, tx.Commit(ctx)
}

// defaultClaimStaleAfter is how long a claim with no egress id may stand
// before another delivery may take it over.
//
// It has to exceed the longest a live attempt can hold an empty claim, which
// the egress call's own 15 second timeout bounds, or a retry would race a
// delivery that is merely slow and both would record the same participant.
// Two minutes is that bound with room for a provider that is slow rather than
// dead, and the cost of erring long is only that a genuinely abandoned track
// waits before a retry can adopt it.
const defaultClaimStaleAfter = "2 minutes"

// claimStaleAfter answers the window, allowing a test to shorten it so the
// takeover path can be proven without waiting two minutes for it.
func (s *Store) claimStaleAfter() string {
	if s.claimStale != "" {
		return s.claimStale
	}
	return defaultClaimStaleAfter
}

// releaseClaim gives up a claim whose egress never started.
func (s *Store) releaseClaim(ctx context.Context, session Session, track string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("interview: beginning claim release: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return err
	}
	if _, err := db.New(tx).ReleaseMediaClaim(ctx, db.ReleaseMediaClaimParams{
		SessionID: session.ID, Track: track,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// recordableStates are the states a recording may begin in.
//
// A closed set rather than "not finished", so a state added later has to be
// considered rather than silently permitting capture.
var recordableStates = map[string]bool{
	"connecting": true, "in_progress": true, "reconnecting": true,
}

func (s *Store) recordEgress(ctx context.Context, session Session, track, egressID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("interview: beginning egress record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return err
	}
	if _, err := db.New(tx).RecordTrackEgress(ctx, db.RecordTrackEgressParams{
		SessionID: session.ID, Track: track, EgressID: egressID,
	}); err != nil {
		return fmt.Errorf("interview: recording %s egress: %w", track, err)
	}
	return tx.Commit(ctx)
}

// FinalizeRecording stops egress and reconciles each track against what
// the object store actually holds. The answer is per track and overall:
// finalized only when every track's object was read back and its digest
// recorded; anything else is missing, stated, never pretended. Runs
// before the seal, which is RTC-05's second box.
func (s *Store) FinalizeRecording(ctx context.Context, recorder Recorder, prober Prober, session Session) (string, error) {
	tracks, err := s.MediaTracks(ctx, session)
	if err != nil {
		return "", err
	}
	if len(tracks) == 0 {
		// Egress never started: the recording is missing in fact.
		return "missing", nil
	}

	status := "finalized"
	for _, track := range tracks {
		if track.State != "recording" {
			if track.State == "missing" {
				status = "missing"
			}
			continue // already resolved; completion retried
		}
		if err := recorder.StopTrack(ctx, track.EgressID); err != nil {
			// A stop that fails does not decide anything: the probe does.
			// The egress may already have ended on its own.
			_ = err
		}
		size, digest, statErr := prober.Stat(ctx, track.StorageKey)
		state, trackDigest, trackSize := "finalized", digest, size
		if statErr != nil || size == 0 {
			state, trackDigest, trackSize = "missing", "", 0
			status = "missing"
		}
		if err := s.resolveTrack(ctx, session, track.Track, state, trackDigest, trackSize); err != nil {
			return "", err
		}
	}
	return status, nil
}

// MediaTracks reads a session's track rows under its own scope.
func (s *Store) MediaTracks(ctx context.Context, session Session) ([]MediaTrack, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("interview: beginning tracks read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).ListMediaTracks(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("interview: listing tracks: %w", err)
	}
	tracks := make([]MediaTrack, 0, len(rows))
	for _, row := range rows {
		tracks = append(tracks, MediaTrack{
			Track: row.Track, StorageKey: row.StorageKey, EgressID: row.EgressID,
			State: row.State, Digest: row.Digest, SizeBytes: row.SizeBytes,
		})
	}
	return tracks, nil
}

func (s *Store) resolveTrack(ctx context.Context, session Session, track, state, digest string, size int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("interview: beginning track resolve: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return err
	}
	if _, err := db.New(tx).ResolveMediaTrack(ctx, db.ResolveMediaTrackParams{
		SessionID: session.ID, Track: track,
		State: state, Digest: digest, SizeBytes: size,
	}); err != nil {
		return fmt.Errorf("interview: resolving the track: %w", err)
	}
	return tx.Commit(ctx)
}

// WithClaimStaleAfter returns a store whose abandoned-claim window is the given
// PostgreSQL interval.
//
// Exported for tests only. The takeover path cannot be proven against the
// production window without a two minute wait, and a guard nobody can afford to
// test is a guard that stops being true without anybody noticing.
func (s *Store) WithClaimStaleAfter(interval string) *Store {
	clone := *s
	clone.claimStale = interval
	return &clone
}
