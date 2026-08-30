-- name: LatestDeterminationFor :one
-- The current determination for a jurisdiction. Absence is the answer that
-- matters: ADR-0020 makes a missing determination a refusal to open, not a
-- permissive default.
SELECT id, jurisdiction, version, result_disclosure, appeal_status, approver, approved_at
FROM recruiting.jurisdiction_determination
WHERE jurisdiction = $1
ORDER BY version DESC
LIMIT 1;

-- name: DeterminationByID :one
SELECT id, jurisdiction, version, result_disclosure, appeal_status, approver, approved_at
FROM recruiting.jurisdiction_determination
WHERE id = $1;

-- name: CreateCampaign :one
INSERT INTO recruiting.campaign (id, tenant_id, name, role_reference, jurisdiction, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, tenant_id, name, status, role_reference, jurisdiction,
          determination_id, opened_at, closed_at, created_at, created_by;

-- name: CampaignByID :one
SELECT id, tenant_id, name, status, role_reference, jurisdiction,
       determination_id, opened_at, closed_at, created_at, created_by
FROM recruiting.campaign
WHERE id = $1;

-- name: PinArtifact :exec
INSERT INTO recruiting.campaign_pin
    (campaign_id, tenant_id, artifact_type, artifact_id, digest, reference, version)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: PinsForCampaign :many
SELECT campaign_id, artifact_type, artifact_id, digest, reference, version, pinned_at
FROM recruiting.campaign_pin
WHERE campaign_id = $1
ORDER BY artifact_type;

-- name: OpenCampaign :one
-- Opening is a guarded transition, not a field update. The WHERE clause carries
-- the guard so that two concurrent opens cannot both succeed: the second finds
-- no draft row and gets no result, rather than overwriting the first one's
-- determination.
UPDATE recruiting.campaign
SET status = 'open', determination_id = $2, opened_at = now()
WHERE id = $1 AND status = 'draft'
RETURNING id, tenant_id, name, status, role_reference, jurisdiction,
          determination_id, opened_at, closed_at, created_at, created_by;

-- name: CloseCampaign :one
UPDATE recruiting.campaign
SET status = 'closed', closed_at = now()
WHERE id = $1 AND status = 'open'
RETURNING id, tenant_id, name, status, role_reference, jurisdiction,
          determination_id, opened_at, closed_at, created_at, created_by;

-- name: GrantCampaignAccess :exec
INSERT INTO recruiting.campaign_recruiter (campaign_id, tenant_id, user_id, granted_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (campaign_id, user_id) DO NOTHING;

-- name: RevokeCampaignAccess :exec
DELETE FROM recruiting.campaign_recruiter WHERE campaign_id = $1 AND user_id = $2;

-- name: RecruiterMayAccess :one
SELECT EXISTS (
    SELECT 1 FROM recruiting.campaign_recruiter
    WHERE campaign_id = $1 AND user_id = $2
) AS allowed;

-- name: CampaignsForRecruiter :many
-- Scoped by the join rather than by a filter the caller supplies, so a caller
-- that forgets to filter gets nothing rather than everything.
SELECT c.id, c.tenant_id, c.name, c.status, c.role_reference, c.jurisdiction,
       c.determination_id, c.opened_at, c.closed_at, c.created_at, c.created_by
FROM recruiting.campaign c
JOIN recruiting.campaign_recruiter r ON r.campaign_id = c.id
WHERE r.user_id = $1
ORDER BY c.created_at DESC;
