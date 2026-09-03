-- +goose Up
-- Pending facts have no invented bank value date. Existing entries stay intact.
ALTER TABLE loan_events ALTER COLUMN value_date DROP NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS loan_event_single_void
ON loan_events (voids_event_id) WHERE kind = 'entry_voided';

-- +goose Down
-- Preserve payment facts and their unknown posting dates on rollback.
SELECT 1;
