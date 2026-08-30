-- 0049: the periodic access review.
--
-- TEN-03's first line is the whole design: "a scheduled prompt with a
-- recorded outcome, not a report nobody opens". A report is a query somebody
-- runs; a review is a row that exists, is due on a date, lists every person
-- who can reach candidate evidence, and cannot be closed until each of them
-- has been confirmed or revoked by name. That difference is why this is two
-- tables rather than a SELECT over memberships.
--
-- The items are a snapshot taken when the review opens, not a live view.
-- A review is a record of a decision made against a stated set of facts:
-- who held what role, and when they were last seen acting in this workspace.
-- Recomputing those at read time would mean a completed review no longer
-- shows what was confirmed, which is exactly the thing an auditor asks for.
--
-- Implements part of TEN-03.

CREATE TABLE tenancy.access_reviews (
    id                 uuid        PRIMARY KEY,
    tenant_id          uuid        NOT NULL REFERENCES tenancy.tenants (id) ON DELETE CASCADE,

    status             text        NOT NULL DEFAULT 'open'
                                   CHECK (status IN ('open', 'completed')),

    opened_at          timestamptz NOT NULL DEFAULT now(),
    opened_by          uuid        REFERENCES identity.users (id),
    -- When this review should have been finished by. Recorded on the row so
    -- an overdue review is a fact rather than a calculation somebody has to
    -- remember to perform.
    due_at             timestamptz NOT NULL,

    -- The dormancy standard this review applied, in days. On the review
    -- rather than in configuration, because a completed review has to keep
    -- meaning what it meant: "nobody was dormant" read against a 30-day
    -- standard is a different statement from the same words read against 180.
    dormant_after_days integer     NOT NULL CHECK (dormant_after_days > 0),

    completed_at       timestamptz,
    completed_by       uuid        REFERENCES identity.users (id),

    CHECK ((status = 'completed') = (completed_at IS NOT NULL))
);

COMMENT ON TABLE tenancy.access_reviews IS
    'One periodic review of who can reach candidate evidence in a workspace. '
    'Completing it requires a recorded decision on every item. See TEN-03.';

-- One open review per workspace. Two would split the roster between them and
-- neither would be the answer to "has access been reviewed".
CREATE UNIQUE INDEX access_reviews_one_open_per_tenant
    ON tenancy.access_reviews (tenant_id) WHERE status = 'open';

CREATE INDEX access_reviews_by_tenant
    ON tenancy.access_reviews (tenant_id, opened_at DESC);

CREATE TABLE tenancy.access_review_items (
    id             uuid        PRIMARY KEY,
    tenant_id      uuid        NOT NULL REFERENCES tenancy.tenants (id) ON DELETE CASCADE,
    review_id      uuid        NOT NULL REFERENCES tenancy.access_reviews (id) ON DELETE CASCADE,

    -- The membership under review, and the person behind it. Both are
    -- carried: the membership is what gets revoked, and the user survives
    -- the membership being removed, which is when somebody asks who it was.
    membership_id  uuid        NOT NULL,
    user_id        uuid        NOT NULL REFERENCES identity.users (id),

    -- Snapshots at the moment the review opened. No email address is stored:
    -- the role is what is being reviewed, and an address that is already in
    -- identity.users does not need a second, staler copy under a different
    -- retention rule.
    role           text        NOT NULL,
    last_active_at timestamptz,
    dormant        boolean     NOT NULL,

    decision       text        NOT NULL DEFAULT 'pending'
                               CHECK (decision IN ('pending', 'confirmed', 'revoked')),
    decided_at     timestamptz,
    decided_by     uuid        REFERENCES identity.users (id),
    -- Why, in the reviewer's words. Small and free text: this is read by the
    -- next reviewer, and "still needed, covering maternity leave" is the
    -- whole value of a review over a checkbox.
    note           text        NOT NULL DEFAULT '',

    CHECK ((decision = 'pending') = (decided_at IS NULL)),

    -- One item per membership per review. The uniqueness is what makes
    -- opening a review idempotent under a retry.
    UNIQUE (review_id, membership_id)
);

COMMENT ON TABLE tenancy.access_review_items IS
    'One person''s access, as it stood when a review opened, and what the '
    'reviewer decided about it. Snapshot rather than live view, so a '
    'completed review still shows what was confirmed.';

CREATE INDEX access_review_items_by_review
    ON tenancy.access_review_items (tenant_id, review_id, decision);

ALTER TABLE tenancy.access_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenancy.access_reviews FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenancy.access_reviews
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

ALTER TABLE tenancy.access_review_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenancy.access_review_items FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenancy.access_review_items
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- No DELETE. A review that can be removed answers "has access been reviewed"
-- with whatever the last person to look wanted it to say.
REVOKE DELETE ON tenancy.access_reviews, tenancy.access_review_items FROM prepeet_app;

GRANT SELECT, INSERT, UPDATE ON tenancy.access_reviews TO prepeet_app;
GRANT SELECT, INSERT, UPDATE ON tenancy.access_review_items TO prepeet_app;
