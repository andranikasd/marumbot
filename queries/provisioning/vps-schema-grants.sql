-- Existing application role, freshly restored/migrated database, owner session.
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO marum_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO marum_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO marum_app;
ALTER DEFAULT PRIVILEGES FOR ROLE marum_owner IN SCHEMA public
 GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO marum_app;
ALTER DEFAULT PRIVILEGES FOR ROLE marum_owner IN SCHEMA public
 GRANT USAGE, SELECT ON SEQUENCES TO marum_app;
REVOKE UPDATE, DELETE, TRUNCATE ON loan_events, billing_events FROM marum_app;
REVOKE DELETE, TRUNCATE ON loans FROM marum_app;
