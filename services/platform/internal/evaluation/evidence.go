// Package evaluation owns what evaluation read from a session: evidence
// spans first, aggregation in the tickets after.
//
// Implements EVL-01. The properties everything here serves: every span
// carries its source segment, character range, clock range and extraction
// version; a span that does not resolve to real transcript text fails
// validation before it is ever stored, whoever produced it; and the stage
// retries by wholesale replacement, so a worker death converges instead of
// duplicating evidence.
package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/evaluation/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Span is one piece of evidence, provenance included.
type Span struct {
	CompetencyID      string
	Kind              string
	SegmentSequence   int
	Quote             string
	CharStart         int
	CharEnd           int
	StartMs           int
	EndMs             int
	ExtractionVersion string
}

// SealedInput is the document evaluation was given, decoded for
// validation: the turns and competencies exactly as Python received them.
type SealedInput struct {
	SessionID    string       `json:"session_id"`
	Competencies []Competency `json:"competencies"`
	Turns        []Turn       `json:"turns"`
}

// Competency names one thing evidence may bear on.
type Competency struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Turn is one sealed turn.
type Turn struct {
	Sequence int    `json:"sequence"`
	Speaker  string `json:"speaker"`
	Text     string `json:"text"`
	StartMs  int    `json:"start_ms"`
	EndMs    int    `json:"end_ms"`
}

// ErrFabricated refuses spans that do not resolve to the sealed input. The
// wrapped message names the first offence; none of the batch is stored.
var ErrFabricated = errors.New("evaluation: EVIDENCE_FABRICATED: a span does not resolve to the sealed transcript")

// Validate holds every span to the sealed input. This is the honesty gate
// the pipeline stands on: it does not matter whether the extractor is a
// deterministic rule set or a model - a quote that is not an exact slice of
// a real turn, a competency nobody asked about, or timing outside the turn
// refuses the whole batch.
func Validate(input SealedInput, spans []Span) error {
	turns := map[int]Turn{}
	for _, turn := range input.Turns {
		turns[turn.Sequence] = turn
	}
	competencies := map[string]bool{}
	for _, competency := range input.Competencies {
		competencies[competency.ID] = true
	}

	for i, span := range spans {
		turn, present := turns[span.SegmentSequence]
		if !present {
			return fmt.Errorf("%w: span %d cites segment %d, which is not in the sealed input",
				ErrFabricated, i, span.SegmentSequence)
		}
		if !competencies[span.CompetencyID] {
			return fmt.Errorf("%w: span %d cites competency %q, which nobody asked about",
				ErrFabricated, i, span.CompetencyID)
		}
		if span.CharStart < 0 || span.CharEnd > len(turn.Text) || span.CharEnd <= span.CharStart {
			return fmt.Errorf("%w: span %d's character range is outside its segment", ErrFabricated, i)
		}
		if turn.Text[span.CharStart:span.CharEnd] != span.Quote {
			return fmt.Errorf("%w: span %d's quote is not the text at its own range", ErrFabricated, i)
		}
		if span.StartMs < turn.StartMs || span.EndMs > turn.EndMs || span.EndMs <= span.StartMs {
			return fmt.Errorf("%w: span %d's clock range is outside its segment", ErrFabricated, i)
		}
		if span.ExtractionVersion == "" {
			return fmt.Errorf("%w: span %d carries no extraction version", ErrFabricated, i)
		}
	}
	return nil
}

// SessionRef names whose evidence is being stored, for the row scope.
type SessionRef struct {
	SessionID   string
	Mode        string
	CandidateID string
	TenantID    string
}

// StoredSpan is one row read back.
type StoredSpan struct {
	ID string
	Span
	CreatedAt time.Time
}

// Store persists evidence.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires the evidence store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Replace stores one extraction wholesale: spans for this session and
// version are deleted and reinserted in one transaction, so a retried
// stage converges on identical rows rather than doubling them.
func (s *Store) Replace(ctx context.Context, ref SessionRef, extractionVersion string, spans []Span) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("evaluation: beginning replace: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, ref); err != nil {
		return err
	}
	q := db.New(tx)

	if err := q.DeleteEvidence(ctx, db.DeleteEvidenceParams{
		SessionID: ref.SessionID, ExtractionVersion: extractionVersion,
	}); err != nil {
		return fmt.Errorf("evaluation: clearing spans: %w", err)
	}
	for _, span := range spans {
		if err := q.InsertEvidenceSpan(ctx, db.InsertEvidenceSpanParams{
			ID: id.New().String(), SessionID: ref.SessionID, Mode: ref.Mode,
			CandidateID: ref.CandidateID, TenantID: ref.TenantID,
			CompetencyID: span.CompetencyID, Kind: span.Kind,
			SegmentSequence: int32(span.SegmentSequence), Quote: span.Quote,
			CharStart: int32(span.CharStart), CharEnd: int32(span.CharEnd),
			StartMs: int32(span.StartMs), EndMs: int32(span.EndMs),
			ExtractionVersion: span.ExtractionVersion,
		}); err != nil {
			return fmt.Errorf("evaluation: storing a span: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// List answers a session's evidence in provenance order.
func (s *Store) List(ctx context.Context, ref SessionRef) ([]StoredSpan, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("evaluation: beginning list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, ref); err != nil {
		return nil, err
	}

	rows, err := db.New(tx).ListEvidence(ctx, ref.SessionID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: listing: %w", err)
	}
	spans := make([]StoredSpan, 0, len(rows))
	for _, row := range rows {
		spans = append(spans, StoredSpan{
			ID: row.ID,
			Span: Span{
				CompetencyID: row.CompetencyID, Kind: row.Kind,
				SegmentSequence: int(row.SegmentSequence), Quote: row.Quote,
				CharStart: int(row.CharStart), CharEnd: int(row.CharEnd),
				StartMs: int(row.StartMs), EndMs: int(row.EndMs),
				ExtractionVersion: row.ExtractionVersion,
			},
			CreatedAt: row.CreatedAt,
		})
	}
	return spans, nil
}

// DecodeSealedInput parses the stored input document.
func DecodeSealedInput(body []byte) (SealedInput, error) {
	var input SealedInput
	if err := json.Unmarshal(body, &input); err != nil {
		return SealedInput{}, fmt.Errorf("evaluation: the sealed input does not decode: %w", err)
	}
	return input, nil
}

// scope enters the session's own row scope: owner for practice, tenant for
// screening, mirroring the schema's dual policy.
func scope(ctx context.Context, tx pgx.Tx, ref SessionRef) error {
	if ref.Mode == "practice" {
		return database.SetUser(ctx, tx, ref.CandidateID)
	}
	return database.SetTenant(ctx, tx, ref.TenantID)
}
