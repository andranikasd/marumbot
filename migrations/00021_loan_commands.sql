-- +goose Up
-- +goose StatementBegin
ALTER TABLE loans ADD COLUMN IF NOT EXISTS mutation_version bigint NOT NULL DEFAULT 1;
CREATE TABLE IF NOT EXISTS loan_command_receipts (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 command_key text NOT NULL CHECK(length(command_key) BETWEEN 16 AND 128),
 request_hash text NOT NULL,
 loan_id uuid NOT NULL REFERENCES loans(id) ON DELETE CASCADE,
 version bigint NOT NULL,
 PRIMARY KEY(user_id,command_key)
);
CREATE OR REPLACE FUNCTION advance_loan_mutation_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 NEW.mutation_version := OLD.mutation_version + 1;
 RETURN NEW;
END;
$$;
CREATE OR REPLACE TRIGGER loan_mutation_version BEFORE UPDATE ON loans
 FOR EACH ROW EXECUTE FUNCTION advance_loan_mutation_version();

-- Every appended source fact invalidates forms, including legacy/admin writers.
CREATE OR REPLACE FUNCTION invalidate_loan_mutation_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE loans SET mutation_version=mutation_version+1 WHERE id=NEW.loan_id;
 RETURN NEW;
END;
$$;
CREATE OR REPLACE TRIGGER loan_contract_mutation_version AFTER INSERT OR UPDATE ON loan_contract_versions
 FOR EACH ROW EXECUTE FUNCTION invalidate_loan_mutation_version();
CREATE OR REPLACE TRIGGER loan_snapshot_mutation_version AFTER INSERT ON loan_snapshots
 FOR EACH ROW EXECUTE FUNCTION invalidate_loan_mutation_version();
CREATE OR REPLACE TRIGGER loan_event_mutation_version AFTER INSERT ON loan_events
 FOR EACH ROW EXECUTE FUNCTION invalidate_loan_mutation_version();

CREATE OR REPLACE FUNCTION protect_loan_command_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' AND NOT EXISTS(SELECT 1 FROM users WHERE id=OLD.user_id) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'loan command receipts are immutable';
END;
$$;
CREATE OR REPLACE TRIGGER loan_command_receipt_immutable BEFORE UPDATE OR DELETE ON loan_command_receipts
 FOR EACH ROW EXECUTE FUNCTION protect_loan_command_receipt();
-- +goose StatementEnd

-- +goose Down
-- Preserve retry identities and monotonic versions through rollback/reapply.
SELECT 1;
