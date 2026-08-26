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
	"fmt"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/interview/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
)

// Recorder starts and stops one egress. Wired to LiveKit in cmd.
type Recorder interface {
	// StartTrack begins egress of one side's audio into the given storage
	// key, answering the egress id. Must be safe to call for a room that
	// is already being recorded elsewhere; idempotency across retries is
	// the store's (unique per session and track), not the recorder's.
	StartTrack(ctx context.Context, roomName, track, storageKey string) (string, error)
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

		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("interview: beginning track record: %w", err)
		}
		if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		existing, err := db.New(tx).ListMediaTracks(ctx, session.ID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("interview: reading tracks: %w", err)
		}
		_ = tx.Rollback(ctx)
		already := false
		for _, row := range existing {
			if row.Track == track {
				already = true
			}
		}
		if already {
			continue
		}

		// Start the egress first, then record it: a crash between the two
		// leaves an orphan egress writing to the derived key, and the
		// retry claims the row for its own egress id; finalization reads
		// the key either way, so the artifact is never lost to the race.
		egressID, err := recorder.StartTrack(ctx, session.ID, track, key.String())
		if err != nil {
			return fmt.Errorf("interview: starting %s egress: %w", track, err)
		}
		tx, err = s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("interview: beginning track insert: %w", err)
		}
		if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if _, err := db.New(tx).InsertMediaTrack(ctx, db.InsertMediaTrackParams{
			ID: id.New().String(), SessionID: session.ID, Mode: session.Mode,
			CandidateID: session.CandidateID, TenantID: session.TenantID,
			Track: track, StorageKey: key.String(), EgressID: egressID,
		}); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("interview: recording the track row: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
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
