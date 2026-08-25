//go:build integration

package candidate_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
)

// PRO-03's pipeline against real PostgreSQL: the upload announces itself, the
// activities store span-linked facts idempotently, and every outcome the
// journey depends on - extracted, unsupported, failed - is a guarded state on
// the document row rather than a hope.
//
// The extractor itself is faked here; its real behaviour over the wire is the
// Python suite's and the worker adapter's subject.

// fakeExtractor answers with a fixed reading, or a typed refusal.
type fakeExtractor struct {
	facts   []candidate.ExtractedFact
	failure *candidate.ExtractFailure
	calls   int
}

func (f *fakeExtractor) Extract(_ context.Context, _ candidate.ExtractRequest) ([]candidate.ExtractedFact, error) {
	f.calls++
	if f.failure != nil {
		return nil, f.failure
	}
	return f.facts, nil
}

func cvFacts() []candidate.ExtractedFact {
	return []candidate.ExtractedFact{
		{Kind: "role", Value: json.RawMessage(`{"title":"Senior Backend Engineer","confidence":0.7}`),
			SpanStart: 12, SpanEnd: 55, Confidence: 0.7, ExtractorVersion: "extract-1"},
		{Kind: "skill", Value: json.RawMessage(`{"name":"Go","confidence":0.8}`),
			SpanStart: 80, SpanEnd: 82, Confidence: 0.8, ExtractorVersion: "extract-1"},
		{Kind: "unparsed", Value: json.RawMessage(`{"text":"I volunteer at a coding club","confidence":0}`),
			SpanStart: 90, SpanEnd: 140, Confidence: 0, ExtractorVersion: "extract-1"},
	}
}

