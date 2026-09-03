-- +goose Up
-- +goose StatementBegin
-- Account erasure removes its private declarations. A live account's source
-- history remains immutable; callers cannot use this exception for row edits.
ALTER TABLE budget_versions DROP CONSTRAINT IF EXISTS budget_versions_user_id_fkey;
ALTER TABLE budget_versions ADD CONSTRAINT budget_versions_user_id_fkey
 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

CREATE OR REPLACE FUNCTION protect_budget_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' AND NOT EXISTS (SELECT 1 FROM users WHERE id = OLD.user_id) THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'budget versions are immutable';
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- Preserve remaining source declarations and the account-erasure behavior.
SELECT 1;
