# ADR-0006: PostgreSQL serves cache, coordination and rate limiting; Redis is deferred

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-24  
**Review date:** 2027-02-24  
**Supersedes:** None  
**Superseded by:** None

Closes the "Redis need" open decision in
[deployment-topology.md](../../operations/deployment-topology.md).

## Context

Redis appears in the proposed topology, and [data-architecture.md](../../architecture/data-architecture.md)
restricts it to three uses: ephemeral cache, rate limiting, and coordination. Whether it is actually
needed was left open.

The question keeps returning because each of the three uses is individually plausible, and because
"obviously you need Redis" is a strong prior in this kind of system. It deserves an answer that names
what would change it rather than a preference.

The architecture brief already sets the bar: introduce a new service only for a measured independent
scaling, ownership, security, availability or release need.

## Decision

**Redis is not provisioned.** Each of its three proposed uses is served by something already present,
and in two of the three cases the existing answer is better rather than merely cheaper.

### Coordination: `FOR UPDATE SKIP LOCKED`

The outbox dispatcher claims work with `SELECT ... FOR UPDATE SKIP LOCKED`, so more than one dispatcher
can run without delivering the same event twice.

This is the better primitive here, not the affordable one. The lock and the work live in one
transactional scope, so they cannot disagree: with a lock in a separate store, a process can hold the
lock while its database transaction rolls back, and then the lock describes work that no longer exists.
A lock with a lease also cannot be made safe without fencing tokens, which is the substance of the
long-running argument about Redlock and is not academic: a dispatcher paused past its lease wakes up
still believing it holds the lock. `SKIP LOCKED` has no lease. The lock dies with the transaction.

It also scales the right way. A distributed lock or leader election makes one dispatcher do all the work
while the others idle; `SKIP LOCKED` means each additional dispatcher steps over what the others hold,
so adding one adds throughput.

Verified by removing it: four dispatchers claimed the same events, one of them four times.

### Rate limiting: a counter in PostgreSQL

`security_rate_limit_counters`, unlogged, incremented with one atomic
`INSERT ... ON CONFLICT ... RETURNING`.

The latency argument for Redis does not apply on this path. The write costs roughly a millisecond
against the hundred that argon2id already spends on the same request.

The better argument is that the counter shares a store with the credentials it protects, which removes a
decision a separate store would force. Redis can be down while PostgreSQL is up, and somebody would then
have to choose between locking every user out and letting every attacker through. Here that state cannot
exist, because authentication cannot happen without this database either, so the limiter fails open and
reports, and costs nothing that is not already lost.

### Cache: real candidates, and none of them want Redis first

An earlier draft of this ADR said there was nothing to cache. That was wrong, and it is corrected here
rather than quietly removed. There are four candidates, and they split cleanly by whether they need
invalidating.

**Immutable and versioned, so cacheable in process with no invalidation at all.** Published artifacts,
meaning rubrics, calibrations, personas, plans and prompts, are immutable once published under
[ADR-0004](0004-contract-conventions-and-code-generation.md) and pinned by version into every session
bundle. Rubric v4 is rubric v4 forever, so the cache key carries the version and a stale entry cannot
exist. The catalogue of disciplines, roles, shapes and personas behaves the same way.

For this class an in-process cache is not the cheap option, it is the better one. It has no network hop,
no serialisation, and no invalidation problem to get wrong. Redis would add a round trip to data that
never changes. This is the largest and easiest win available, and it needs no new infrastructure.

**Mutable and shared, so needing cross-instance invalidation.** Tenant policy, meaning retention,
disclosure version and feature approval such as whether comparison is enabled. The authorization context
behind every request, meaning capabilities and scopes for a membership. And session lookup.

Here an in-process cache with a time to live is a correctness problem rather than a performance choice.
A revoked membership that stays cached for sixty seconds is sixty seconds of a removed recruiter reading
candidate evidence, which is exactly what [ADR-0003](0003-identity-built-in-go.md) chose revocable
sessions to prevent. Caching these correctly means invalidating across instances, which is the fan-out
problem in the trigger table below, so this class collapses into that trigger rather than standing as
its own reason for Redis.

**Some of it should not be cached at all, but not requested.** The catalogue endpoints are read
repeatedly by a wizard that mostly does not change between steps. `ETag` and `Cache-Control` remove the
request entirely, which beats serving it from anywhere. That is a gap in ADR-0004, which covers
versioning, errors, idempotency and pagination but says nothing about caching, and it is now recorded on
[CTR-01](../../delivery/tickets/03-contracts-and-codegen.md).

**And PostgreSQL is already a cache.** A small hot table lives in `shared_buffers` and the operating
system page cache, so a catalogue read is already a memory read plus a round trip. The marginal win from
Redis for small hot reads is mostly that round trip, and an in-process cache removes it entirely.

What is left for Redis is narrow: shared mutable data, at a volume where an in-process time to live is
too stale to be correct and the database is too loaded to ask. That is a real place to be, and it is not
where this system is.

## What Redis would genuinely bring

Stated fairly, because the case against provisioning it now is not a case against the technology.

**The write pattern of counters is what Redis is built for and what PostgreSQL handles worst.** Many
small updates to few rows is the shape that generates MVCC dead tuples fastest. Every increment leaves a
dead version, and a hot key on a small table can outrun autovacuum, at which point the table bloats and
reads slow. Redis has no equivalent cost.

**Expiry is free.** Redis has native TTL, so a window expires itself. PostgreSQL needs a sweep job,
which is code, a schedule, and a coordination problem of its own.

