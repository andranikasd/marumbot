-- Owner-only local diagnostic. Caller captures this privately; never log output.
SELECT json_build_object('enabled', enabled, 'enrolled', totp_secret <> '',
                         'password_hash', password_hash)
FROM admin_identities WHERE username = :'username';
