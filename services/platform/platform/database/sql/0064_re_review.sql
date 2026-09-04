-- 0064: re-review, the appeal against a recorded decision.
--
-- REV-06 and responsible-hiring.md. An appeal is raised against a decision
-- that exists, freezes what that decision was informed by at the moment of
-- raising - the evaluation's identity and digests, and the bundle the
-- session actually ran - and is answered by somebody other than the person
-- whose decision is under appeal. The freeze is the point: a re-review
-- that read newer evidence than the decision did would be reviewing a
-- different question and calling it the same one.
--
-- The row has one one-way life: raised, assigned (reassignable while
-- open), resolved once. The outcome trigger refuses any change to a
-- resolution and any DELETE ever, because an appeal history that can be
-- rewritten is not a safeguard, it is scenery. Whether the re-review right
-- is legal or platform policy per jurisdiction stays DEC-11's; nothing
-- here presumes the answer.
--
-- Implements REV-06.

CREATE TABLE recruiting.re_review (
    id           uuid        PRIMARY KEY,
    campaign_id  uuid        NOT NULL REFERENCES recruiting.campaign (id),
    tenant_id    uuid        NOT NULL,
    session_id   uuid        NOT NULL,

    -- Who asked and why. The requester is a named person: today the
    -- recruiter's queue (REV-07 adds the candidate's own request), and the
    -- reason is required with no default.
    requested_by uuid        NOT NULL REFERENCES identity.users (id),
    reason       text        NOT NULL CHECK (length(trim(reason)) > 0),

    -- The decision under appeal, and the frozen inputs: captured at raise
    -- time from the decision row and the session, never resolved later.
    appealed_decision uuid   NOT NULL REFERENCES recruiting.review_decision (id),
    original_reviewer uuid   NOT NULL REFERENCES identity.users (id),
    evaluation_id     uuid   NOT NULL,
    result_digest     text   NOT NULL CHECK (result_digest <> ''),
    rubric_digest     text   NOT NULL CHECK (rubric_digest <> ''),
    bundle_digest     text   NOT NULL,

    -- Assignment. Nullable until assigned; the independence rule - the
    -- assignee is never the original reviewer - is a CHECK so no code path
    -- can bypass it.
    assigned_to  uuid        REFERENCES identity.users (id),
    CONSTRAINT re_review_independent CHECK (assigned_to IS NULL OR assigned_to <> original_reviewer),

    raised_at    timestamptz NOT NULL DEFAULT now(),
    -- The answer-by time, from the platform's provisional default at raise
    -- (a versioned policy the moment DEC-14's screening policy lands).
    due_at       timestamptz NOT NULL,

    -- The resolution, write-once as a unit: outcome, rationale, the
    -- disclosure permitted to the candidate, who resolved and when are all
    -- present together or all absent.
    outcome           text   CHECK (outcome IN ('upheld', 'revised')),
    outcome_rationale text,
    candidate_disclosure text,
    resolved_by  uuid        REFERENCES identity.users (id),
    resolved_at  timestamptz,
    CONSTRAINT re_review_resolution_whole CHECK (
        (outcome IS NULL) = (outcome_rationale IS NULL)
        AND (outcome IS NULL) = (candidate_disclosure IS NULL)
        AND (outcome IS NULL) = (resolved_by IS NULL)
        AND (outcome IS NULL) = (resolved_at IS NULL)
    ),
    -- The resolver is independent too, by the same rule as assignment.
    CONSTRAINT re_review_resolver_independent CHECK (resolved_by IS NULL OR resolved_by <> original_reviewer)
);

COMMENT ON TABLE recruiting.re_review IS
    'An appeal against one recorded decision, with the evidence and '
    'configuration frozen at the moment it was raised. Assigned to somebody '
    'other than the original reviewer; resolved once; never deleted.';

CREATE INDEX re_review_session
    ON recruiting.re_review (session_id, raised_at);

-- A resolution is written once and a row is never deleted. UPDATE is
-- allowed only while unresolved (assignment, resolution itself); any
-- change to a resolved row, and any DELETE, is refused whoever asks.
CREATE FUNCTION recruiting.refuse_re_review_rewrite() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 're-reviews are never deleted (REV-06)'
            USING ERRCODE = 'raise_exception';
    END IF;
    IF OLD.outcome IS NOT NULL THEN
        RAISE EXCEPTION 'a resolved re-review is immutable (REV-06)'
            USING ERRCODE = 'raise_exception';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER re_review_write_once
    BEFORE UPDATE OR DELETE ON recruiting.re_review
    FOR EACH ROW EXECUTE FUNCTION recruiting.refuse_re_review_rewrite();

ALTER TABLE recruiting.re_review ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.re_review FORCE ROW LEVEL SECURITY;

-- The tenant's. What a candidate learns of an outcome is the recorded
-- candidate_disclosure, delivered through their own surface (REV-07),
-- never a read of this table.
CREATE POLICY re_review_tenant ON recruiting.re_review
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
