# ADR-0004: REST, RPC, event and generated-contract conventions

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-24  
**Review date:** 2027-02-24  
**Supersedes:** None  
**Superseded by:** None

Implements [DEC-08](../../delivery/tickets/01-decisions-and-adrs.md).

## Context

Three languages implement one system. The browser talks REST to Go, Go talks Protobuf to Python, and
durable facts travel as events to integrations, analytics and audit. Without agreed conventions each
surface acquires its own error shape, its own pagination and its own idea of what a breaking change is,
and the cost of that is paid later by whoever has to reconcile them.

The decision that matters most is not which conventions to adopt. It is **which artifact is the source
of truth**, because that determines whether the web and Go workstreams can proceed in parallel or
whether one waits on the other. [CTR-01](../../delivery/tickets/03-contracts-and-codegen.md) exists
specifically to unblock both at once.

`docs/contracts/` already proposes most of the conventions. This ADR settles them, resolves an ambiguity
about what is generated from what, and fixes the parts the documents leave open: versioning triggers,
idempotency semantics, the deprecation process, and the definition of a breaking change on each surface.

**On DEC-03.** DEC-08 is listed as depending on it. The deployable boundaries are already fixed by the
architecture brief, and [ADR-0002](0002-postgresql-schema-rls-and-connection-roles.md) fixed
schema-per-module ownership, so nothing outstanding in DEC-03 changes a wire contract. DEC-03 still
needs its own ADR for the internal Go import rules that
[PLT-04](../../delivery/tickets/02-platform-foundation.md) enforces; it does not block this one.

## Decision

### The contract is written first, and everything else is generated from it

**Hand-authored, checked into `packages/contracts/`:** the OpenAPI document, the Protobuf definitions,
and the event schemas. **Generated, never edited, and regenerated in CI:** Go server interfaces, the Go
and Python RPC stubs, and the TypeScript client and types.

Spec-first rather than code-first, deliberately.

The usual argument is parallel workstreams, and it is real but not the one that decided this. It is
weak on a small team, because one person cannot write both sides at once anyway.

The argument that carries it is specific to this product. An unusual amount of this API surface is a
compliance obligation rather than an implementation detail: returning `404` where existence is
sensitive, error codes that never change meaning, idempotency that cannot bill an interview twice, and
result disclosure enforced at the API rather than by hiding a link. Legal, security and an external
auditor need to read those in one reviewable artifact before a screening pilot, which
[REL-03](../../delivery/tickets/22-release-readiness.md) requires. A contract inferred from Go
annotations is not that artifact, and retro-writing one at the moment the gate demands it is the worst
available time to start.

Two further properties follow: the Go handler implements a generated interface, so drift is a compile
error rather than a lint warning, and a breaking change is detectable before any code exists to break
it. Generating OpenAPI from annotations gives neither, because a contract derived from code can only
report a break after it has happened.

This resolves an ambiguity in [public-api.md](../../contracts/public-api.md), whose status line reads
"generated OpenAPI is authoritative after implementation". The authoritative artifact is the
hand-authored OpenAPI document in `packages/contracts/api/`. The document in `docs/contracts/` is the
capability inventory and the reasoning behind it, which is a different thing and remains useful.

| Surface | Source | Generated | Tooling |
|---|---|---|---|
| Browser to Go | OpenAPI 3.1 | Go server interface, TypeScript client and types | `oapi-codegen`, `openapi-typescript` with `openapi-fetch` |
| Go to Python | Protobuf | Go and Python stubs | `buf` |
| Durable events | JSON Schema | Go and TypeScript types | `buf` for envelope, JSON Schema for payloads |

Linting is Spectral against a checked-in ruleset for OpenAPI and `buf lint` for Protobuf. Breaking-change
detection is `oasdiff` and `buf breaking`, both against the previous release rather than against the
previous commit, so a change can be revised in progress without tripping the gate.

### REST

`/api/v1` in the path. The version changes only for a break that cannot be expressed additively, and
that is expected to be rare enough that a v2 is a project rather than a release. Versioning in the path
rather than a header because it is visible in a log, a support ticket and a curl command, and because
header-based versioning is routinely lost by proxies and caches.

