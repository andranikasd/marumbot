-- Owner-only recovery status; no password hash or authenticator secret returned.
SELECT json_build_object('username', username, 'enabled', enabled)
FROM admin_identities WHERE bootstrap;
