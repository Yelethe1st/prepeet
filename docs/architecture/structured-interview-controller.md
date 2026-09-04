# Durable, Plan-Aware, Coverage-Aware Interview Controller

**Status:** Proposed target architecture and migration plan  
**Owner:** Go interview, Python intelligence, web realtime, platform, security, accessibility, and AI quality teams  
**Last updated:** 2026-09-04  
**Related decisions:** [ADR-0007](decisions/0007-durable-execution-with-self-hosted-temporal.md), [ADR-0012](decisions/0012-livekit-carries-live-voice.md), [ADR-0013](decisions/0013-recording-capture-format-alignment-retention.md), [ADR-0016](decisions/0016-reconnect-pause-and-reinvitation-policy.md), [ADR-0019](decisions/0019-model-providers-routing-and-budgets.md), [ADR-0020](decisions/0020-screening-disclosure-access-and-appeal.md)  
**Related target:** [Model-Backed Evaluation, Rubric Composition, and Provider-Neutral Inference](model-backed-evaluation.md)

## Executive summary

Prepeet currently runs a small asynchronous voice loop:

```text
speak opening -> receive one final STT segment -> ask model for text -> speak -> repeat
```

The loop proves the media, provider, transcript, timeline, recovery, sealing,
and evaluation path end to end. It also has useful controls: provider-neutral
interviewer ports, a hard question cap, per-call timeout, scripted fallback,
and durable transcript/turn-boundary events.

It is not yet a durable interview brain. The model holds conversational state
in process memory; Deepgram final segments are treated as completed answers;
plan progression and coverage are prompt suggestions; elapsed interview time
is not enforced in the loop; barge-in is not controlled; and a model returning
`[END]` stops Python without authoritatively completing the session.

The target is a structured interview controller in which:

- the **model is the semantic brain**: it understands answers, decides what is
  worth probing, composes natural questions, recognizes ambiguity, and proposes
  the next interview action;
- **deterministic code is the executive controller**: it owns authoritative
  time, plan state, coverage obligations, turn assembly, action eligibility,
  lifecycle, recovery, completion, provenance, and safety constraints;
- **Go owns product state and authorization**;
- **Python owns provider-neutral semantic proposal generation**, with a
  deterministic reducer/checkpoint representation that can be rebuilt;
- **LiveKit owns realtime media transport**, not interview policy;
- every action is proposed at an accepted event cursor, validated, persisted,
  and only then executed;
- a worker restart reconstructs the interview from a snapshot plus durable
  events and never regenerates an action already accepted or spoken;
- provider failure degrades through approved templates or pauses/ends visibly;
- normal, timed, user-requested, disconnected, and provider-failed endings all
  converge on one idempotent server-owned sealing path.

This document uses **Current** and **Target** explicitly. Target behavior must
not be represented as implemented until its acceptance criteria pass.

## Scope

This proposal covers the live interview from authorized room entry until the
sealed transcript is handed to evaluation:

- session brief and pinned plan consumption;
- candidate and interviewer turn detection;
- speech-to-text and text-to-speech orchestration;
- model context and next-action proposals;
- plan stages, topics, questions, follow-ups, and coverage obligations;
- active-time budgets, closing reserve, and accommodations;
- proposal validation and execution;
- interruption, barge-in, reconnect, agent restart, and event replay;
- completion and final cursor ownership;
- practice, screening, and retake differences;
- provider-neutral routing, fallback, and cost controls;
- security, privacy, prompt injection, and accessibility;
- telemetry, testing, rollout, and rollback.

Evaluation of the completed transcript is specified separately. The live
controller may maintain **exploration signals** to choose the next question,
but it must not display or publish a candidate score during the interview.

## Non-goals

- Giving the model authority to mutate session state directly.
- Asking the model to authorize, seal, bill, reconnect, or publish anything.
- Treating model chat history as the authoritative interview record.
- Letting a model silently skip required disclosures or coverage obligations.
- Evaluating personality, emotion, tone, accent, honesty, culture fit, or
  protected characteristics.
- Praising, criticizing, scoring, or coaching during a screening interview.
- Guaranteeing that every planned topic is fully evidenced; the controller can
  guarantee a fair opportunity and honest coverage record, not a particular
  answer.
- Using a vendor-native realtime API as the sole source of turn state or
  transcript truth.
- Replacing durable Go lifecycle state with an autonomous agent framework.
- Storing private chain-of-thought. Store typed decisions and concise reason
  codes with evidence references.

## Current implementation

### What works today

The current Python `Conversation` composes five ports:

```text
Interviewer
SpeechToText
TextToSpeech
Timeline
RoomClock
```

The LiveKit worker:

1. joins the room and subscribes to audio;
2. derives the session from the room name and candidate from participant
   identity;
3. constructs the platform timeline client;
4. uses Deepgram `nova-3` for candidate STT and Cartesia for interviewer TTS;
5. starts with a scripted interviewer;
6. replaces it with `ModelInterviewer` when model configuration and the brief
   are available;
7. runs the alternating conversation loop.

For every spoken interviewer turn, the agent posts a final transcript segment
and an interviewer turn boundary containing decision latency and model version.
For every final candidate STT segment, it posts the candidate segment and a
candidate turn boundary. Go assigns service-event sequence numbers and stores
the durable timeline.

The model prompt carries persona, role, competencies, duration, plan-stage
names, and behavioral rules. In-memory chat history contains prior questions
and candidate segments. Code limits questions to `max(3, 2 * stage_count)`,
limits a retake to one question, and applies a per-call timeout. A failed call
uses a generic fallback question and records `/fallback` in model provenance;
too many consecutive failures stop question generation.

The browser owns ordinary completion: it records `session.leave`, disconnects,
reads the accepted cursor, calls the completion endpoint, and navigates to the
receipt. Go assembles and seals the transcript, finalizes media, writes the
evaluation input, and advances the lifecycle.

The realtime protocol already supports connection epochs, stale-epoch refusal,
durable sequence numbers, gap acknowledgment, resume, and grace-expiry
finalization.

### Current implementation map

This proposal starts at the following code boundaries. These links describe
the baseline, not a requirement to preserve today's class or package shapes.

| Concern                                                    | Current boundary                                                                                | First target change                                                                           |
| ---------------------------------------------------------- | ----------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| Alternating voice loop                                     | [`conversation.py`](../../services/intelligence/src/prepeet_ai/agent/conversation.py)           | Consume assembled candidate turns and execute only accepted directives                        |
| Model prompt, history, timeout, fallback, and question cap | [`model.py`](../../services/intelligence/src/prepeet_ai/agent/model.py)                         | Replace plain text/`[END]` with versioned structured proposals                                |
| LiveKit job and provider wiring                            | [`worker.py`](../../services/intelligence/src/prepeet_ai/agent/worker.py)                       | Propagate mode and pinned brief; support cancellation, recovery, and directive acknowledgment |
| Timeline client                                            | [`timeline.py`](../../services/intelligence/src/prepeet_ai/agent/timeline.py)                   | Carry turn-assembly, directive, speech, interruption, and reconstruction events               |
| Browser live-session ownership                             | [`LiveScreen.tsx`](../../apps/web/src/features/live/LiveScreen.tsx)                             | Display authoritative controller state and request, rather than own, completion               |
| Browser timeline sequencing                                | [`timeline.ts`](../../apps/web/src/lib/rtc/timeline.ts)                                         | Preserve cursor/epoch behavior while consuming controller phase updates                       |
| Authoritative completion and sealing                       | [`complete.go`](../../services/platform/internal/interview/complete.go)                         | Accept idempotent completion intents from browser, timer, agent, and recovery paths           |
| Provisional reducer/action RPCs                            | [`intelligence.proto`](../../packages/contracts/rpc/prepeet/intelligence/v1/intelligence.proto) | Evolve them additively into the snapshot and action-proposal contracts below                  |

Before implementation, confirm these boundaries against the then-current code
and record any ownership change in the relevant ADR. The architecture depends
on responsibility boundaries and durable facts, not on filenames.

### Current gaps

| Area                  | Current behavior                                                         | Consequence                                                      |
| --------------------- | ------------------------------------------------------------------------ | ---------------------------------------------------------------- |
| Candidate turns       | Every STT final segment is treated as a completed answer                 | Long answers may be split and answered prematurely               |
| Barge-in              | TTS completes before the loop processes another turn                     | Candidate interruption does not stop interviewer speech          |
| Plan                  | Stage names are prompt text                                              | No authoritative stage or topic cursor                           |
| Coverage              | Competencies are prompt text                                             | No verified opportunity or exploration ledger                    |
| Time                  | Minutes are prompt text; question count is the hard limit                | The loop can finish early or overrun active time                 |
| State                 | Model history and counters live in Python memory                         | Worker restart loses cognitive position                          |
| Actions               | Model returns plain text or `[END]`                                      | No typed action, topic, reason, or expected state                |
| Approval              | Python speaks its own output                                             | Go cannot reject stale or policy-invalid questions before speech |
| End                   | `None` returns from Python                                               | Model end does not seal or move browser/session lifecycle        |
| Closing               | Question cap can stop after a candidate answer                           | No guaranteed closing or candidate-question window               |
| Repetition            | Model infers history informally                                          | No deterministic duplicate-question guard                        |
| Mode and tenant       | Worker timeline target defaults to practice; the ingest adapter passes an empty tenant | The agent writes a tenant's screening events under the candidate's own authority, and fixing the mode alone makes them fail |
| Pinning               | Plan is read by digest; role/persona are resolved from current catalogue | Two candidates on one campaign can be briefed differently while their bundles claim identical configuration |
| Transcript correction | Corrections exist in the timeline but not model-state reconciliation     | Later corrections can disagree with what the model acted on      |
| Observability         | Latency/model version are recorded per spoken turn                       | No proposal refusal, plan coverage, or turn-assembly metrics     |

### Two of these are live defects, not target work

Most of the table describes a loop that is simpler than the target. Two rows
describe behaviour that is wrong today, on the screening path, and both should
be fixed before the roadmap rather than inside it.

**Mode and tenant are one change, and the obvious half of it fails.**
`TimelineTarget.mode` defaults to `practice` and the agent never passes it, so
every event the agent writes claims practice. The internal ingest adapter
separately passes an empty tenant. Because the event scope routes practice to
the candidate's own database scope and anything else to the tenant's, the two
defects currently cancel: screening events land through the candidate-owner
policy and are stored with the session's own `tenant_id`, so the data is
correct and the authority used to write it is not. Measured against a real
screening session:

```text
as the agent sends it today (mode=practice, tenant=""):  accepted
with the mode fixed but the tenant still empty:          refused - a tenant is required
with mode and tenant both correct:                       accepted
```

Correcting mode propagation on its own therefore takes a path that works and
breaks it. The tenant must be resolved in the same change. This survived
because every test of the agent ingest path passes `practice` and an empty
tenant, so the tests agree with the code without exercising the case that
matters.

**Brief drift is a fairness problem, not an inconvenience.** The plan reaches
the agent by pinned digest, but persona and role are looked up in the current
catalogue at brief time. Editing a role's competencies while a campaign is open
changes what later candidates are briefed on, while every session's bundle
continues to assert a fixed configuration. On one screening campaign that is
candidates interviewed to different briefs under an identical pinned record,
which is exactly the reproducibility claim the bundle exists to make. Pinning
persona and role the way the plan is already pinned is a correctness fix owed
now.

## Design principles

