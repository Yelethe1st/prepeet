-- 0005: rate limit counters.
--
-- SEC-10 needs counting that survives more than one instance. ADR-0001 runs
-- ECS Fargate, where more than one task is the normal shape for availability,
-- so an in-memory counter is not a smaller version of this: it is wrong, and an
-- attacker gets the limit multiplied by the task count.
--
-- PostgreSQL rather than Redis, and not only to avoid a dependency. The counter
-- lives in the same store as the credentials it protects, which removes a
-- choice that would otherwise have to be made: with a separate store, that
-- store can be down while the database is up, and someone has to decide whether
-- to lock every user out or let every attacker through. Here there is no such
-- state. If this database is unreachable, authentication cannot happen at all,
-- so the limiter failing open costs nothing that is not already lost.
--
-- UNLOGGED deliberately. These rows are ephemeral by nature and writing them to
-- the write-ahead log would double the cost of the hottest write in the system
-- for no benefit. The consequence is that they do not survive a crash or a
-- replica promotion, which means an attacker gets a fresh window after one.
-- That is a real weakening and a small one: it requires them to have crashed
-- the database first.
CREATE UNLOGGED TABLE security_rate_limit_counters (
    -- What is being counted: an address, a network, or anything else the caller
    -- chooses. Opaque here, because this table must never be able to tell a
    -- registered address from an unknown one.
    key          text        NOT NULL,
    -- The window this count belongs to, truncated by the caller. A fixed window
    -- rather than a sliding one: sliding costs a timestamp list per key, and
    -- these limits guard against thousands of attempts rather than against six
    -- instead of five.
    window_start timestamptz NOT NULL,
    count        integer     NOT NULL DEFAULT 0,
    PRIMARY KEY (key, window_start)
);

COMMENT ON TABLE security_rate_limit_counters IS
    'Ephemeral counters for SEC-10. Unlogged, so they do not survive a crash, '
    'and swept rather than retained. Carries no tenant_id: an attacker does not '
    'belong to a tenant, and counting them per tenant would let one tenant''s '
    'traffic exhaust another''s allowance.';

-- The sweep's query. Old windows are deleted rather than retained, because a
-- counter nobody will read again is only a row somebody has to store, and these
-- keys are email addresses and network addresses, which is personal data.
CREATE INDEX rate_limit_window_idx ON security_rate_limit_counters (window_start);

GRANT SELECT, INSERT, UPDATE, DELETE ON security_rate_limit_counters TO prepeet_app;
