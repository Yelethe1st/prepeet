package interview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/interview/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// Idempotent completion and transcript sealing: SES-04, to the completion
// contract in session-lifecycle.md.
//
// Completing freezes the conversational record. The final cursor is
// accepted, the gaps standing under it are RECORDED - never silently
// closed, because a gap is coverage information evaluation must see, not
// noise to tidy - the effective transcript is digested, the media status is
// decided from the session's own recording preference, and the whole seal
// persists as one immutable row. A duplicate completion converges on that
// row and answers the same receipt; the second caller causes no second
// transition, no second event and no second evaluation.
//
// The bounded media wait is the recording preference's honest floor today:
// a transcript-only session has no media by choice, and an audio session
// has none yet because nothing produces it until RTC-05; it completes with
// the explicit MEDIA_MISSING warning the spec requires rather than
// waiting for a pipeline that does not exist. When egress lands, the wait
// becomes a real bounded timer in the finalization flow.

// Completion refusals.
var (
	ErrCompleteNotRunning = errors.New("interview: SESSION_NOT_RUNNING: only a running session can complete")
	ErrSealConflict       = errors.New("interview: SEAL_CONFLICT: this session sealed at a different cursor")
	ErrSealed             = errors.New("interview: EVENT_AFTER_SEAL: the transcript is sealed; conversational events are over")
)

// Warning codes attached to a seal.
const (
	WarningMediaMissing = "MEDIA_MISSING"
	WarningGapsRecorded = "SEQUENCE_GAPS_RECORDED"
)

// Receipt is completion's answer, identical however many times it is asked.
type Receipt struct {
	SessionID        string
	State            State
	SealedEpoch      int
	SealedSequence   int
	Gaps             []SequenceRange
	TranscriptDigest string
	BundleDigest     string
	MediaStatus      string
	Warnings         []string
	SealedAt         time.Time
}

// Completer runs the completion command.
type Completer struct {
	store  *Store
	events *Events
}

// NewCompleter wires the command.
func NewCompleter(store *Store) *Completer {
	return &Completer{store: store, events: NewEvents(store)}
}

