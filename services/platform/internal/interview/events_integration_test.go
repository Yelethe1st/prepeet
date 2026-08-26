//go:build integration

package interview_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// RTC-02 against real PostgreSQL: the three boxes as behaviour. Duplicates
// and disorder cannot corrupt the timeline, a stale epoch cannot write into
// a session that moved on, and replay from a cursor is deterministic.

// startedPractice walks a session to connecting with epoch one open.
func startedPractice(t *testing.T) interview.Session {
	t.Helper()
	ctx := context.Background()
	starter := interview.NewStarter(interview.NewStore(pool), newScriptedLedger(10), &grantRecorder{})
	session := readySession(t)
	started, err := starter.Start(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.Session.ConnectionEpoch != 1 {
		t.Fatalf("start opened epoch %d, want 1", started.Session.ConnectionEpoch)
	}
	return started.Session
}

func event(sequence int, kind string) interview.ControlEvent {
	return interview.ControlEvent{
		EventID: id.New().String(), Epoch: 1, Sequence: sequence, Type: kind,
		Payload:    json.RawMessage(fmt.Sprintf(`{"n":%d}`, sequence)),
		OccurredAt: time.Date(2026, 8, 26, 12, 0, sequence, 0, time.UTC),
	}
}

func TestDisorderAndDuplicatesCannotCorruptTheTimeline(t *testing.T) {
	ctx := context.Background()
	events := interview.NewEvents(interview.NewStore(pool))
	session := startedPractice(t)

	established := event(1, "connection.established")
	second := event(2, "transcript.segment.final")
	fourth := event(4, "transcript.segment.final")

	// Out of order, with a gap, and the first event twice in one batch.
	ack, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{fourth, established, established, second})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if ack.Accepted != 2 {
		t.Fatalf("accepted = %d, want 2: contiguity stops at the gap", ack.Accepted)
	}
	if len(ack.Missing) != 1 || ack.Missing[0].From != 3 || ack.Missing[0].To != 3 {
		t.Fatalf("missing = %+v, want exactly [3,3]", ack.Missing)
	}
	statuses := map[string]int{}
	for _, outcome := range ack.Outcomes {
		statuses[outcome.Status]++
	}
	if statuses["accepted"] != 3 || statuses["duplicate"] != 1 {
		t.Fatalf("outcomes = %v", statuses)
	}

	// The whole batch retried: everything converges to duplicate, the
	// cursor stands, nothing doubled.
	again, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{fourth, established, second})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if again.Accepted != 2 {
		t.Fatalf("retried accepted = %d", again.Accepted)
	}
	for _, outcome := range again.Outcomes {
		if outcome.Status != "duplicate" {
			t.Fatalf("retried outcome = %+v", outcome)
		}
	}

	// Filling the gap closes it and the cursor catches up past it.
	closed, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(3, "turn.boundary")})
	if err != nil {
		t.Fatalf("closing the gap: %v", err)
	}
	if closed.Accepted != 4 || len(closed.Missing) != 0 {
		t.Fatalf("after the gap closed: accepted %d missing %+v", closed.Accepted, closed.Missing)
	}

	// A DIFFERENT event claiming an occupied slot is corruption, refused.
	usurper := event(2, "transcript.segment.final")
	conflicted, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{usurper})
	if err != nil {
		t.Fatalf("conflict ingest: %v", err)
	}
	if conflicted.Outcomes[0].Status != "refused" || conflicted.Outcomes[0].Reason != "SEQUENCE_CONFLICT" {
		t.Fatalf("slot conflict = %+v", conflicted.Outcomes[0])
	}

	// The persisted cursor survives on the session row, not in anybody's
	// browser: a fresh read answers it.
	current, err := interview.NewStore(pool).Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if current.AcceptedSequence != 4 || current.ConnectionEpoch != 1 {
		t.Fatalf("persisted cursor = epoch %d seq %d", current.ConnectionEpoch, current.AcceptedSequence)
	}
	// And the established connection moved the session into progress.
	if current.State != interview.StateInProgress {
		t.Fatalf("state = %s, want in_progress after connection.established", current.State)
	}
}