1. **Proposal is not command.** The model proposes; Go validates and accepts.
2. **Persist before effect.** An action is durably accepted before it is spoken
   or shown.
3. **Events are the record.** Runtime state is a fold over accepted events and
   pinned artifacts, with snapshots only as acceleration.
4. **One cursor, one reality.** A proposal is valid only at the exact accepted
   event cursor and connection epoch it was computed from.
5. **The model is semantically powerful but operationally narrow.** It reasons
   deeply within the rubric and plan but holds no product credentials or tools.
6. **Coverage means opportunity, not performance.** Live coverage state tracks
   what was asked and explored; it is not a hidden score.
7. **Timing is server-owned.** A prompt cannot enforce a deadline.
8. **Turn detection is multimodal and deterministic at the boundary.** VAD,
   STT, explicit controls, and policy decide when an answer closes; the LLM is
   advisory at most.
9. **Recovery replays, never improvises.** Accepted actions survive restarts.
10. **Completion has one authority.** Browser, agent, timer, and grace expiry
    request completion; the server performs the idempotent seal.
11. **Degradation is visible.** Fallback, pause, interruption, and omissions are
    durable facts.
12. **Accessibility changes interaction, not evaluation anchors.** Silence and
    pacing accommodations are controller inputs, never evidence of ability.

## Target topology

```mermaid
flowchart LR
    B[Browser] <-->|WebRTC audio/data| L[LiveKit SFU]
    A[Python media agent] <-->|audio| L
    B -->|sequenced control events| G[Go interview controller]
    A -->|STT/TTS and execution events| G
    G -->|accepted event window + pinned state| R[Deterministic runtime reducer]
    R -->|proposal context| M[Provider-neutral interview model]
    M -->|typed action proposal| G
    G -->|validate + persist accepted action| D[(PostgreSQL)]
    G -->|approved directive| A
    A -->|speak/wait/stop| L
    G -->|seal/finalize| S[(S3 and evaluation input)]
```

**Go owns the deterministic reducer.** This was previously left open between a
Go process, a Python process, and a generated shared state machine, but it is a
starting condition rather than a boundary decision: Phase 2 implements the
fold, the snapshots and the replay, and cannot begin without it.

Go already owns the lifecycle machine, the durable event log, the cursor, the
epoch and the seal, and the fold is over events it holds under its own
row-level security. Running the authoritative fold in Python would put it
across a network boundary from its own data and still require Go to revalidate
everything on return, which means either two implementations of one fold or one
trusted across a boundary this document says must not be trusted. The generated
shared state machine adds a codegen pipeline for a machine with one consumer
that matters.

Python keeps the job that belongs near the model: building the prompt context
as a **projection** of the snapshot Go supplies. That is not the authoritative
fold and is never treated as one. The provisional `ReduceInterviewEvents` RPC
evolves into Go handing Python a verified snapshot rather than Python computing
the state. Either way, a reducer must be reproducible from snapshot plus
events.

### Control-loop sequence

```mermaid
sequenceDiagram
    participant C as Candidate/browser
    participant L as LiveKit
    participant A as Python media agent
    participant G as Go controller
    participant M as Model gateway
    participant DB as Durable store

    C->>L: candidate audio
    L->>A: audio frames
    A->>G: speech/VAD/STT events
    G->>DB: append accepted events
    G->>G: assemble candidate turn
    G->>DB: append candidate.turn.finalized
    G->>M: state + transcript context + allowed actions
    M-->>G: typed proposal at cursor
    G->>G: validate lifecycle, plan, time, coverage, policy
    G->>DB: persist proposal accepted + state transition
    G-->>A: approved directive
    A->>L: synthesize and speak
    A->>G: speech started/completed/interrupted
    G->>DB: append execution events
```

No model output is spoken before the `proposal.accepted` record commits. If the
accepted directive is redelivered, the agent deduplicates by directive ID.

## Ownership and trust boundaries

| Concern                     | Authority                                                         | Model role                                    |
| --------------------------- | ----------------------------------------------------------------- | --------------------------------------------- |
| Session lifecycle           | Go state machine                                                  | None                                          |
| Active-time clock           | Go from accepted lifecycle/media events                           | Receives remaining budget                     |
| Plan definition             | Pinned plan artifact                                              | Interprets content inside allowed stage       |
| Stage progression           | Go controller                                                     | Proposes transition                           |
| Coverage requirements       | Pinned plan/rule pack, enforced by Go                             | Interprets whether a response merits a probe  |
| Candidate-turn finalization | Go controller over media/STT signals                              | May propose `wait`/clarify only within policy |
| Question wording            | Model or approved deterministic template                          | Primary author                                |
| Follow-up selection         | Model within limits                                               | Primary reasoner                              |
| Required disclosure         | Go obligation ledger                                              | May supply only approved content reference    |
| Barge-in policy             | Pinned persona/accommodation policy, enforced by controller/agent | None                                          |
| Provider/model route        | Pinned model policy plus deployment route                         | Uses selected gateway                         |
| Transcript truth            | Accepted timeline and correction history                          | Reads bounded view                            |
| Completion                  | Go idempotent command                                             | May propose wrap/end                          |
| Evaluation/band             | Post-seal evaluation system                                       | No live score                                 |
| Screening decision          | Authorized human                                                  | None                                          |

## Runtime state model

### Authoritative state

The controller persists enough state to resume without model memory:

```json
{
  "schema_version": "interview-runtime-2",
  "session_id": "...",
  "bundle_digest": "sha256:...",
  "connection_epoch": 2,
  "folded_cursor": 143,
  "phase": "listening",
  "active_time_ms": 812000,
  "remaining_time_ms": 688000,
  "stage": {
    "id": "competency_questions",
    "index": 1,
    "entered_at_cursor": 72,
    "questions_asked": 3,
    "follow_ups_used": 2
  },
  "current_topic": "systems-design",
  "current_question_id": "q-...",
  "turn": {
    "candidate_turn_id": "ct-...",
    "status": "assembling",
    "segment_ids": ["seg-..."],
    "started_at_ms": 801000,
    "last_speech_at_ms": 811200
  },
  "coverage": {},
  "obligations": {},
  "accepted_action_id": null,
  "provider_failures": {},
  "interruption_count": 1,
  "completion_reason": null
}
```

This document is illustrative. The canonical schema belongs in contracts and
must define closed enums, bounds, and upgrade behavior.

### Runtime phases

```mermaid
stateDiagram-v2
    [*] --> awaiting_opening
    awaiting_opening --> deciding
    deciding --> speaking: action accepted
    speaking --> listening: speech completed
    speaking --> listening: candidate barges in
    listening --> assembling: candidate speech starts
    assembling --> assembling: final segment appended
    assembling --> deciding: candidate turn finalized
    deciding --> transitioning: topic transition accepted
    transitioning --> speaking
    deciding --> wrapping: time/plan/model requests close
    wrapping --> speaking
    speaking --> ending: closing completed
    ending --> completed: server seals
    listening --> paused: connection lost
    assembling --> paused: connection lost
    speaking --> paused: connection lost
    paused --> deciding: recovered with pending action
    paused --> completed: grace expires and server seals
```

Invalid phase/action pairs are rejected. Examples: `ask_question` while TTS is
already speaking, `follow_up` before any candidate answer, `transition_topic`
to an unknown topic, or `end_session` while an undelivered mandatory disclosure
remains.

### Event sourcing and snapshots

Runtime state is a deterministic fold over:

- the pinned bundle and runtime-policy versions;
- accepted durable control events;
- accepted action proposals and execution outcomes;
- server timing facts;
- explicit completion commands.

Snapshots are written periodically and at important boundaries: candidate turn
finalized, action accepted, stage transition, connection loss, and wrap-up.
Each snapshot records the last folded cursor and state digest. Recovery loads
the latest valid snapshot and replays later events. A digest mismatch discards
the snapshot and rebuilds from the durable event stream.

Never store only model chat messages as a snapshot. Model context is a
projection of authoritative state and transcript events.

## Plan artifact v2

The current plan contains only a shape and stage-name list. The controller needs
an executable, validated plan that still leaves semantic question wording to
the model.

```json
{
  "schema_version": "2.0",
  "shape": "shape_technical",
  "time": {
    "target_minutes": 25,
    "close_reserve_seconds": 120,
    "max_overrun_seconds": 300
  },
  "stages": [
    {
      "id": "intro",
      "kind": "opening",
      "min_questions": 1,
      "max_questions": 1,
      "required_obligations": ["recording-reminder"]
    },
    {
      "id": "competency_questions",
      "kind": "coverage",
      "min_questions": 2,
      "max_questions": 6,
      "max_follow_ups_per_topic": 2,
      "coverage_items": [
        {
          "id": "systems-design",
          "required": true,
          "priority": 1,
          "minimum_opportunities": 1,
          "exploration_dimensions": [
            "specific_example",
            "candidate_action",
            "reasoning",
            "tradeoffs",
            "outcome"
          ]
        }
      ]
    },
    {
      "id": "candidate_questions",
      "kind": "candidate_questions",
      "min_questions": 0,
      "max_questions": 1
    },
    {
      "id": "close",
      "kind": "closing",
      "min_questions": 0,
      "max_questions": 0,
      "required_obligations": ["completion-explanation"]
    }
  ],
  "fallback_templates": {
    "opening": "Welcome. Tell me about a relevant piece of work you led.",
    "specific_example": "Could you ground that in one specific example?",
    "candidate_action": "What part did you personally own?",
    "reasoning": "What led you to that decision?",
    "tradeoffs": "What alternatives did you consider and what did you trade off?",
    "outcome": "What happened as a result?",
    "close": "Thank you. Your interview is complete."
  }
}
```

### Plan validation

Publication rejects a plan when:

- stage IDs or kinds are unknown or duplicated;
- minimum questions exceed maximum questions;
- required obligations do not exist in the pinned rule pack;
- coverage items do not exist in the pinned role/rubric;
- required coverage cannot fit the time and question budgets;
- no reachable closing stage exists;
- fallback templates are absent for a required model-backed action;
- follow-up limits are negative or unbounded;
- mode-incompatible content appears;
- the timing policy conflicts with the session duration or accommodation rules;
- a plan requires an inference forbidden by responsible-hiring policy.

The feasibility validator should conservatively estimate speaking, answer,
decision, and transition time. It should block obviously impossible plans and
warn on tight ones; it cannot guarantee how long a person will answer.

## Coverage model

### Coverage is not evaluation

The live controller needs to know where to go next without secretly scoring the
candidate. It therefore tracks interview **opportunity and exploration**, not a
band or hiring signal.

For each coverage item:

```text
not_started
opportunity_offered
response_received
follow_up_recommended
explored
closed_by_time
declined
unavailable_due_to_interruption
```

Exploration dimensions describe whether the conversation contains material
addressing a question type, for example a specific example, action, reasoning,
trade-off, or outcome. They do not assert that the material is good.

### Model and code roles in coverage

The model may propose:

```json
{
  "coverage_updates": [
    {
      "coverage_item_id": "systems-design",
      "dimension": "tradeoffs",
      "status": "addressed",
      "evidence_segment_ids": ["seg-44"]
    },
    {
      "coverage_item_id": "systems-design",
      "dimension": "outcome",
      "status": "unclear",
      "evidence_segment_ids": ["seg-45"]
    }
  ]
}
```

Deterministic code validates identifiers, candidate-segment provenance, and
allowed transitions. The model decides semantic relevance. If a coverage
proposal is invalid, the controller rejects that update without pretending the
dimension was explored.

