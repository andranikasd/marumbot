-- +goose Up
-- Only source choices are persisted. The history is append-only and does not
-- alter old budgets, receipts, loan events or replayable plan manifests.
CREATE TABLE IF NOT EXISTS monthly_projection_settings (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 version bigint NOT NULL CHECK(version>0),
 enabled boolean NOT NULL,
 currencies jsonb NOT NULL CHECK(jsonb_typeof(currencies)='object'),
 PRIMARY KEY(user_id,version)
);
CREATE OR REPLACE TRIGGER monthly_projection_settings_immutable BEFORE UPDATE OR DELETE
 ON monthly_projection_settings FOR EACH ROW EXECUTE FUNCTION protect_budget_version();
CREATE OR REPLACE TRIGGER monthly_projection_settings_no_truncate BEFORE TRUNCATE
 ON monthly_projection_settings FOR EACH STATEMENT EXECUTE FUNCTION protect_budget_version();
-- +goose Down
-- Expand-only: retain user declarations for forward recovery.
SELECT 1;
