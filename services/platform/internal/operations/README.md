# operations — the work backlog and failed-work recovery

## What this owns

The judgement about durable work that has not been delivered: how deep the
backlog is, how old the oldest item is, whether that is now a candidate
waiting rather than a busy minute, and the two things an operator may do
about work that has failed for good — retry it, or decide it must never be
delivered.

It owns no tables. The work belongs to `platform/outbox`, reached through the
`WorkQueue` port declared here and adapted in `cmd/wiring`; the record of what
an operator did belongs to `audit.events`, the one table every context shares
so that the trail can be read in a single query.

## What this must never do

**It never delivers anything itself.** A retry moves the item back into the
queue and the ordinary dispatcher picks it up, so a retried item travels the
same path as a first attempt: the same handler, the same workflow identity,
the same duplicate rejection. A second delivery path would be a second set of
guarantees to keep in step, and it would be the path nobody tested.

**It never acts without recording who acted.** The transition and its audit
row share one transaction. An action that cannot be audited does not happen,
and an action that was refused is still recorded, because during an incident
a refusal is usually the first sign that two people are working the same
queue.

**It never reads a payload.** An operator decides from what kind of work it
is, whose it is, how long it has been failing and what the failure said.
Nothing here needs the contents, and an operations screen is not a place for
them.
