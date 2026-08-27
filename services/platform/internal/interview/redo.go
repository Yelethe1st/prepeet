package interview

// Answer redo: PRC-03.
//
// A redo is a new practice session composed from the parent's own
// configuration plus the one question it retakes. Nothing about the
// parent changes: its transcript, evidence and timing live in different
// rows, so "the original survives" is structural rather than a promise.
// One retake per question is the redos table's primary key, practice-only
// is its mode CHECK, and a redo may only be asked for once the parent has
// results to compare against.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/interview/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

var (
	// ErrRedoNotAllowed refuses a redo the session's mode or state does
	// not permit: screening, or a parent without results yet.
	ErrRedoNotAllowed = errors.New("interview: REDO_NOT_ALLOWED: this session does not offer a redo")
	// ErrRedoExists refuses a second retake of the same answer.
	ErrRedoExists = errors.New("interview: REDO_EXISTS: this answer has already been redone")
	// ErrRedoTurnUnknown refuses a sequence that is not one of the
	// candidate's own answers.
	ErrRedoTurnUnknown = errors.New("interview: REDO_TURN_UNKNOWN: that turn is not an answer of yours")
)

// RedoMinutes bounds a retake: one question, one answer.
const RedoMinutes = 5

// Redo is one recorded link from a parent answer to its retake session.
type Redo struct {
	Sequence      int
	RedoSessionID string
	CreatedAt     time.Time
}

// RedoOf is what a redo session's config carries about its origin, and
// what the agent's brief reads to ask exactly that question.
type RedoOf struct {
	SessionID string `json:"session_id"`
	Sequence  int    `json:"sequence"`
	Question  string `json:"question"`
}

// Redos answers the parent's recorded retakes under its own scope.
func (s *Store) Redos(ctx context.Context, parent Session) ([]Redo, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("interview: beginning redos read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, parent.Mode, parent.CandidateID, parent.TenantID); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).ListRedos(ctx, parent.ID)
	if err != nil {
		return nil, fmt.Errorf("interview: listing redos: %w", err)
	}
	redos := make([]Redo, 0, len(rows))
	for _, row := range rows {
		redos = append(redos, Redo{Sequence: int(row.Sequence), RedoSessionID: row.RedoSessionID, CreatedAt: row.CreatedAt})
	}
	return redos, nil
}

// CreateRedo creates the retake session for one of the parent's answers.
//
// The parent must be a practice session with results (review_ready or
// archived), the sequence must be one of the candidate's own transcript
// turns, and no retake may exist for it yet. The child inherits the
// parent's selection, records its origin and the question it retakes in
// its own immutable config, and enters the lifecycle at draft exactly
// like any other session: composition, prepare, start, the agent asking
// that one question, completion, evaluation. The parent is read and
// never written.
func (s *Store) CreateRedo(ctx context.Context, events *Events, parent Session, sequence int, actor Actor) (Session, error) {
	if parent.Mode != "practice" {
		return Session{}, ErrRedoNotAllowed
	}
	if parent.State != StateReviewReady && parent.State != StateArchived {
		return Session{}, ErrRedoNotAllowed
	}
	var config map[string]any
	if err := json.Unmarshal(parent.Config, &config); err != nil {
		return Session{}, fmt.Errorf("interview: decoding the parent config: %w", err)
	}
	if allowed, present := config["redo_allowed"]; present {
		if enabled, ok := allowed.(bool); ok && !enabled {
			return Session{}, ErrRedoNotAllowed
		}
	}
	if _, isRedo := config["redo_of"]; isRedo {
		// A redo of a redo would fork the history; the parent is the one
		// that gets retaken.
		return Session{}, ErrRedoNotAllowed
	}

	transcript, err := events.AssembleTranscript(ctx, parent.ID, parent.Mode, parent.CandidateID, parent.TenantID)
	if err != nil {
		return Session{}, err
	}
	question := ""
	found := false
	for _, segment := range transcript.EffectiveText() {
		if segment.Speaker == "interviewer" {
			question = segment.Text
		}
		if segment.Sequence == sequence && segment.Speaker == "candidate" {
			found = true
			break
		}
	}
	if !found {
		return Session{}, ErrRedoTurnUnknown
	}

	child := map[string]any{}
	for key, value := range config {
		child[key] = value
	}
	child["minutes"] = RedoMinutes
	child["redo_of"] = RedoOf{SessionID: parent.ID, Sequence: sequence, Question: question}
	childConfig, err := json.Marshal(child)
	if err != nil {
		return Session{}, fmt.Errorf("interview: encoding the redo config: %w", err)
	}

	session := Session{
		ID: id.New().String(), Mode: "practice", CandidateID: parent.CandidateID,
		BlueprintID: parent.BlueprintID, Config: childConfig,
		RecordingPreference: parent.RecordingPreference, ConsentVersion: parent.ConsentVersion,
	}
	if err := s.Create(ctx, session, actor); err != nil {
		return Session{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("interview: beginning redo link: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, parent.Mode, parent.CandidateID, parent.TenantID); err != nil {
		return Session{}, err
	}
	if err := db.New(tx).InsertRedo(ctx, db.InsertRedoParams{
		ParentSessionID: parent.ID, Sequence: int32(sequence),
		RedoSessionID: session.ID, CandidateID: parent.CandidateID,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// The link lost the race; the just-created child is an orphan
			// draft that expires like any other, and the earlier redo
			// stands.
			return Session{}, ErrRedoExists
		}
		return Session{}, fmt.Errorf("interview: linking the redo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Session{}, err
	}
	return s.Get(ctx, session.ID, "practice", parent.CandidateID, "")
}
