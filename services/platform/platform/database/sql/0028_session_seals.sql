-- 0028: the seal - what evaluation will be given, fixed at completion.
--
-- SES-04, to session-lifecycle.md's completion contract. Completing a
-- session freezes its conversational record: the final cursor, the gaps
-- that stand in it (recorded, never silently closed), the digest of the
-- effective transcript, the bundle digest it ran under, and the media
-- status with any warning attached. One row per session, immutable, so a
-- duplicate completion converges on the same receipt and an evaluation can
-- prove exactly what it was given a year later.

CREATE TABLE interview.seals (
    session_id        uuid        PRIMARY KEY REFERENCES interview.sessions (id),
    mode              text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    candidate_id      uuid        NOT NULL,
    tenant_id         uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),

    sealed_epoch      integer     NOT NULL CHECK (sealed_epoch >= 1),
    sealed_sequence   integer     NOT NULL CHECK (sealed_sequence >= 0),

    -- The gaps under the final cursor, as [[from,to],...]. Recorded rather
    -- than closed: evaluation reads them as coverage, never as absence of
    -- anything to say.
    gaps              jsonb       NOT NULL DEFAULT '[]'::jsonb,

    transcript_digest text        NOT NULL,
    bundle_digest     text        NOT NULL,

    media_status      text        NOT NULL CHECK (media_status IN
                                      ('none_by_choice', 'missing', 'finalized')),
    warnings          jsonb       NOT NULL DEFAULT '[]'::jsonb,

    created_at        timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE interview.seals IS
    'The frozen evaluation input per session: final cursor, recorded gaps, '
    'transcript and bundle digests, media status. Written once at '
    'completion; a duplicate completion converges on this row.';

CREATE OR REPLACE FUNCTION interview.refuse_seal_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'interview.seals is immutable: a seal that could change is not a seal';
END $$;

CREATE TRIGGER seals_immutable
    BEFORE UPDATE OR DELETE ON interview.seals
    FOR EACH ROW EXECUTE FUNCTION interview.refuse_seal_change();

ALTER TABLE interview.seals ENABLE ROW LEVEL SECURITY;
ALTER TABLE interview.seals FORCE ROW LEVEL SECURITY;

CREATE POLICY seals_tenant ON interview.seals
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY seals_practice_owner ON interview.seals
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT ON interview.seals TO prepeet_app;
