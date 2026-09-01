-- name: RecordShadowRecommendation
-- Idempotent per account, day and goal: the shadow walk rides the hourly
-- tick, and only the first computation of a day writes. A changed input on
-- the same day does NOT overwrite -- the first answer of the day is the
-- evidence, and the next day's row captures the change.
INSERT INTO shadow_recommendations (
    id, user_id, computed_on, goal, engine, input_fingerprint, sheet
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (user_id, computed_on, goal) DO NOTHING
RETURNING id;

-- name: ShadowRecommendationDays
-- How many distinct days of shadow evidence an account has, and the span.
-- The field gates read this: DK.2 needs two consecutive bank cycles.
SELECT count(DISTINCT computed_on),
       coalesce(min(computed_on)::text, ''),
       coalesce(max(computed_on)::text, '')
  FROM shadow_recommendations WHERE user_id = $1;
