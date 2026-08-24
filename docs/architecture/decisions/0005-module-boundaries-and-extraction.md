# ADR-0005: Go module boundaries, cross-context communication and extraction criteria

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-24  
**Review date:** 2027-02-24  
**Supersedes:** None  
**Superseded by:** None

Implements [DEC-03](../../delivery/tickets/01-decisions-and-adrs.md).

## Context

Three deployables, not a service mesh. The Go control plane holds twelve bounded contexts in one
process, which is the right shape for a product with no traffic yet and the wrong shape to arrive at by
accident later.

The dependency graph is currently clean: no context imports another. That is not a discipline anyone
has been exercising, it is that nothing has needed to yet. The next handler changes it. `GET /me`
returns a user's session, which `identity` owns, alongside the tenants they belong to, which `tenancy`
owns. So the decision is being made before the first cross-context call rather than after the
twentieth, which is the only time it is cheap.

[ADR-0002](0002-postgresql-schema-rls-and-connection-roles.md) already fixed schema-per-module and
table ownership. What is open is how two contexts talk in one process, and what has to be true before
one is pulled out into its own service.

## Decision

### Synchronous reads go through interfaces the consumer declares

**No package under `internal/` imports another package under `internal/`.** Only `cmd/` may see more
than one context.

When a context needs something another one owns, it declares the narrow interface it needs, in its own
package, in its own words. The producing context happens to satisfy it. `cmd/api` wires the two
together.

```go
// in internal/api
type Memberships interface {
    ForUser(ctx context.Context, userID string) ([]Membership, error)
}
```

`internal/tenancy` never learns that `internal/api` exists. The dependency points one way, is visible
at the wiring point rather than buried in an import block, and every consumer gets a fake for free in
tests without a mocking framework.

The property that matters most is what it does to extraction. Pulling `tenancy` into its own service
becomes swapping the implementation passed in `cmd/api` for an RPC client. The consumer does not change,
because it never depended on the producer, only on a shape.

### State changes other contexts care about travel as events

A context that changes its own state and needs others to react publishes an event through the
transactional outbox, in the same transaction as the state change, per
[event-catalog.md](../../contracts/event-catalog.md). It does not call the interested contexts, and it
does not know who they are.

The division is deliberate. A synchronous interface is for a question with an answer the caller needs
now. An event is for a fact the publisher is finished with. Using an event for the first makes a read
asynchronous for no reason; using an interface for the second makes the publisher responsible for its
consumers, which is how a monolith gets its reputation.

### `platform/` is infrastructure and never depends on a context

`platform/*` holds adapters: database, object storage, HTTP, identifiers, passwords, tokens,
authorization. A package there may import another `platform` package, and may never import anything
under `internal/`. Infrastructure that knows about a bounded context has stopped being infrastructure.

### The rules are enforced by a test, not by review

`internal/architecture` walks the module's own import graph and fails the build on a violation. A test
rather than a separate linter or a new tool: it runs with `go test ./...`, it is already in CI, it
needs no dependency, and a rule that requires an extra command is a rule someone eventually stops
running.

The rules it enforces:

| Rule | Reason |
|---|---|
| No `internal/x` imports `internal/y` | The boundary this ADR exists for |
| No `platform/*` imports `internal/*` | Infrastructure must not depend on a context |
| No AWS SDK outside `platform/*` | Promised in [ADR-0001](0001-hosting-platform-and-regional-topology.md), which relies on it to keep the cloud reversible |

### Extraction is a decision with evidence, never a tidying

A context is extracted into its own deployable only when all of the following hold, and extraction
requires its own ADR recording them:

- the module API has been stable long enough that a network boundary would not immediately churn;
- per-module telemetry exists, so the claim that it needs separate scaling is measured rather than felt;
- measured load or cost shows an independent scaling, availability, security or release need;
- an owner is named who will carry the operational cost;
- a data migration and consistency plan exists, including what happens to a transaction that currently
  spans both sides.

Tidiness, team preference and the belief that services are more modern than packages are explicitly not
criteria. The modular monolith is the target state until evidence says otherwise, per the architecture
brief.

## Alternatives considered

**Direct imports between contexts.** Simplest, no ceremony, and what most teams do. Rejected because
the cost is deferred rather than avoided: the graph becomes a web, ownership erodes because anything can
reach anything, and extraction later means finding every call site. It is the alternative to revisit if
the interface ceremony proves to outweigh the benefit, and the trigger would be interfaces that exist
only to satisfy the rule rather than to describe a need.

**A designated public surface per context, such as an `api.go` other contexts may import.** Softer and
easier to live with. Rejected because it is much harder to enforce mechanically: a checker can tell
whether one package imports another, and cannot easily tell whether the thing imported was meant to be
public.

**Events for everything.** Rejected as architecture for its own sake. Listing the tenants a person
belongs to is a question with an answer, and making it asynchronous would complicate the caller without
decoupling anything real.

**A dedicated architecture linter such as go-arch-lint or depguard.** More expressive than a test and
another dependency, another configuration format and another command someone has to remember. The rules
here are simple enough that fifty lines of Go expresses them exactly, and the test runs whether or not
anyone remembers it.

## Consequences

Positive. The dependency graph stays a graph rather than a web. Extraction becomes a swap at one wiring
point. Every consumer has a test double without a mocking library. A violation is a build failure with
the offending import named.

Negative, and it is real: ceremony. Twelve contexts with genuine relationships means many small
interfaces and a growing wiring function in `cmd/api`. On a small team that will feel like paperwork
before it feels like architecture, and the honest answer is that it is paperwork until the first
extraction or the first time someone has to reason about who owns what.

Operational. `cmd/api` becomes the one place that knows the whole shape of the system. That is a
readable thing to have and a large file to maintain, and it should be composed of small functions per
context rather than one long body.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Interfaces multiply until they describe nothing | Interfaces are declared by the consumer in its own words, so an interface with one method used once is correct rather than a smell |
| The rule is worked around through `platform/` | `platform/*` may not import `internal/*`, enforced by the same test |
| Wiring in `cmd/api` becomes unreadable | Composed per context, and reviewed as the place the architecture is visible rather than as glue |
| Extraction happens for taste | Extraction requires its own ADR carrying the five pieces of evidence above |
| A cyclic need appears between two contexts | A cycle is a signal the boundary is wrong. The response is to move the shared concept, not to add a back edge |

## Reversibility and migration

Cheap to relax, expensive to impose later. Dropping to direct imports is deleting a test. Adopting this
rule after a year of direct imports means untangling every cross-context call under time pressure,
which is the situation this ADR exists to avoid. That asymmetry is the whole argument for deciding now,
while the graph is still clean.

## Validation

- A test fails the build when one context imports another.
- A test fails the build when `platform/*` imports `internal/*`.
- A test fails the build when the AWS SDK appears outside `platform/*`.
- The enforcement is verified by introducing a violation deliberately and confirming the failure names it.
- Every extraction carries its own ADR with measured evidence, not an assertion of need.