func TestStoringADocumentAnnouncesItAndQueuesExtraction(t *testing.T) {
	// The seam between PRO-02 and PRO-03: completing an upload marks the
	// document pending and publishes candidate.document_uploaded.v1 in the
	// same transaction, so there is no stored CV the pipeline never heard of.
	ctx := context.Background()
	service := documents(t)

	stored := uploadCV(t, service, amaraID, []byte("%PDF-1.7 announce me"))

	if stored.ExtractionState != "pending" {
		t.Fatalf("extraction state after storing = %q, want pending", stored.ExtractionState)
	}

	var payload []byte
	if err := pool.QueryRow(ctx, `
		SELECT payload FROM integration.outbox
		WHERE event_type = 'candidate.document_uploaded.v1'
		  AND payload->>'document_id' = $1`, stored.ID).Scan(&payload); err != nil {
		t.Fatalf("the uploaded event was not published: %v", err)
	}
	var event struct {
		CandidateID string `json:"candidate_id"`
		MediaType   string `json:"media_type"`
		ByteSize    int64  `json:"byte_size"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decoding the event: %v", err)
	}
	if event.CandidateID != amaraID || event.MediaType != "application/pdf" || event.ByteSize != stored.SizeBytes {
		t.Fatalf("the event does not describe the upload: %+v", event)
	}
}

func TestExtractionStoresFactsWithTheirExactSpans(t *testing.T) {
	ctx := context.Background()
	service := documents(t)
	stored := uploadCV(t, service, amaraID, []byte("%PDF-1.7 extract me"))

	extractor := &fakeExtractor{facts: cvFacts()}
	activities := candidate.NewExtractionActivities(candidate.NewStore(pool), extractor)
	input := candidate.ExtractionInput{DocumentID: stored.ID, CandidateID: amaraID}

	facts, err := activities.Extract(ctx, input)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if err := activities.StoreFacts(ctx, input, facts); err != nil {
		t.Fatalf("StoreFacts: %v", err)
	}

	rows := factRows(t, amaraID, stored.ID)
	if len(rows) != 3 {
		t.Fatalf("%d facts stored, want 3", len(rows))
	}
	// The first acceptance criterion: the stored span is the extractor's, exactly.
	if rows[0].kind != "role" || rows[0].spanStart != 12 || rows[0].spanEnd != 55 {
		t.Fatalf("first fact = %+v; the span is the provenance and must survive storage", rows[0])
	}
	// The second: unparsed text is a stored fact, not a silent drop.
	last := rows[len(rows)-1]
	if last.kind != "unparsed" {
		t.Fatalf("the unparsed remainder was not stored: %+v", rows)
	}

	// And the document says so.
	versions, err := service.List(ctx, amaraID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, version := range versions {
		if version.ID == stored.ID && version.ExtractionState != "extracted" {
			t.Fatalf("extraction state = %q, want extracted", version.ExtractionState)
		}
	}
}

func TestAReplayedStoreConvergesAndSparesTheCandidatesOwnWork(t *testing.T) {
	// The exactly-once shape: a worker death after commit replays StoreFacts.
	// Proposals are replaced wholesale - same rows, not doubled rows - and a
	// fact the candidate has already confirmed is not extraction's to touch.
	ctx := context.Background()
	service := documents(t)
	stored := uploadCV(t, service, priyaID, []byte("%PDF-1.7 replay me"))

	activities := candidate.NewExtractionActivities(candidate.NewStore(pool), &fakeExtractor{})
	input := candidate.ExtractionInput{DocumentID: stored.ID, CandidateID: priyaID}
	facts := cvFacts()

	if err := activities.StoreFacts(ctx, input, facts); err != nil {
		t.Fatalf("first store: %v", err)
	}

	// The candidate confirms the skill; PRO-04's move, made directly here.
	if _, err := ownerExec(ctx, priyaID, `
		UPDATE candidate.extracted_facts SET status = 'confirmed'
		WHERE document_id = $1 AND kind = 'skill'`, stored.ID); err != nil {
		t.Fatalf("confirming: %v", err)
	}

	// The replay.
	if err := activities.StoreFacts(ctx, input, facts); err != nil {
		t.Fatalf("replayed store: %v", err)
	}

	rows := factRows(t, priyaID, stored.ID)
	confirmed := 0
	for _, row := range rows {
		if row.status == "confirmed" {
			confirmed++
		}
	}
	if confirmed != 1 {
		t.Fatalf("%d confirmed facts after replay, want the candidate's 1 kept", confirmed)
	}
	// One replaced set plus the confirmed row the delete spared: the replay
	// re-proposes the skill beside the confirmed copy rather than deleting
	// the candidate's decision.
	if len(rows) != len(facts)+1 {
		t.Fatalf("%d facts after replay, want %d", len(rows), len(facts)+1)
	}
}

func TestTheOutcomeStatesAreGuarded(t *testing.T) {
	// The degradation criterion's storage half: unsupported is a state the
	// document reaches from pending, holds against a late failed, and repeats
	// idempotently.
	ctx := context.Background()
	service := documents(t)
	stored := uploadCV(t, service, priyaID, []byte("%PDF-1.7 unsupported"))

	activities := candidate.NewExtractionActivities(candidate.NewStore(pool), &fakeExtractor{})
	input := candidate.ExtractionInput{DocumentID: stored.ID, CandidateID: priyaID}

	if err := activities.MarkExtractionOutcome(ctx, input, "unsupported"); err != nil {
		t.Fatalf("marking unsupported: %v", err)
	}
	// The replay lands on success, because the guard admits the target state.
	if err := activities.MarkExtractionOutcome(ctx, input, "unsupported"); err != nil {
		t.Fatalf("replayed mark: %v", err)
	}
	// A different outcome cannot overwrite a decided one.
	if err := activities.MarkExtractionOutcome(ctx, input, "failed"); !errors.Is(err, candidate.ErrDocumentState) {
		t.Fatalf("overwriting unsupported with failed = %v, want ErrDocumentState", err)
	}

	// And a decided document refuses re-extraction rather than resurrecting.
	if _, err := activities.Extract(ctx, input); err == nil {
		t.Fatal("extracting an already-decided document must refuse")
	}
}

// ownerExec runs one statement under the owner's row scope, the way every
// candidate-schema access must: app.user_id set, no tenant context.
func ownerExec(ctx context.Context, userID, statement string, args ...any) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.user_id', $1, true)", userID); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, statement, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), tx.Commit(ctx)
}

// factRows reads the stored facts in span order under the owner's scope.
type factRow struct {
	kind      string
	spanStart int
	spanEnd   int
	status    string
}

func factRows(t *testing.T, userID, documentID string) []factRow {
	t.Helper()
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning read: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.user_id', $1, true)", userID); err != nil {
		t.Fatalf("scoping: %v", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT kind, span_start, span_end, status
		FROM candidate.extracted_facts WHERE document_id = $1
		ORDER BY span_start, span_end, status`, documentID)
	if err != nil {
		t.Fatalf("reading facts: %v", err)
	}
	defer rows.Close()

	var out []factRow
	for rows.Next() {
		var row factRow
		if err := rows.Scan(&row.kind, &row.spanStart, &row.spanEnd, &row.status); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		out = append(out, row)
	}
	return out
}
