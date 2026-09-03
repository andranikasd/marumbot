-- +goose Up
CREATE TABLE IF NOT EXISTS budget_command_receipts (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 idempotency_key text NOT NULL,
 request_hash text NOT NULL,
 budget_version bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,idempotency_key)
);
CREATE OR REPLACE TRIGGER budget_command_receipts_immutable
BEFORE UPDATE OR DELETE ON budget_command_receipts
FOR EACH ROW EXECUTE FUNCTION protect_plan_history_update();
CREATE OR REPLACE TRIGGER budget_command_receipts_no_truncate
BEFORE TRUNCATE ON budget_command_receipts
FOR EACH STATEMENT EXECUTE FUNCTION protect_plan_history_update();
-- +goose Down
SELECT 1;
