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

-- name: RecordAcceptance :exec
INSERT INTO recruiting.disclosure_acceptance
    (id, tenant_id, campaign_id, candidate_id, disclosure_digest, disclosure_version)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: AcceptancesFor :many
-- Every acceptance, newest first. All of them rather than the latest, because
-- "what had this person been told when they sat the interview" is a question
-- about a moment, and answering it needs the history.
SELECT id, campaign_id, candidate_id, disclosure_digest, disclosure_version, accepted_at
FROM recruiting.disclosure_acceptance
WHERE campaign_id = $1 AND candidate_id = $2
ORDER BY accepted_at DESC;

-- name: RecordConsentDecision :exec
INSERT INTO recruiting.consent_decision
    (id, tenant_id, campaign_id, candidate_id, purpose, required, granted, disclosure_digest)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: StandingConsent :many
-- The latest decision per purpose. DISTINCT ON rather than a join on max, so
-- one index scan answers it and a withdrawal recorded later automatically
-- becomes the standing answer without any row being edited.
SELECT DISTINCT ON (purpose)
       purpose, required, granted, disclosure_digest, decided_at
FROM recruiting.consent_decision
WHERE campaign_id = $1 AND candidate_id = $2
ORDER BY purpose, decided_at DESC;

-- name: CampaignsUsingArtifact :many
-- The open campaigns that pinned a given artifact reference.
--
-- By reference rather than by digest, which is the opposite of how a campaign
-- identifies its configuration and is deliberate. The question this answers is
-- the author's: "may I remove this rubric", and they think in references. A
-- digest match would answer only for the exact version pinned and would let
-- the draft behind a running campaign be discarded.
--
-- Open only. A closed campaign runs nothing and issues nothing, so it does not
-- block an author from tidying up; what it already evaluated is pinned by
-- digest and stays resolvable either way.
--
-- Tenant scoping comes from the row-level security policy rather than a
-- predicate here, so a caller who forgets to scope gets nothing rather than
-- another workspace's campaign names.
SELECT c.name
FROM recruiting.campaign c
JOIN recruiting.campaign_pin p ON p.campaign_id = c.id
WHERE c.status = 'open' AND p.reference = sqlc.arg(reference)::text
ORDER BY c.name;

-- name: ListCampaigns :many
-- Every campaign in the tenant, newest first.
--
-- Deliberately not joined to campaign_recruiter: campaign.read is unscoped in
-- the catalogue precisely so a recruiter can see which campaigns exist before
-- being assigned to one. What stays behind the join is everything about a
-- particular campaign. Tenant scoping is the row-level security policy's, so a
-- caller who forgets to scope gets nothing rather than another workspace's
-- roster of roles.
SELECT id, tenant_id, name, status, role_reference, jurisdiction,
       determination_id, opened_at, closed_at, created_at, created_by
FROM recruiting.campaign
ORDER BY created_at DESC;

-- name: RequestAccommodation :one
INSERT INTO recruiting.accommodation_request
    (id, tenant_id, campaign_id, candidate_id, adjustment)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, tenant_id, campaign_id, candidate_id, adjustment, requested_at;

-- name: RecordAccommodationDecision :execrows
-- The campaign guard is a join, not decoration: a decision lands only on a
-- request that belongs to the named campaign, so a recruiter admitted to one
-- campaign cannot answer another's request in the same tenant by its id. Zero
-- rows means no such request on this campaign, which the store turns into a
-- not-found rather than a silent no-op.
INSERT INTO recruiting.accommodation_decision (id, tenant_id, request_id, granted, decided_by)
SELECT sqlc.arg(id)::uuid, sqlc.arg(tenant_id)::uuid, r.id, sqlc.arg(granted)::boolean, sqlc.arg(decided_by)::uuid
FROM recruiting.accommodation_request r
WHERE r.id = sqlc.arg(request_id)::uuid AND r.campaign_id = sqlc.arg(campaign_id)::uuid;

