//go:build integration

package interview_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// SES-06 against real PostgreSQL: the drop is the server's state, not the
// tab's. A connection loss moves a running session to reconnecting and
// announces it; recovery returns it to in_progress; and the interruption
// fact lands from the timeline's own durable event, deduplicated by the
// same identity every other control event is.

// inProgressPractice walks a session to in_progress on epoch one.
func inProgressPractice(t *testing.T) interview.Session {
	t.Helper()
	ctx := context.Background()
	events := interview.NewEvents(interview.NewStore(pool))
	session := startedPractice(t)
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(1, "connection.established")}); err != nil {
		t.Fatalf("establishing: %v", err)
	}
	current, err := interview.NewStore(pool).Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if current.State != interview.StateInProgress {
		t.Fatalf("state = %s, want in_progress", current.State)
	}
	return current
}

// interruptedEvents reads the session's interrupted announcements as the
// migrator, because the outbox is no context's own table.
func interruptedEvents(t *testing.T, sessionID string) []map[string]any {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT payload FROM integration.outbox
		WHERE event_type = 'interview.session_interrupted.v1'
		  AND payload->>'session_id' = $1
		ORDER BY occurred_at`, sessionID)
	if err != nil {
		t.Fatalf("outbox read: %v", err)
	}
	defer rows.Close()
	var payloads []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		payloads = append(payloads, payload)
	}
	return payloads
}

func TestAConnectionLossMovesARunningSessionToReconnecting(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	session := inProgressPractice(t)

	ack, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(2, "connection.lost")})
	if err != nil {
		t.Fatalf("ingest lost: %v", err)
	}
	if ack.Outcomes[0].Status != "accepted" {
		t.Fatalf("lost outcome = %+v", ack.Outcomes[0])
	}

	current, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if current.State != interview.StateReconnecting {
		t.Fatalf("state = %s, want reconnecting: the drop is the server's state", current.State)
	}

	// The announcement operations aggregates across sessions: reason from
	// the catalogue's closed set, resumable because grace is still open,
	// attempt naming which interruption this is.
	announced := interruptedEvents(t, session.ID)
	if len(announced) != 1 {
		t.Fatalf("interrupted events = %d, want 1", len(announced))
	}
	payload := announced[0]
	if payload["reason"] != "network" || payload["resumable"] != true {
		t.Fatalf("payload = %+v", payload)
	}
	if payload["attempt"] != float64(1) {
		t.Fatalf("attempt = %v, want 1", payload["attempt"])
	}

	// A second loss while already reconnecting is a durable record, not a
	// second transition and not a second announcement: the session cannot
	// drop out of a state it is already in.
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(3, "connection.lost")}); err != nil {
		t.Fatalf("second lost: %v", err)
	}
	still, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if still.State != interview.StateReconnecting {
		t.Fatalf("state = %s after second loss", still.State)
	}
	if again := interruptedEvents(t, session.ID); len(again) != 1 {
		t.Fatalf("interrupted events after second loss = %d, want still 1", len(again))
	}
}

func TestRecoveryReturnsAReconnectingSessionToProgress(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	session := inProgressPractice(t)

	// A same-epoch blip: the tab lost the room and got it back without a
	// takeover. connection.resumed is the recovery the protocol names for it.
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(2, "connection.lost")}); err != nil {
		t.Fatalf("lost: %v", err)
	}
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(3, "connection.resumed")}); err != nil {
		t.Fatalf("resumed: %v", err)
	}
	recovered, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if recovered.State != interview.StateInProgress {
		t.Fatalf("state = %s after connection.resumed, want in_progress", recovered.State)
	}

	// A drop recovered through a fresh attempt: the new epoch's established
	// connection is the same signal that started the interview, and it must
	// return a reconnecting session to progress, not only a connecting one.
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(4, "connection.lost")}); err != nil {
		t.Fatalf("second lost: %v", err)
	}
	dropped, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if dropped.State != interview.StateReconnecting {
		t.Fatalf("state = %s, want reconnecting", dropped.State)
	}
	epoch, err := events.BeginAttempt(ctx, dropped)
	if err != nil {
		t.Fatalf("begin attempt: %v", err)
	}
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", epoch,
		[]interview.ControlEvent{{
			EventID: id.New().String(), Epoch: epoch, Sequence: 1,
			Type:       "connection.established",
			OccurredAt: time.Date(2026, 8, 26, 12, 5, 0, 0, time.UTC),
		}}); err != nil {
		t.Fatalf("established in new epoch: %v", err)
	}
	current, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if current.State != interview.StateInProgress {
		t.Fatalf("state = %s after new-epoch established, want in_progress", current.State)
	}
	if current.ConnectionEpoch != epoch {
		t.Fatalf("epoch = %d, want %d", current.ConnectionEpoch, epoch)
	}
}

func TestAnInterruptionEventRecordsTheDurableFact(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	session := inProgressPractice(t)

	interruption := interview.ControlEvent{
		EventID: id.New().String(), Epoch: 1, Sequence: 2, Type: "interruption",
		Payload:    json.RawMessage(`{"cause":"connection_lost","duration_seconds":42}`),
		OccurredAt: time.Date(2026, 8, 26, 12, 1, 0, 0, time.UTC),
	}
	ack, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{interruption})
	if err != nil {
		t.Fatalf("ingest interruption: %v", err)
	}
	if ack.Outcomes[0].Status != "accepted" {
		t.Fatalf("interruption outcome = %+v", ack.Outcomes[0])
	}

	facts, err := store.Interruptions(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("reading interruptions: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("interruptions = %d, want 1", len(facts))
	}
	if facts[0].Cause != interview.CauseConnectionLost || facts[0].DurationSeconds != 42 {
		t.Fatalf("fact = %+v", facts[0])
	}
	if facts[0].ConnectionEpoch != 1 {
		t.Fatalf("fact epoch = %d, want 1", facts[0].ConnectionEpoch)
	}

	// The resend converges exactly as any other durable event: same
	// identity, duplicate outcome, no second fact invented.
	again, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{interruption})
	if err != nil {
		t.Fatalf("resend: %v", err)
	}
	if again.Outcomes[0].Status != "duplicate" {
		t.Fatalf("resend outcome = %+v", again.Outcomes[0])
	}
	if facts, err = store.Interruptions(ctx, session.ID, "practice", candidateID, ""); err != nil || len(facts) != 1 {
		t.Fatalf("facts after resend = %d (err %v), want still 1", len(facts), err)
	}

	// A cause outside the vocabulary, or a nonsensical duration, is refused
	// at the door: an interruption record evaluation cannot act on is not a
	// lesser record, it is a wrong one.
	for name, payload := range map[string]string{
		"unknown cause":     `{"cause":"gremlins","duration_seconds":5}`,
		"negative duration": `{"cause":"device_failure","duration_seconds":-1}`,
		"no cause":          `{"duration_seconds":5}`,
	} {
		refused, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
			[]interview.ControlEvent{{
				EventID: id.New().String(), Epoch: 1, Sequence: 9, Type: "interruption",
				Payload:    json.RawMessage(payload),
				OccurredAt: time.Date(2026, 8, 26, 12, 2, 0, 0, time.UTC),
			}})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if refused.Outcomes[0].Status != "refused" || refused.Outcomes[0].Reason != "INTERRUPTION_INVALID" {
			t.Fatalf("%s: outcome = %+v", name, refused.Outcomes[0])
		}
	}
}
