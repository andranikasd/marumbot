-- Owner-only recovery; new_hash is provided privately on psql stdin.
WITH changed AS (
  UPDATE admin_identities SET password_hash = :'new_hash', version = version + 1
  WHERE bootstrap AND enabled
  RETURNING id, username, (totp_secret <> '') AS enrolled
), audited AS (
  INSERT INTO admin_audit(actor_id, action, target, purpose, outcome, occurred_at)
  SELECT 'vps-root-recovery', 'password_recovered', id,
         'Operator requested forgotten password recovery via VPS root', 'allowed', CURRENT_TIMESTAMP
  FROM changed RETURNING target
)
SELECT json_build_object('username', changed.username, 'enrolled', changed.enrolled)
FROM changed JOIN audited ON audited.target = changed.id;
