# Epic CAT — Catalogue, artifacts and session composition

**Phase 2–3** · **Workstream** Python, Go, Web

Roles, shapes, personas and rubrics come from the server, not from hardcoded lists. Composition turns a
candidate's or recruiter's choices into an immutable bundle that pins every artifact version the session
will use — which is what makes a result reproducible a year later.

---

### CAT-01 · Build the artifact registry with review, publication and rollback

**Depends on** DEC-09, PLT-03 · **Blocks** CAT-02, TEN-04, QUA-04

Rubrics, calibrations, prompts, personas and interview blueprints as versioned, digest-addressed
artifacts with an approval step and a rollback path.

**Done when**
- [x] An artifact can be drafted, reviewed, published, pinned and rolled back.
- [x] Published artifacts are immutable; a change creates a new version.
- [x] Publication never mutates an in-flight or historical session.

**Done, to ADR-0011.** The registry lives in `internal/content`: lifecycle machine held to the
domain model's chain, immutability and no-deletion by trigger (proven by dropping the trigger and
watching the suite refuse), separation of duties in the aggregate so the drafter cannot publish
their own version whatever they hold, and rollback as an audited pointer move that deprecates
rather than deletes. Pins are digests over canonical JSON, verified on every read; the
never-mutates-a-session box is the test that publishes v2 and finds a previously pinned digest
still resolving byte-identical. One registry serves platform and tenant artifacts, with a tenant's
pointer overriding the platform's for the same reference. What remains for composition to use it:
CAT-02's composer resolving and pinning from here - done since - and the git-authored loader,
which landed with CAT-03's catalogue: `contentctl` walks `services/intelligence/artifacts/`
through the registry's own lifecycle, idempotently, refusing an edited file that wears an
already-published version number.

**Spec** [domain-model.md](../../architecture/domain-model.md)

---

### CAT-02 · Implement interview composition as a durable workflow

**Depends on** CAT-01, PLT-06, CTR-02 · **Blocks** SES-02

Go accepts the request; a Temporal workflow calls Python to compose the interview plan; the result is
persisted as an immutable bundle with every artifact version and input digest recorded.

**Done when**
- [x] Composition is idempotent and safe to retry without producing two bundles.
- [x] The bundle records every pinned artifact version, input digest and policy version.
- [x] Composition failure is a visible, retryable state rather than a dead session.

**The floor is in and the walking skeleton crosses it.** The Temporal workflow drives a session
from composing to ready through a real gRPC call into Python's composer, and the restart proof in
the interview package shows a worker death mid-composition converging on one bundle, one event and
one audit row. Idempotency is by construction: composition is deterministic over its pinned
inputs, tested from both languages, so a retried activity re-presents the same request and arrives
at the same digest rather than forking the session's identity. Failure lands in
composition_failed carrying the refusal's own taxonomy code, retryable to composing per the
machine.

The middle box closed when CAT-01's registry arrived. Go resolves the blueprint's plan from the
registry and pins it - reference, version, schema version, digest, body - and Python composes over
exactly what arrived pinned, verifying each pin's digest against its body first, because a bundle
asserting inputs it did not read is the reproducibility lie composition exists to prevent. The
bundle document records every pin and is persisted by Go in the ready transition's transaction,
immutable and permanent by trigger from that moment. The cross-language test walks the whole
chain: publish through the registry's own lifecycle, resolve, pin, compose, and the bundle's
recorded digest equals the registry's published one. What widens later is the pin set - the floor
pins the plan; personas, rubrics and prompts join as their capabilities arrive - and each addition
is a new bundle schema version, additively.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

---

### CAT-03 · Serve the discipline, role, shape and persona catalogue

**Depends on** CAT-01 · **Blocks** CAT-04

Server-provided metadata with validation and duration limits, so the product is not quietly restricted
to software roles by a hardcoded list.

**Done when**
- [x] Catalogue endpoints serve disciplines, roles, shapes and personas with their valid combinations.
- [x] Invalid combinations are rejected server-side with a field-level error, not filtered in the browser.
- [x] Adding a profession is a data change, not a deployment.

**Serving and data are done; the refusal has its logic and awaits its endpoint.** The catalogue
is one registry artifact - ADR-0011's first git-authored content, published by `contentctl`
through the same lifecycle as everything else - so the four endpoints resolve it per tenant (a
tenant's pointer overrides the platform's) and serve collections nothing in the binary knows the
names of: six disciplines, eight roles across them, five shapes, four personas, straight from the
prototype's vocabulary. The combination rules ride the data - a role's shapes, a shape's runnable
lengths, a persona's shapes with empty meaning unrestricted - and `catalog.Validate` refuses a
selection field by field with stable codes, proven against every invalid combination. The middle box closed when
CAT-04 landed POST /interviews: the adapter validates the selection against the catalogue before
the interview context hears about it, and a refused combination answers 400 with every bad field
named by its stable code. Coherence is checked at the door - a role in
no discipline, a ghost shape, a duplicated id or a shape with no duration never publishes,
because the loader runs the reading context's own parse as its validating step.

