-- +goose Up
-- Borrower source declarations; unknown interest is not a zero-rate assertion.
CREATE TABLE IF NOT EXISTS loan_setup_sources (
 loan_id uuid PRIMARY KEY REFERENCES loans(id) ON DELETE CASCADE,
 interest_known boolean NOT NULL,
 projection_terms_confirmed boolean NOT NULL DEFAULT false,
 original_principal_minor bigint NOT NULL CHECK (original_principal_minor > 0),
 accrued_interest_minor bigint CHECK(accrued_interest_minor>=0),
 snapshot_id uuid REFERENCES loan_snapshots(id) ON DELETE CASCADE,
 source_date date NOT NULL
);
ALTER TABLE loan_setup_sources ADD COLUMN IF NOT EXISTS accrued_interest_minor bigint CHECK(accrued_interest_minor>=0);
ALTER TABLE loan_setup_sources ADD COLUMN IF NOT EXISTS snapshot_id uuid REFERENCES loan_snapshots(id) ON DELETE CASCADE;
CREATE OR REPLACE TRIGGER loan_setup_source_mutation_version AFTER INSERT OR UPDATE ON loan_setup_sources
 FOR EACH ROW EXECUTE FUNCTION invalidate_loan_mutation_version();
-- +goose Down
SELECT 1;
