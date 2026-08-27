-- 0036: answer redos, one per question, as linked practice sessions.
--
-- PRC-03. A redo is a NEW practice session composed from the parent's
-- own configuration plus the one question it retakes, so the original
-- session, its transcript, its evidence and its timing are never touched:
-- they live in a different row set entirely. This table is the link, and
-- its UNIQUE is the rule "one retake per question" made structural. A
-- redo is practice-only by the parent's mode CHECK below; screening has
-- no retakes (session-lifecycle.md: screen retry is a human decision).
--
-- Implements part of PRC-03.

CREATE TABLE interview.redos (
    parent_session_id uuid        NOT NULL REFERENCES interview.sessions (id),
    sequence          integer     NOT NULL CHECK (sequence >= 1),
    redo_session_id   uuid        NOT NULL UNIQUE REFERENCES interview.sessions (id),
    mode              text        NOT NULL CHECK (mode = 'practice'),
    candidate_id      uuid        NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (parent_session_id, sequence)
);

COMMENT ON TABLE interview.redos IS
    'Which practice session retakes which answer. One per question by '
    'primary key; the original session is never modified.';

ALTER TABLE interview.redos ENABLE ROW LEVEL SECURITY;
ALTER TABLE interview.redos FORCE ROW LEVEL SECURITY;

CREATE POLICY redos_practice_owner ON interview.redos
    USING (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT ON interview.redos TO prepeet_app;