**Spec** [public-api.md](../../contracts/public-api.md)

---

### CAT-04 · Build the practice interview configuration wizard

**Depends on** CAT-03, WEB-04 · **Blocks** SES-01

Role and focus, shape, interviewer, length and difficulty, review and start — URL-addressable, validated
per step, preserving entered data when validation fails.

**Done when**
- [x] Each step is addressable, restorable and validated independently.
- [x] Failed validation moves focus to the first problem and preserves everything already entered.
- [x] The wizard refuses to compose a screening interview, which only a recruiter can create.

**Done, wizard to workflow.** /practice/new walks the prototype's five steps - role, shape,
interviewer, length, review - with every option fetched from the catalogue endpoints; nothing in
the frontend knows a discipline's name either. The URL carries the step and every choice, so a
copied or reloaded link restores exactly where the person was; each step validates independently
through the same pure rules the tests pin, an invalid advance names the problem and moves focus
to it, and a change that invalidates later choices trims exactly those and nothing else. A server
refusal returns to the offending field's own step, focused, with everything entered intact.

The screening refusal is strongest at the server: the contract's mode enum names practice alone,
the handler enforces it (the generated decoder does not), and the test proves a screening request
never reaches the port. Creation is the whole path: the selection is validated against the
catalogue in the adapter - CAT-03's enforcement point - stored immutably on the session (config
column, trigger-guarded, proven by attacking it), the session moves straight to composing, and
the worker turns the session_created event into the composition workflow exactly-once, the same
outbox-to-workflow shape extraction uses. Five per-shape plan artifacts shipped in git give the
composer something real to pin for every shape the catalogue offers.

What the wizard deliberately does not yet have: the prototype's job-description paste and focus
picker (they join when composition consumes them) and the difficulty option (the profile's
seniority default covers it until then). The prepare screen it hands off to is SES-03.

**Spec** [practice-mode.md](../../product/practice-mode.md)

---

### CAT-05 · Collect recording preference and practice consent at composition

**Depends on** CAT-04 · **Blocks** SES-03

*Implemented in the prototype; carry it into production.* The candidate chooses what is kept — audio and
transcript, or transcript only — and understands what each choice costs them.

**Done when**
- [ ] The recording preference is stored on the session and honoured by RTC-05.
- [x] Choosing transcript-only visibly forfeits replay and delivery measurement for that session.
- [x] The preference is versioned with the consent text presented alongside it.

**Stored, versioned, refused when stale; the honouring is RTC-05's.** The consent text itself is
a registry artifact - consent/practice-recording, the versioning with the strongest requirement
in the product, because a session stores the version it presented and that pointer must resolve
to identical words forever. The loader runs the interview context's own parse as its validating
step, and the parse refuses a text whose transcript-only choice names no forfeit: an unnamed cost
is one nobody agreed to. The wizard's review step shows the statements and both choices from the
served document (GET /interviews/practice-consent), audio pre-checked as the prototype has it,
and choosing transcript-only surfaces "you are choosing to lose" with replay and delivery
measurement by name, at the moment of choosing.

Creation echoes the version back and the adapter refuses a stale one (CONSENT_STALE, field
error) - nobody consents to words they were not shown; the wizard refetches and re-presents. The
preference and version land on the session under the same immutability trigger as config, proven
by attacking it, with transcript_only as the schema's legacy default because data-minimising is
the only defensible reading of a silence. The first box stays half-open honestly: the row is
what RTC-05 reads when media capture exists, and nothing captures anything today.

**Spec** [practice-mode.md](../../product/practice-mode.md) · [retention-and-deletion.md](../../security/retention-and-deletion.md)

---

### CAT-06 · Build the content authoring and publication-approval surface

**Depends on** CAT-01, IAM-04 · **Blocks** nothing

*Gap found against the prototype: the content-author and publication-approver roles exist in the
authorization model, but no screen lets anyone author, test or approve an interview artifact.*

**Done when**
- [ ] A content author can draft a blueprint, question set or persona and test it against fixtures.
- [ ] A separate approver publishes; the author cannot approve their own artifact.
- [ ] Publication records approver, time, digest and the artifacts it supersedes.

**Spec** [authorization-model.md](../../architecture/authorization-model.md) · [domain-model.md](../../architecture/domain-model.md)
