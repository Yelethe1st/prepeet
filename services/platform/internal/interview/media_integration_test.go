//go:build integration

package interview_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
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
	// attempted names every track this recorder was asked for, including the
	// ones it failed, which is what identifies the abandoned claim.
	attempted []string
	// err makes StartTrack fail, which is how a provider outage is reproduced:
	// the claim is already taken by then, so the row it leaves behind is the
	// abandoned claim the takeover branch exists to adopt.
	err error
}

func (f *fakeRecorder) StartTrack(_ context.Context, room, identity, key string) (string, error) {
	if f.err != nil {
		// Recorded even though it failed: the claim was taken before this call,
		// so this names the track whose claim is now abandoned.
		f.attempted = append(f.attempted, identity)
		return "", f.err
	}
	f.started = append(f.started, identity)
	return "eg-" + room[:8] + "-" + identity, nil
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

// ---------------------------------------------------------------------------
// Regressions from the RTC-05 review.

// recordingFailure is a recorder that refuses, so the claim-release path can
// be exercised.
type recordingFailure struct{ attempts int }

func (r *recordingFailure) StartTrack(context.Context, string, string, string) (string, error) {
	r.attempts++
	return "", errors.New("provider unavailable")
}

func (r *recordingFailure) StopTrack(context.Context, string) error { return nil }

// Finding: StartRecording checked the recording preference and nothing else,
// so a delayed or retried interview.session_started.v1 could begin egress
// after the candidate had finished and the session was sealed. Capture would
// continue past the consent it was given, and because completion had already
// run there was no later step to stop it or account for it.
func TestARecordingDoesNotStartAfterTheSessionIsSealed(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	session := audioSession(t)
	recorder := &fakeRecorder{}

	// Complete first, which seals. This is the delayed-delivery order: the
	// candidate has finished before the start event is handled.
	if _, err := interview.NewCompleter(store).
		Complete(ctx, session.ID, "practice", candidateID, "", 1, 2); err != nil {
		t.Fatalf("complete: %v", err)
	}

	current, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if err := store.StartRecording(ctx, recorder, current); err != nil {
		t.Fatalf("start after completion: %v", err)
	}

	if len(recorder.started) != 0 {
		t.Fatalf("egress began after the session was sealed: %v", recorder.started)
	}
}

// The claim and the state check are one transaction, so the slot is only ever
// taken by a session that may still record.
func TestARecordingDoesNotStartOnceTheSessionHasLeftTheLiveStates(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	session := audioSession(t)
	recorder := &fakeRecorder{}

	current, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	ended, err := store.Transition(ctx, current, interview.StateFinalizing, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("transition: %v", err)
	}

	if err := store.StartRecording(ctx, recorder, ended); err != nil {
		t.Fatalf("start after finalizing: %v", err)
	}

	if len(recorder.started) != 0 {
		t.Fatalf("egress began for a session that had left the live states: %v", recorder.started)
	}
}

// Finding: the check, the provider call and the insert were three steps, so
// two deliveries could both reach the provider and only one keep its egress
// id. The other job records the same participant to the same key and nothing
// ever stops it. The claim is taken first now, and losing it means starting
// nothing.
func TestTwoConcurrentDeliveriesStartOneEgressPerTrack(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	session := audioSession(t)

	first := &fakeRecorder{}
	second := &fakeRecorder{}
	errs := make(chan error, 2)
	for _, recorder := range []*fakeRecorder{first, second} {
		go func(r *fakeRecorder) { errs <- store.StartRecording(ctx, r, session) }(recorder)
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent start: %v", err)
		}
	}

	// One egress per track across both deliveries, not one per delivery.
	started := len(first.started) + len(second.started)
	if started != len(first.started)+len(second.started) || started > 2 {
		t.Fatalf("started %d egress jobs for two tracks: %v then %v",
			started, first.started, second.started)
	}

	tracks, err := store.MediaTracks(ctx, session)
	if err != nil {
		t.Fatalf("reading tracks: %v", err)
	}
	if len(tracks) != started {
		t.Fatalf("%d egress jobs and %d rows: an unstopped job is one nobody will finalise",
			started, len(tracks))
	}
}

// A provider that refuses must give the slot back, or the claim stays with no
// egress behind it and every retry starts nothing at all: a session that
// records silently nothing.
func TestAFailedStartReleasesTheClaimSoARetryCanWork(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	session := audioSession(t)

	failing := &recordingFailure{}
	if err := store.StartRecording(ctx, failing, session); err == nil {
		t.Fatal("a refusing provider was reported as success")
	}

	working := &fakeRecorder{}
	if err := store.StartRecording(ctx, working, session); err != nil {
		t.Fatalf("retry: %v", err)
	}

	if len(working.started) == 0 {
		t.Fatal("the retry started nothing, so the failed claim was never released")
	}
	tracks, err := store.MediaTracks(ctx, session)
	if err != nil {
		t.Fatalf("reading tracks: %v", err)
	}
	for _, track := range tracks {
		if track.EgressID == "" {
			t.Fatalf("track %s has a claim and no egress", track.Track)
		}
	}
}

// The window protects a claim whose owner never came back.
//
// A delivery that fails releases its own claim, so the ordinary retry is
// immediate and TestAFailedStartReleasesTheClaimSoARetryCanWork covers it. The
// case left over is the one nobody can signal: a worker that took the claim and
// died before it could either record an egress id or release. Only age
// separates that from a delivery still mid-call, which is why the window exists
// and why conditioning takeover on an empty egress id alone was a bug: it
// adopted live claims and started a second recording of the same participant.
func TestAClaimWhoseOwnerDiedIsAdoptedOnlyOnceItIsStale(t *testing.T) {
	ctx := context.Background()
	session := audioSession(t)

	// A claim with no egress id and nobody coming back for it, written the way
	// a crashed worker would leave it.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := database.SetUser(ctx, tx, session.CandidateID); err != nil {
		t.Fatalf("scope: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO interview.media_tracks
		 (id, session_id, mode, candidate_id, tenant_id, track, storage_key, egress_id)
		 VALUES ($1, $2, 'practice', $3, NULL, 'candidate', 'k', '')`,
		id.New().String(), session.ID, session.CandidateID); err != nil {
		t.Fatalf("planting the abandoned claim: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Inside the window it is indistinguishable from a delivery in flight.
	live := &fakeRecorder{}
	if err := interview.NewStore(pool).WithClaimStaleAfter("1 hour").
		StartRecording(ctx, live, session); err != nil {
		t.Fatalf("retry inside the window: %v", err)
	}
	if slices.Contains(live.started, session.CandidateID) {
		t.Fatalf("a claim still inside its window was adopted: %v", live.started)
	}

	// Past it, the retry adopts and records.
	adopting := &fakeRecorder{}
	if err := interview.NewStore(pool).WithClaimStaleAfter("0 seconds").
		StartRecording(ctx, adopting, session); err != nil {
		t.Fatalf("retry past the window: %v", err)
	}
	if !slices.Contains(adopting.started, session.CandidateID) {
		t.Fatalf("a dead owner's claim was never adopted, so that track can never record: %v",
			adopting.started)
	}
}
