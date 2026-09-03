-- +goose Up
CREATE TABLE IF NOT EXISTS admin_identities (
 id text PRIMARY KEY,
 username text NOT NULL UNIQUE,
 password_hash text NOT NULL,
 totp_secret text NOT NULL DEFAULT '',
 roles text[] NOT NULL,
 version bigint NOT NULL CHECK (version > 0),
 enabled boolean NOT NULL,
 bootstrap boolean NOT NULL DEFAULT false,
 last_otp_counter bigint NOT NULL DEFAULT -1,
 CHECK (roles <@ ARRAY['support_reader','support_operator','financial_verifier','policy_publisher','operations','billing_operator','security_auditor','administrator']::text[])
);
CREATE UNIQUE INDEX IF NOT EXISTS admin_single_bootstrap ON admin_identities(bootstrap) WHERE bootstrap;
CREATE TABLE IF NOT EXISTS admin_audit (
 sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor_id text NOT NULL,
 action text NOT NULL,
 target text NOT NULL,
 purpose text NOT NULL CHECK (octet_length(purpose)<=512),
 outcome text NOT NULL,
 occurred_at timestamptz NOT NULL
);
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION reject_admin_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'admin audit is append-only';
END;
$$;
-- +goose StatementEnd
CREATE OR REPLACE TRIGGER admin_audit_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_audit
 FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_audit_mutation();
CREATE TABLE IF NOT EXISTS admin_policy_drafts (
 id text PRIMARY KEY,
 payload jsonb NOT NULL,
 revision bigint NOT NULL CHECK (revision>0)
);
CREATE TABLE IF NOT EXISTS admin_policy_history (
 id text NOT NULL, revision bigint NOT NULL, payload jsonb NOT NULL, PRIMARY KEY(id,revision)
);
CREATE OR REPLACE TRIGGER admin_policy_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_policy_history
 FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_audit_mutation();
CREATE TABLE IF NOT EXISTS admin_controls (
 kind text NOT NULL, id text NOT NULL, revision bigint NOT NULL CHECK(revision>0), body jsonb NOT NULL, PRIMARY KEY(kind,id)
);
CREATE TABLE IF NOT EXISTS admin_control_history (
 kind text NOT NULL, id text NOT NULL, revision bigint NOT NULL, body jsonb NOT NULL, PRIMARY KEY(kind,id,revision)
);
CREATE OR REPLACE TRIGGER admin_control_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_control_history
 FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_audit_mutation();
-- +goose Down
-- Expand-only: identities, evidence and audit history must survive rollback.
SELECT 1;
