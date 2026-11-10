-- name: GetSkillState :one
SELECT state, updated_at
FROM skill_state
WHERE session_id = ? LIMIT 1;

-- name: EnsureSkillState :one
INSERT INTO skill_state (
    session_id,
    state,
    updated_at
) VALUES (
    ?,
    ?,
    strftime('%s', 'now')
) ON CONFLICT(session_id) DO UPDATE SET
    state = excluded.state,
    updated_at = excluded.updated_at
WHERE json_extract(skill_state.state, '$.status') = 'done'
RETURNING state;

-- name: UpsertSkillState :exec
INSERT INTO skill_state (
    session_id,
    state,
    updated_at
) VALUES (
    ?,
    ?,
    strftime('%s', 'now')
) ON CONFLICT(session_id) DO UPDATE SET
    state = excluded.state,
    updated_at = excluded.updated_at;

-- name: DeleteSkillState :exec
DELETE FROM skill_state
WHERE session_id = ?;
