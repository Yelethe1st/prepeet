-- The profile's queries. sqlc generates the Go beside this file; ADR-0010
-- records why no SQL lives in Go source.

-- name: GetProfile :one
SELECT user_id::text AS user_id, disciplines, target_roles,
       coalesce(seniority, '')::text AS seniority,
       coalesce(career_context, '')::text AS career_context,
       coalesce(default_duration_minutes, 0)::integer AS default_duration_minutes,
       coalesce(default_style, '')::text AS default_style,
       coalesce(default_pressure, '')::text AS default_pressure,
       extended_time, captions, reduced_motion,
       coalesce(accessibility_notes, '')::text AS accessibility_notes,
       notify_product_updates, notify_practice_reminders,
       created_at, updated_at
FROM candidate.profiles
WHERE user_id = sqlc.arg(user_id)::uuid;

-- name: UpsertProfile :exec
-- One statement for create and update, because the row's existence is an
-- implementation detail: a candidate who never saved has the empty profile,
-- and their first save is not a different operation from their tenth.
INSERT INTO candidate.profiles
    (user_id, disciplines, target_roles, seniority, career_context,
     default_duration_minutes, default_style, default_pressure,
     extended_time, captions, reduced_motion, accessibility_notes,
     notify_product_updates, notify_practice_reminders)
VALUES (sqlc.arg(user_id)::uuid, sqlc.arg(disciplines)::text[], sqlc.arg(target_roles)::text[],
        nullif(sqlc.arg(seniority)::text, ''), nullif(sqlc.arg(career_context)::text, ''),
        nullif(sqlc.arg(default_duration_minutes)::integer, 0),
        nullif(sqlc.arg(default_style)::text, ''), nullif(sqlc.arg(default_pressure)::text, ''),
        sqlc.arg(extended_time)::boolean, sqlc.arg(captions)::boolean,
        sqlc.arg(reduced_motion)::boolean, nullif(sqlc.arg(accessibility_notes)::text, ''),
        sqlc.arg(notify_product_updates)::boolean, sqlc.arg(notify_practice_reminders)::boolean)
ON CONFLICT (user_id) DO UPDATE SET
    disciplines = excluded.disciplines,
    target_roles = excluded.target_roles,
    seniority = excluded.seniority,
    career_context = excluded.career_context,
    default_duration_minutes = excluded.default_duration_minutes,
    default_style = excluded.default_style,
    default_pressure = excluded.default_pressure,
    extended_time = excluded.extended_time,
    captions = excluded.captions,
    reduced_motion = excluded.reduced_motion,
    accessibility_notes = excluded.accessibility_notes,
    notify_product_updates = excluded.notify_product_updates,
    notify_practice_reminders = excluded.notify_practice_reminders,
    updated_at = now();
