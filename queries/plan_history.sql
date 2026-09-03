-- name: PlanSources
SELECT jsonb_build_object('timezone',timezone,
 'profile_flags',coalesce((SELECT jsonb_agg(jsonb_build_object('id',id,'revision',revision) ORDER BY id) FROM admin_controls WHERE kind='profile_flag'),'[]'::jsonb),
 'loans',coalesce((SELECT jsonb_agg(jsonb_build_object('loan',to_jsonb(l),
 'contract',(SELECT id FROM loan_contract_versions WHERE loan_id=l.id ORDER BY version DESC LIMIT 1),
 'snapshot',(SELECT id FROM loan_snapshots WHERE loan_id=l.id ORDER BY as_of DESC,captured_at DESC LIMIT 1)) ORDER BY l.id)
 FROM loans l WHERE l.user_id=$1 AND l.archived_at IS NULL),'[]'::jsonb),
 'budgets',coalesce((SELECT jsonb_agg(jsonb_build_object('currency',currency,'version',version) ORDER BY currency) FROM budgets WHERE user_id=$1),'[]'::jsonb)
)::text FROM users WHERE id=$1 AND deleted_at IS NULL;

-- name: LockPlanUser
-- Serialize commands without blocking source-history foreign-key checks.
SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR NO KEY UPDATE;
-- name: LockPlanLoans
SELECT id FROM loans WHERE user_id=$1 ORDER BY id FOR UPDATE;
-- name: LockPlanBudgets
SELECT currency FROM budgets WHERE user_id=$1 ORDER BY currency FOR UPDATE;
-- name: PlanActivationReceipt
SELECT plan_id::text,revision,proposal FROM plan_activation_events WHERE user_id=$1 AND idempotency_key=$2;
-- name: PlanActivationRevision
SELECT coalesce(max(revision),0)::bigint FROM plan_activation_events WHERE user_id=$1;
-- name: InsertPlanVersion
INSERT INTO plan_versions(id,user_id,currency,manifest) VALUES($1,$2,$3,$4) RETURNING id::text;
-- name: InsertPlanActivation
INSERT INTO plan_activation_events(id,user_id,plan_id,revision,idempotency_key,proposal)
VALUES($1,$2,$3,$4,$5,$6) RETURNING plan_id::text,revision;
-- name: ListPlanVersions
SELECT p.id::text,p.currency,p.manifest::text,p.created_at::text,
 p.id=(SELECT a.plan_id FROM plan_activation_events a JOIN plan_versions v ON v.id=a.plan_id WHERE a.user_id=$1 AND v.currency=p.currency ORDER BY a.revision DESC LIMIT 1)
FROM plan_versions p WHERE p.user_id=$1 ORDER BY p.created_at DESC,p.id DESC;
-- name: GetPlanVersion
SELECT p.id::text,p.currency,p.manifest::text,p.created_at::text FROM plan_versions p WHERE p.user_id=$1 AND p.id=$2;

-- name: ActivePlanVersions
SELECT p.id::text,p.currency,p.manifest::text,p.created_at::text,true
FROM plan_versions p JOIN (
 SELECT DISTINCT ON(v.currency) a.plan_id
 FROM plan_activation_events a JOIN plan_versions v ON v.id=a.plan_id
 WHERE a.user_id=$1 ORDER BY v.currency,a.revision DESC
) active ON active.plan_id=p.id WHERE p.user_id=$1;