**One error envelope**, already implemented in `platform/httpserver`:

```json
{"error": {"code": "SESSION_INVALID_STATE", "message": "...", "retryable": false,
           "field_errors": [], "request_id": "req_..."}}
```

Codes are `SCREAMING_SNAKE`, stable forever, and never reused for a different meaning. They are
generated into both clients as an enum, so a client can handle them exhaustively and a new code is a
compile-time prompt rather than a runtime surprise. Messages are for humans, may be reworded at any
time, and are never machine logic.

**Status codes** are used narrowly, because a status that could mean three things means none of them.
`400` for validation, with `field_errors` carrying the detail; there is no `422`. `401` for
unauthenticated, `403` for authenticated but not permitted, and `404` where revealing existence would
itself leak, which is the rule that keeps invitation and candidate lookups from confirming an address.
`409` for a state conflict or an idempotency mismatch, `410` for something that existed and is now
permanently gone, such as a consumed invitation. `429` with `Retry-After`. Anything that takes longer
than a request returns `202` with a status resource rather than holding the connection.

**Idempotency.** `Idempotency-Key` on every retryable mutation, scoped to the tuple of tenant, endpoint
and key. The first request stores the key with a digest of the request body and the response it
produced. A replay carrying the same digest returns the stored response; a replay carrying a different
digest is `409`, because two different requests under one key is a client bug that silently succeeding
would hide. Keys are retained 24 hours, which is longer than any client retry policy should need.

**Pagination** is cursor based, never offset. The cursor is opaque, and the sort always includes the
resource identifier as a tiebreaker, without which a cursor over equal sort keys silently skips or
repeats rows. Page size has a default and a maximum, and a request over the maximum is clamped rather
than refused.

**Deprecation.** A deprecated operation returns the `Deprecation` and `Sunset` headers from RFC 8594,
and stays available for at least ninety days after the announcement. Removal without a sunset date does
not happen, including for operations we believe nobody uses, because we cannot see what a tenant's
integration calls.

### RPC

Protobuf over gRPC, `buf` for lint, generation and breaking checks. Fields are added, never renumbered;
removed fields are reserved so a number cannot be reused with a different meaning. Enums always carry an
explicit zero value meaning unspecified, and consumers tolerate values they do not recognise, which is
what allows a new enum member to ship without coordinating a release.

Every request carries schema version, idempotency identifier, tenant and purpose, input references and
digests, deadline, and correlation. Every response carries its result and the versions that produced it,
its assessability, its usage and latency, and a typed failure category. This is already implemented in
`prepeet_ai.transport.envelope`, which refuses an unversioned result and pairs each failure code with a
retry decision, so Go never parses a message string to decide whether to try again.

Large inputs travel as short-lived, purpose-scoped object references rather than embedded bytes.
Retries happen in the durable workflow layer where idempotency is enforced, and gRPC middleware does not
retry on its own, because a blind retry of a non-idempotent operation is how one interview becomes two
evaluations and two charges.

### Events

The envelope in [event-catalog.md](../../contracts/event-catalog.md) stands. Two things it leaves
ambiguous are settled here.

**The version in the event type is the contract version, and it is the only one consumers subscribe
against.** `evaluation.completed.v1` and `evaluation.completed.v2` are different events that may be
emitted side by side during a migration. The `schema_version` field describes the payload's evolution
within that contract and bumps for additive change. A consumer keys its handler on the event type and
tolerates a higher `schema_version`.

**Only the context owning the authoritative state emits its event**, and it emits through the
transactional outbox in the same transaction as the state change. An event published outside that
transaction is a fact that may not have happened.

Payloads carry identifiers and the minimum needed to act, never a row dump. Restricted content, meaning
transcript text, evaluation prose and candidate contact details, never appears in an event payload,
because events reach integrations and analytics where the retention and access rules are somebody
else's.

### What counts as a breaking change

| Surface | Breaking |
|---|---|
| REST | Removing or renaming a field, narrowing a type, adding a required request field, removing an enum value, changing a status code for the same condition, changing what an error code means |
| RPC | Renumbering or reusing a field number, removing a field without reserving it, changing a type, removing an RPC |
| Events | Removing a payload field, changing a field's meaning, removing an event type without a successor |

