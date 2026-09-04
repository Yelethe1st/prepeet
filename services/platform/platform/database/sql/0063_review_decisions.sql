-- 0063: the hiring decision, a named human's and append-only.
--
-- REV-03 and responsible-hiring.md. Nothing in this product advances or
-- rejects a candidate automatically: a decision row names the person who
-- decided and why, both NOT NULL with no default, so the schema refuses an
-- outcome that cannot say whose it was. The evidence version the decision
-- was informed by rides the row - the evaluation's identity and digests as
-- they stood at the moment of deciding - so a later re-evaluation can never
-- quietly become the basis a past decision is read against.
--
-- Append-only by trigger, not by convention. A decision that changes is a
-- new row; the history keeps every earlier one with its true actor, which
-- is what an appeal reads. Overrides are part of the row: where the
-- reviewer disagreed with an assessed band, the band they disagreed with
-- and their rationale are recorded together, because a disagreement
-- without its reasoning is indistinguishable from a whim.
--
-- Implements REV-03.

CREATE TABLE recruiting.review_decision (
    id           uuid        PRIMARY KEY,
    campaign_id  uuid        NOT NULL REFERENCES recruiting.campaign (id),
    tenant_id    uuid        NOT NULL,

    -- The session decided on. A soft reference: interview.sessions is
    -- another context's, and cmd verified the session belongs to the
    -- campaign before this row could be written.
    session_id   uuid        NOT NULL,

    -- The person and the outcome. decided_by references identity.users so
    -- a decision can never name a person who does not exist, and the
    -- decision vocabulary is the event catalogue's, verbatim.
    decided_by   uuid        NOT NULL REFERENCES identity.users (id),
    decision     text        NOT NULL CHECK (decision IN ('advance', 'reject', 'hold')),
    reason       text        NOT NULL CHECK (length(trim(reason)) > 0),

    -- The evidence version this decision was informed by, captured
    -- server-side at decision time: the stored result's identity, its
    -- digest, and the rubric's.
    evaluation_id uuid       NOT NULL,
    result_digest text       NOT NULL CHECK (result_digest <> ''),
    rubric_digest text       NOT NULL CHECK (rubric_digest <> ''),

    -- Where the reviewer disagreed with an assessed band: a list of
    -- {competency_id, recorded_band, override_band, rationale}, each
    -- rationale required and each recorded_band captured from the stored
    -- result rather than the request. Validated in code; stored whole so
    -- the disagreement reads exactly as it was made.
    overrides    jsonb       NOT NULL DEFAULT '[]'::jsonb,

    created_at   timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE recruiting.review_decision IS
    'A named human''s hiring decision on one screening session, with the '
    'evidence version it was informed by. Append-only: changing a decision '
    'is a new row, and the history keeps every earlier one.';

CREATE INDEX review_decision_session
    ON recruiting.review_decision (session_id, created_at);

-- Append-only, enforced where conventions cannot reach: an UPDATE or
-- DELETE is refused whoever asks, because a decision history that can be
-- rewritten is not a history.
CREATE FUNCTION recruiting.refuse_decision_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'review decisions are append-only: a change is a new decision (REV-03)'
        USING ERRCODE = 'raise_exception';
END $$;

CREATE TRIGGER review_decision_append_only
    BEFORE UPDATE OR DELETE ON recruiting.review_decision
    FOR EACH ROW EXECUTE FUNCTION recruiting.refuse_decision_mutation();

ALTER TABLE recruiting.review_decision ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.review_decision FORCE ROW LEVEL SECURITY;

-- The tenant's, for the reviewers who record and read decisions. The
-- candidate does not read this table: what a candidate learns of an
-- outcome is the disclosure policy's to decide (SCR-07), never a table
-- read.
CREATE POLICY review_decision_tenant ON recruiting.review_decision
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
