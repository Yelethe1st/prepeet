# The durable event contracts

## What this owns

The schema of every fact this system publishes. `envelope.schema.json` is what
every event carries; `payloads/` holds one file per event type.

This directory is the catalogue. There is no index listing which events exist,
because an index is a second place to edit and therefore a place to forget. The
filename is the event type, which also means two files cannot claim the same
event and have the one loaded last quietly win.

Go and TypeScript are generated from here by `tools/eventgen` and never edited.
`docs/contracts/event-catalog.md` is the reasoning and the producer/consumer
map; a test fails if the two disagree in either direction.

## What this must never do

**It never carries restricted content.** No transcript text, no evaluation
prose, no contact details. Events reach integrations and analytics, where the
retention and access rules belong to somebody else, so a field added here is a
field added to their retention policy. `identity.user_registered.v1` deliberately
carries no email address: notification looks the person up.

**It never dumps a row.** Identifiers and the minimum needed to act. A consumer
that needs more fetches it under its own authority, which is also what keeps the
authorisation decision in one place.

**It never carries a result somebody will rank.** `evaluation.completed.v1`
carries no score and no verdict. A number in an event is a number that will be
compared, retained and acted on under rules that are not ours, and once it has
been delivered it cannot be withdrawn.

**A payload field is never removed.** Removing one breaks every consumer reading
it. Stop populating it, or publish a new contract version alongside the old one.
`make check-events` refuses the former against the previous release.

## Versions, and which one means what

Two, and they answer different questions.

The **version in the event type**, as in `evaluation.completed.v1`, is the
contract. It is the only thing a consumer subscribes against, and `.v1` and `.v2`
are different events that may be emitted side by side while a migration runs.

**`x-since`** is the payload's version within that contract. It bumps when a
field is added, and a consumer reads it to decide what it can handle. Adding a
field without bumping it is refused, because an addition a consumer cannot
detect is not an additive change from where they are standing.

## What is provisional

The payloads for contexts that do not exist yet are a first reading of what each
event has to carry, constrained by the rules above. `evaluation`, `media`,
`recruiting` and `privacy` have no implementation to check them against, and
their own tickets will revise them. That revision is additive or it is a new
contract version, exactly as it would be after launch, because a consumer that
subscribed early has the same claim as a consumer that subscribes late.
