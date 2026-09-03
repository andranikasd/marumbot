-- +goose Up
-- +goose StatementBegin
-- Approved policy declarations only. Reconciliation continues to update its
-- own cash facts; the existing immutable budget_versions captures both.
ALTER TABLE budgets ADD COLUMN IF NOT EXISTS policies jsonb NOT NULL DEFAULT '[]'::jsonb
 CHECK (jsonb_typeof(policies) = 'array');
-- +goose StatementEnd

-- +goose Down
-- Keep declarations and immutable history on rollback.
SELECT 1;
