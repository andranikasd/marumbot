-- +goose Up
-- +goose StatementBegin
ALTER TABLE budgets
    ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD COLUMN IF NOT EXISTS funding jsonb;

-- These are user declarations, not cached calculation outputs. The trigger
-- covers old chat commands as well as the complete Mini App configuration.
CREATE TABLE IF NOT EXISTS budget_versions (
    user_id uuid NOT NULL REFERENCES users(id),
    currency text NOT NULL,
    version bigint NOT NULL,
    declared_at timestamptz NOT NULL,
    facts jsonb NOT NULL,
    PRIMARY KEY (user_id, currency, version)
);
INSERT INTO budget_versions (user_id, currency, version, declared_at, facts)
SELECT user_id, currency, version, updated_at, to_jsonb(b) FROM budgets b
ON CONFLICT (user_id, currency, version) DO NOTHING;

CREATE OR REPLACE FUNCTION advance_budget_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.version := OLD.version + 1;
    RETURN NEW;
END;
$$;
CREATE OR REPLACE TRIGGER budget_version_advance BEFORE UPDATE ON budgets
FOR EACH ROW EXECUTE FUNCTION advance_budget_version();

CREATE OR REPLACE FUNCTION record_budget_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO budget_versions(user_id, currency, version, declared_at, facts)
    VALUES (NEW.user_id, NEW.currency, NEW.version, NEW.updated_at, to_jsonb(NEW));
    RETURN NEW;
END;
$$;
CREATE OR REPLACE TRIGGER budget_version_write AFTER INSERT OR UPDATE ON budgets
FOR EACH ROW EXECUTE FUNCTION record_budget_version();

CREATE OR REPLACE FUNCTION protect_budget_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'budget versions are immutable';
END;
$$;
CREATE OR REPLACE TRIGGER budget_version_immutable BEFORE UPDATE OR DELETE ON budget_versions
FOR EACH ROW EXECUTE FUNCTION protect_budget_version();
-- +goose StatementEnd

-- +goose Down
-- Version history must survive rollback; this migration is expand-only.
SELECT 1;
