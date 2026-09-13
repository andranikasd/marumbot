-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS reminders_enabled boolean NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS reminder_lead_days smallint CHECK (reminder_lead_days BETWEEN 0 AND 7);
ALTER TABLE users ADD COLUMN IF NOT EXISTS reminder_minute smallint CHECK (reminder_minute BETWEEN 0 AND 1439);
-- +goose Down
-- Expand-only: retain source preferences for older application rollback.
SELECT 1;
