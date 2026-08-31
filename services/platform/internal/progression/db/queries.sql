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

-- name: InsertGoal :exec
-- A goal the candidate set. Idempotent on the identifier so a retried
-- create converges rather than raising a second target for one decision.
INSERT INTO progression.goals
    (id, candidate_id, origin, origin_reference, competency_id, target_band,
     rubric_reference, bands, status)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(candidate_id)::uuid,
        sqlc.arg(origin)::text, sqlc.arg(origin_reference)::text,
        sqlc.arg(competency_id)::text, sqlc.arg(target_band)::text,
        sqlc.arg(rubric_reference)::text, sqlc.arg(bands)::text[],
        sqlc.arg(status)::text)
ON CONFLICT (id) DO NOTHING;

-- name: ListGoals :many
-- Every goal this candidate has, retired ones included: a retired goal is
-- part of the record of what they worked on, not a row to hide.
SELECT id::text AS id, origin, origin_reference, competency_id, target_band,
       rubric_reference, bands, status, created_at
FROM progression.goals
ORDER BY created_at, id;

-- name: SetGoalStatus :execrows
-- Pause, resume or retire. The trigger refuses everything else about a
-- goal changing, and refuses a retired goal coming back.
UPDATE progression.goals
SET status = sqlc.arg(status)::text
WHERE id = sqlc.arg(id)::uuid;

-- name: InsertMilestone :exec
-- One band reached, once. The conflict clause is what makes recomputing a
-- goal's progress safe to run as often as anybody likes.
INSERT INTO progression.goal_milestones
    (goal_id, candidate_id, band, rubric_reference, rubric_version,
     observation_id, reached_at)
VALUES (sqlc.arg(goal_id)::uuid, sqlc.arg(candidate_id)::uuid,
        sqlc.arg(band)::text, sqlc.arg(rubric_reference)::text,
        sqlc.arg(rubric_version)::text, sqlc.arg(observation_id)::uuid,
        sqlc.arg(reached_at)::timestamptz)
ON CONFLICT (goal_id, band) DO NOTHING;

-- name: ListMilestones :many
-- Everything earned, oldest first, across every goal.
SELECT goal_id::text AS goal_id, band, rubric_reference, rubric_version,
       observation_id::text AS observation_id, reached_at
FROM progression.goal_milestones
ORDER BY reached_at, goal_id, band;

-- name: InsertPersonalRequirement :exec
-- Version 1 of a requirement the candidate wrote.
INSERT INTO progression.personal_requirements
    (id, candidate_id, intent, status, version, reframing, prohibited)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(candidate_id)::uuid,
        sqlc.arg(intent)::text, sqlc.arg(status)::text,
        sqlc.arg(version)::integer, sqlc.arg(reframing)::text,
        sqlc.arg(prohibited)::text)
ON CONFLICT (id) DO NOTHING;

-- name: ReviseRequirement :execrows
-- The next version in use. The trigger refuses a version that falls and a
-- retired requirement coming back.
UPDATE progression.personal_requirements
SET intent = sqlc.arg(intent)::text,
    version = sqlc.arg(version)::integer,
    reframing = sqlc.arg(reframing)::text,
    prohibited = sqlc.arg(prohibited)::text
WHERE id = sqlc.arg(id)::uuid;

-- name: SetRequirementStatus :execrows
UPDATE progression.personal_requirements
SET status = sqlc.arg(status)::text
WHERE id = sqlc.arg(id)::uuid;

-- name: ListPersonalRequirements :many
-- Every requirement this candidate has written, retired ones included.
SELECT id::text AS id, intent, status, version, reframing, prohibited, created_at
FROM progression.personal_requirements
ORDER BY created_at, id;

-- name: InsertRequirementCriterion :exec
-- One criterion of one version. Immutable once written, so a redelivery
-- converges on what is already there.
INSERT INTO progression.requirement_criteria
    (requirement_id, candidate_id, version, criterion_id, position, statement, observable)