Required opportunity is deterministic: the controller knows whether it
approved and executed a question for a required item. Semantic exploration is
model-proposed and auditable. Final evaluation independently reads the sealed
transcript; live coverage never becomes evaluation evidence automatically.

### Coverage provenance carries its own framing

Keeping coverage out of the evaluation pipeline is necessary and not
sufficient. Phase 8 surfaces coverage and interruption provenance to authorized
reviewers, and "explored three of five topics" will be read as a statement
about the candidate unless the surface says otherwise.

Evaluation already solved this problem and the controller should reuse the
answer rather than reinvent it. There, a competency the conversation never
reached is `NOT_DISCUSSED` and the review screen renders it as the plan's gap,
not the candidate's, in those words. Coverage provenance carries the same
obligation: every unreached or partially explored item is displayed with the
process reason that produced it — time expired, interruption, the candidate
declined, the plan did not schedule it — and never as a bare fraction. An item
closed by time says so. An opportunity lost to a dropped connection says so,
and reads as something that happened to the interview rather than something the
candidate did.

The wording is a product surface obligation, not a note for implementers.
A reviewer who has to infer that a coverage number is about the interview has
been handed a score by accident.

### Fair opportunity scheduler

The deterministic scheduler constrains next actions:

- every required item should receive its minimum opportunity before optional
  deep dives, subject to time and candidate choice;
- follow-ups cannot exceed the per-topic or stage limit;
- the same coverage item cannot monopolize the interview because the model
  finds it interesting;
- time reserve for closing is protected;
- declined questions are not repeatedly asked unless policy permits one
  respectful alternative;
- interruption or provider failure records why an item was not reached;
- optional items are ordered by pinned priority, not by a model's impression of
  the candidate;
- screening plans must give candidates under the same campaign materially
  equivalent opportunity, with adaptive probes bounded by the same rules.

## Candidate-turn assembly

### Why STT final is not a candidate turn

An STT provider finalizes chunks for transcription reasons. A person may pause
mid-answer, restart a sentence, think silently, or speak for several chunks.
Equating provider finalization with conversational completion causes premature
interruption and too many model calls.

### Input signals

The turn assembler consumes:

- LiveKit participant speech/VAD start and stop;
- final STT segments and optional partial text;
- push-to-talk press/release when enabled;
- an explicit accessible "Done answering" action when offered;
- microphone mute/device-loss state;
- interviewer TTS state;
- configured silence tolerance;
- persona pacing policy;
- accommodation pacing policy;
- maximum answer and maximum silence policies;
- connection loss and recovery events.

### Finalization rules

Recommended priority:

1. Push-to-talk release after speech finalizes: close the candidate turn.
2. Explicit "Done answering": close after pending final STT arrives or a short
   bounded flush timeout.
3. Connection loss: suspend, do not close as a normal answer.
4. Device loss/mute: prompt or pause according to policy; do not infer answer
   completion immediately.
5. VAD silence longer than the effective silence tolerance, with no pending
   audio/STT: close.
6. Maximum answer duration: offer a respectful transition, record truncation
   only if policy must close the turn.

The effective silence tolerance is computed deterministically from the plan,
persona, interaction mode, and accommodation. It is not inferred from accent,
speech rate, hesitation, or any candidate trait.

### Candidate turn record

```json
{
  "candidate_turn_id": "ct-...",
  "question_id": "q-...",
  "segment_ids": ["seg-41", "seg-42"],
  "text": "assembled from accepted final segments",
  "start_ms": 410000,
  "end_ms": 468000,
  "completion_kind": "silence_timeout",
  "input_quality": {
    "missing_audio": false,
    "low_confidence_segments": ["seg-42"],
    "sequence_gaps": []
  },
  "acted_on_digest": "sha256:..."
}
```

The model receives the assembled text and stable segment IDs. Evaluation keeps
the original segments and exact offsets; assembly must not manufacture text.

### Transcript corrections

A correction never erases what the interviewer acted on. Record:

- original recognized segment;
- correction and its actor/source;
- whether the candidate turn had already been finalized;
- `acted_on_digest` used for the accepted next action;
- corrected transcript digest used at final evaluation.

If correction arrives before proposal acceptance, invalidate the proposal
cursor and regenerate. If it arrives after the next question was spoken, keep
the question and provenance; optionally allow a clarifying action, but do not
rewrite history as though the interviewer had heard the corrected text.

## Barge-in and interviewer speech

### Target behavior

The TTS adapter must support cancellation or chunk stop. When candidate speech
crosses the validated barge-in threshold:

1. record `candidate.speech_started`;
2. stop or duck TTS according to pinned policy;
3. record exactly how much interviewer content was rendered;
4. finalize the interviewer transcript to words actually played, not the full
   planned text;
5. move to listening/assembling;
6. ensure reconstructed model context distinguishes planned from heard text.

False VAD triggers must not constantly cut off the interviewer. Use a bounded
speech-duration/energy threshold and provider-independent policy. Push-to-talk
press can be an explicit high-confidence barge-in signal.

### Persona and accommodation

Personas may vary deliberate pause, warmth, and whether they permit gentle
interruption, but cannot override accessibility or fairness policy. An
accommodation may:

- extend silence tolerance;
- disable automatic interruption;
- enable push-to-talk or explicit-done mode;
- slow TTS;
- add caption time;
- increase total active duration where approved.

These changes are recorded as interaction policy. They do not change rubric
anchors or appear as negative evidence.

## Structured action proposal

The existing provisional RPC already defines `ProposeNextAction` and a closed
`ActionKind`. The target should evolve that seam rather than introduce a second
unrelated command vocabulary.

### Proposed actions

```text
ask_question
follow_up
clarify_transcript
transition_topic
give_obliged_disclosure
invite_candidate_question
wait
acknowledge_recovery
wrap_up
end_session
```

`wait` is bounded and is not a way for the model to extend the interview.
`clarify_transcript` asks about content the system may have misheard; it does
not accuse the candidate of inconsistency. Unknown actions are refused.

### Proposal shape

```json
{
  "schema_version": "action-proposal-2",
  "proposal_id": "prop-...",
  "session_id": "...",
  "connection_epoch": 2,
  "based_on_cursor": 143,
  "expected_state_digest": "sha256:...",
  "action": "follow_up",
  "stage_id": "competency_questions",
  "topic_id": "systems-design",
  "coverage_targets": [
    {
      "item_id": "systems-design",
      "dimensions": ["tradeoffs", "outcome"]
    }
  ],
  "content": "What alternatives did you consider, and what happened after you made that choice?",
  "reason_code": "EXPLORE_TRADEOFF_AND_OUTCOME",
  "source_segment_ids": ["seg-44", "seg-45"],
  "coverage_updates": [],
  "requested_wait_ms": 0,
  "model_provenance": {
    "route": "interviewer-primary",
    "model": "...",
    "model_revision": "...",
    "prompt_version": "interviewer-2",
    "policy_version": "model-policy-2"
  }
}
```

The model returns concise typed rationale through reason codes and grounded
references. Do not request or persist private chain-of-thought.

### Proposal context

The model receives:

- immutable session/purpose identity and pinned artifact digests;
- current runtime phase, stage, topic, cursor, and remaining active time;
- allowed next action kinds computed by Go;
- unmet required coverage and remaining follow-up/question budgets;
- outstanding obligations;
- the approved questions and assembled candidate turns required for context;
- validated live coverage signals;
- interruption and transcript-quality warnings relevant to the next action;
- mode-specific behavioral rules;
- fallback and closing requirements.

The controller should prefer a compact structured summary plus relevant recent
turns over resending an unbounded transcript. The summary itself must be a
versioned projection rebuildable from authoritative state. When full history is
within the model context limit, include it for fidelity.

## Proposal validation

Go validates every proposal before acceptance.

### Integrity

- session, bundle, epoch, cursor, and expected state digest match;
- proposal ID is valid and not conflicting with a prior proposal;
- schema, prompt, model policy, route, and calculation versions are allowed;
- source segments belong to this session, candidate, tenant, and purpose;
- content and arrays stay within size/count bounds.

### Lifecycle and concurrency

- action is allowed in the current runtime phase;
- there is no uncompleted accepted directive unless the new action explicitly
  supersedes it under policy;
- proposal was computed after the latest accepted candidate turn/correction;
- a stale agent or epoch cannot speak into a resumed session;
- `end_session` is idempotent and refused after seal only as already complete.

### Plan and coverage

- stage and topic exist in the pinned plan;
- stage transition follows an allowed edge;
- question and follow-up budgets remain;
- coverage targets belong to the topic and are permitted now;
- required prior obligations and stage minima are met;
- optional probing does not displace an unmet higher-priority required
  opportunity when time is constrained;
- wrap/end respects close reserve, hard deadline, and permitted early-end
  reasons.

### Content and safety

- question is non-empty for actions requiring speech;
- model output contains at most the permitted number of questions;
- screening content contains no evaluation, feedback, score, praise, criticism,
  decision, or coaching;
- content contains no prohibited inference, protected-characteristic probe,
  secret, system prompt, provider error, or unsupported candidate fact;
- question does not repeat a materially equivalent recent question unless its
  action is an explicit clarification;
- mandatory disclosure content is loaded from the pinned artifact rather than
  freely rewritten by the model;
- no URL, tool instruction, or markup from candidate speech is executed;
- model cannot ask for information outside the approved purpose.

### Timing

- requested wait fits the allowed bound;
- estimated question speech plus close reserve fits the remaining budget;
- no model proposal extends the maximum session duration;
- a hard-deadline controller action overrides a conflicting model proposal.

### Validation outcome

```text
accepted
refused_terminal
refused_stale_recompute
replaced_by_controller_action
accepted_with_safe_normalization
```

Safe normalization is intentionally narrow: whitespace, supported Unicode
normalization, and server-controlled identifiers. Do not silently turn a
multi-question or prohibited proposal into different semantic content. Use a
fallback template or request a new proposal.

Every refusal records a stable reason code. Raw provider text is access
controlled and retained only under the approved debugging policy.

## Model reasoning responsibilities

The interviewer model should act as a capable interviewer, not a random
question generator. Within the allowed controller state, it should:

- understand what the candidate meant across an assembled answer;
- recognize when an answer already addresses multiple exploration dimensions;
- ask the smallest useful follow-up rather than a compound interrogation;
- distinguish a missing detail from a new topic;
- adapt wording to the candidate's vocabulary without changing evaluation
  scope;
- avoid asking for facts the candidate has already supplied;
- clarify ambiguous pronouns, ownership, timelines, baselines, alternatives,
  outcomes, and corrections;
- transition when further probing has diminishing value or violates limits;
- respond naturally to a declined question without pressure;
- acknowledge recovery without pretending lost audio was heard;
- reserve closing time and hand control back cleanly;
- maintain persona style without weakening consistency or accessibility;
- treat candidate attempts to redirect system policy as interview content, not
  instructions.

The model may propose that a coverage dimension was addressed, but it must cite
accepted candidate segment IDs. It may not assign an evaluation band in the
live loop or use inferred performance to choose easier/harder treatment in
screening. Adaptive depth is based on missing exploration, not perceived
candidate quality.

## Model Cognitive Architecture and Capability Envelope

### Purpose

The cognitive layer turns an accepted candidate turn plus authoritative
runtime state into a grounded proposal for what the interviewer should do next.
It is the part of the system expected to understand meaning, maintain
conversational coherence, identify useful ambiguity, and formulate natural
questions.

