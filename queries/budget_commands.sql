-- name: BudgetCommandReceipt
SELECT request_hash, budget_version FROM budget_command_receipts
WHERE user_id=$1 AND idempotency_key=$2;

-- name: InsertBudgetCommandReceipt
INSERT INTO budget_command_receipts(user_id,idempotency_key,request_hash,budget_version)
VALUES($1,$2,$3,$4);

-- name: UpdateBudgetFunding
UPDATE budgets SET pay_day=$4,
 funding=jsonb_set(jsonb_set(funding,'{monthly_minor}',to_jsonb($5::bigint)),'{events}',$6::jsonb),
 updated_at=now()
WHERE user_id=$1 AND currency=$2 AND version=$3 AND funding IS NOT NULL
RETURNING version;
