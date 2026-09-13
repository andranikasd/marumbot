-- name: ProjectionSettings
SELECT version,enabled,currencies FROM monthly_projection_settings
WHERE user_id=$1 ORDER BY version DESC LIMIT 1;

-- name: SaveProjectionSettings
INSERT INTO monthly_projection_settings(user_id,version,enabled,currencies)
SELECT $1,$2::bigint+1,$3,$4::jsonb
WHERE COALESCE((SELECT MAX(version) FROM monthly_projection_settings WHERE user_id=$1),0)=$2
RETURNING version;
