-- 0055: accommodation requests, decisions and fulfilments.
--
-- SCR-06. Three append-only tables, one per fact the ticket says must never
-- be rewritable: what was requested, what was granted and by whom, and which
-- session it was actually applied to. The state a candidate sees is derived
-- from these rows at read time, never stored, so it cannot disagree with
-- them. Nothing here is ever UPDATEd; even attaching a request to its
-- session is an insert into the fulfilment table rather than an edit of the
-- request.
--
-- What is deliberately absent matters as much as what is here. The request
-- carries no free-text column, because a "reason" box on an accommodation
-- form is where a medical condition gets asked for whether or not anybody
-- meant to ask; the request is for a named adjustment from screen-mode.md's
-- list, not a diagnosis. And nothing in the evaluation schema references any
-- of this: an accommodation must never reach evaluation as a signal, which
-- is enforced by evaluation being unable to name the recruiting schema at
-- all (internal/architecture's ownership gate), not by a flag it promises
-- not to read.
--
-- Implements SCR-06.

-- A campaign fixes one accommodation policy the way it fixes one rubric:
-- as a registry artifact pinned by digest. 0043's pin table already grows by
-- artifact type, so admitting the type is all the schema change there is.
ALTER TABLE content.artifacts
    DROP CONSTRAINT artifacts_artifact_type_check;

ALTER TABLE content.artifacts
    ADD CONSTRAINT artifacts_artifact_type_check CHECK (artifact_type IN (
        'persona', 'plan', 'rule_pack', 'rubric', 'role_standard',
        'prompt', 'model_policy', 'articulation_policy', 'catalogue',
        'consent_text', 'screening_disclosure', 'accommodation_policy'));

-- One candidate asking for one named adjustment on one campaign.
--
-- session_id is not here on purpose: a request is admitted before any
-- session exists, and which session an adjustment was applied to is the
-- fulfilment's fact, recorded there when it happens. Keeping it out of this
-- table is what lets this table refuse every UPDATE.
CREATE TABLE recruiting.accommodation_request (
    id           uuid        PRIMARY KEY,
    tenant_id    uuid        NOT NULL,
    campaign_id  uuid        NOT NULL REFERENCES recruiting.campaign (id),
    candidate_id uuid        NOT NULL,

    -- screen-mode.md's vocabulary, verbatim. An enum-shaped CHECK rather
    -- than free text, so the column cannot drift into collecting the
    -- explanation the design refuses to ask for.
    adjustment   text        NOT NULL CHECK (adjustment IN (
                                 'captions', 'push_to_talk',
                                 'extra_time', 'alternative_path')),

    requested_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE recruiting.accommodation_request IS
    'A candidate asking for one named adjustment on one campaign. Append-only '
    'and deliberately without any free-text column: the request names an '
    'adjustment, never a condition or a diagnosis. Evaluation cannot read '
    'this table, or any table in this schema, which is how "never a scoring '
    'signal" is enforced.';

CREATE INDEX accommodation_request_by_candidate
    ON recruiting.accommodation_request (campaign_id, candidate_id, requested_at DESC);

CREATE OR REPLACE FUNCTION recruiting.refuse_accommodation_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'recruiting accommodation records are append-only: a change is a new row';
END $$;

CREATE TRIGGER accommodation_request_append_only
    BEFORE UPDATE OR DELETE ON recruiting.accommodation_request
    FOR EACH ROW EXECUTE FUNCTION recruiting.refuse_accommodation_change();

ALTER TABLE recruiting.accommodation_request ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.accommodation_request FORCE ROW LEVEL SECURITY;

CREATE POLICY accommodation_request_tenant ON recruiting.accommodation_request
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON recruiting.accommodation_request TO prepeet_app;

-- One human's answer to one request.
--
-- Append-only, latest row standing, exactly as consent decisions work: a
-- grant later withdrawn or a decline later reversed is a new row, and "what
-- had been decided when the interview ran" stays answerable forever.
-- decided_by is NOT NULL with no default for the reason the jurisdiction
-- determination's approver is: a decision without a named human is not a
-- decision, and responsible-hiring.md puts a human at the end of every path
-- that affects a candidate.
CREATE TABLE recruiting.accommodation_decision (
    id         uuid        PRIMARY KEY,
    tenant_id  uuid        NOT NULL,
    request_id uuid        NOT NULL REFERENCES recruiting.accommodation_request (id),

    granted    boolean     NOT NULL,
    decided_by uuid        NOT NULL,
    decided_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE recruiting.accommodation_decision IS
    'One human''s answer to one accommodation request. Append-only; the '
    'latest row per request is the standing answer. A decision always names '
    'who made it.';

CREATE INDEX accommodation_decision_standing
    ON recruiting.accommodation_decision (request_id, decided_at DESC);

CREATE TRIGGER accommodation_decision_append_only
    BEFORE UPDATE OR DELETE ON recruiting.accommodation_decision
    FOR EACH ROW EXECUTE FUNCTION recruiting.refuse_accommodation_change();

ALTER TABLE recruiting.accommodation_decision ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.accommodation_decision FORCE ROW LEVEL SECURITY;

CREATE POLICY accommodation_decision_tenant ON recruiting.accommodation_decision
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON recruiting.accommodation_decision TO prepeet_app;

-- A granted adjustment actually applied to a named session.
--
-- This row is the difference between an accommodation policy and an
-- accommodated interview, which is what "exercised, not merely promised"
-- means as a schema. session_id is a plain uuid rather than a foreign key
-- into the interview schema, the same treatment candidate_id gets: sessions
-- are another context's rows, and a cross-schema constraint would be the
-- coupling ADR-0005 exists to refuse.
CREATE TABLE recruiting.accommodation_fulfilment (
    id           uuid        PRIMARY KEY,
    tenant_id    uuid        NOT NULL,
    request_id   uuid        NOT NULL REFERENCES recruiting.accommodation_request (id),
    session_id   uuid        NOT NULL,

    fulfilled_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE recruiting.accommodation_fulfilment IS
    'A granted adjustment applied to a named session: the record that an '
    'accommodation was exercised rather than promised. Refused by trigger '
    'unless the standing decision for the request is a grant.';

CREATE INDEX accommodation_fulfilment_by_session
    ON recruiting.accommodation_fulfilment (session_id, fulfilled_at);

-- The database's half of the grant rule; the store enforces the same rule in
-- Go first. Twice, because a rule only in Go is one a future query walks
-- around, and a rule only here produces an error nobody can act on.
--
-- The SELECT inside runs as the inserting role, so row-level security still
-- applies: a decision under another tenant is invisible to it, which means a
-- fulfilment can never be justified by a grant the caller could not see.
CREATE OR REPLACE FUNCTION recruiting.require_standing_grant()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    standing boolean;
BEGIN
    SELECT granted INTO standing
    FROM recruiting.accommodation_decision
    WHERE request_id = NEW.request_id
    ORDER BY decided_at DESC
    LIMIT 1;

    IF standing IS DISTINCT FROM true THEN
        RAISE EXCEPTION
            'recruiting.accommodation_fulfilment requires a standing grant for request %',
            NEW.request_id;
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER accommodation_fulfilment_requires_grant
    BEFORE INSERT ON recruiting.accommodation_fulfilment
    FOR EACH ROW EXECUTE FUNCTION recruiting.require_standing_grant();

CREATE TRIGGER accommodation_fulfilment_append_only
    BEFORE UPDATE OR DELETE ON recruiting.accommodation_fulfilment
    FOR EACH ROW EXECUTE FUNCTION recruiting.refuse_accommodation_change();

ALTER TABLE recruiting.accommodation_fulfilment ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.accommodation_fulfilment FORCE ROW LEVEL SECURITY;

CREATE POLICY accommodation_fulfilment_tenant ON recruiting.accommodation_fulfilment
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON recruiting.accommodation_fulfilment TO prepeet_app;
