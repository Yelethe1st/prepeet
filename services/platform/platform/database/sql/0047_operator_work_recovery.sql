-- 0047: the operator's two decisions about failed work, and the index the
-- backlog alert is measured through.
--
-- OPS-03. An event that has exhausted its delivery attempts is dead lettered
-- and waits for a person. That person has exactly two answers: try it again,
-- or decide it must never be delivered. The first already has a home, because
-- dead_at going back to NULL is a state this table understands. The second did
-- not, and the obvious implementation of it is a bug.
--
-- Discarding by DELETE was rejected, twice over.
--
-- The reason that applies to this table is evidence. A deleted row cannot be
-- shown to whoever asks later what happened to that notification, and "it is
-- not in the table" is indistinguishable from "it was never written". The
-- application role holds DELETE here from migration 0004 and this table
-- deliberately has no row-level security, so the delete would succeed and take
-- the evidence with it.
--
-- The second reason is the habit rather than this table. Elsewhere in this
-- schema every table forces row-level security, and under FORCE a DELETE with
-- no matching policy affects zero rows and raises nothing: the caller sees
-- success and the row stays where it was. This project has been bitten by
-- exactly that. An explicit state transition cannot fail either way, because
-- the UPDATE returns the row it changed and a transition that did not happen
-- returns nothing, which is a result the caller has to handle rather than a
-- silence it can miss. Writing the release the same way everywhere is what
-- stops the safe version being the one somebody remembered.
--
-- Implements part of OPS-03.

ALTER TABLE integration.outbox
    ADD COLUMN discarded_at    timestamptz,
    ADD COLUMN discard_reason  text;

COMMENT ON COLUMN integration.outbox.discarded_at IS
    'Set when an operator decided this event must never be delivered. Terminal '
    'and deliberate, unlike dead_at, which the dispatcher sets when attempts '
    'run out. The row stays so the decision remains answerable.';

COMMENT ON COLUMN integration.outbox.discard_reason IS
    'Why an operator discarded it. Required by the application, because a '
    'discard without a reason is an audit row nobody can review.';

-- A discarded event is out of the dispatcher's sight independently of dead_at.
--
-- Today discarding requires an already dead-lettered row, so dead_at alone
-- would keep it out of the claim. That is a property of the current rules
-- rather than of the table, and the day discarding a still-retrying poison
-- event becomes allowed, the claim would silently pick it up again. Building
-- the index on the terminal column keeps the two facts from drifting apart.
DROP INDEX integration.outbox_pending_idx;

CREATE INDEX outbox_pending_idx
    ON integration.outbox (next_attempt_at, id)
    WHERE published_at IS NULL AND dead_at IS NULL AND discarded_at IS NULL;

-- The alert's own index.
--
-- The backlog is measured every few seconds, forever, and the outbox keeps
-- delivered rows: a count and a min over the whole table would be a sequential
-- scan that gets slower exactly as the table gets more valuable. Ordered by
-- occurred_at rather than by next_attempt_at because the age that matters is
-- how long the fact has been waiting, which is the wait a candidate feels, not
-- when delivery is next due.
CREATE INDEX outbox_backlog_age_idx
    ON integration.outbox (occurred_at)
    WHERE published_at IS NULL AND dead_at IS NULL AND discarded_at IS NULL;

-- No row-level security is added here, and that is not an omission.
--
-- integration.outbox deliberately has none, for the reason migration 0004
-- states: it is read by a dispatcher acting for no tenant, which a tenant
-- policy would blind, and the usual workaround is a role that bypasses every
-- other policy too. It is the single exemption in the forced row-level security
-- test, and these columns are delivery state rather than tenant data, so the
-- exemption's reasoning covers them unchanged.
--
-- The update grant already exists from 0004. The application role needs no new
-- privilege to make either transition, which is what keeps this migration to
-- columns and indexes.