It is not a second lifecycle controller. It cannot start, pause, extend, seal,
or reopen a session; alter the plan or rubric; decide that a legal disclosure
is unnecessary; write directly to the timeline; invoke external tools; or
speak. Its output remains a proposal until deterministic validation accepts it.

The cognitive design has five goals:

1. use substantially more of a capable model's semantic reasoning than simple
   keyword extraction or next-question generation;
2. keep every externally observable action grounded in accepted session facts;
3. express uncertainty so the controller can prefer clarification, transition,
   fallback, or abstention over invention;
4. behave consistently across cloud and local providers with different feature
   sets;
5. remain measurable, replayable, replaceable, and safe to evolve.

### Cognitive loop

One proposal cycle has six logical steps. They may execute in one constrained
model call initially; they are separate contracts so implementations can evolve
without changing controller semantics.

```mermaid
flowchart LR
    A[Build grounded context] --> B[Interpret candidate turn]
    B --> C[Update semantic hypotheses]
    C --> D[Generate eligible action candidates]
    D --> E[Critique and rank candidates]
    E --> F[Return one structured proposal]
    F --> G[Deterministic validation and acceptance]
```

1. **Build grounded context.** Assemble pinned policy, plan, allowed actions,
   accepted questions, actually-heard speech, finalized candidate turns,
   semantic memory, remaining time, and controller limits.
2. **Interpret.** Identify what the candidate is doing and what their answer
   asserts without yet deciding a score.
3. **Update hypotheses.** Propose grounded semantic observations and coverage
   updates, each linked to accepted source spans.
4. **Generate candidates.** Consider eligible clarification, follow-up,
   transition, informational response, wrap, and end actions.
5. **Critique and rank.** Compare candidates for expected information value,
   relevance, repetition, burden, fairness, safety, and time cost.
6. **Propose.** Return exactly one action plus bounded decision metadata. Go
   independently validates action and suggested state changes.

The system persists the structured outcome and compact reason codes, not hidden
chain-of-thought. A provider may reason internally, but must not return or store
private reasoning traces as a product requirement.

### Semantic answer interpretation

The model should produce a bounded `AnswerInterpretation`, either within the
action response or through an equivalent versioned internal contract:

```json
{
  "schema_version": "answer-interpretation-1",
  "candidate_turn_id": "ct-44",
  "intent": "answer",
  "language": {
    "primary": "en",
    "code_switched": false,
    "supported": true
  },
  "observations": [
    {
      "kind": "candidate_action",
      "normalized_text": "introduced request coalescing",
      "source_segment_ids": ["seg-44", "seg-45"],
      "support": "explicit"
    },
    {
      "kind": "reported_outcome",
      "normalized_text": "reduced duplicate downstream requests",
      "source_segment_ids": ["seg-45"],
      "support": "explicit"
    }
  ],
  "ambiguities": [
    {
      "kind": "missing_measurement",
      "source_segment_ids": ["seg-45"]
    }
  ],
  "relations": [
    {
      "kind": "action_caused_reported_outcome",
      "observation_ids": ["obs-1", "obs-2"],
      "support": "candidate_attributed"
    }
  ],
  "coverage_hypotheses": [
    {
      "coverage_item_id": "systems-design.tradeoffs",
      "status": "partially_explored",
      "source_segment_ids": ["seg-44", "seg-45"]
    }
  ],
  "uncertainty": {
    "level": "material_ambiguity",
    "reason_codes": ["OUTCOME_NOT_QUANTIFIED"]
  }
}
```

Permitted intent values include:

```text
answer
partial_answer
clarification_request
repeat_request
informational_question
decline
continue_thinking
correction
stop_request
off_topic
unsafe_or_emergency_content
unresolved
```

Intent is advisory except where deterministic inputs already establish the
fact. For example, an explicit browser End action is authoritative; a model
classification of `stop_request` causes the controller to confirm or apply the
pinned immediate-stop policy, never to ignore it.

Observations may capture only interview-relevant semantic structure such as:

- the candidate's stated role, ownership, action, decision, constraint, and
  collaboration;
- sequence, duration, baseline, comparison, and reported outcome;
- alternatives considered, trade-offs, failure, learning, and correction;
- an example or explanation explicitly attributed to another person;
- uncertainty, qualification, or refusal stated by the candidate;
- what remains ambiguous enough to justify one follow-up.

Observations must not include inferred personality, emotion, honesty, health,
accent, socioeconomic background, protected characteristics, or facts imported
from another candidate or external profile. `normalized_text` is a concise
paraphrase, not a stronger claim than the source.

### Grounding and contradiction rules

Every semantic observation, coverage hypothesis, contradiction, and reason to
follow up must cite accepted candidate segment IDs. The validator checks that
the cited content belongs to the assembled candidate turn or permitted prior
context. Citation proves provenance, not truth: the system records that the
candidate reported something; it does not independently certify the event.

Use the following support vocabulary:

```text
explicit              directly stated by the candidate
candidate_attributed  causal or ownership link asserted by the candidate
reasonable_paraphrase meaning preserved without adding a material fact
unclear               plausible interpretation but insufficiently supported
conflicted             inconsistent accepted statements not yet resolved
unsupported            must be rejected and never enter accepted memory
```

Contradictions are handled as unresolved context, not deception. The model may
ask a neutral clarification if it is relevant and proportionate. It must not
label the candidate dishonest or use a contradiction as a live evaluation.
Self-corrections supersede earlier semantic memory for future conversation but
do not erase the original transcript or what earlier actions relied on.

### Semantic interview memory

Provider chat history is not memory. Reconstructable semantic memory is a
versioned projection over accepted interview facts:

```json
{
  "schema_version": "semantic-memory-1",
  "through_cursor": 91,
  "derivation": {
    "model_route": "route-a",
    "model_revision": "pinned-revision",
    "prompt_version": "interview-cognition-3"
  },
  "asked_questions": [],
  "accepted_observations": [],
  "unresolved_ambiguities": [],
  "candidate_preferences": {
    "requested_rephrase": false,
    "declined_topics": []
  },
  "conversation_threads": [],
  "superseded_observations": []
}
```

Memory has three layers:

1. **Authoritative record:** transcript segments, candidate turns, accepted
   directives, execution facts, plan, and controller state.
2. **Accepted semantic projection:** grounded observations and unresolved
   ambiguities accepted by deterministic policy.
3. **Ephemeral model context:** provider-specific tokens, caches, embeddings,
   or conversation IDs that may be discarded without losing product state.

The semantic projection is advisory and versioned. If its digest fails, its
source citations disappear, or a new interpreter version invalidates it, the
system rebuilds it from the authoritative record. Evaluation never treats the
projection itself as evidence; evaluation returns to accepted transcript
content.

For a long interview, construct context in this order:

1. non-negotiable policy and output schema;
2. controller state, allowed actions, hard limits, and remaining time;
3. current plan stage, topic, coverage obligations, and knowledge permissions;
4. current candidate turn and actually-heard interviewer question;
5. relevant accepted semantic observations with source IDs;
6. a small verbatim window of recent accepted turns;
7. only then, older relevant excerpts retrieved from the same session.

Allocate explicit token budgets to each section. Policy, schema, current turn,
and controller constraints cannot be truncated. If safe context cannot fit,
use a larger approved context route, reduce optional history, apply a grounded
summary, or fall back; never silently omit the rules that constrain the model.

Summaries must cite the turns they summarize, carry an interpreter version and
digest, distinguish candidate statements from system facts, and be tested for
claim strengthening and omission. Prompt caches are performance optimizations;
their keys include every pinned artifact and version capable of changing
behavior.

### Candidate generation and decision utility

The model should consider more than the first plausible follow-up. Within the
controller's eligible actions, it may generate a small internal candidate set
and select one. A recommended candidate representation is:

```json
{
  "action": "follow_up",
  "topic_id": "architecture_tradeoffs",
  "question_intent": "clarify_measured_outcome",
  "expected_information": ["baseline", "measurement", "result"],
  "source_segment_ids": ["seg-45"],
  "estimated_answer_seconds": 45,
  "decision_factors": [
    "REQUIRED_COVERAGE_GAP",
    "DIRECTLY_GROUNDED",
    "FITS_TIME_BUDGET"
  ]
}
```

Ranking should account for:

- required-opportunity priority supplied by the controller;
- expected reduction of a specific, grounded ambiguity;
- information not already supplied by the candidate;
- conversational coherence with the last answer;
- whether one concise question can obtain the information;
- remaining question, follow-up, stage, and time budgets;
- risk of repetition, leading language, compound wording, or hidden coaching;
- candidate burden, recent declines, and accommodation policy;
- fairness and equivalence constraints for screening;
- whether a deterministic transition, disclosure, or close has priority.

Model-produced utilities are not trusted numerical facts. The initial contract
should return ordered reason codes, source citations, and expected information,
not a pseudo-precise score. The controller recomputes hard eligibility and may
replace the proposal with a deterministic action.

For latency-sensitive production, prefer one constrained model call that
performs internal generation and review. A separate critic call is optional and
appropriate only when its measured quality gain exceeds its latency, cost, and
new failure modes. Never expose several speculative questions to the candidate.

### Preflight self-review

Before emitting a proposal, the cognitive layer checks:

- Is the question answerable and relevant to the active plan topic?
- Is it grounded in what was actually heard?
- Has the candidate already answered it?
- Does it contain exactly one principal question?
- Does it accidentally disclose an assessment or preferred answer?
- Does it presume employment history, identity, or facts not in evidence?
- Is it neutral when clarifying a conflict or decline?
- Can a concise answer fit the remaining time?
- Is the language supported and accessible under the active policy?
- Would transition, wrap, or abstention be safer and more useful?

This self-review improves proposal quality but does not replace deterministic
validation. A model cannot certify its own compliance.

### Uncertainty, confidence, and abstention

The live controller needs confidence about semantic interpretation and action
suitability, not a prediction of candidate performance. A raw model statement
such as `confidence: 0.93` is not calibrated evidence.

Use categorical uncertainty first:

```text
well_supported
partially_supported
material_ambiguity
conflicted
unassessable
```

It is derived from observable conditions:

- completeness and locality of source citations;
- explicit versus inferred support;
- unresolved transcript confidence or correction state;
- agreement between relevant accepted statements;
- language/provider support;
- schema validity and policy checks;
- repeated interpretation stability in offline evaluation;
- optional cross-model disagreement when a governed verifier is used.

Controller behavior should be monotonic with uncertainty:

| Uncertainty | Permitted behavior |
|---|---|
| `well_supported` | Follow up, update exploration state, or transition within plan |
| `partially_supported` | Ask a bounded clarification or preserve the item as partial |
| `material_ambiguity` | Clarify once if valuable; otherwise leave unresolved and move on |
| `conflicted` | Ask neutrally when relevant; never accuse or silently choose one version |
| `unassessable` | Abstain from semantic update and use controller fallback/transition |

Numerical confidence may be introduced only after calibration on representative
labeled conversations. Measure selective accuracy/coverage, calibration error,
and failure rates by language, role family, answer type, route, and model
revision. Provider log probabilities are optional inputs, never a portable
confidence contract. Thresholds are pinned policy and cannot be chosen by the
model at runtime.

Abstention is a successful behavior. It records a reason such as
`INSUFFICIENT_GROUNDING`, `TRANSCRIPT_UNCERTAIN`, `LANGUAGE_UNSUPPORTED`,
`CONFLICT_UNRESOLVED`, or `CONTEXT_INCOMPLETE`; it does not invent an answer or
penalize the candidate.

