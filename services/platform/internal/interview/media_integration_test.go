//go:build integration

package interview_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// RTC-05 against real PostgreSQL, the recorder and prober faked: egress
// starts once per track however many times the start retries, finalization
// trusts only what the store reads back, and a missing artifact is stated,
// never pretended. The LiveKit adapter behind the Recorder port is
// exercised when the agent lands; these are the reconciliation semantics.

type fakeRecorder struct {
	started []string
	stopped []string
}

func (f *fakeRecorder) StartTrack(_ context.Context, room, track, key string) (string, error) {
	f.started = append(f.started, track)
	return "eg-" + room[:8] + "-" + track, nil
}

func (f *fakeRecorder) StopTrack(_ context.Context, egressID string) error {
	f.stopped = append(f.stopped, egressID)
	return nil
}

// fakeProber answers per storage key: bytes present or absence.
type fakeProber struct {
	missing map[string]bool
}

func (f fakeProber) Stat(_ context.Context, key string) (int64, string, error) {
	if f.missing[key] {
		return 0, "", errors.New("no such object")
	}
	return 48_000, "sha256:" + fmt.Sprintf("%064d", len(key)), nil
}

// audioSession walks an audio-preference practice session to in_progress.
func audioSession(t *testing.T) interview.Session {
	t.Helper()
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)

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
	current, err := store.Get(ctx, started.Session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return current
}

func TestARetriedStartNeverRecordsTwice(t *testing.T) {
	// RTC-05's first box, reinterpreted by ADR-0013: there is no client
	// upload to resume; what must survive interruption is the recording
	// itself, so a redelivered start or a reconnection converges on the
	// one egress per track already running.
	ctx := context.Background()
	store := interview.NewStore(pool)
	recorder := &fakeRecorder{}
	session := audioSession(t)

	for run := 0; run < 3; run++ {
		if err := store.StartRecording(ctx, recorder, session); err != nil {
			t.Fatalf("start %d: %v", run, err)
		}
	}

	if len(recorder.started) != 2 {
		t.Fatalf("three starts began %d egresses, want 2 (one per track)", len(recorder.started))
	}
	tracks, err := store.MediaTracks(ctx, session)
	if err != nil {
		t.Fatalf("tracks: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("%d track rows, want 2", len(tracks))
	}
	for _, track := range tracks {
		if track.State != "recording" || track.EgressID == "" || track.StorageKey == "" {
			t.Fatalf("track = %+v", track)
		}
	}
}

func TestTranscriptOnlyNeverStartsEgress(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	recorder := &fakeRecorder{}
	session := runningSession(t) // the harness default: transcript_only

	if err := store.StartRecording(ctx, recorder, session); err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(recorder.started) != 0 {
		t.Fatal("a transcript-only session started egress; durable audio must never exist")
	}
}

func TestFinalizationVerifiesEveryTrackBeforeTheSeal(t *testing.T) {
	// The second box: completion stops egress, reads each artifact back,
	// records its digest, and only then seals with media finalized.
	ctx := context.Background()
	store := interview.NewStore(pool)
	recorder := &fakeRecorder{}
	prober := fakeProber{}
	session := audioSession(t)
	if err := store.StartRecording(ctx, recorder, session); err != nil {
		t.Fatalf("start: %v", err)
	}

	completer := interview.NewCompleter(store).WithMedia(recorder, prober)
	receipt, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 2)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if receipt.MediaStatus != "finalized" {
		t.Fatalf("media status = %q, want finalized", receipt.MediaStatus)
	}
	for _, warning := range receipt.Warnings {
		if warning == interview.WarningMediaMissing {
			t.Fatal("a finalized recording still warned MEDIA_MISSING")
		}
	}
	if len(recorder.stopped) != 2 {
		t.Fatalf("%d egresses stopped, want 2", len(recorder.stopped))
	}
	tracks, err := store.MediaTracks(ctx, session)
	if err != nil {
		t.Fatalf("tracks: %v", err)
	}
	for _, track := range tracks {
		if track.State != "finalized" || track.Digest == "" || track.SizeBytes == 0 {
			t.Fatalf("track after finalize = %+v", track)
		}
	}
}

func TestAPartialRecordingIsMissingNeverPretended(t *testing.T) {
	// The third box: one artifact absent means the recording is missing,
	// stated on the seal with its warning, and the absent track's row says
	// so while the present one keeps its digest.
	ctx := context.Background()
	store := interview.NewStore(pool)
	recorder := &fakeRecorder{}
	session := audioSession(t)
	if err := store.StartRecording(ctx, recorder, session); err != nil {
		t.Fatalf("start: %v", err)
	}
	tracks, _ := store.MediaTracks(ctx, session)
	prober := fakeProber{missing: map[string]bool{tracks[0].StorageKey: true}}

	completer := interview.NewCompleter(store).WithMedia(recorder, prober)
	receipt, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 2)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if receipt.MediaStatus != "missing" {
		t.Fatalf("media status = %q, want missing", receipt.MediaStatus)
	}
	warned := false
	for _, warning := range receipt.Warnings {
		if warning == interview.WarningMediaMissing {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("warnings = %v, want MEDIA_MISSING", receipt.Warnings)
	}
	after, _ := store.MediaTracks(ctx, session)
	states := map[string]string{}
	for _, track := range after {
		states[track.Track] = track.State
	}
	if states[after[0].Track] == states[after[1].Track] {
		t.Fatalf("both tracks resolved alike (%v); one landed and one did not", states)
	}
}

func TestStartPublishesTheSessionStartedEvent(t *testing.T) {
	// The event the worker's egress route consumes, published atomically
	// with the in_progress transition and carrying the preference.
	ctx := context.Background()
	session := audioSession(t)

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	defer admin.Close(ctx)
	var count int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM integration.outbox
		WHERE event_type = 'interview.session_started.v1'
		  AND payload->>'session_id' = $1`, session.ID).Scan(&count); err != nil {
		t.Fatalf("reading the started event: %v", err)
	}
	if count != 1 {
		t.Fatalf("%d started events, want exactly 1", count)
	}
}
