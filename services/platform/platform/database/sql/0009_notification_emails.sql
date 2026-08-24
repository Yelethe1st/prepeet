-- 0009: the transactional email queue.
--
-- Emails are enqueued in the same transaction as the state change that wants
-- them sent, for the outbox's reason: a verification token whose email commit
-- fails independently of the token is a token nobody can ever use, and an
-- email sent before its transaction commits offers a link to something that
-- may not exist. The worker drains this table and speaks SMTP.
--
-- Implements part of INT-01.

CREATE SCHEMA IF NOT EXISTS notification;

GRANT USAGE ON SCHEMA notification TO prepeet_app, prepeet_readonly;

CREATE TABLE notification.emails (
    -- UUIDv7, so draining in id order is draining in enqueue order.
    id               uuid        PRIMARY KEY,

    recipient        citext      NOT NULL,

    -- Which template, at which version, produced the content. Recorded so
    -- that "what did we send this person" is answerable after the template
    -- changes, which is the reason templates carry versions at all.
    template         text        NOT NULL,
    template_version text        NOT NULL,

    -- Rendered at enqueue time, against the version recorded above, so the
    -- send path applies no logic and a template edit cannot change a message
    -- that was already promised. Nulled once sent: a verification link is a
    -- secret, and a sent secret has no reason to stay readable at rest.
    subject          text,
    body             text,

    created_at       timestamptz NOT NULL DEFAULT now(),

    -- Delivery state, in the outbox's shape: a future next_attempt_at rather
    -- than a sleep, so a worker restart does not reset the backoff.
    sent_at          timestamptz,
    attempts         integer     NOT NULL DEFAULT 0,
    next_attempt_at  timestamptz NOT NULL DEFAULT now(),
    last_error       text,
    dead_at          timestamptz,

    -- Provider feedback. The columns exist now so status has one home; the
    -- ingestion path (webhook or mailbox poll) lands with the provider
    -- decision and is recorded as outstanding in INT-01.
    provider_message_id text,
    bounced_at       timestamptz,
    complained_at    timestamptz
);

COMMENT ON TABLE notification.emails IS
    'Transactional email, enqueued in the same transaction as the state change '
    'that wants it sent and drained by cmd/worker. Nothing else reads or '
    'writes here.';

COMMENT ON COLUMN notification.emails.body IS
    'Rendered at enqueue, nulled after send. Never carries transcript or '
    'evaluation content: templates accept only their declared typed variables, '
    'and a test refuses a template with an undeclared one.';

-- The sender's query: unsent, not dead, due now, oldest first.
CREATE INDEX emails_pending_idx
    ON notification.emails (next_attempt_at, id)
    WHERE sent_at IS NULL AND dead_at IS NULL;

-- Operators looking for what could not be delivered.
CREATE INDEX emails_dead_idx ON notification.emails (dead_at) WHERE dead_at IS NOT NULL;

-- Row-level security is deliberately not enabled, for the outbox's reason:
-- rows are written inside a caller's scoped transaction but drained by a
-- worker acting for no tenant, and the bypass role that would let a policy
-- coexist with the drain is worse than the policy is good. What replaces it:
-- only the sender reads this table, and the readonly role sees status columns
-- through views it may gain later, not message bodies today.
GRANT SELECT, INSERT, UPDATE ON notification.emails TO prepeet_app;
