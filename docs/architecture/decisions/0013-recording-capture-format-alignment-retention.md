# ADR-0013: Recording is SFU egress: Opus, one clock, and sometimes never written

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-26  
**Review date:** 2027-02-26  
**Supersedes:** None  
**Superseded by:** None

Closes DEC-07: where audio is captured, in what format, how it aligns to
the transcript, and how long each artifact lives. Builds on ADR-0012's
topology.

## Context

Word-level timing is what makes articulation measurable and evidence
playable: [articulation-system.md](../architecture/../articulation-system.md)
computes words per minute, pause duration and fillers from objective audio
features a model is never allowed to invent, and evaluation evidence spans
must point into both transcript and audio. CAT-05 already stores each
session's recording preference, chosen against a versioned consent text:
audio and transcript, or transcript only, whose named forfeits are replay
and delivery measurement.

## Decision

### Capture point: server-side egress at the SFU, and nowhere else

Recording is LiveKit egress of the room's audio, written through the
existing object store under the session's key layout. One capture point on
our infrastructure means one codec, one path to audit, and no trust in a
client upload. The browser records nothing (ADR-0012 records the deviation
from the prototype's device-capture copy).

Two tracks are kept when recording at all: the candidate's audio and the
interviewer's synthesized audio, as separate egress tracks rather than a
mix. Delivery measurement concerns the candidate's speech alone, and a mix
would put the interviewer's voice inside the candidate's acoustic features.
Replay mixes at playback.

### Format: Opus in WebM

Opus at 48 kHz, in WebM. It is what WebRTC already speaks, so egress is a
remux rather than a transcode; it is playable in every browser the product
supports, which makes replay a presigned GET of the artifact itself (the
playback path PLT-05 built); and it is compact enough that retention cost
is dominated by policy, not codec. Transcoding pipelines, mezzanine
formats and WAV masters are rejected as machinery for a fidelity nothing
downstream consumes: STT runs on the live stream, not the recording.

### Alignment: the room's timebase is the only clock

Every artifact carries timestamps from the SFU's room clock: transcript
segments as the agent emits them, word timings from STT, turn boundaries,
and the egress recording's start offset. Alignment is therefore arithmetic
against one clock, never reconciliation between clocks. The transcript
artifact stores, per turn: speaker, room-time span, text, and word timings
with confidence. An evidence span is a room-time interval, which locates
the same moment in transcript and audio by construction.

Recording-quality metrics (dropout, signal level) are computed at capture
and stored beside the artifact, because practice-mode.md forbids poor
recording quality from becoming poor articulation: segments below the
quality floor are marked unassessable before any model sees them.

### Retention: the preference decides what exists, DEC-15 decides how long

- **Transcript only.** Audio egress is never started. The promise the
  consent text makes ("audio is discarded the moment the session ends") is
  exceeded structurally: durable audio never exists, so there is nothing
  to discard, delete late, or leak. This is the strongest honouring
  available and it is trivially auditable.
- **Audio and transcript.** Egress runs; the artifact lands under the
  session key with its digest recorded, deletable by the candidate for
  practice sessions per the privacy surface.
- **Durations are DEC-15's.** This ADR fixes the artifact set and the
  deletion mechanics (session-keyed objects, rows outliving objects, the
  digest answerable after deletion, exactly as PRO-02 established for
  documents); the schedules per data category, legal basis included,
  belong to the retention decision and are not pre-empted here.

### The consequence of declining audio retention

Recorded plainly, because DEC-07 requires it: a transcript-only session
keeps its score, its evidence and its coaching, and permanently forfeits
replay and delivery measurement (pace, pauses, fillers) for that session.
Articulation features that require audio are marked not-collected rather
than zero, and the results screen says so in those words. The forfeit is
named at the moment of choosing (CAT-05's wizard consent card) and again on
the results the person reads.

## Consequences

- RTC-05 implements egress honouring the session row's preference; ART-01
  consumes word timings that already share the recording's clock.
- The quality floor and its thresholds land with QUA-05's measured
  word-error evidence; until then the floor is conservative and marks
  borderline segments unassessable rather than guessing.
- Replay is a presigned GET plus a transcript; no player infrastructure is
  owed.

## Alternatives considered

- **Client-side capture** (rejected in ADR-0012: second clock, per-browser
  codecs, trusted upload, cannot save a conversation anyway).
- **Mixed single-track recording** (rejected: pollutes the candidate's
  acoustic features with the interviewer's voice; mixing belongs at
  playback).
- **Transcode to AAC/MP4 for compatibility** (rejected: WebM/Opus plays
  everywhere the product supports, and a transcode step is a place for
  fidelity and alignment to quietly drift).
