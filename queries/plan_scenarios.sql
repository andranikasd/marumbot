-- name: SavePlanScenario
INSERT INTO plan_scenarios(user_id,id,declaration) VALUES($1,$2,$3::jsonb)
ON CONFLICT(user_id,id) DO NOTHING;
-- name: GetPlanScenario
SELECT declaration::text FROM plan_scenarios WHERE user_id=$1 AND id=$2;
-- name: ListPlanScenarios
SELECT declaration::text FROM plan_scenarios WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT 100;
-- name: ApplyScenarioBudget
-- Same declaration shape as SetBudgetConfiguration, preserving unrelated facts.
-- The existing trigger appends budget_versions and advances version exactly once.
UPDATE budgets SET monthly_amount_minor=$4, pay_day=$5, reserve_floor_minor=$6,
 funding=$7::jsonb, policies=policies || $8::jsonb, overrides=$9::jsonb, updated_at=now()
WHERE user_id=$1 AND currency=$2 AND version=$3
RETURNING version;
-- name: ScenarioBudgetVersion
SELECT facts->>'currency', (facts->>'monthly_amount_minor')::bigint,
 (facts->>'pay_day')::integer, coalesce(facts->>'overrides','{}'),
 (facts->>'opening_cash_minor')::bigint, facts->>'opening_as_of',
 (facts->>'reserve_floor_minor')::bigint, facts->>'funding', version,
 coalesce(facts->>'policies','[]')
FROM budget_versions WHERE user_id=$1 AND currency=$2 AND version=$3;
