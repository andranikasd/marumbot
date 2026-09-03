-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS settings_version bigint NOT NULL DEFAULT 0 CHECK (settings_version >= 0),
 ADD COLUMN IF NOT EXISTS quiet_enabled boolean NOT NULL DEFAULT false,
 ADD COLUMN IF NOT EXISTS quiet_start integer NOT NULL DEFAULT 1320 CHECK (quiet_start BETWEEN 0 AND 1439),
 ADD COLUMN IF NOT EXISTS quiet_end integer NOT NULL DEFAULT 480 CHECK (quiet_end BETWEEN 0 AND 1439);
-- +goose StatementBegin
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='users'::regclass AND conname='quiet_window_nonempty') THEN
  ALTER TABLE users ADD CONSTRAINT quiet_window_nonempty CHECK (NOT quiet_enabled OR quiet_start <> quiet_end);
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE reminder_occurrences ADD COLUMN IF NOT EXISTS preference_version bigint NOT NULL DEFAULT 0 CHECK (preference_version >= 0),
 ADD COLUMN IF NOT EXISTS snoozed boolean NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS user_preference_receipts (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 command_key text NOT NULL,
 payload text NOT NULL,
 result jsonb NOT NULL,
 PRIMARY KEY (user_id,command_key)
);
-- +goose Down
-- Expand-only: retained so rollback cannot discard user choices or retry receipts.
SELECT 1;
