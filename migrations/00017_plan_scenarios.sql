-- +goose Up
CREATE TABLE IF NOT EXISTS plan_scenarios (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 id text NOT NULL CHECK (length(id)=64),
 declaration jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,id)
);
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_plan_scenario() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' AND NOT EXISTS (SELECT 1 FROM users WHERE id=OLD.user_id) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'plan scenarios are immutable';
END;
$$;
CREATE OR REPLACE TRIGGER plan_scenarios_immutable BEFORE UPDATE OR DELETE ON plan_scenarios
FOR EACH ROW EXECUTE FUNCTION protect_plan_scenario();
-- +goose StatementEnd
-- +goose Down
-- Preserve original user declarations.
SELECT 1;
