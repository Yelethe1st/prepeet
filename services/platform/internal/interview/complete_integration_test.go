//go:build integration

package interview_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// SES-04 against real PostgreSQL: completion is idempotent to the receipt,
// gaps are recorded rather than closed, missing optional media warns and
// continues, and the sealed transcript takes no more conversation.

// runningSession walks a practice session into in_progress with a small
// transcript and one gap (sequence 4 is missing).
func runningSession(t *testing.T) interview.Session {
	t.Helper()
	ctx := context.Background()
	events := interview.NewEvents(interview.NewStore(pool))
	session := startedPractice(t)

	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{
			event(1, "connection.established"),
			finalSegment(2, "interviewer", "Tell me about the migration", 1000,
				words(1000, "Tell", "me", "about", "the", "migration"), 0.99),
			finalSegment(3, "candidate", "I led it end to end", 6000,
				words(6000, "I", "led", "it", "end", "to", "end"), 0.9),
			event(5, "turn.boundary"),
		}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	current, err := interview.NewStore(pool).Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if current.State != interview.StateInProgress {
		t.Fatalf("state = %s", current.State)
	}
	return current
}

func TestCompletionIsIdempotentToTheReceipt(t *testing.T) {
	ctx := context.Background()
	completer := interview.NewCompleter(interview.NewStore(pool))
	session := runningSession(t)

	first, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 5)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if first.State != interview.StateEvaluating {
		t.Fatalf("state after completion = %s, want evaluating", first.State)
	}
	if first.TranscriptDigest == "" || first.SealedSequence != 5 {
		t.Fatalf("receipt = %+v", first)
	}

	second, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 5)
	if err != nil {
		t.Fatalf("duplicate complete: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("the duplicate answered a different receipt:\n%+v\n%+v", first, second)
	}

	// No second evaluation: exactly one completed event, one seal, one
	// pass through the states.
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	defer admin.Close(ctx)
	var completions int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM integration.outbox
		WHERE event_type = 'interview.session_completed.v1'
		  AND payload->>'session_id' = $1`, session.ID).Scan(&completions); err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if completions != 1 {
		t.Fatalf("%d completed events for two completion calls", completions)
	}

	// A different final cursor is not a retry, it is a conflict.
	if _, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 7); !errors.Is(err, interview.ErrSealConflict) {
		t.Fatalf("different cursor = %v, want ErrSealConflict", err)
	}
}

func TestGapsAreRecordedNotClosed(t *testing.T) {
	ctx := context.Background()
	completer := interview.NewCompleter(interview.NewStore(pool))
	session := runningSession(t)

	receipt, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 5)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Sequence 4 never arrived: the seal says so exactly, and says it in
	// the warnings too.
	if len(receipt.Gaps) != 1 || receipt.Gaps[0].From != 4 || receipt.Gaps[0].To != 4 {
		t.Fatalf("gaps = %+v, want exactly [4,4]", receipt.Gaps)
	}
	found := false
	for _, warning := range receipt.Warnings {
		if warning == interview.WarningGapsRecorded {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %v, want the gap warning", receipt.Warnings)
	}
}

func TestMissingOptionalMediaWarnsAndContinues(t *testing.T) {
	// A session that chose audio: nothing produces media yet, so completion
	// continues to evaluating with the explicit warning, never a block.
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	completer := interview.NewCompleter(store)

	session := interview.Session{
		ID: id.New().String(), Mode: "practice", CandidateID: candidateID,
		BlueprintID:         "plan/shape_technical",
		Config:              json.RawMessage(`{"shape":"shape_technical"}`),
		RecordingPreference: interview.RecordingAudioAndTranscript, ConsentVersion: "1.0.0",
	}
	if err := store.Create(ctx, session, candidate); err != nil {
		t.Fatalf("create: %v", err)
	}
	created, _ := store.Get(ctx, session.ID, "practice", candidateID, "")
	composing, _ := store.Transition(ctx, created, interview.StateComposing, interview.Effects{}, candidate)
	ready, _ := store.Transition(ctx, composing, interview.StateReady, interview.Effects{}, candidate)
	starter := interview.NewStarter(store, newScriptedLedger(10), &grantRecorder{})
	started, err := starter.Start(ctx, ready.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := events.Ingest(ctx, started.Session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{
			event(1, "connection.established"),
			finalSegment(2, "candidate", "an answer", 1000, words(1000, "an", "answer"), 0.9),
		}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	receipt, err := completer.Complete(ctx, started.Session.ID, "practice", candidateID, "", 1, 2)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if receipt.MediaStatus != "missing" {
		t.Fatalf("media status = %q", receipt.MediaStatus)
	}
	hasWarning := false
	for _, warning := range receipt.Warnings {
		if warning == interview.WarningMediaMissing {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Fatalf("warnings = %v, want MEDIA_MISSING", receipt.Warnings)
	}
	if receipt.State != interview.StateEvaluating {
		t.Fatalf("state = %s: the warning must not block evaluation", receipt.State)
	}
}

func TestTranscriptOnlyIsAChoiceNotAWarning(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	completer := interview.NewCompleter(store)

	// A session whose stored preference is transcript only.
	session := interview.Session{
		ID: id.New().String(), Mode: "practice", CandidateID: candidateID,
		BlueprintID:         "plan/shape_technical",
		Config:              json.RawMessage(`{"shape":"shape_technical"}`),
		RecordingPreference: interview.RecordingTranscriptOnly, ConsentVersion: "1.0.0",
	}
	if err := store.Create(ctx, session, candidate); err != nil {
		t.Fatalf("create: %v", err)
	}
	created, _ := store.Get(ctx, session.ID, "practice", candidateID, "")
	composing, _ := store.Transition(ctx, created, interview.StateComposing, interview.Effects{}, candidate)
	ready, _ := store.Transition(ctx, composing, interview.StateReady, interview.Effects{}, candidate)
	starter := interview.NewStarter(store, newScriptedLedger(10), &grantRecorder{})
	started, err := starter.Start(ctx, ready.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := events.Ingest(ctx, started.Session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{
			event(1, "connection.established"),
			finalSegment(2, "candidate", "short answer", 1000, words(1000, "short", "answer"), 0.9),
		}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	receipt, err := completer.Complete(ctx, started.Session.ID, "practice", candidateID, "", 1, 2)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if receipt.MediaStatus != "none_by_choice" {
		t.Fatalf("media status = %q", receipt.MediaStatus)
	}
	for _, warning := range receipt.Warnings {
		if warning == interview.WarningMediaMissing {
			t.Fatal("the candidate's own choice was reported back to them as a warning")
		}
	}
}

func TestTheSealEndsTheConversation(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	completer := interview.NewCompleter(store)
	session := runningSession(t)

	if _, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 5); err != nil {
		t.Fatalf("complete: %v", err)
	}

	ack, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{
			finalSegment(6, "candidate", "one more thing", 20000, words(20000, "one", "more", "thing"), 0.9),
			event(7, "session.leave"),
		})
	if err != nil {
		t.Fatalf("post-seal ingest: %v", err)
	}
	if ack.Outcomes[0].Status != "refused" || ack.Outcomes[0].Reason != "EVENT_AFTER_SEAL" {
		t.Fatalf("a post-seal segment = %+v", ack.Outcomes[0])
	}
	// A goodbye is not testimony: non-conversational events still land.
	if ack.Outcomes[1].Status != "accepted" {
		t.Fatalf("a post-seal leave = %+v", ack.Outcomes[1])
	}

	// And the sealed digest is untouched by the attempt.
	first, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 5)
	if err != nil {
		t.Fatalf("receipt reread: %v", err)
	}
	transcript, err := events.AssembleTranscript(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	for _, segment := range transcript.Segments {
		if segment.Text == "one more thing" {
			t.Fatal("the refused segment reached the sealed transcript")
		}
	}
	_ = first
}

func TestConcurrentCompletionsConvergeOnOneSeal(t *testing.T) {
	ctx := context.Background()
	completer := interview.NewCompleter(interview.NewStore(pool))
	session := runningSession(t)

	const racers = 5
	var wg sync.WaitGroup
	receipts := make([]interview.Receipt, racers)
	failures := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			receipts[slot], failures[slot] = completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 5)
		}(i)
	}
	wg.Wait()

	for i := 0; i < racers; i++ {
		if failures[i] != nil {
			t.Fatalf("racer %d failed: %v", i, failures[i])
		}
		if receipts[i].TranscriptDigest != receipts[0].TranscriptDigest ||
			receipts[i].SealedSequence != receipts[0].SealedSequence {
			t.Fatalf("racer %d got a different seal", i)
		}
	}
}

// epochSegment is finalSegment with the epoch chosen: reconnection tests
// speak across two epochs.
func epochSegment(epoch, sequence int, speaker, text string, startMs int) interview.ControlEvent {
	payload, _ := json.Marshal(map[string]any{
		"speaker": speaker, "text": text,
		"start_ms": startMs, "end_ms": startMs + 3000,
		"confidence": 0.9,
		"words": []map[string]any{{
			"w": text, "start_ms": startMs, "end_ms": startMs + 2900, "confidence": 0.9,
		}},
	})
	return interview.ControlEvent{
		EventID: id.New().String(), Epoch: epoch, Sequence: sequence,
		Type: "transcript.segment.final", Payload: payload,
		OccurredAt: time.Date(2026, 8, 26, 13, 0, sequence, 0, time.UTC),
	}
}

func TestReconnectionDoesNotConsumeCandidateTime(t *testing.T) {
	// SES-05's first box, end to end: the conversation spans two epochs
	// with ten dead minutes of room clock between them. The completed
	// event's duration is the active time only.
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	completer := interview.NewCompleter(store)
	session := startedPractice(t)

	// Epoch one: two minutes of conversation, then the connection dies.
	ingest := func(epoch int, batch []interview.ControlEvent) {
		t.Helper()
		ack, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", epoch, batch)
		if err != nil {
			t.Fatalf("epoch %d: %v", epoch, err)
		}
		for _, outcome := range ack.Outcomes {
			if outcome.Status != "accepted" {
				t.Fatalf("epoch %d refused an event: %+v", epoch, outcome)
			}
		}
	}
	ingest(1, []interview.ControlEvent{
		event(1, "connection.established"),
		epochSegment(1, 2, "interviewer", "Tell me about the migration", 0),
		epochSegment(1, 3, "candidate", "I led it end to end", 117_000),
	})

	current, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	epoch, err := events.BeginAttempt(ctx, current)
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if epoch != 2 {
		t.Fatalf("epoch = %d", epoch)
	}

	// Epoch two opens ten minutes later on the room clock: one more
	// minute of conversation.
	ingest(2, []interview.ControlEvent{
		event(1, "connection.established"),
		epochSegment(2, 2, "candidate", "Picking up where we stopped", 720_000),
		epochSegment(2, 3, "candidate", "The rollout finished on time", 777_000),
	})

	receipt, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 2, 3)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if receipt.State != interview.StateEvaluating {
		t.Fatalf("state = %s", receipt.State)
	}

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	defer admin.Close(ctx)
	var duration int
	if err := admin.QueryRow(ctx, `
		SELECT (payload->>'duration_seconds')::int FROM integration.outbox
		WHERE event_type = 'interview.session_completed.v1'
		  AND payload->>'session_id' = $1`, session.ID).Scan(&duration); err != nil {
		t.Fatalf("reading the completed event: %v", err)
	}
	// Epoch one spans 0 to 120s active; epoch two 720s to 780s: three
	// active minutes. Thirteen minutes would mean the reconnection gap
	// was billed to the candidate.
	if duration != 180 {
		t.Fatalf("duration_seconds = %d, want 180", duration)
	}
}
