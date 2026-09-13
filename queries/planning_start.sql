-- name: SetPlanningStart
UPDATE budgets SET funding=jsonb_set(funding,'{planning_start_month}',to_jsonb($4::text)), updated_at=now()
WHERE user_id=$1 AND currency=$2 AND version=$3 AND funding IS NOT NULL
RETURNING version;
