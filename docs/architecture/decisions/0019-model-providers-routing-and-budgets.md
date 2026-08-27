# ADR-0019: Model providers, routing, fallback and budgets

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-27  
**Review date:** 2027-02-27  
**Supersedes:** None  
**Superseded by:** None

Closes DEC-10, deliberately late: everything shipped before this ADR runs
deterministic floors (evidence-1, aggregate-1, coaching-1) with model and
policy versions honestly recorded as none, behind validation gates built
to hold a model to the same rules. The first model-backed stage is the
voice agent itself, and a provider decision made any earlier would have
been made without the thing that needs it.

## Context

[evaluation-system.md](../evaluation-system.md) requires per-stage
versions and usage on every activity, provider fallback only where
equivalence is validated, and budget exhaustion that preserves the
required result while marking optional omissions. ADR-0012 fixed the
topology: browser to our SFU to our own Python agent, with STT, LLM and
TTS behind adapters for this decision to fill. ADR-0001 fixed the region
(eu-west-2) and [data-classification.md](../../security/data-classification.md)
requires every AI provider transfer to carry an approved purpose, region,
retention and training policy, and minimisation.

## Decision

### Providers, one per capability, adapters in between

| Capability | Primary | Validated fallback candidate | Why |
|---|---|---|---|
| Speech to text | Deepgram, streaming, EU endpoint | Speechmatics | Our transcript contract demands per-word timings and confidence on one clock; Deepgram's streaming output is that shape natively, so the adapter is a mapping, not a reconstruction. |
| Language model | Anthropic Claude: Sonnet for the interviewer loop and evaluation stages, Haiku for fast classification (turn-taking, routing) | OpenAI, only after equivalence is measured per stage | Question quality drives evidence quality; streaming fits ADR-0012's p50 900ms budget; per-stage model choice keeps cost proportional to what each stage decides. |
| Text to speech | Cartesia, streaming | ElevenLabs | Latency-optimised streaming synthesis is what a voice loop's budget is spent on; the persona's voice is a pinned artifact field, so switching providers is a re-pin, not a rewrite. |

Every provider sits behind the adapter ADR-0012 named, in the Python
agent. The adapter contract is the platform's, not the provider's: the
provider is a configuration value plus terms on file.

### The terms that filter every provider

A provider is admissible only with, in writing: zero retention of our
inputs and outputs beyond request processing; no training on our data;
processing in the UK or EU; a data-processing agreement compatible with
DEC-15's schedules. A provider that cannot meet all four is not a
fallback, whatever its quality. The record of each provider's terms lives
beside this ADR as it is signed.

### Routing and fallback

- Routing is per stage and pinned: which provider and model a stage used
  is recorded on the activity (model_version, policy_version), and a
  session's evaluation can name every model that touched it.
- **Fallback is opt-in per stage and only after measured equivalence**
  (QUA-05/QUA-06's job): a stage falls back to a second provider only
  when a benchmark shows the second produces results within the stage's
  recorded tolerance. Until then, a provider outage at a stage is the
  stage's own failure, handled by the next rule, never a silent switch.
- **The deterministic floor is the terminal fallback everywhere it
  exists.** evidence-1, aggregate-1 and coaching-1 stay in the tree
  behind the same contracts and gates; a model stage that fails after
  bounded retry hands back to the floor, and the result records that it
  did. The live voice loop has no floor: an outage there follows
  ADR-0012's degradation ladder and ends in pause-with-resume.

### Budgets and exhaustion

- Each stage carries a budget in cost units (the Usage message the
  contract already carries), set per stage in the model policy artifact,
  versioned like everything else.
- **Exhaustion never degrades a required result silently.** The
  deterministic result and its status are always produced; optional
  narrative (coaching prose beyond coaching-1, articulation commentary)
  is omitted and the omission is marked on the result and shown to the
  candidate in words. EVL-07 builds this; the rule is fixed here.
- Budget exhaustion mid-interview never ends the interview: the voice
  loop's budget is provisioned per session at start, and the rule that
  nothing touches an interview in flight (ADR-0014) applies to money too.

### The live voice loop specifically

The agent joins the room as the interviewer with a service identity, is
the source of transcript truth (per-word timings on the room clock,
written through the same ingest path the browser uses, under a service
credential), and speaks the pinned persona's voice. Turn boundaries are
the agent's decision, recorded as control events, so evaluation never
infers them.

## Consequences

- The agent can be built now with real adapters behind configuration and
  scripted adapters in tests; provider keys are deployment secrets, never
  fixtures.
- QUA-05/QUA-06 gain a concrete job: measure equivalence per stage
  before any fallback switches on.
- Anthropic is the author's own maker; the honesty owed here is that the
  adapter boundary makes this decision cheap to revisit, and the review
  date exists to force the revisit.

## Revisit when

Equivalence measurements land; a provider's terms change; the voice
loop's measured latency misses ADR-0012's budget on the chosen stack; or
a jurisdiction's determination under DEC-11 constrains processing
location further.
