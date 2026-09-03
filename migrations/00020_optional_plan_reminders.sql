-- +goose Up
-- Delivery state references an immutable approved action. Amounts remain replayed.
ALTER TABLE reminder_occurrences
 ADD COLUMN approved_plan_id uuid REFERENCES plan_versions(id) ON DELETE CASCADE,
 ADD COLUMN plan_action_index integer;
ALTER TABLE reminder_occurrences ADD CONSTRAINT optional_reminder_reference
 CHECK ((approved_plan_id IS NULL AND plan_action_index IS NULL)
 OR (approved_plan_id IS NOT NULL AND plan_action_index IS NOT NULL AND plan_action_index>=0 AND offset_days=0));
CREATE UNIQUE INDEX optional_reminder_action_once
 ON reminder_occurrences(approved_plan_id,plan_action_index) WHERE approved_plan_id IS NOT NULL;

-- +goose Down
-- Expand-only: older required-reminder code ignores the additional reference.
SELECT 1;
