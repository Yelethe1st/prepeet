-- 0004: the transactional outbox.
--
-- ADR-0005 makes this load bearing rather than convenient. No context imports
-- another, so a state change one context needs to tell others about has exactly
-- one route out: a row written here, in the same transaction as the change
-- itself.
--
-- That transaction is the whole point. An event published outside it is a fact
-- that may not have happened: the process can die between committing the state
-- and reaching the broker, and then the world believes something the database
-- does not. Writing both together makes that impossible, at the cost of needing
-- a dispatcher to carry rows onward afterwards.
--
-- The envelope columns match docs/contracts/event-catalog.md. They are typed
-- columns rather than a JSON blob because every one of them is queried: by
-- tenant for isolation, by type for routing, by occurred_at for ordering.

CREATE TABLE integration.outbox (
    -- UUIDv7, so the primary key sorts by creation time and delivery in id
    -- order is delivery in publication order.
    id             uuid        PRIMARY KEY,

    -- The envelope. See docs/contracts/event-catalog.md.
    event_type     text        NOT NULL,
    schema_version text        NOT NULL,
    tenant_id      uuid,
    occurred_at    timestamptz NOT NULL,
    producer       text        NOT NULL,
    actor_type     text        NOT NULL,
    actor_id       text        NOT NULL,
    purpose        text,
    correlation_id text,
    causation_id   text,

    -- Identifiers and the minimum needed to act, never a row dump. Restricted
    -- content does not appear here: events reach integrations and analytics,
    -- where the retention and access rules belong to somebody else.
    payload        jsonb       NOT NULL DEFAULT '{}'::jsonb,

    -- Delivery state. Separate from the event itself, because the event is a
    -- fact and the delivery is an attempt.
    published_at   timestamptz,
    attempts       integer     NOT NULL DEFAULT 0,
    -- When the dispatcher may next try. Backoff is a future time rather than a
    -- sleep, so a restart does not reset it.
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error     text,
    -- Set when attempts are exhausted. A dead row is visible rather than
    -- silently retried forever, because an event nobody can deliver is an
    -- operational fact somebody needs to see.
    dead_at        timestamptz
);

COMMENT ON TABLE integration.outbox IS
    'Events written in the same transaction as the state change they describe. '
    'The dispatcher in cmd/worker carries them onward. Nothing else writes here.';

COMMENT ON COLUMN integration.outbox.tenant_id IS
    'Nullable: identity events belong to a person rather than a tenant, because '
    'the same person may belong to several. See ADR-0002.';

-- The dispatcher's query: undelivered, not dead, due now, oldest first.
CREATE INDEX outbox_pending_idx
    ON integration.outbox (next_attempt_at, id)
    WHERE published_at IS NULL AND dead_at IS NULL;

-- Operators looking for what failed.
CREATE INDEX outbox_dead_idx ON integration.outbox (dead_at) WHERE dead_at IS NOT NULL;

-- Row-level security is deliberately not enabled here.
--
-- The outbox is written by the application inside a tenant-scoped transaction
-- and read by the dispatcher, which is not acting for any tenant and must see
-- every row. A tenant policy would make the dispatcher see nothing, and the
-- usual fix for that is a role that bypasses row-level security, which is worse
-- than not having the policy: it would be a role able to bypass every other
-- policy too.
--
-- What replaces it is that nothing except the dispatcher reads this table, and
-- payloads carry no restricted content.
GRANT SELECT, INSERT, UPDATE, DELETE ON integration.outbox TO prepeet_app;
GRANT SELECT ON integration.outbox TO prepeet_readonly;