Adding an optional field, adding an enum value where consumers tolerate unknowns, and adding an
operation are all additive. The gates enforce this mechanically; the table exists so a reviewer and the
tool agree about what they are looking at.

## Alternatives considered

**Code-first, generating OpenAPI from Go.** Faster for whoever is writing the handler, and it inverts
the dependency so the web workstream waits. It also weakens the compatibility gate into a report of
what already changed. Rejected for those two reasons rather than on taste.

**Connect for the browser too.** One schema language across both boundaries instead of two toolchains,
with better end-to-end type safety. Genuinely attractive, and rejected on who else reads this surface
rather than on developer experience.

An earlier draft of this ADR rejected Connect on the grounds that REST is curl-able and Connect is not.
That was wrong: the Connect protocol speaks JSON over plain HTTP, so the two are equally inspectable,
and the argument is recorded here as mistaken rather than quietly removed.

What stands is the audience. Tenant integrators, ATS adapters and auditors are all handed this surface,
webhooks already leave as JSON under [webhook-protocol.md](../../contracts/webhook-protocol.md), and any
eventual public API for tenants wants OpenAPI regardless. One toolchain would be better for the team
building it and worse for the people receiving it, and the second group cannot be renegotiated with.

**GraphQL for the browser.** Rejected in the architecture brief already, and this ADR does not reopen
it. The client surfaces are known and stable, and the authorization model is capability and scope based,
which is awkward to enforce over an arbitrary query graph.

**No version in the path, relying on additive evolution forever.** Honest about how rarely a v2 should
happen, and leaves no escape hatch when one is genuinely needed. The path segment costs nothing.

## Consequences

Positive. The web and Go workstreams start together rather than in sequence. Drift between contract and
implementation becomes a compile error. A breaking change is caught by a tool before review rather than
by a tenant after release.

Negative. Authoring OpenAPI by hand is slower than annotating handlers, and the team has to become
fluent in it. Generated code appears in review diffs, which is noise, and mitigating that by not
committing generated code trades the noise for a build-time dependency. Generated code is committed
here, and CI regenerates and fails on a diff, which keeps the repository self-contained.

Operational. Four generators and three linters are now build dependencies, and their versions are
pinned. An unpinned generator producing different output on two machines is the drift the whole
arrangement exists to prevent.

Security. Error codes being stable and public means they must not leak internal structure. The
`404`-where-existence-is-sensitive rule is a contract obligation rather than a handler's discretion, and
CTR-01 carries it into the OpenAPI document so it can be reviewed.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| The OpenAPI document drifts from the handlers | Handlers implement a generated interface, so drift does not compile |
| Generated code is edited by hand | CI regenerates and fails on a diff |
| A breaking change ships unnoticed | `oasdiff` and `buf breaking` against the last release, in the required gates |
| Error codes multiply without meaning | Codes live in one file, are generated into both clients, and adding one is a reviewed change |
| Idempotency keys collide across tenants | Keys are scoped to tenant, endpoint and key together, never to the key alone |
| An event payload carries restricted content | Payload schemas are reviewed against `data-classification.md`, and the telemetry scanner in SEC-08 covers the outbox |

## Reversibility and migration

Cheap to reverse in principle and increasingly expensive in practice. Moving to code-first later means
abandoning the generated server interfaces and accepting that the contract follows the code, which is a
one-way door in culture more than in tooling. The path version means a v2 is available without
rearchitecting. Swapping a generator is a day's work as long as the source artifacts stay hand-authored,
which is the property worth protecting.

## Validation

- The build fails if generated code differs from what the contracts produce.
- The build fails on a breaking change to REST, RPC or events without an explicit version bump.
- A handler that does not satisfy the generated interface does not compile.
- Every error code in the OpenAPI document appears in the generated client enum, and vice versa.
- A replayed idempotent request with a different body returns `409` rather than acting twice.
- A cursor over rows with equal sort keys neither skips nor repeats, proven by test.
- No event payload contains restricted content, checked against the data classification inventory.
