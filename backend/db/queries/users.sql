-- name: FindUser :one
-- Misses when the settings row is absent too, so an orphaned user falls
-- through to CreateUser and heals.
SELECT u.id
FROM users u
JOIN user_settings s ON s.user_id = u.id
WHERE u.clerk_user_id = $1;

-- name: CreateUser :one
-- DO UPDATE, not DO NOTHING, so RETURNING yields the row even when a
-- concurrent transaction inserted it after this statement's snapshot.
-- Postgres runs the settings CTE even though the outer SELECT never reads it.
WITH u AS (
    INSERT INTO users (clerk_user_id) VALUES ($1)
    ON CONFLICT (clerk_user_id) DO UPDATE SET clerk_user_id = EXCLUDED.clerk_user_id
    RETURNING id
), s AS (
    INSERT INTO user_settings (user_id) SELECT id FROM u
    ON CONFLICT (user_id) DO NOTHING
)
SELECT id FROM u;
