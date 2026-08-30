-- The progression store's queries. sqlc generates the Go beside this
-- file; ADR-0010 records why no SQL lives in Go source.

-- name: InsertObservation :execrows
-- Idempotent per (evaluation, competency): the consumer's retry converges
-- on the history already written.
INSERT INTO progression.observations
    (id, candidate_id, mode, tenant_id, session_id, evaluation_id,
     competency_id, status, band, confidence, evidence_count,
     supporting, contradictory, unverified, gaps,
     rubric_reference, rubric_version, rubric_digest,
     aggregation_version, extraction_version, model_version, policy_version,
     supersedes, observed_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(candidate_id)::uuid, sqlc.arg(mode)::text,
        nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(session_id)::uuid, sqlc.arg(evaluation_id)::uuid,
        sqlc.arg(competency_id)::text, sqlc.arg(status)::text,
        sqlc.arg(band)::text, sqlc.arg(confidence)::text,
        sqlc.arg(evidence_count)::integer,
        sqlc.arg(supporting)::integer, sqlc.arg(contradictory)::integer,
        sqlc.arg(unverified)::integer, sqlc.arg(gaps)::integer,
        sqlc.arg(rubric_reference)::text, sqlc.arg(rubric_version)::text,
        sqlc.arg(rubric_digest)::text,
        sqlc.arg(aggregation_version)::text, sqlc.arg(extraction_version)::text,
        sqlc.arg(model_version)::text, sqlc.arg(policy_version)::text,
        nullif(sqlc.arg(supersedes)::text, '')::uuid,
        sqlc.arg(observed_at)::timestamptz)
ON CONFLICT (evaluation_id, competency_id) DO NOTHING;

-- name: ListObservations :many
-- The whole history for one candidate, oldest first: what a progression
-- chart plots, every version of every reading included.
SELECT id::text AS id, session_id::text AS session_id,
       evaluation_id::text AS evaluation_id, competency_id,
       status, band, confidence, evidence_count,
       supporting, contradictory, unverified, gaps,
       rubric_reference, rubric_version, rubric_digest,
       aggregation_version, extraction_version, model_version, policy_version,
       coalesce(supersedes::text, '')::text AS supersedes,
       observed_at, created_at
FROM progression.observations
ORDER BY observed_at, competency_id;

-- name: InsertReadinessSnapshot :execrows
-- Idempotent per (candidate, standard, answer): recomputing an answer that
-- has not changed converges on the snapshot already written, so history
-- records what changed rather than how often somebody looked.
INSERT INTO progression.readiness_snapshots
    (id, candidate_id, mode, tenant_id,
     standard_reference, standard_version, standard_digest,
     role_id, discipline_id, rubric_reference, answer_digest, computed_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(candidate_id)::uuid, sqlc.arg(mode)::text,
        nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(standard_reference)::text, sqlc.arg(standard_version)::text,
        sqlc.arg(standard_digest)::text,
        sqlc.arg(role_id)::text, sqlc.arg(discipline_id)::text,
        sqlc.arg(rubric_reference)::text,
        sqlc.arg(answer_digest)::text, sqlc.arg(computed_at)::timestamptz)
ON CONFLICT (candidate_id, standard_reference, answer_digest) DO NOTHING;

-- name: InsertReadinessCompetency :exec
-- One requirement's outcome. The schema refuses an unassessed row that
-- carries a band, an observation or a date, so the empties below are not
-- a convention but the only accepted shape.
INSERT INTO progression.readiness_competencies
    (snapshot_id, candidate_id, mode, tenant_id, competency_id, target_band,
     outcome, observed_band, observation_id, observed_at, reason)
VALUES (sqlc.arg(snapshot_id)::uuid, sqlc.arg(candidate_id)::uuid,
        sqlc.arg(mode)::text, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(competency_id)::text, sqlc.arg(target_band)::text,
        sqlc.arg(outcome)::text, sqlc.arg(observed_band)::text,
        nullif(sqlc.arg(observation_id)::text, '')::uuid,
        sqlc.narg(observed_at)::timestamptz,
        sqlc.arg(reason)::text)
ON CONFLICT (snapshot_id, competency_id) DO NOTHING;

-- name: ListReadiness :many
-- The current readiness for every standard this owner has one for: the
-- newest snapshot per standard, with every one of its requirements,
-- ordered by discipline and role. Grouped rather than aggregated - there
-- is no row here that spans two standards, because incomparable roles are
-- never averaged. How many were met is counted from these rows rather
-- than stored, so no summary can disagree with what it summarises.
WITH latest AS (
    SELECT DISTINCT ON (standard_reference)
           id, standard_reference, standard_version, standard_digest,
           role_id, discipline_id, rubric_reference, computed_at
    FROM progression.readiness_snapshots
    ORDER BY standard_reference, computed_at DESC, created_at DESC
)
SELECT latest.id::text AS snapshot_id,
       latest.standard_reference, latest.standard_version, latest.standard_digest,
       latest.role_id, latest.discipline_id, latest.rubric_reference,
       latest.computed_at,
       requirement.competency_id, requirement.target_band, requirement.outcome,
       requirement.observed_band, requirement.reason,
       coalesce(requirement.observation_id::text, '')::text AS observation_id,
       requirement.observed_at
FROM latest
JOIN progression.readiness_competencies AS requirement
     ON requirement.snapshot_id = latest.id
ORDER BY latest.discipline_id, latest.role_id, latest.standard_reference,
         requirement.competency_id;