-- name: StandingAccommodationDecision :one
-- The latest decision for one request, which is the standing answer: a grant
-- later withdrawn or a decline later reversed is a newer row, never an edit.
SELECT request_id, granted, decided_by, decided_at
FROM recruiting.accommodation_decision
WHERE request_id = $1
ORDER BY decided_at DESC
LIMIT 1;

-- name: RecordAccommodationFulfilment :exec
-- The requires-a-standing-grant rule is enforced by trigger here as well as
-- by the store, so a future caller reaching for this query directly meets the
-- same refusal the store would have given it.
INSERT INTO recruiting.accommodation_fulfilment
    (id, tenant_id, request_id, session_id)
VALUES ($1, $2, $3, $4);

-- name: AccommodationRequestsFor :many
-- Every request this candidate made on this campaign, newest first. The
-- standing decision is read per request through StandingAccommodationDecision
-- rather than joined here: a request nobody has answered has no decision row,
-- and "no row" is an answer this store gives a specific meaning to
-- ("requested") rather than a null it has to reinterpret.
SELECT id, campaign_id, candidate_id, adjustment, requested_at
FROM recruiting.accommodation_request
WHERE campaign_id = $1 AND candidate_id = $2
ORDER BY requested_at DESC;

-- name: AccommodationsForSession :many
-- What was actually applied to one session: the read the interview runner's
-- port will serve when the composition root wires it. This is the record of
-- an accommodation being exercised, and it lives here so that evaluation,
-- which cannot name this schema, can never make a signal of it.
SELECT r.adjustment, f.request_id, f.session_id, f.fulfilled_at
FROM recruiting.accommodation_fulfilment f
JOIN recruiting.accommodation_request r ON r.id = f.request_id
WHERE f.session_id = $1
ORDER BY f.fulfilled_at;