VALUES (sqlc.arg(requirement_id)::uuid, sqlc.arg(candidate_id)::uuid,
        sqlc.arg(version)::integer, sqlc.arg(criterion_id)::text,
        sqlc.arg(position)::integer, sqlc.arg(statement)::text,
        sqlc.arg(observable)::text)
ON CONFLICT (requirement_id, version, criterion_id) DO NOTHING;

-- name: ListRequirementCriteria :many
-- Every version's criteria, so an outcome from March can be read against
-- exactly what judged it.
SELECT requirement_id::text AS requirement_id, version, criterion_id,
       position, statement, observable
FROM progression.requirement_criteria
ORDER BY requirement_id, version, position, criterion_id;

-- name: DeleteRequirement :execrows
-- The candidate erasing their own requirement. Criteria and outcomes
-- cascade, which is the point: erasure that left the results behind would
-- not be erasure.
DELETE FROM progression.personal_requirements WHERE id = sqlc.arg(id)::uuid;

-- name: InsertRequirementOutcome :exec
-- One session's answer. Idempotent per (session, requirement), so a
-- redelivered projection cannot count one session twice in a metric.
INSERT INTO progression.requirement_outcomes
    (id, requirement_id, candidate_id, criterion_version, session_id,
     role_id, shape_id, outcome, reason,
     demonstrated, missing, evidence, next_actions, observed_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(requirement_id)::uuid,
        sqlc.arg(candidate_id)::uuid, sqlc.arg(criterion_version)::integer,
        sqlc.arg(session_id)::uuid, sqlc.arg(role_id)::text,
        sqlc.arg(shape_id)::text, sqlc.arg(outcome)::text,
        sqlc.arg(reason)::text, sqlc.arg(demonstrated)::text[],
        sqlc.arg(missing)::text[], sqlc.arg(evidence)::text[],
        sqlc.arg(next_actions)::text[], sqlc.arg(observed_at)::timestamptz)
ON CONFLICT (session_id, requirement_id) DO NOTHING;

-- name: ListRequirementOutcomes :many
-- Everything every session said, oldest first.
SELECT id::text AS id, requirement_id::text AS requirement_id, criterion_version,
       session_id::text AS session_id, role_id, shape_id, outcome, reason,
       demonstrated, missing, evidence, next_actions, observed_at
FROM progression.requirement_outcomes
ORDER BY observed_at, requirement_id, session_id;

-- name: InsertSelfReport :exec
-- The candidate's own rating. Replacing an earlier one for the same phase
-- is a correction of their own answer, which is theirs to make.
INSERT INTO progression.confidence_self_reports
    (candidate_id, session_id, phase, rating, reported_at)
VALUES (sqlc.arg(candidate_id)::uuid, sqlc.arg(session_id)::uuid,
        sqlc.arg(phase)::text, sqlc.arg(rating)::smallint,
        sqlc.arg(reported_at)::timestamptz)
ON CONFLICT (candidate_id, session_id, phase) DO NOTHING;

-- name: ListSelfReports :many
-- Read on its own, never joined to an observation.
SELECT session_id::text AS session_id, phase, rating, reported_at
FROM progression.confidence_self_reports
ORDER BY reported_at, session_id, phase;

-- name: SetPersonalisation :exec
-- The candidate's switch. Absent means the default, which is on.
INSERT INTO progression.personalisation (candidate_id, enabled)
VALUES (sqlc.arg(candidate_id)::uuid, sqlc.arg(enabled)::boolean)
ON CONFLICT (candidate_id) DO UPDATE
SET enabled = EXCLUDED.enabled, updated_at = now();

-- name: PersonalisationEnabled :one
-- Defaults to on when the candidate has never expressed a preference.
SELECT coalesce(bool_and(enabled), true)::boolean AS enabled
FROM progression.personalisation;