**Better primitives for better limiting.** `INCR` with `EXPIRE`, or a short Lua script, gives a sliding
window or a token bucket cheaply. Sorted sets make a true sliding window trivial. The fixed window
implemented here permits a burst of up to twice the limit across a window boundary, which is an accepted
trade rather than a desirable property.

**Pub/sub without the ceiling.** `LISTEN/NOTIFY` serialises through a single queue and caps payloads at
8000 bytes. Redis pub/sub does neither.

**It takes load off the primary database, which is the scarce resource.** This is the strongest argument
and the one that will eventually decide it. Every counter increment competes with real product
transactions for connections, write-ahead log bandwidth and vacuum attention. Moving high-frequency,
low-value writes off the primary protects the store that actually matters.

**Horizontal scale.** Redis cluster scales across nodes in a way a single PostgreSQL primary does not.

## When it becomes necessary

Four triggers, each with what to measure. The estimates are arithmetic rather than measurement, and
replacing them with measurement is the point of [PLT-08](../../delivery/tickets/02-platform-foundation.md)
and [OPS-02](../../delivery/tickets/18-platform-operations.md).

| Trigger | Measure | Estimated threshold | Try first |
|---|---|---|---|
| Rate limiting extended beyond authentication to every request | Lock waits on the counter table, dead tuple ratio in `pg_stat_user_tables` | Sustained 1,000 writes per second, or any visible lock wait on a hot key | Limit only what needs limiting; put volumetric protection at the edge, where it belongs |
| Live interview progress fanned out across instances | Notification delivery latency, `NOTIFY` in wait events | A few hundred notifications per second, roughly 500 concurrent interviews | `LISTEN/NOTIFY`, which the outbox dispatcher already uses |
| Session lookup or authorization context dominating latency | Share of p99 attributable to the lookup span, connection pool saturation | Sustained 1,000 requests per second, probably later | PgBouncer first, since pool exhaustion is a pooling problem before it is a caching problem. Then an in-process cache, which needs the fan-out trigger above to invalidate correctly |
| Immutable artifact and catalogue reads costing measurable latency | Share of p99 attributable to artifact loading | Whenever it is measurable, which may be soon | An in-process cache keyed by version, which needs no invalidation and no new infrastructure. `ETag` on the catalogue endpoints, which removes the request |
| Provider concurrency capped across workers | In-flight model calls per provider | When the cap must span work Temporal is not orchestrating | Temporal task queue concurrency, which expresses this directly |

The most likely first trigger is fan-out, not any of the three uses Redis was originally proposed for.

Two caveats on the estimates. At that scale the data layer would be under review generally, so Redis
would be one option beside read replicas and connection pooling rather than the obvious first move. And
a trigger firing is a prompt to measure, not a decision.

## Alternatives considered

**Provision it now, before it is needed.** Avoids a migration under pressure later, and the usual
argument is that adding infrastructure mid-incident is worse than having it. Rejected because it is a
fourth stateful service to provision, secure, monitor, patch and reason about during an incident, for a
product with no traffic. The reversibility analysis below is what makes deferring safe.

**Provision it for rate limiting alone.** The narrowest version, and the one with the weakest case: the
millisecond it saves is invisible next to argon2id, and it reintroduces the availability question that
sharing a store removes.

**Use it for coordination.** Rejected on correctness rather than cost, for the reasons above. This would
be a downgrade even with Redis already running.

## Consequences

Positive. Three stateful services instead of four. One durability story instead of two. Nothing new in
the data classification inventory, the retention schedule or the residency commitment.

Negative, stated plainly. The fixed window permits a burst of up to twice the limit at a boundary.
Counter writes land on the primary database, so they compete with product transactions and will need
watching as volume grows. The sweep is code we own rather than a TTL we configure. And when a trigger
does fire it will fire under load, which is a worse moment to integrate than a quiet one.

Security. Not provisioning Redis keeps rate limit keys, which are email and network addresses and
therefore personal data, inside the store already covered by the residency commitment in
[ADR-0001](0001-hosting-platform-and-regional-topology.md). Adding Redis later means adding it to the
data inventory, the retention schedule and, if managed by a third party, the sub-processor list that
candidate disclosure depends on.

## Reversibility and migration

Deliberately cheap, which is what makes deferring defensible rather than optimistic.

**Rate limiting: hours.** `ratelimit.Counter` is an interface with two implementations, and the
behaviour every counter must satisfy is written once as a shared contract each one runs. A Redis
implementation is a new adapter plus one line to run the existing contract against it, and one changed
wiring line. Nothing in the authentication path knows which counter it holds.

**Fan-out: build it behind an interface when it is built.** No streaming endpoint exists yet, so there
is nothing to migrate. What matters is that whoever builds it puts a publish and subscribe interface in
front of the transport rather than calling `LISTEN/NOTIFY` from the handler. Recorded on the ticket so
it is not discovered later.

**Session caching: not independent.** Correct invalidation needs cross-instance messaging, so it cannot
be done before fan-out exists.

**Operational: about a week, and most of it is not code.** Terraform, subnet and security group,
encryption in transit and at rest, monitoring and a runbook, plus the classification, retention and
residency paperwork above.

[ADR-0005](0005-module-boundaries-and-extraction.md) provides the structural insurance: a Redis client
would live in `platform/`, and the boundary test would stop it leaking into a bounded context the same
way it already stops the AWS SDK.

## Validation

- A Redis implementation of `ratelimit.Counter` passes the existing shared contract without the contract changing.
- The counter table's dead tuple ratio and lock wait time are on a dashboard before authentication carries real traffic.
- Notification latency is measured once a streaming endpoint exists, rather than assumed.
- Any streaming implementation sits behind a publish and subscribe interface, checked at review.
- This ADR is revisited when any trigger above is measured rather than estimated.