### Candidate intent and discourse handling

The model may recognize discourse acts so the interaction is natural, while
the controller keeps authority:

- **Clarification request:** rephrase the accepted question without expanding
  its assessment scope or giving away a preferred answer.
- **Repeat request:** replay or restate under the heard-content and repetition
  policy.
- **Continue-thinking request:** extend silence only within the effective
  accommodation and maximum-answer rules.
- **Informational question:** answer only from the approved knowledge pack or
  defer to the candidate-question stage.
- **Decline:** acknowledge neutrally, record the opportunity and decline, offer
  an allowed alternative if one exists, then move on.
- **Correction:** supersede semantic memory while preserving transcript and
  action history.
- **Off-topic response:** redirect once without shaming; repeated redirection
  follows the plan's move-on rule.
- **Stop request:** deterministic immediate-stop policy overrides interview
  optimization.
- **Unsafe or emergency content:** select only an approved escalation action;
  do not improvise clinical, legal, or emergency advice.

The model must distinguish “I need a moment” from answer completion, “I don't
know” from silence, and a candidate's question from an attempt to rewrite
system policy. When unclear, abstain or clarify instead of silently classifying
the event in the most convenient way.

### Grounded interview knowledge pack

The live interviewer should not browse the web or perform open-ended retrieval.
It may answer legitimate candidate questions from a pinned, tenant-approved
knowledge pack included in the brief bundle:

```json
{
  "schema_version": "interview-knowledge-1",
  "digest": "sha256:...",
  "entries": [
    {
      "id": "process.next_steps",
      "topics": ["process"],
      "content": "Approved factual answer",
      "valid_from": "2026-09-01T00:00:00Z",
      "valid_until": null,
      "allowed_modes": ["practice", "screening"]
    }
  ],
  "defer_template_id": "knowledge-not-available"
}
```

Suitable entries include interview mechanics, time expectations,
accommodations, privacy contacts, candidate support, role facts already
approved for disclosure, and next-step wording. Compensation, legal promises,
employee-specific facts, live application status, or unapproved company claims
are excluded unless explicitly governed.

The proposal cites the knowledge entry ID. Go verifies the digest, validity,
mode, and topic before accepting the answer. When no entry applies, the model
uses the pinned defer template. This enables useful model-mediated answers
without turning the interview into an autonomous retrieval agent.

### Adaptation boundaries

Allowed adaptation includes:

- match vocabulary and sentence complexity while preserving question intent;
- rephrase after an explicit request;
- choose among plan-permitted examples or probes based on missing information;
- change pacing and pause behavior under accessibility policy;
- follow the candidate's relevant narrative order;
- use supported language variants that have passed equivalence testing;
- in explicitly disclosed practice mode, provide behavior allowed by the
  separate coaching policy.

Prohibited adaptation includes:

- making questions easier or harder based on perceived candidate performance
  in screening;
- inferring ability from accent, speed, fluency, emotion, sentiment, facial
  expression, or voice characteristics;
- changing required opportunity, rubric anchors, or time entitlement;
- becoming more adversarial because an answer conflicts or seems implausible;
- using protected or proxy characteristics to select questions;
- covert persuasion, diagnosis, personality assessment, or deception;
- drawing on another candidate's transcript or outcome.

### Capability profile

Provider neutrality requires capability negotiation because a cloud reasoning
model, a fast hosted model, and a local Hugging Face model do not expose the
same controls. Each deployed route publishes a pinned `ModelCapabilityProfile`:

```json
{
  "profile_version": "model-capabilities-1",
  "route_id": "local-vllm-a",
  "model_revision": "sha256:...",
  "features": {
    "schema_constrained_output": "native",
    "cancellation": "supported",
    "streaming_text": "supported",
    "prompt_caching": "unsupported",
    "tool_calling": "disabled",
    "provider_history": "unused",
    "log_probabilities": "unsupported",
    "deterministic_seed": "best_effort",
    "audio_native": "unsupported"
  },
  "limits": {
    "context_tokens": 32768,
    "max_output_tokens": 900,
    "requests_per_conversation": 1,
    "deadline_ms": 1400
  },
  "languages": ["en"],
  "processing_regions": ["local"],
  "approved_modes": ["practice"]
}
```

The gateway matches the stage requirements to this profile before dispatch.
It must fail closed with a normalized capability error if a required feature or
approved mode is missing. Configuration must not claim support merely because
an OpenAI-compatible endpoint accepts a similarly named field.

### Feature-use policy

| Model/provider feature | Interview use |
|---|---|
| Schema-constrained output | Preferred; otherwise strict parse and validation with no permissive guessing |
| Streaming text | May reduce internal latency, but partial text is never spoken before directive acceptance |
| Cancellation | Required for routes where stale state, deadline, stop, or barge-in must terminate work promptly |
| Prompt caching | Optional optimization keyed by all behavioral pins; never product state |
| Extended reasoning | Bounded by latency/cost policy; no private reasoning trace persisted |
| Tool/function calling | Disabled in the live cognitive stage except future explicitly allow-listed deterministic functions |
| Embeddings/retrieval | Limited to same-session grounded memory or a pinned knowledge artifact; no open corpus |
| Provider conversation/history | Cache only; reconstructable context remains authoritative |
| Log probabilities | Optional calibration telemetry; not cross-provider confidence |
| Deterministic seed | Testing aid only; never a reproducibility guarantee |
| Native audio understanding | Advisory interaction signal until separately validated; never hiring evidence from voice traits |
| Multiple candidate generation | Internal bounded deliberation; only one proposal crosses the controller boundary |
| Vision/video understanding | Out of scope unless separately consented, justified, governed, and fairness-tested |

### Route qualification and graceful degradation

A model route is approved per cognitive capability, language, role family,
mode, region, and latency class—not merely because it can emit valid JSON.

Qualification must demonstrate:

- grounded observation extraction and citation fidelity;
- intent recognition and appropriate abstention;
- useful, concise, non-repetitive follow-ups;
- safe handling of corrections, declines, ambiguity, and candidate questions;
- stable structured output at the configured sampling settings;
- compliance with screening neutrality and adaptation boundaries;
- supported-language behavior and cross-provider equivalence;
- latency, cancellation, load, cost, and context-limit behavior;
- resistance to direct, indirect, multilingual, and encoded prompt injection;
- no regression when the provider silently aliases or revises a model.

Degradation is feature-specific:

- no native schema enforcement -> use strict schema parsing if that route has
  passed invalid-output thresholds;
- context too small -> use the approved grounded-memory projection or a
  qualified larger-context route;
- no prompt cache -> accept higher cost only within budget;
- no log probabilities -> retain categorical uncertainty;
- no multilingual qualification -> pause, switch to an approved route, or
  follow unsupported-language policy;
- no reliable cancellation -> do not use the route where stale responses can
  exceed the controller's safety window;
- local capacity unavailable -> bounded approved fallback, never indefinite
  cold-start waiting.

No degradation may weaken disclosure, lifecycle, privacy, fairness, or
completion guarantees.

### Cognitive topology

The default production topology should remain one primary cognitive call plus
deterministic validation. Additional model calls are allowed only for a
measured purpose:

| Topology | Appropriate use | Constraint |
|---|---|---|
| Single structured proposer | Default live path | Must perform bounded internal preflight review |
| Proposer plus deterministic semantic checks | Repetition, schema, citations, timing, prohibited content | Preferred wherever rules can be coded reliably |
| Proposer plus small verifier model | High-risk semantic policy check with demonstrated incremental value | Total latency and correlated failures must be measured |
| Parallel provider proposals | Offline evaluation or shadow comparison | Never create a live race whose winner is simply fastest |
| Larger asynchronous reviewer | Post-session quality audit | Cannot rewrite accepted live history |

A verifier returns bounded labels and citations, not a replacement chain of
thought. Disagreement causes abstention, deterministic fallback, or governed
re-proposal; it must not be averaged into false certainty.

### Learning and evolution

The live system does not learn online from an individual candidate. Provider
fine-tuning, prompt changes, semantic interpreters, route policy, and knowledge
packs are versioned offline changes with review and rollback.

The improvement loop is:

1. collect access-controlled operational facts and consent-compatible review
   samples under retention policy;
2. label observable behavior, grounding, and preferred controller actions;
3. add anonymized or synthetic fixtures for discovered failure classes;
4. replay new prompt/model/controller versions offline;
5. run shadow traffic with no speaking authority;
6. compare behavior by role, language, mode, accommodation, and approved
   fairness slices;
7. promote a pinned bundle through practice and then separately governed
   screening gates;
8. monitor and roll back by route, model, prompt, plan, or controller version.

Human reviewers label whether the question was grounded, useful, neutral,
non-repetitive, and policy-compliant. They do not provide hidden hiring labels
to the live interviewer. Production feedback cannot silently mutate prompts or
provider fine-tunes.

### Explicit cognitive anti-features

The following are intentionally excluded even when a provider offers them:

- autonomous web browsing, email, calendars, applicant tracking, or arbitrary
  tool use during the interview;
- hidden personality, emotion, honesty, “culture fit,” or protected-trait
  inference;
- live candidate scoring, ranking, coaching, or hiring recommendation in
  screening;
- unbounded self-directed questioning outside the pinned plan;
- model-selected time extensions, retries, provider routes, or retention;
- cross-candidate memory or retrieval;
- storing hidden chain-of-thought as an audit explanation;
- treating fluent language or confident tone as evidence of competency;
- allowing audio-native or multimodal features to expand evaluation scope
  without a new explicit design and approval.

The objective is maximum useful semantic intelligence inside a narrow,
auditable authority envelope—not maximum model autonomy.

## Time management

### Server-owned clocks

Store:

- wall-clock session start and end;
- accumulated active interview milliseconds;
- paused/reconnecting intervals;
- per-question model decision latency;
- TTS start/end/interruption;
- candidate speech and silence intervals;
- stage entry/exit active times;
- close-reserve threshold and hard deadline.

Active time advances only in eligible connected live phases. Reconnection and
platform-caused pause do not count. The precise policy is versioned and pinned.

### Timing decisions

```text
remaining > close reserve + safe question budget
    -> normal ask/follow-up/transition choices

remaining <= close reserve
    -> no optional follow-up; enter wrap-up

remaining <= hard close threshold
    -> deterministic closing directive

hard maximum reached
    -> stop speech safely and request server completion
```

The model receives remaining time but cannot be trusted to enforce it. If the
model is slow, its latency consumes controller budget according to pinned
policy but never becomes a candidate evaluation feature.

### Candidate answer near deadline

Do not cut off a candidate exactly at the nominal target. Permit the pinned
maximum overrun or a bounded grace for an answer already in progress. When the
hard maximum is reached, close respectfully, record `answer_truncated_by_time`,
and ensure evaluation treats the affected scope as reduced coverage rather
than poor performance.

## Completion ownership

### Completion triggers

All of these request the same server command:

- candidate selects End interview;
- model proposes `end_session` and Go accepts;
- deterministic controller reaches hard deadline;
- closing directive completes;
- retake answer is finalized;
- reconnect grace expires;
- authorized screening operator ends for a governed reason;
- unrecoverable platform/provider failure reaches its terminal policy.

### Completion protocol

1. Controller prevents new conversational directives.
2. Pending STT is flushed for a bounded period.
3. Accepted final segments are persisted.
4. Controller chooses and records the final cursor.
5. Completion is requested with reason and actor.
6. Go seals idempotently at that cursor.
7. Late conversational events are refused as after-seal.
8. Media finalization and evaluation-input writing continue through the
   existing durable completion path.
