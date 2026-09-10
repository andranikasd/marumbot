-- Fresh VPS database only; executed by the owner, never by the application.
\getenv app_password MARUM_DB_PASSWORD
CREATE ROLE marum_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
SELECT format('ALTER ROLE marum_app PASSWORD %L', :'app_password') \gexec
GRANT CONNECT ON DATABASE marum TO marum_app;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO marum_app;
ALTER DEFAULT PRIVILEGES FOR ROLE marum_owner IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO marum_app;
ALTER DEFAULT PRIVILEGES FOR ROLE marum_owner IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO marum_app;
