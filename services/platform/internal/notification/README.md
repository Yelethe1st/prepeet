# notification — transactional email

## What this owns

The email queue (`notification.emails`), the versioned templates, and the
sender that drains the queue through SMTP. What was sent to whom, in which
version of which wording, and whether it arrived.

## What this must never do

**It never decides that an email is warranted.** The context that owns the
state change enqueues inside its own transaction; this package carries the
message out. A notification module that decides when to notify has quietly
become the owner of everyone else's rules.

**It never sends restricted content.** No transcript text, no evaluation
prose. A template accepts exactly its declared variable struct, rendering
fails on anything undeclared, and adding a field to one of those structs in
`templates.go` is the review point the rule hangs on.

**It never keeps a delivered secret.** A verification link is a secret with a
purpose; the statement that records the send erases the subject and body, and
a test proves the erasure by breaking it.

**It never retries forever.** Five attempts with capped backoff, then the row
dead-letters and an operator can see it. Tighter than the outbox, because the
content expires: a token email that cannot be delivered inside its own expiry
window is not worth delivering.

## Why enqueue takes the caller's transaction

The outbox's reason, applied to mail. A token committed without its email is a
token nobody can ever use; an email sent for a transaction that rolled back
offers a link to something that does not exist. Writing both in one
transaction makes both impossible, and the price is a worker to drain the
queue, which already existed.

## Delivery is at-least-once

A sender that dies between the SMTP conversation and recording it leaves a row
the visibility window re-offers, so a person can receive the same email twice.
That is why every template says its link works once, and why the tokens the
links carry are single-use at the store rather than trusted to arrive once.

## What INT-01 leaves outstanding

Bounce and complaint ingestion. The columns exist (`bounced_at`,
`complained_at`) so status has one home, but filling them needs provider
feedback — a webhook or a return-path mailbox — and that lands with the
production provider decision. Until then, "delivered" means the relay accepted
the message.
