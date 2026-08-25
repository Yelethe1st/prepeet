-- 0021: the platform's service principals.
--
-- Automation acts, and the audit trail's foreign keys rightly insist that
-- every actor exists. These two rows are the content pipeline's identities:
-- the loader drafts as one and publishes as the other, keeping the
-- registry's separation of duties structural even for git-authored content
-- (the human review happened in the pull request; these record which
-- automation carried it in).
--
-- No email and no credential, so neither can ever sign in: they exist to be
-- pointed at by audit rows and artifact provenance, nothing more. Seeded by
-- migration rather than by hand because a fresh environment must be able to
-- publish content before any person has registered.
--
-- Implements part of CAT-03.

INSERT INTO identity.users (id, email, email_verified, status)
VALUES
    ('00000000-0000-7000-8000-0000000000c9', NULL, false, 'active'),
    ('00000000-0000-7000-8000-0000000000ca', NULL, false, 'active')
ON CONFLICT (id) DO NOTHING;

COMMENT ON COLUMN identity.users.email IS
    'NULL for service principals, which can never sign in; unique when present.';