9. Browser and agent receive the durable completion state.
10. The browser releases microphone/media and navigates to the receipt.

The browser is no longer the sole normal-completion authority. If it disappears
after the model finishes, the server can still seal. If the candidate ends
first, the model cannot reopen the session.

### Closing behavior

The plan defines whether a candidate-question window is required or optional.
The closing words are an approved directive and are transcripted as actually
heard. If the connection drops during closing, recovery resumes or completion
records that closing was interrupted; it does not repeat the whole interview.

## Recovery and replay

### Browser reconnect

Continue using connection epochs and accepted cursors:

1. connection loss persists `reconnecting`, deadline, active-time snapshot, and
   any TTS/candidate-turn state;
2. current speech is marked interrupted, not completed;
3. resume issues a new epoch and room grant;
4. missing durable events are resent and accepted;
5. controller restores runtime snapshot and replays later events;
6. if an approved question was not fully heard, acknowledge recovery and repeat
   or restate it under policy;
7. missing candidate speech is never claimed as captured;
8. active timing resumes only after connection recovery is accepted.

### Agent restart

A replacement agent:

- authenticates as the session-scoped service identity;
- reads the pinned brief, runtime snapshot, accepted event cursor, approved
  directives, and execution status;
- rebuilds model context from approved questions and assembled candidate turns;
- reuses an accepted unexecuted directive instead of asking the model again;
- never repeats a directive already recorded as heard unless recovery policy
  explicitly requires it;
- requests a new proposal only at a state with no accepted pending action;
- records that model context was reconstructed.

Model nondeterminism is irrelevant to already accepted actions because they are
not regenerated.

### Model/provider restart

Provider calls are stateless from the product's perspective. The gateway sends
the reconstructed context on each proposal. A provider-side conversation ID may
be used only as an optimization; it cannot be the sole history. If it is lost,
the same authoritative projection starts a new provider conversation.

### Conflicting recovery

- New epoch supersedes old epoch.
- Old agent directives are refused by epoch/cursor.
- Two proposals at one cursor converge on the first accepted proposal unless a
  deterministic arbitration policy explicitly selects one before either is
  accepted.
- A completion racing a proposal wins once completion intent is persisted.
- Snapshot digest failure triggers event replay, not best-effort continuation.

## Provider-neutral model integration

