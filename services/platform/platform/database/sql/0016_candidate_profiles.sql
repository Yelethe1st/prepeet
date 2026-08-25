-- 0016: the candidate profile.
--
-- Disciplines, target roles, seniority, career context, interview defaults,
-- accessibility preferences and notification settings. One row per person,
-- owner-scoped like everything in the candidate schema: the structural guards
-- IAM-06 put on this schema - forced owner-only policy, no tenant dimension,
-- the tenant-context write tripwire - apply to this table by existing.
--
-- Every column is optional or defaulted, deliberately: PRO-01 requires that
-- a partial profile is usable and nothing hides behind a completeness score.
-- The row itself is created on first write, so a candidate who never opens
-- the profile screen simply has no row, which reads as the empty profile.
--
-- Implements part of PRO-01.

CREATE TABLE candidate.profiles (
    user_id       uuid        PRIMARY KEY REFERENCES identity.users (id),

    -- What they practise for. Arrays of short labels rather than a taxonomy
    -- table, because the catalogue of disciplines is CAT-03's and until it
    -- exists these are the candidate's own words.
    disciplines   text[]      NOT NULL DEFAULT '{}',
    target_roles  text[]      NOT NULL DEFAULT '{}',
    seniority     text,

    -- Their own account of where they are and where they are going. Free
    -- text, bounded in the service; it feeds composition as context, never
    -- as criteria.
    career_context text,

    -- Interview defaults, applied when creating a practice session and
    -- overridable there. NULL means "no preference recorded".
    default_duration_minutes integer CHECK (default_duration_minutes BETWEEN 10 AND 90),
    default_style            text,
    default_pressure         text CHECK (default_pressure IN ('low', 'standard', 'high')),

    -- Accessibility preferences, voluntarily stored, honoured by the prepare
    -- and live screens by default. Explicit columns rather than a settings
    -- blob, because each one is a promise a screen has to keep and a blob
    -- key is a promise nothing can find.
    extended_time  boolean    NOT NULL DEFAULT false,
    captions       boolean    NOT NULL DEFAULT false,
    reduced_motion boolean    NOT NULL DEFAULT false,
    accessibility_notes text,

    -- Notification settings. Transactional email - verification, recovery -
    -- is unaffected by these: it is how the account works, not marketing.
    notify_product_updates    boolean NOT NULL DEFAULT false,
    notify_practice_reminders boolean NOT NULL DEFAULT true,

    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE candidate.profiles IS
    'The candidate''s own profile: practice targets, interview defaults, '
    'accessibility and notification preferences. Owner-scoped; no tenant '
    'authority reaches it, structurally, per IAM-06.';

ALTER TABLE candidate.profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE candidate.profiles FORCE ROW LEVEL SECURITY;

CREATE POLICY profiles_owner ON candidate.profiles
    USING (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

-- The same tripwire practice sessions carry: no write under tenant context,
-- because a tenant-scoped code path has no business touching a profile even
-- as its owner.
CREATE TRIGGER profiles_no_tenant_context
    BEFORE INSERT OR UPDATE OR DELETE ON candidate.profiles
    FOR EACH ROW EXECUTE FUNCTION candidate.refuse_tenant_context();

GRANT SELECT, INSERT, UPDATE, DELETE ON candidate.profiles TO prepeet_app;
