-- +goose Up
-- Original source manifests and activation intent; no cached calculation rows.
CREATE TABLE IF NOT EXISTS plan_versions (
 id uuid PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 currency text NOT NULL,
 manifest jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS plan_activation_events (
 id uuid PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 plan_id uuid NOT NULL REFERENCES plan_versions(id) ON DELETE CASCADE,
 revision bigint NOT NULL,
 idempotency_key text NOT NULL,
 proposal text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(user_id,revision),UNIQUE(user_id,idempotency_key)
);
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_plan_history_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' AND pg_trigger_depth()>1 THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'plan history is immutable'; END;
$$;
CREATE OR REPLACE TRIGGER plan_versions_immutable BEFORE UPDATE OR DELETE ON plan_versions
FOR EACH ROW EXECUTE FUNCTION protect_plan_history_update();
CREATE OR REPLACE TRIGGER plan_activation_immutable BEFORE UPDATE OR DELETE ON plan_activation_events
FOR EACH ROW EXECUTE FUNCTION protect_plan_history_update();
CREATE OR REPLACE TRIGGER plan_versions_no_truncate BEFORE TRUNCATE ON plan_versions
FOR EACH STATEMENT EXECUTE FUNCTION protect_plan_history_update();
CREATE OR REPLACE TRIGGER plan_activation_no_truncate BEFORE TRUNCATE ON plan_activation_events
FOR EACH STATEMENT EXECUTE FUNCTION protect_plan_history_update();
-- +goose StatementEnd
-- +goose Down
SELECT 1;