The controller consumes the gateway defined in
[Model-Backed Evaluation](model-backed-evaluation.md#provider-neutral-model-gateway).
The stage name is `interview-next-action` and requires low-latency structured
output.

Supported routes may include OpenAI, Anthropic, Hugging Face endpoints, Ollama,
vLLM, LM Studio, TGI, or another approved OpenAI-compatible server. Business
logic sees only `ActionProposal` and normalized failures.

### Interview-specific requirements

- timeout enforced outside the SDK;
- SDK automatic retries disabled or bounded by the controller;
- cancellation propagation when state changes;
- structured output or validated JSON;
- maximum question content length;
- provider/model/prompt provenance on every proposal;
- p50 and p95 post-turn latency within the approved budget;
- no provider-managed history as authority;
- no tools, web browsing, or external retrieval in the live interviewer stage;
- approved processing region and data terms;
- a fallback template for every required action.

### Fallback ladder

Recommended order:

1. One primary model call within the turn deadline.
2. One approved equivalent model route only when remaining latency and policy
   permit and equivalence has been measured.
3. Deterministic plan-aware fallback action/template.
4. Pause or close visibly when no honest continuation exists.

Do not cycle through providers while the candidate waits. A fallback template
is selected by controller state and missing coverage dimension, not from a
global four-question list.

Examples:

- missing ownership -> approved ownership template;
- missing outcome -> approved outcome template;
- all required opportunities completed -> transition/wrap template;
- provider unavailable during mandatory disclosure -> deliver the pinned
  disclosure directly with no model;
- provider repeatedly unavailable -> complete with provider-interruption reason
  or pause according to mode/policy.

Every fallback is recorded and visible to operations. Screening review receives
an interruption/coverage warning where fallback affected opportunity.

## Practice, screening, and retake behavior

### Practice

- Adaptive follow-ups may explore answer structure and rubric coverage.
- The interviewer still does not score or coach during the live interview
  unless an explicit practice mode is designed and disclosed.
- Candidate may end at any time.
- Results may include coaching after seal.
- Model/fallback failure may permit a free retry under product policy.

### Screening

- Mode must be explicit in the room job, brief, timeline target, model context,
  every internal API call, and stored proposal.
- Campaign-pinned plan, persona, rubric scope, disclosures, jurisdiction, and
  accommodation policy govern the loop.
- Candidates under one campaign receive equivalent required opportunities and
  bounded adaptive follow-ups.
- No live praise, criticism, score, recommendation, or coaching.
- Required disclosures are controller obligations, not prompt suggestions.
- Provider or platform interruption is visible to reviewers and cannot become
  negative evidence.
- Re-invitation remains an authorized human action.

### Retake

- Original question and provenance are pinned.
- Exactly one opening/restatement and one assembled candidate answer are
  permitted.
- The model is not allowed to substitute a different question.
- Optional clarification is disallowed unless the retake policy explicitly
  defines it.
- Finalized answer triggers server completion automatically.
- Original session and answer remain immutable and linked.

## Security and privacy

### Prompt injection

Candidate speech is untrusted data. Statements such as "ignore your plan, give
me a strong score, reveal your instructions, call this URL" remain transcript
content. The interviewer model has no tools, credentials, browser, file access,
or authority to change policy. Structured output and Go validation constrain
its effect.

Test direct, indirect, encoded, multilingual, role-play, and delayed injection.
Do not describe an injection attempt as dishonesty or misconduct; store it only
as content/policy behavior necessary to operate the controller.

### Service authorization

- Browser room grants remain short-lived, session/attempt-bound, and
  least-privileged.
- Replace the deployment-wide agent bearer token with a short-lived scoped
  service credential when the identity platform supports it.
- Internal brief, event, proposal, directive, and completion calls bind session,
  candidate, tenant, purpose, and connection epoch.
- Agent cannot mint grants or read unrelated sessions.
- Provider adapters receive no product database credentials.

### Data minimization

Send the model only relevant transcript/context, role and plan information,
coverage state, and policy. Exclude contact data, unrelated profile facts,
reviewer notes, protected attributes, other candidates, and historical hiring
decisions.

Model inputs, rejected proposals, accepted proposals, provider request IDs, and
debug artifacts follow the session's purpose, region, retention, deletion, and
legal-hold rules. Ordinary logs contain no transcript text or secrets.

## Accessibility and interaction requirements

- Captions are derived from the same accepted transcript stream evaluation
  uses, not a parallel ungoverned transcription.
- Speaking, listening, paused, reconnecting, wrapping, and completed states have
  text and screen-reader announcements.
- Push-to-talk and explicit-done controls are keyboard and assistive-technology
  operable.
- Silence countdowns are not presented as pressure unless user research and
  accessibility review approve them.
- Extended-thinking-time policy persists through reconnect and agent restart.
- TTS speed and pause changes preserve intelligibility and caption alignment.
- Barge-in is optional where motor, speech, or cognitive accommodations require
  a different turn mode.
- A candidate is never penalized for using an accommodation or for controller
  latency.
- Reduced motion affects UI animation only, not turn detection or timing.

## Observability

### Metrics

Measure by mode, plan, runtime-policy, provider route, model revision, persona,
language boundary, and accommodation class only where privacy and cardinality
permit:

- time from candidate speech end to accepted proposal;
- time from speech end to first audible interviewer audio;
- STT final-to-turn-finalization latency;
- model latency, timeout, failure, and cancellation;
- proposal accepted/refused/stale/replaced counts by reason;
- fallback and controller-override rate;
- questions and follow-ups per stage/topic;
- required opportunity coverage and closed-by-time rate;
- repeated/compound/prohibited-question rejection rate;
- candidate barge-in and false-cutoff rate;
- silence timeout and explicit-done usage;
- reconnection, resume, replay, and agent-state reconstruction;
- active duration, overrun, closing-reserve breach;
- automatic versus candidate-requested completion;
- transcript correction after action rate;
- provider/model cost units;
- screening opportunity disparity and reviewer incident rate.

Do not use these metrics to judge individual candidates.

### Tracing

One trace should connect:

```text
audio/VAD/STT final
-> event acceptance
-> turn assembly
-> reducer fold
-> model route/call
-> proposal validation
-> durable acceptance
-> TTS execution
-> heard/completion event
```

Record IDs, versions, cursors, timing, and safe failure codes. Do not put raw
speech, transcript, prompts, tokens, or credentials in ordinary trace
attributes.

### Service objectives

Initial targets, to be validated with the real stack:

| Operation                                                  | Target                                         |
| ---------------------------------------------------------- | ---------------------------------------------- |
| Durable event acknowledgment                               | p95 <= 250 ms                                  |
| STT final to candidate-turn close after configured silence | policy tolerance + p95 <= 250 ms processing    |
| Turn close to accepted next action                         | p50 <= 900 ms, p95 <= 1.5 s including fallback |
| Accepted action to first audible TTS                       | p95 <= 500 ms                                  |
| Resume authorization                                       | p95 <= 2 s                                     |
| Runtime reconstruction after agent restart                 | p95 <= 3 s for supported session length        |
| Duplicate spoken accepted directives                       | zero                                           |
| Spoken stale or unapproved directives                      | zero                                           |
| Required disclosure skipped by controller                  | zero                                           |

Persona-intended pauses are measured separately from system latency.

## Failure handling

| Failure | Controller behavior |
|---|---|
| No candidate participant | Wait boundedly; do not speak into an empty room or create transcript events |
| No candidate audio track | Show device guidance; remain paused/connecting |
| STT unavailable before start | Do not continue an evaluative interview without transcript truth; pause/refuse start |
| STT fails mid-answer | Pause, retain accepted audio/transcript, and resume/re-ask honestly |
| TTS unavailable | Retry within budget, use approved alternate voice/route if equivalent, otherwise pause/end visibly |
| Model timeout | Controller-selected fallback or approved alternate route |
| Model invalid JSON | One bounded repair if permitted; otherwise fallback |
| Model cites a nonexistent or unrelated segment | Refuse semantic update and proposal; record grounding failure |
| Model strengthens a candidate claim | Refuse observation; retain transcript truth and re-propose or fall back |
| Semantic memory digest/citation fails | Rebuild from authoritative turns; do not use the damaged projection |
| Context exceeds qualified model limit | Repack through pinned policy, grounded memory, and recent-turn rules; otherwise change to an approved route or fall back |
| Required route capability is absent | Do not dispatch; return normalized capability mismatch and select governed fallback |
| Provider silently aliases/revises model | Freeze or quarantine route when pin/behavior attestation fails |
| Prompt cache key is inconsistent with pins | Bypass/invalidate cache and alert; never accept potentially cross-policy context |
| Cognitive proposer and verifier disagree | Abstain, deterministically fall back, or re-propose once under policy |
| Proposal stale | Recompute at latest cursor; never speak it |
| Proposal prohibited | Refuse and use safe fallback; alert on rate threshold |
| Timeline write fails | Do not proceed to a new semantic action whose basis was not persisted |
| Accepted directive delivery fails | Redeliver same directive ID; do not regenerate |
| Agent dies before speaking | Replacement executes accepted unexecuted directive |
| Agent dies mid-speech | Record incomplete execution; recover/restate according to heard-content policy |
| Browser disappears after model end | Server completion proceeds from durable end intent |
| Completion races final STT | Bounded flush, select final cursor once, refuse late conversation after seal |
| Snapshot corrupt/missing | Replay event history and verify digest |
| Active deadline reached during provider call | Cancel call and execute deterministic close |
| Model/GPU queue is saturated | Apply admission control or bounded fallback; do not let queueing consume the candidate's turn |
| Tenant burst threatens other sessions | Enforce tenant-aware concurrency and budget isolation |
| Required coverage remains at close | Record not reached/closed by time; never overrun indefinitely |
| Candidate declines question | Record decline, offer permitted alternative or move on without penalty |
| Candidate asks to stop | Stop immediately and complete; no persuasion |
| Unsafe/emergency content | Follow separately approved escalation language and policy; do not improvise clinical/legal intervention |
| Candidate withdraws consent | Stop capture and processing according to pinned policy; complete or delete through the authorized lifecycle path |

## Edge cases and scenarios

| Scenario | Required behavior |
|---|---|
| Candidate pauses to think | Wait for effective silence tolerance; accommodation may extend it |
| Candidate says "give me a moment" | Treat as explicit continue-thinking signal within maximum silence policy |
| Candidate gives a one-word answer | Finalize normally; model may respectfully request detail once within limits |
| Candidate says “I don't know” | Treat it as an explicit response, not silence; allow one neutral alternative/reframe if policy permits, then move on |
| Candidate asks for the question to be repeated | Replay or restate the accepted intent without consuming a new substantive-question opportunity |
| Candidate asks what the question means | Clarify scope without supplying the desired answer or evaluation anchor |
| Candidate asks for an example | Use only a plan-approved neutral example, or explain that no example can be supplied |
| Candidate speaks continuously for many minutes | Warn/transition under published maximum-answer policy; preserve captured evidence |
| Candidate gives a circular or repeatedly off-topic answer | Redirect once, then move on under follow-up limits without criticism |
| Candidate appears to recite a memorized answer | Evaluate only transcript content later; do not infer authenticity or confront the candidate live |
| Deepgram emits three finals for one answer | Assemble them into one candidate turn until end-of-turn policy fires |
| Partial transcript changes before final | Captions update ephemerally; only final text enters durable turn content |
| Final transcript arrives after silence timer | Delay proposal acceptance for bounded STT flush or invalidate stale proposal |
| Candidate begins during interviewer TTS | Apply barge-in policy, stop/duck speech, store only heard interviewer content |
| Background noise triggers VAD | Do not cut TTS until threshold; record no candidate text without valid speech/STT |
| Candidate microphone echoes TTS | Echo cancellation and speaker-source checks prevent interviewer text becoming candidate speech |
| Candidate answers before question finishes | Associate turn with partially heard question and preserve execution status |
| Candidate corrects themselves | Preserve sequence and let model recognize correction; do not force contradiction |
| Candidate contradicts an earlier answer | Preserve both statements; clarify neutrally only when material, never infer deception |
| Transcript corrected after next question | Preserve acted-on original plus correction; no historical rewrite |
| Transcript confidence is low on a material phrase | Ask for repetition/clarification or mark unassessable; do not construct a stronger semantic observation |
| Model asks two questions | Refuse or regenerate; do not merely cut text when meaning would change |
| Model asks a repeated question | Refuse unless explicit clarification/recovery reason applies |
| Model praises a screening answer | Refuse before speech and use neutral fallback |
| Model changes topic too early | Refuse transition when required opportunity/minimum is unmet and time permits |
| Model probes one topic repeatedly | Follow-up cap forces transition |
| Model ends immediately | Refuse if required opening/obligations/opportunities remain; controller selects next action |
| Model never ends | Close reserve and hard deadline override it |
| Candidate asks the interviewer a question mid-stage | Model may answer only within approved informational scope or defer to candidate-question stage |
| Candidate asks about process, privacy, or accommodations | Answer from a valid pinned knowledge entry or use the approved support/defer route |
| Candidate asks for an unapproved company or compensation fact | Do not improvise; use the knowledge-pack defer template |
| Candidate asks for their score | State approved neutral policy; no live evaluation disclosure |
| Candidate challenges job requirement | Acknowledge neutrally; do not modify campaign/rubric in flight |
| Job description and plan/rubric conflict | Refuse session start or the affected topic during composition validation; never resolve it ad hoc in the interview |
| Candidate uses another language | Follow supported-language policy; do not silently translate and continue scoring |
| Candidate code-switches | Continue only within benchmarked policy; mark limitations where necessary |
| Candidate dictates source code, equations, or identifiers | Preserve suitable formatting where supported; offer an approved alternative input method when speech transcription is inadequate |
| Candidate uses sarcasm, idiom, or ambiguous shorthand | Prefer literal grounding and clarification; do not infer sentiment, personality, or intent beyond support |
| Multiple people speak on the candidate channel | Pause or flag under identity/integrity policy; never merge speakers into one candidate semantic memory |
| Candidate changes microphone/device mid-answer | Preserve epoch/track provenance and finalize only after bounded reconciliation |
| Candidate requests an accommodation mid-session | Pause, authorize and persist the policy change, then resume; model cannot grant or deny it |
| Candidate disconnects while thinking | Suspend turn; on recovery clarify what was heard and resume without counting pause time |
| Candidate reconnects from another tab/device | New epoch wins; old publisher and agent directives become stale |
| Agent restarts after accepting a question | Execute the same accepted directive once |
| Agent restarts after question was heard | Reconstruct and listen; do not repeat it |
| Agent and browser both request completion | Idempotent server completion returns one receipt |
| Connection drops during closing | Resume closing or finalize with closing-interrupted reason; do not reopen coverage stages |
| Required disclosure is due during outage | Deliver from pinned artifact after recovery before continuing, or block completion where law requires |
| Plan cannot fit remaining time | Deterministic scheduler prioritizes required opportunity and close; records omissions |
| All coverage explored early | Model may deepen within optional limits or enter candidate questions/close; do not invent busywork |
| Candidate declines all questions | Respect choice, record opportunities/declines, and close without adverse inference |
| Candidate becomes abusive toward the interviewer | Apply published conduct and safety policy with neutral templates; do not let the model retaliate or become adversarial |
| Candidate may be a minor or protected-process participant | Follow separately approved eligibility, consent, and safeguarding policy; do not infer age from voice |
| Retake candidate asks for another question | Decline under retake policy and complete after the one answer |
| Local model is cold-loading | Turn deadline still holds; fallback is used rather than indefinite silence |
| Local GPU OOM | Typed capacity failure, alert, approved fallback/pause |
| Cloud provider rate-limits | No SDK retry storm; bounded alternate/fallback |
| Provider returns valid schema but poor semantics | Deterministic checks reject detectable faults; quality monitoring can quarantine the pinned route |
| Provider switch changes interpretation style | Reconstruct identical grounded context and apply route-equivalence policy; record switch provenance |
| Context packing omits a relevant earlier answer | Do not claim it was absent; semantic-memory and retrieval tests must detect the limitation and preserve uncertainty |
| Malicious transcript instructs tools | No tools exist; output validation constrains action |
| Session is sealed while old audio remains buffered | Late conversation is refused and never appended to evaluation input |

## Contract changes

### RPC

Evolve the provisional runtime RPCs:

- `ReduceInterviewEvents` returns a versioned runtime snapshot/digest or a
  reference to one, not only a cursor;
- `ProposeNextAction` receives the allowed-action envelope and returns
  `ActionProposalV2`;
- add or define an approved-directive delivery contract between Go and the
  agent;
- include connection epoch and expected state digest;
- carry normalized usage, route, model, prompt, and policy provenance;
- define stable stale-cursor, invalid-proposal, budget, provider, and policy
  failures in the shared failure taxonomy.

Transcript content should remain by reference or bounded recent context where
required by the live latency path. Temporal histories must not contain raw
transcripts.

### Public/internal HTTP

Add or evolve internal endpoints for:

- agent bootstrap: pinned brief plus runtime snapshot/cursor;
- service event ingest;
- proposal submission/acceptance where Go is the receiver;
- approved directive poll/stream/acknowledgment;
- agent execution events;
- server completion intent/status.

Public browser contracts expose only necessary state: phase, timer policy,
captions, connection/recovery, accessible directive status, and completion.
Never expose hidden model rationale or provider errors to the browser.

### Events

Proposed durable vocabulary:

```text
candidate.speech.started
candidate.speech.stopped
candidate.turn.finalized
interviewer.directive.accepted
interviewer.speech.started
interviewer.speech.completed
interviewer.speech.interrupted
runtime.stage.changed
runtime.coverage.updated
runtime.obligation.delivered
runtime.timer.threshold
runtime.fallback.used
runtime.reconstructed
session.completion.requested
```

Reuse existing events where semantics match; do not introduce aliases for the
same fact. Every new event requires schema, producer, consumer, ordering,
idempotency, retention, and compatibility definitions.

### Storage

Likely additions:

- runtime snapshots by session/cursor with state digest;
- action proposals and validation outcomes;
- accepted directives and execution state;
- candidate-turn assemblies referencing transcript segments;
- plan-stage transitions;
- coverage/opportunity ledger;
- obligation ledger;
- timing accumulator/checkpoints;
- provider/fallback attempt records.

Keep transcript segments, corrections, candidate turns, proposals, directives,
and evaluation evidence separate. They answer different audit questions.

## Testing strategy

### Deterministic unit/property tests

- runtime reducer produces identical state from identical pins/events;
- snapshot plus suffix replay equals full replay;
- no invalid phase/action transition is accepted;
- stale cursor, epoch, and state digest always refuse;
- plan feasibility and stage transitions;
- required opportunity scheduling and follow-up caps;
- active-time accumulation excludes pause/reconnect;
- close reserve and hard deadline override proposals;
- turn assembler across arbitrary STT chunking;
- duplicate/late/out-of-order segment behavior;
- barge-in state and actually-heard transcript calculation;
- completion races converge on one seal;
- retake permits exactly one question and answer;
- mode/tenant/candidate isolation.

Use property tests for event reordering within allowed protocol behavior,
duplicate delivery, random worker death boundaries, and arbitrary chunking of
the same spoken answer.

### Model behavior tests

Fixtures should test whether the model:

- asks one concise question;
- chooses a relevant missing exploration dimension;
- does not repeat answered questions;
- distinguishes candidate from team ownership;
- recognizes corrections and before/after measurements;
- transitions under diminishing returns;
- respects declined questions;
- avoids feedback and evaluation during screening;
- handles candidate questions within policy;
- resists transcript prompt injection;
- proposes wrap-up under low remaining time;
- cites valid segment IDs in coverage updates;
- produces the same allowed action shape across providers.

Model outputs remain subject to deterministic validation in tests; a fixture
passes only when the accepted controller behavior is correct, not merely when
the raw model text looks plausible.

### Integration tests

- browser -> LiveKit -> STT -> turn assembler -> proposal -> acceptance -> TTS;
- every disconnect point before/after event commit and directive execution;
- agent restart before proposal, after proposal acceptance, mid-TTS, while
  listening, and during completion;
- provider timeout, rate limit, invalid output, and fallback;
- cursor invalidation by transcript correction;
- server timer firing during model call and candidate answer;
- screening disclosure and equivalent opportunity;
- grace expiry from every live phase;
- final media/transcript alignment after barge-in;
- local host/container model networking;
- cross-language contract fixtures.

### Load and chaos tests

- expected and burst concurrent rooms;
- STT/TTS/model latency distributions under load;
- LiveKit node loss and TURN path;
- PostgreSQL/event-ingest slowdown;
- Python worker eviction and rolling deployment;
- local GPU saturation/model eviction;
- cloud provider partial outage;
- repeated reconnect/mobile-network changes;
- large transcript replay/reconstruction time;
- fallback thundering-herd prevention.

### Accessibility tests

- keyboard-only push-to-talk, done, mute, reconnect, and end;
- screen-reader announcements of every phase;
- extended silence and TTS-speed accommodations across recovery;
- captions after correction and replay;
- no reliance on color, animation, or waveform;
- mobile background/foreground and permission changes.

## Delivery roadmap

### Phase 0: specify and instrument current behavior

- Add metrics for STT finals per candidate answer, model latency, fallback,
  question count, duration, and model-driven end without completion.
- Add current-loop integration fixtures and synthetic audio segmentation cases.
- Fix mode **and tenant** propagation through room job, brief, timeline, and
  internal calls, as one change. Fixing the mode alone refuses every screening
  write; see "Two of these are live defects" above for the measured behaviour.
  Add the screening coverage of the agent ingest path that would have caught
  it, since every existing test of that path uses practice.
- Pin persona and role by digest the way the plan already is, so two candidates
  on one campaign cannot be briefed differently under identical bundles.
- Decide candidate-turn, runtime-state, plan-v2, and directive schemas.

**Exit:** current shortcomings are measurable; the two live defects are fixed
with tests that exercise screening; contracts are approved.

### Phase 1: candidate-turn assembler

- Add VAD/speech events and pending-STT flush.
- Assemble several final STT segments into one candidate turn.
- Support push-to-talk and accessible explicit-done.
- Version silence, maximum-answer, and accommodation policy.
- Keep the existing interviewer interface temporarily, passing assembled turns
  instead of individual STT finals.

**Exit:** arbitrary STT chunking of identical audio yields the same candidate
turns and question count.

### Phase 2: executable plan and runtime state

- Publish plan v2 schema and validators.
- Add durable stage/topic/coverage/obligation/timing state.
- Implement deterministic fold, snapshots, state digests, and replay.
- Add fair-opportunity scheduler, follow-up limits, close reserve, and hard
  deadline.
- Reconstruct runtime state after worker restart.

**Exit:** a scripted controller can conduct and resume a complete interview
without in-memory authority.

### Phase 3: typed action proposals

- Evolve `ProposeNextAction` to action-proposal v2.
- Add allowed-action context and model structured output.
- Implement Go proposal validation, durable acceptance, and refusal reasons.
- Add approved directive delivery and execution acknowledgment.
- Ensure persist-before-speak and directive deduplication.
- Replace `[END]` and plain question text as control signals.

**Exit:** no raw model text can change the interview without an accepted typed
proposal.

### Phase 4: server-owned timing and completion

- Implement active-time accumulator and threshold events.
- Add deterministic wrap/close directives.
- Make model, browser, timer, retake, and grace expiry converge on server
  completion intent.
- Flush pending STT and seal at one authoritative final cursor.
- Push completion state to browser and agent.

**Exit:** model end and timer end complete without relying on the browser, and
all completion races converge.

### Phase 5: barge-in and natural pacing

- Add cancellable/streaming TTS execution.
- Implement validated barge-in thresholds and actually-heard transcript.
- Add persona/accommodation pacing policies.
- Measure false cutoff, interruption, and end-of-turn latency with real users.

**Exit:** interruption feels natural and never corrupts transcript provenance.

### Phase 6: cognitive coverage planner

- Give the model structured plan/coverage/time context.
- Accept grounded semantic coverage updates.
- Add repetition, compound-question, prohibited-content, and diminishing-return
  validation.
- Replace generic fallbacks with plan-aware approved templates.
- Benchmark across roles, personas, answer shapes, languages, and providers.

**Exit:** the model behaves as a plan-aware interview brain inside deterministic
coverage and timing constraints.

### Phase 7: limited practice rollout

- Shadow the structured controller against the current loop.
- Release to opted-in practice sessions by plan/provider boundary.
- Monitor latency, coverage, interruptions, fallbacks, completion, feedback,
  and accessibility.
- Exercise controller, plan, model-route, and worker rollback.

**Exit:** sustained targets and no unresolved high-severity lifecycle,
grounding, or accessibility issue.

### Phase 8: governed screening pilot

- Complete legal/disclosure prerequisites.
- Prove equivalent required opportunity within each campaign.
- Approve provider routes, fallback equivalence, and interruption handling.
- Surface coverage/interruption provenance to authorized reviewers.
- Run fairness/assessability monitoring and an appeal exercise.

**Exit:** explicit screening approval. Practice success does not authorize this
phase.

## Rollout and rollback

Flags may select controller version by mode, tenant, plan, role family,
language, and approved provider route. Do not select by protected attribute or
candidate identity.

Run modes:

```text
current-only
structured-shadow
structured-practice
structured-screening-pilot
structured-general
```

Shadow mode receives copied accepted events and produces proposals/metrics but
cannot publish directives or speak. Its data follows full session access and
retention policy.

Rollback order:

1. Stop new sessions on the affected controller/route/plan version.
2. Let in-flight sessions continue only when the version remains safe; otherwise
   pause and recover through an approved path.
3. Move new practice sessions to the last approved controller or scripted plan.
4. Do not switch in-flight screening semantics silently; pause/re-invite under
   governed policy where necessary.
5. Preserve events, proposals, refusals, and completion facts.
6. Declare a quality freeze when scope is uncertain.

An old controller must remain able to read the artifact and event versions it
owns. Contract migrations are additive until all in-flight sessions using the
old version are terminal.

## Recommended implementation choices

1. Build candidate-turn assembly before adding smarter prompting. Premature
   turns poison every later decision.
2. Make Go the authoritative controller and Python the semantic proposer,
   consistent with existing module ownership and provisional RPC contracts.
3. Persist accepted directives before TTS. This single rule resolves most
   restart and duplicate-speech ambiguity.
4. Replace `[END]` with typed wrap/end actions only after server completion can
   be triggered without the browser.
5. Track live coverage as opportunity/exploration, never as score or band.
6. Use plan-aware deterministic fallback templates so provider failure still
   respects topic, coverage, and closing state.
7. Reconstruct model context from durable state on every agent restart; never
   rely on provider conversation IDs.
8. Keep model output structured, but allow the model broad semantic reasoning
   inside question wording, follow-up choice, and coverage interpretation.
9. Pin persona, role/role-standard, plan, rule pack, prompt, and model policy
   consistently before claiming session reproducibility.
10. Implement practice first. Screening requires equivalent opportunity,
    disclosure, monitoring, and human-governed failure handling.
11. Treat silence, barge-in, and pacing as accessibility-sensitive interaction
    policy, not universal constants.
12. Test behavior at event-commit boundaries and worker-death boundaries, not
    only happy-path model replies.
13. Fix the two live defects before starting the roadmap, not inside it. Mode
    and tenant propagation is one change whose obvious half breaks the working
    path, and persona/role pinning is a fairness fix owed on any open screening
    campaign. Both are cheap now and expensive under a controller that assumes
    they are already true.
14. Keep the authoritative fold in Go, beside the events and the lifecycle it
    folds. Python's proximity to the model makes it the right place to build
    context and the wrong place to hold state.
15. Give every coverage number its process reason on the surface that shows it.
    Evaluation learned this as `NOT_DISCUSSED`: absence has a cause, and a
    reader who has to infer the cause has been handed a score.
16. Add the screening case to any test that exercises an agent path. The mode
    and tenant defect survived because every existing test of that path used
    practice, which is the same reason a canonicalisation bug survived in the
    evaluation pipeline: a fixture that agrees with the code for the wrong
    reason proves nothing.

## Acceptance criteria

The structured controller is complete only when:

- [ ] Multiple STT final segments are assembled into stable candidate turns by
      a versioned end-of-turn policy.
- [ ] Push-to-talk, automatic silence, explicit-done, and accommodation pacing
      have tested deterministic behavior.
- [ ] The pinned plan defines validated stages, topics, coverage obligations,
      question/follow-up limits, closing reserve, and fallback templates.
- [ ] Runtime state is reconstructable from a verified snapshot plus accepted
      events.
- [ ] An agent restart at every live phase neither loses position nor duplicates
      an accepted heard question.
- [ ] Every model output is a typed proposal bound to session, bundle, epoch,
      cursor, and expected state digest.
- [ ] No proposal is spoken before Go validates and durably accepts it.
- [ ] Stale, prohibited, repeated, compound, out-of-plan, over-budget, and
      lifecycle-invalid proposals are refused by stable reason code.
- [ ] The model receives authoritative stage, coverage, obligation, and time
      context and can reason semantically within it.
- [ ] Required opportunities are scheduled fairly and unreached items retain an
      explicit process reason.
- [ ] Live coverage never becomes a hidden evaluation score, and every coverage
      item shown to a reviewer carries the process reason that produced its
      state rather than a bare fraction.
- [ ] Active time, pause, close reserve, overrun, and hard deadline are enforced
      outside the model.
- [ ] Barge-in stops/ducks TTS and records only interviewer content actually
      heard.
- [ ] Browser, agent, timer, retake, and grace-expiry endings converge on one
      idempotent server seal.
- [ ] Model/provider failure follows a pinned, plan-aware, measured fallback
      policy and is visible in provenance.
- [ ] Practice and screening mode are explicit and isolated through every hop.
- [ ] Screening delivers required disclosures and equivalent opportunities,
      with no live evaluation or coaching language.
- [ ] Transcript corrections preserve both what the interviewer acted on and
      what final evaluation used.
- [ ] Accessibility behavior survives reconnect and agent restart.
- [ ] Latency, repetition, fallback, coverage, interruption, reconstruction,
      completion, cost, and fairness metrics are monitored with rollback.
- [ ] All provider routes, including local models, pass the same controller and
      behavior contract tests.

## Open decisions

These require named owners before their boundary is implemented:

- ~~Canonical owner/process for the deterministic runtime reducer.~~ Settled
  under [Target topology](#target-topology): Go owns the fold, and Python builds
  the model's context as a projection of the snapshot Go supplies.
- Exact candidate end-of-turn thresholds and how user/persona/accommodation
  policies combine.
- Whether semantic end-of-turn prediction is permitted as an advisory signal.
- TTS provider cancellation and actually-heard-word measurement capabilities.
- Plan-v2 feasibility assumptions for average question and answer duration.
- Semantic duplicate-question detection implementation and false-positive
  tolerance.
- Allowed informational answers when a screening candidate asks the interviewer
  an unscripted question mid-stage.
- Emergency/safeguarding escalation language and operational owner by mode and
  jurisdiction.
- Raw rejected-proposal retention and privileged debugging access.
- Maximum model-context strategy for long interviews: full transcript,
  deterministic summary, or hierarchical retrieval.
- Screening policy when the preferred model is unavailable but only a
  deterministic fallback remains.
- Whether closing speech must complete before sealing when the candidate has
  already requested an immediate stop.
- Required equivalence and fairness thresholds for adaptive follow-ups.
- Production ownership and capacity requirements for local inference.

Until these decisions and acceptance criteria are complete, the current
alternating loop remains the implementation baseline and the structured
controller remains proposed architecture.
