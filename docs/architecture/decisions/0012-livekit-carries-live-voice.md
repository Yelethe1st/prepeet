# ADR-0012: LiveKit carries live voice, self-hosted, behind our own agent

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-26  
**Review date:** 2027-02-26  
**Supersedes:** None  
**Superseded by:** None

Closes DEC-06: the realtime provider, the media topology, the authorization
model for media, and what happens when the transport degrades mid-interview.

## Context

The live interview is a low-latency voice loop: the candidate speaks, the
system transcribes, an interviewer persona responds, and speech comes back,
with barge-in, configurable silence tolerance and captions. Several shipped
promises hang on the loop's knobs: extra thinking time doubles the silence
the interviewer waits through, push to talk exists, personas differ in
pacing and interruption style, and captions render each question as it is
spoken.

Interview audio is Restricted in
[data-classification.md](../../security/data-classification.md): a
candidate's voice, under stress, discussing their employment. ADR-0001
commits residency to eu-west-2 with per-tenant residency recorded at
creation. [cost-and-capacity-model.md](../../operations/cost-and-capacity-model.md)
expects model and realtime spend to dominate infrastructure spend. And
practice-mode.md fixes one rule this decision must never break: poor
recording quality never becomes poor articulation.

## Decision

### Provider: LiveKit, self-hosted in eu-west-2

The SFU runs in our VPC. Audio never terminates on infrastructure whose
terms we do not already hold, which is what keeps the residency commitment
about the audio itself rather than about everything except the audio.

The interview loop runs on LiveKit's Python agent framework inside the
intelligence plane. The loop's knobs - VAD windows, turn detection,
barge-in policy, silence tolerance - are ours to set per persona and per
accommodation, which is what the shipped promises require. STT, LLM and TTS
are composed behind our own adapters as separate capabilities; which
providers fill them is DEC-10's decision, made per stage, changeable
without touching transport.

**Rejected: a model vendor's native realtime API (OpenAI Realtime, Gemini
Live).** Fastest demo, worst position: audio terminates at a US vendor's
edge, the loop's turn-taking is theirs, recording needs a second capture
path, and the interviewer becomes inseparable from one vendor's roadmap.
The registry-and-pinning architecture exists to keep what ran swappable and
reproducible; binding transport and conversation loop to one model vendor
is the largest lock-in available in this product.

**Rejected: managed WebRTC or telephony (Daily, Twilio).** Daily is
credible and is the fallback we would pick if we abandoned self-hosting,
but managed media means their regions and their processing terms for
Restricted content, plus a per-minute transport tax on top of the model
bill for every interview minute. Twilio's telephony center of gravity and
pricing fit a phone product, not this one.

### Topology: browser to our SFU to the agent, one capture point

```
browser <-> LiveKit SFU (our VPC, eu-west-2) <-> Python agent (intelligence plane)
                     |                                <-> STT / LLM / TTS via adapters (DEC-10)
                     +-- server-side egress -> S3 (objectstore, session-keyed)
```

The agent joins the room as a participant. Captions travel over the room's
data channel. Recording, when the session's stored preference asks for it,
is server-side egress into the object store under the existing session key
layout; ADR-0013 fixes the format and alignment. One capture point at the
SFU gives every artifact a single clock, which is what makes word-level
alignment a property instead of a reconciliation job.

### Authorization: Go mints the room grant at start

SES-02's start endpoint mints the room token: scoped to exactly one session
and one attempt, identity carried in the grant, TTL bounded by the
session's duration ceiling, never reusable across attempts. The browser
holds no durable media credential, which is the same discipline the
presigned upload and playback paths already follow. The agent's own
credential is a service identity scoped to joining rooms, not to minting
grants.

### Degradation: three tiers, ending in pause, never in silent quality loss

- **Blips, under roughly twenty seconds.** The jitter buffer absorbs what
  it can and the interviewer waits. Waiting through silence is already a
  product feature; a hiccup is indistinguishable from thinking.
- **A real drop.** The session moves to `reconnecting` (the state machine
  has carried the state since SES-01), the interview pauses, and the
  candidate has ten minutes to rejoin and resume from the same question;
  the agent's position is checkpointed in session events, so resume is a
  state read. Past the window the session is `interrupted`, and the
  interruption is represented in evidence as reduced coverage, never as a
  low score.
- **Our own outage.** Every in-flight session takes the same pause and
  interrupt path. Practice candidates rerun free, and an interview
  interrupted by us must not count as a billable start (recorded here for
  DEC-16 to honour). Screening re-invitation authority is DEC-14's.

**Continue-degraded is not offered.** Degraded audio flowing silently into
evaluation converts a network problem into a scoring problem, which is the
one conversion practice-mode.md forbids. Segments the transport damaged are
marked unassessable and surface through the insufficient-evidence
machinery.

### A recorded deviation from the prototype

The prototype's reconnect copy says answers are "captured on your device
and uploaded a few seconds behind you." We are not building client-side
capture. A conversational interviewer cannot respond to audio it never
received, so device capture cannot save a dropped answer's conversation,
only its bytes, at the price of a second clock, per-browser codecs and an
upload path that must be trusted. The promise "a short drop loses nothing"
is kept differently and more honestly: the interview pauses, and anything
the interviewer did not hear is re-asked on resume. The prototype copy is
amended when the RTC screens land.

## Terms review against data-classification.md

Self-hosting collapses the review: no third party processes interview
audio, and the infrastructure terms are the AWS terms ADR-0001 already
covers. Two gates stand:

- **LiveKit Cloud may not be activated** (see exit criteria) until its EU
  data-processing terms have been reviewed against data-classification.md
  and signed by the owner. The fallback is named here; using it is a
  separate, gated act.
- STT, LLM and TTS providers touch Restricted audio and transcript
  content. Their terms are reviewed under DEC-10, per provider, before
  first use. This ADR grants transport nothing it does not grant them.

## Consequences

- We own SFU operations: upgrades, TURN, capacity. Bounded at the start:
  LiveKit is a single binary with embedded TURN, and single-node serves the
  local stack and first production scale.
- Multi-node LiveKit requires Redis. ADR-0006's trigger table gains that
  row rather than being contradicted: the trigger is measured concurrent
  interview load approaching one node's ceiling, and the Redis it brings is
  LiveKit's coordination store, not an application cache.
- **Exit criteria.** If media operations exceed roughly two incidents a
  quarter or a sustained engineering week per month, we move to LiveKit
  Cloud's EU region, which is protocol-identical: a config change plus the
  gated terms review above, not a rewrite. The port shape (rooms, grants,
  egress) also keeps a Daily migration to adapter size if LiveKit itself is
  the mistake.
- RTC-01 gains ground to stand on; DEC-07 is decided in ADR-0013 on top of
  this topology.

## Alternatives considered

Recorded inline above, each with the reason it lost: vendor-native realtime
(residency, loop control, lock-in), Daily (terms and per-minute cost,
retained as the managed fallback), Twilio (wrong center of gravity).