-- name: IssueInvitation :one
-- Store one invitation. Only the hash reaches the table; the plaintext was
-- handed to the email in the same transaction and exists nowhere else. Tenant
-- scoping is the policy's, so an unscoped caller inserts nothing it can then
-- read back.
INSERT INTO recruiting.invitation
    (id, tenant_id, campaign_id, recipient, token_hash, email_id, issued_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, tenant_id, campaign_id, recipient, email_id, issued_by,
          issued_at, expires_at, outcome, outcome_at;

-- name: SupersedeLiveInvitations :exec
-- Retire every live link for this recipient on this campaign, which is what a
-- resend does before it issues a fresh one: the old link stops working the
-- instant the new one is promised, so a recipient forwarded two emails cannot
-- accept twice. Only live rows are touched; a spent or revoked invitation
-- keeps the ending it already has.
UPDATE recruiting.invitation
SET outcome = 'superseded', outcome_at = now()
WHERE campaign_id = $1 AND recipient = $2 AND outcome IS NULL;

-- name: RevokeInvitation :one
-- Revoke one invitation by id on one campaign, but only while it is still live.
-- The campaign_id in the guard is the per-campaign scope: a recruiter admitted
-- to one campaign cannot revoke another campaign's invitation in the same
-- tenant by knowing its id, because the id alone matches nothing without the
-- campaign the caller was checked against. The guard on a null outcome is what
-- makes revocation honest: a link the candidate has already accepted or
-- declined cannot be quietly revoked out from under the record of what they
-- did, and revoking an already-revoked one is a no-op that returns nothing
-- rather than a second ending. Nothing is deleted; the row and everything it
-- points at stay exactly where they are.
UPDATE recruiting.invitation
SET outcome = 'revoked', outcome_at = now()
WHERE id = $1 AND campaign_id = $2 AND outcome IS NULL
RETURNING id, tenant_id, campaign_id, recipient, email_id, issued_by,
          issued_at, expires_at, outcome, outcome_at;

-- name: InvitationsForCampaign :many
-- The recruiter's roster for one campaign, newest first. email_id rides along
-- so cmd can join delivery status from notification, which this context does
-- not read. Tenant scoping is the policy's.
SELECT id, tenant_id, campaign_id, recipient, email_id, issued_by,
       issued_at, expires_at, outcome, outcome_at
FROM recruiting.invitation
WHERE campaign_id = $1
ORDER BY issued_at DESC;

-- name: InvitationByID :one
-- One invitation on one campaign, for the resend path that needs its recipient
-- and its outcome before deciding whether a fresh link may be sent. Scoped by
-- campaign_id for the same reason revoke is: a recruiter on one campaign cannot
-- reach another's invitation by id. Tenant scoping is the policy's.
SELECT id, tenant_id, campaign_id, recipient, email_id, issued_by,
       issued_at, expires_at, outcome, outcome_at
FROM recruiting.invitation
WHERE id = $1 AND campaign_id = $2;

-- name: ResolveInvitationByToken :one
-- Read one invitation by the hash of the presented token, for the candidate
-- acceptance path. Access is the token-scoped policy's: the row is visible only
-- because the caller set app.invitation_token_hash to this same hash, which is
-- proof they hold the token. The WHERE is that hash again, so the query names
-- the one row the policy already narrowed to rather than trusting the scope
-- alone.
SELECT id, tenant_id, campaign_id, recipient, email_id, issued_by,
       issued_at, expires_at, outcome, outcome_at
FROM recruiting.invitation
WHERE token_hash = $1;

-- name: AcceptInvitationByToken :one
-- Accept, single-use and not past expiry. The guard on a null outcome makes the
-- accept a one-winner race: two clicks on one link produce one accepted
-- interview and one refusal, not two. The expires_at guard refuses a link that
-- lapsed while it sat in an inbox; the caller reads the current state first and
-- tells the candidate which of expired, revoked or already-answered it was, so
-- this returning no row is an outcome to explain rather than an error.
UPDATE recruiting.invitation
SET outcome = 'accepted', outcome_at = now(), accepted_candidate = sqlc.arg(candidate)::uuid
WHERE token_hash = sqlc.arg(token_hash)::text AND outcome IS NULL AND expires_at > now()
RETURNING id, tenant_id, campaign_id, recipient, email_id, issued_by,
          issued_at, expires_at, outcome, outcome_at;

-- name: DeclineInvitationByToken :one
-- Decline, the candidate's first-class no. Guarded on a null outcome like
-- accept, so declining a link already answered or revoked changes nothing and
-- returns nothing. Declining is recorded, never penalised: the row simply ends
-- as declined, and nothing downstream treats that differently from never having
-- been asked.
UPDATE recruiting.invitation
SET outcome = 'declined', outcome_at = now()
WHERE token_hash = $1 AND outcome IS NULL AND expires_at > now()
RETURNING id, tenant_id, campaign_id, recipient, email_id, issued_by,
          issued_at, expires_at, outcome, outcome_at;

-- name: CampaignByID :one
-- One campaign by id, tenant-scoped by the row-level security policy. Unlike
-- CampaignsForRecruiter this carries no recruiter join, so it is not a way to
-- read a campaign a recruiter is not on: it exists for the candidate
-- acceptance path, which is authorised by a valid invitation token rather than
-- by campaign membership, and needs the role the invitation is for. A recruiter
-- surface must keep using the join; this is not that.
SELECT id, tenant_id, name, status, role_reference, jurisdiction,
       determination_id, opened_at, closed_at, created_at, created_by
FROM recruiting.campaign
WHERE id = $1;

-- name: AcceptedInvitationForCandidate :one
-- The invitation this candidate accepted to this campaign, read as the
-- candidate themselves through the owner policy 0059 added. It is how the
-- screening session creation path proves authority: a candidate who did not
-- accept an invitation to the campaign finds no row here and cannot start one.
SELECT id, tenant_id, campaign_id, recipient, email_id, issued_by,
       issued_at, expires_at, outcome, outcome_at
FROM recruiting.invitation
WHERE campaign_id = $1 AND accepted_candidate = $2 AND outcome = 'accepted';

-- name: UpsertJobContext :exec
-- The job context, one per campaign, replaced wholesale on resubmission. The
-- source is stored verbatim so requirement spans index into the exact bytes.
INSERT INTO recruiting.job_context (campaign_id, tenant_id, source_text, extraction_version)
VALUES ($1, $2, $3, $4)
ON CONFLICT (campaign_id) DO UPDATE
SET source_text = EXCLUDED.source_text,
    extraction_version = EXCLUDED.extraction_version,
    submitted_at = now();

-- name: JobContextFor :one
SELECT campaign_id, source_text, extraction_version, submitted_at
FROM recruiting.job_context
WHERE campaign_id = $1;

-- name: DeleteRequirementsForCampaign :exec
-- Clears a draft campaign's requirements before a fresh extraction replaces
-- them. The freeze trigger refuses this once the campaign has opened, so a
-- running campaign's requirements cannot be cleared out from under it.
DELETE FROM recruiting.campaign_requirement WHERE campaign_id = $1;

-- name: InsertRequirement :one
INSERT INTO recruiting.campaign_requirement
    (id, campaign_id, tenant_id, text, span_start, span_end, extraction_version)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, campaign_id, text, span_start, span_end, status, extraction_version, created_at;

-- name: RequirementsForCampaign :many
-- The campaign's requirements, in the order they were extracted, so the recruiter
-- reads them against the job description's own order. Tenant scoping is the policy's.
SELECT id, campaign_id, text, span_start, span_end, status, extraction_version, created_at
FROM recruiting.campaign_requirement
WHERE campaign_id = $1
ORDER BY created_at, id;

-- name: CorrectRequirement :one
-- Correct or reject one requirement on one campaign. Scoped by campaign_id so a
-- recruiter admitted to one campaign cannot rewrite another's requirement by id;
-- the freeze trigger refuses it once the campaign has opened.
UPDATE recruiting.campaign_requirement
SET text = COALESCE(NULLIF(sqlc.arg(text)::text, ''), text), status = sqlc.arg(status)::text
WHERE id = sqlc.arg(id)::uuid AND campaign_id = sqlc.arg(campaign_id)::uuid
RETURNING id, campaign_id, text, span_start, span_end, status, extraction_version, created_at;

-- name: AuthorizeReInvitation :one
-- Records a recruiter's decision to let a candidate take one further session.
-- The reason and decider are required by the table; this writes them.
INSERT INTO recruiting.re_invitation
    (id, campaign_id, tenant_id, candidate_id, reason, decided_by, interrupted_session)
VALUES ($1, $2, $3, $4, $5, $6, nullif(sqlc.arg(interrupted_session)::text, '')::uuid)
RETURNING id, campaign_id, candidate_id, reason, decided_by, interrupted_session, consumed_session, created_at;

-- name: ReInvitationsForCandidate :many
-- The re-invitations a candidate holds on a campaign, for the recruiter's
-- audit. Tenant scoping is the policy's.
SELECT id, campaign_id, candidate_id, reason, decided_by, interrupted_session, consumed_session, created_at
FROM recruiting.re_invitation
WHERE campaign_id = $1 AND candidate_id = $2
ORDER BY created_at;

-- name: ClaimReInvitation :one
-- The candidate claims their oldest unclaimed re-invitation, binding it to the
-- new session, so one authorization admits exactly one further attempt. Run as
-- the candidate through the claim policy; returns nothing when they hold none.
UPDATE recruiting.re_invitation
SET consumed_session = sqlc.arg(session_id)::uuid
WHERE id = (
    SELECT id FROM recruiting.re_invitation
    WHERE campaign_id = sqlc.arg(campaign_id)::uuid
      AND candidate_id = sqlc.arg(candidate_id)::uuid
      AND consumed_session IS NULL
    ORDER BY created_at
    LIMIT 1
)
RETURNING id, campaign_id, candidate_id, consumed_session;