func TestAStaleEpochCannotMutateASessionThatMovedOn(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	session := startedPractice(t)

	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(1, "connection.established")}); err != nil {
		t.Fatalf("epoch one ingest: %v", err)
	}

	// Takeover: a second attempt opens epoch two.
	current, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	epoch, err := events.BeginAttempt(ctx, current)
	if err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	if epoch != 2 {
		t.Fatalf("second attempt epoch = %d", epoch)
	}

	// The stale tab keeps talking: refused whole, by name, storing nothing.
	_, err = events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(2, "transcript.segment.final")})
	if !errors.Is(err, interview.ErrEpochStale) {
		t.Fatalf("stale epoch = %v, want ErrEpochStale", err)
	}
	replayed, err := events.Replay(ctx, session.ID, "practice", candidateID, "", 0, 0)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	for _, stored := range replayed {
		if stored.Epoch == 1 && stored.Sequence == 2 {
			t.Fatal("the stale tab's event reached the timeline")
		}
	}

	// The cursor reset with the new epoch: sequence orders within an epoch.
	fresh, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if fresh.ConnectionEpoch != 2 || fresh.AcceptedSequence != 0 {
		t.Fatalf("after takeover: epoch %d cursor %d", fresh.ConnectionEpoch, fresh.AcceptedSequence)
	}
}

func TestReplayFromACursorIsDeterministic(t *testing.T) {
	ctx := context.Background()
	events := interview.NewEvents(interview.NewStore(pool))
	session := startedPractice(t)

	batch := []interview.ControlEvent{
		event(1, "connection.established"),
		event(2, "transcript.segment.final"),
		event(3, "turn.boundary"),
		event(4, "transcript.segment.corrected"),
		// An ephemeral partial rides along: acknowledged, never stored,
		// never a slot, so its absence from replay is correctness.
		{EventID: id.New().String(), Epoch: 1, Type: "transcript.segment.partial",
			Payload: json.RawMessage(`{"text":"um"}`), OccurredAt: time.Now()},
	}
	// Ingested in shuffled order across two batches with a repeat.
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{batch[2], batch[0], batch[4]}); err != nil {
		t.Fatalf("first batch: %v", err)
	}
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{batch[3], batch[1], batch[2]}); err != nil {
		t.Fatalf("second batch: %v", err)
	}

	first, err := events.Replay(ctx, session.ID, "practice", candidateID, "", 0, 0)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	second, err := events.Replay(ctx, session.ID, "practice", candidateID, "", 0, 0)
	if err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("two replays from one cursor disagreed")
	}

	// The order is the timeline's, whatever order ingestion saw.
	if len(first) != 4 {
		t.Fatalf("replayed %d events, want the 4 durable ones", len(first))
	}
	for i, stored := range first {
		if stored.Sequence != i+1 {
			t.Fatalf("position %d holds sequence %d", i, stored.Sequence)
		}
	}

	// From a mid-timeline cursor: exactly the tail, same both times.
	tail, err := events.Replay(ctx, session.ID, "practice", candidateID, "", 1, 2)
	if err != nil {
		t.Fatalf("tail replay: %v", err)
	}
	if len(tail) != 2 || tail[0].Sequence != 3 || tail[1].Sequence != 4 {
		t.Fatalf("tail = %+v", tail)
	}
}

func TestTheEventLogIsAppendOnlyAndUnknownTypesAreRefused(t *testing.T) {
	ctx := context.Background()
	events := interview.NewEvents(interview.NewStore(pool))
	session := startedPractice(t)

	ack, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{
			event(1, "connection.established"),
			event(2, "telemetry.everything"),
		})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if ack.Outcomes[1].Status != "refused" || ack.Outcomes[1].Reason != "EVENT_TYPE_UNKNOWN" {
		t.Fatalf("unknown type = %+v", ack.Outcomes[1])
	}

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	defer admin.Close(ctx)
	_, err = admin.Exec(ctx,
		`UPDATE interview.control_events SET payload = '{}' WHERE session_id = $1`, session.ID)
	if err == nil {
		t.Fatal("the event log accepted an edit")
	}
}
