-- +goose Up
-- Preserve the existing requested-account erasure cascade, while rejecting
-- all ordinary mutations of financial facts (including empty-table TRUNCATE).
-- +goose StatementBegin
CREATE FUNCTION guard_financial_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' AND pg_trigger_depth() > 1 THEN
    IF TG_TABLE_NAME = 'loan_events' THEN
      IF NOT EXISTS (SELECT 1 FROM loans WHERE id = OLD.loan_id) THEN RETURN OLD; END IF;
    ELSIF TG_TABLE_NAME = 'billing_events' OR TG_TABLE_NAME = 'loans' THEN
      IF NOT EXISTS (SELECT 1 FROM users WHERE id = OLD.user_id) THEN RETURN OLD; END IF;
    END IF;
  END IF;
  RAISE EXCEPTION 'financial history is append-only' USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER loan_events_immutable BEFORE UPDATE OR DELETE ON loan_events
FOR EACH ROW EXECUTE FUNCTION guard_financial_history();
CREATE TRIGGER billing_events_immutable BEFORE UPDATE OR DELETE ON billing_events
FOR EACH ROW EXECUTE FUNCTION guard_financial_history();
CREATE TRIGGER loan_events_no_truncate BEFORE TRUNCATE ON loan_events
FOR EACH STATEMENT EXECUTE FUNCTION guard_financial_history();
CREATE TRIGGER billing_events_no_truncate BEFORE TRUNCATE ON billing_events
FOR EACH STATEMENT EXECUTE FUNCTION guard_financial_history();
CREATE TRIGGER loans_erasure_only BEFORE DELETE ON loans
FOR EACH ROW EXECUTE FUNCTION guard_financial_history();
CREATE FUNCTION guard_account_erasure() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.deletion_requested_at IS NULL THEN
    RAISE EXCEPTION 'account erasure requires a deletion request' USING ERRCODE = '55000';
  END IF;
  RETURN OLD;
END;
$$;
CREATE TRIGGER users_requested_erasure BEFORE DELETE ON users
FOR EACH ROW EXECUTE FUNCTION guard_account_erasure();
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='marum_app') THEN
    REVOKE UPDATE, DELETE, TRUNCATE ON loan_events, billing_events FROM marum_app;
    REVOKE DELETE, TRUNCATE ON loans FROM marum_app;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER users_requested_erasure ON users;
DROP FUNCTION guard_account_erasure();
DROP TRIGGER loans_erasure_only ON loans;
DROP TRIGGER billing_events_no_truncate ON billing_events;
DROP TRIGGER loan_events_no_truncate ON loan_events;
DROP TRIGGER billing_events_immutable ON billing_events;
DROP TRIGGER loan_events_immutable ON loan_events;
DROP FUNCTION guard_financial_history();
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='marum_app') THEN
    GRANT UPDATE, DELETE ON loan_events, billing_events TO marum_app;
    GRANT DELETE ON loans TO marum_app;
  END IF;
END $$;
-- +goose StatementEnd
