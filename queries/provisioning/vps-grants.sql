-- Owner-only repair for an existing VPS whose initial provisioning was skipped.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO marum_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO marum_app;
REVOKE UPDATE, DELETE, TRUNCATE ON loan_events, billing_events FROM marum_app;
REVOKE DELETE, TRUNCATE ON loans FROM marum_app;
