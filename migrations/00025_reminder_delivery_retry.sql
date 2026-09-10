-- +goose Up
-- Operational retry state, independent of the borrower's reminder date.
ALTER TABLE reminder_occurrences ADD COLUMN delivery_attempts integer NOT NULL DEFAULT 0,
 ADD COLUMN retry_at timestamptz NOT NULL DEFAULT '-infinity';
CREATE INDEX reminder_retry_due ON reminder_occurrences (retry_at,target_send_at,id)
 WHERE status='scheduled';
-- +goose Down
DROP INDEX reminder_retry_due;
ALTER TABLE reminder_occurrences DROP COLUMN retry_at, DROP COLUMN delivery_attempts;