// Complete seals a running session at the given final cursor and moves it
// through finalizing into evaluating.
func (c *Completer) Complete(ctx context.Context, sessionID, mode, candidateID, tenantID string, finalEpoch, finalSequence int) (Receipt, error) {
	session, err := c.store.Get(ctx, sessionID, mode, candidateID, tenantID)
	if err != nil {
		return Receipt{}, err
	}

	// Idempotency first: a session already sealed answers its receipt,
	// whatever state the pipeline has moved it to since.
	if existing, err := c.receipt(ctx, session); err == nil {
		if existing.SealedEpoch != finalEpoch || existing.SealedSequence != finalSequence {
			return Receipt{}, ErrSealConflict
		}
		return existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, err
	}

	switch session.State {
	case StateInProgress, StateReconnecting:
		// The states completion applies to: running, or dropped and being
		// completed by policy.
	default:
		return Receipt{}, ErrCompleteNotRunning
	}
	if finalEpoch != session.ConnectionEpoch {
		return Receipt{}, ErrEpochStale
	}

	// The record as it stands: the transcript, the turn count, and the
	// gaps under the final cursor - recorded exactly, closed never.
	transcript, err := c.events.AssembleTranscript(ctx, sessionID, mode, candidateID, tenantID)
	if err != nil {
		return Receipt{}, err
	}
	gaps, err := c.gapsUnder(ctx, session, finalEpoch, finalSequence)
	if err != nil {
		return Receipt{}, err
	}

	digest, err := transcriptDigest(transcript)
	if err != nil {
		return Receipt{}, err
	}

	mediaStatus := "missing"
	warnings := []string{}
	if session.RecordingPreference == RecordingTranscriptOnly {
		// No media, by the candidate's own recorded choice: a fact, not a
		// warning.
		mediaStatus = "none_by_choice"
	} else {
		warnings = append(warnings, WarningMediaMissing)
	}
	if len(gaps) > 0 {
		warnings = append(warnings, WarningGapsRecorded)
	}

	if err := c.seal(ctx, session, finalEpoch, finalSequence, gaps, digest, mediaStatus, warnings); err != nil {
		return Receipt{}, err
	}

	// The transitions carry the machine's own idempotency: a crash between
	// seal and transition retries into the receipt path above, and a stale
	// version means a concurrent completion won.
	actor := Actor{ID: candidateID, Type: "user"}
	event, err := completedEvent(session, transcript)
	if err != nil {
		return Receipt{}, err
	}
	finalizing, err := c.store.Transition(ctx, session, StateFinalizing, Effects{Event: event}, actor)
	if err != nil && !errors.Is(err, ErrStaleVersion) {
		return Receipt{}, err
	}
	if err == nil {
		// Media status is known the moment the seal is written (see the
		// bounded-wait note above), so evaluating's entry condition holds.
		if _, err := c.store.Transition(ctx, finalizing, StateEvaluating, Effects{}, actor); err != nil &&
			!errors.Is(err, ErrStaleVersion) {
			return Receipt{}, err
		}
	}

	final, err := c.store.Get(ctx, sessionID, mode, candidateID, tenantID)
	if err != nil {
		return Receipt{}, err
	}
	receipt, err := c.receipt(ctx, final)
	if err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

// gapsUnder answers the missing ranges below the final cursor.
func (c *Completer) gapsUnder(ctx context.Context, session Session, epoch, finalSequence int) ([]SequenceRange, error) {
	tx, err := c.store.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("interview: beginning gap read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return nil, err
	}

	stored, err := db.New(tx).StoredSequences(ctx, db.StoredSequencesParams{
		SessionID: session.ID, Epoch: int32(epoch),
	})
	if err != nil {
		return nil, fmt.Errorf("interview: reading sequences: %w", err)
	}

	present := map[int]bool{}
	for _, sequence := range stored {
		present[int(sequence)] = true
	}
	var gaps []SequenceRange
	start := 0
	for sequence := 1; sequence <= finalSequence; sequence++ {
		if !present[sequence] {
			if start == 0 {
				start = sequence
			}
			continue
		}
		if start != 0 {
			gaps = append(gaps, SequenceRange{From: start, To: sequence - 1})
			start = 0
		}
	}
	if start != 0 {
		gaps = append(gaps, SequenceRange{From: start, To: finalSequence})
	}
	return gaps, nil
}

// seal writes the immutable row, converging on an identical existing one.
func (c *Completer) seal(ctx context.Context, session Session, epoch, sequence int, gaps []SequenceRange, digest, mediaStatus string, warnings []string) error {
	tx, err := c.store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("interview: beginning seal: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return err
	}

	encodedGaps, err := json.Marshal(gapPairs(gaps))
	if err != nil {
		return fmt.Errorf("interview: encoding gaps: %w", err)
	}
	encodedWarnings, err := json.Marshal(warnings)
	if err != nil {
		return fmt.Errorf("interview: encoding warnings: %w", err)
	}

	err = db.New(tx).InsertSeal(ctx, db.InsertSealParams{
		SessionID: session.ID, Mode: session.Mode, CandidateID: session.CandidateID,
		TenantID: session.TenantID, SealedEpoch: int32(epoch), SealedSequence: int32(sequence),
		Gaps: encodedGaps, TranscriptDigest: digest, BundleDigest: session.BundleDigest,
		MediaStatus: mediaStatus, Warnings: encodedWarnings,
	})
	if err != nil {
		if isUniqueViolation(err) {
			// A concurrent completion sealed first; the caller's receipt
			// path will answer it, or refuse a different cursor.
			return nil
		}
		return fmt.Errorf("interview: sealing: %w", err)
	}
	return tx.Commit(ctx)
}

// receipt reads the seal back as the answer.
func (c *Completer) receipt(ctx context.Context, session Session) (Receipt, error) {
	tx, err := c.store.pool.Begin(ctx)
	if err != nil {
		return Receipt{}, fmt.Errorf("interview: beginning receipt: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, session.Mode, session.CandidateID, session.TenantID); err != nil {
		return Receipt{}, err
	}

	row, err := db.New(tx).GetSeal(ctx, session.ID)
	if err != nil {
		return Receipt{}, err
	}
	var pairs [][2]int
	if err := json.Unmarshal(row.Gaps, &pairs); err != nil {
		return Receipt{}, fmt.Errorf("interview: decoding gaps: %w", err)
	}
	var warnings []string
	if err := json.Unmarshal(row.Warnings, &warnings); err != nil {
		return Receipt{}, fmt.Errorf("interview: decoding warnings: %w", err)
	}
	gaps := make([]SequenceRange, 0, len(pairs))
	for _, pair := range pairs {
		gaps = append(gaps, SequenceRange{From: pair[0], To: pair[1]})
	}
	return Receipt{
		SessionID: row.SessionID, State: session.State,
		SealedEpoch: int(row.SealedEpoch), SealedSequence: int(row.SealedSequence),
		Gaps: gaps, TranscriptDigest: row.TranscriptDigest, BundleDigest: row.BundleDigest,
		MediaStatus: row.MediaStatus, Warnings: warnings, SealedAt: row.CreatedAt,
	}, nil
}

// Sealed answers whether a session's transcript is sealed; ingest consults
// it to reject later conversational events.
func (c *Completer) Sealed(ctx context.Context, session Session) (bool, error) {
	_, err := c.receipt(ctx, session)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// transcriptDigest hashes the effective transcript deterministically:
// struct serialization is stable, and the segments arrive in timeline
// order, so the digest is reproducible from the log alone.
func transcriptDigest(transcript Transcript) (string, error) {
	effective := transcript.EffectiveText()
	encoded, err := json.Marshal(effective)
	if err != nil {
		return "", fmt.Errorf("interview: encoding transcript: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// completedEvent builds the catalogue's session_completed event.
func completedEvent(session Session, transcript Transcript) (*outbox.Event, error) {
	turns := 0
	for _, segment := range transcript.EffectiveText() {
		if segment.Speaker == "candidate" {
			turns++
		}
	}
	duration := 0
	if segments := transcript.EffectiveText(); len(segments) > 0 {
		duration = (segments[len(segments)-1].EndMs - segments[0].StartMs) / 1000
	}
	payload, err := json.Marshal(map[string]any{
		"session_id":       session.ID,
		"completion":       "completed",
		"turn_count":       turns,
		"duration_seconds": duration,
	})
	if err != nil {
		return nil, fmt.Errorf("interview: encoding the completed event: %w", err)
	}
	return &outbox.Event{
		Type:          "interview.session_completed.v1",
		SchemaVersion: "1.0",
		TenantID:      session.TenantID,
		Producer:      "interview",
		Actor:         outbox.Actor{Type: "user", ID: session.CandidateID},
		Purpose:       session.Mode,
		Payload:       payload,
	}, nil
}

func gapPairs(gaps []SequenceRange) [][2]int {
	pairs := make([][2]int, 0, len(gaps))
	for _, gap := range gaps {
		pairs = append(pairs, [2]int{gap.From, gap.To})
	}
	return pairs
}
