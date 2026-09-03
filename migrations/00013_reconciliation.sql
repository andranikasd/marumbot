-- +goose Up
ALTER TABLE loan_snapshots ADD COLUMN IF NOT EXISTS observed_event_seq bigint,
 ADD COLUMN IF NOT EXISTS reconciliation_hash text;
-- +goose Down
-- Keep source statements and their idempotency evidence during rollback.
SELECT 1;
