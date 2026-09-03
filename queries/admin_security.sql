-- name: AdminIdentityByUsername
SELECT id,username,password_hash,totp_secret,roles,version,enabled FROM admin_identities WHERE username=$1;

-- name: AdminIdentity
SELECT id,username,password_hash,totp_secret,roles,version,enabled FROM admin_identities WHERE id=$1 FOR SHARE;

-- name: BootstrapAdminIdentity
-- An advisory transaction lock serializes concurrent bootstrap attempts.
WITH guard AS MATERIALIZED (SELECT pg_advisory_xact_lock(714152015)), inserted AS (
 INSERT INTO admin_identities(id,username,password_hash,roles,version,enabled,bootstrap)
 SELECT $1,$2,$3,ARRAY['administrator']::text[],1,true,true FROM guard
 WHERE NOT EXISTS (SELECT 1 FROM admin_identities)
 ON CONFLICT DO NOTHING RETURNING id
) SELECT count(*) FROM inserted;

-- name: CreateAdminIdentity
INSERT INTO admin_identities(id,username,password_hash,totp_secret,roles,version,enabled)
VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING;

-- name: UpdateAdminIdentity
UPDATE admin_identities SET username=$2,password_hash=$3,totp_secret=$4,roles=$5,version=$6,enabled=$7
 WHERE id=$1 AND version=$8;

-- name: AppendAdminAudit
INSERT INTO admin_audit(actor_id,action,target,purpose,outcome,occurred_at) VALUES ($1,$2,$3,$4,$5,$6);

-- name: AdminAuditEvents
SELECT actor_id,action,target,purpose,outcome,occurred_at FROM admin_audit ORDER BY sequence DESC LIMIT 200;

-- name: AdminPolicy
SELECT payload FROM admin_policy_drafts WHERE id=$1 FOR UPDATE;

-- name: CreateAdminPolicy
WITH saved AS (
 INSERT INTO admin_policy_drafts(id,payload,revision) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING RETURNING id,payload,revision
) INSERT INTO admin_policy_history(id,payload,revision) SELECT id,payload,revision FROM saved;

-- name: UpdateAdminPolicy
WITH saved AS (
 UPDATE admin_policy_drafts SET payload=$2,revision=$3 WHERE id=$1 AND revision=$4 RETURNING id,payload,revision
) INSERT INTO admin_policy_history(id,payload,revision) SELECT id,payload,revision FROM saved;

-- name: ConsumeAdminOTP
UPDATE admin_identities SET last_otp_counter=$3 WHERE id=$1 AND version=$2 AND enabled AND last_otp_counter<$3;

-- name: AdminControl
SELECT kind,id,revision,body FROM admin_controls WHERE kind=$1 AND id=$2 FOR UPDATE;

-- name: CreateAdminControl
WITH saved AS (
 INSERT INTO admin_controls(kind,id,revision,body) VALUES($1,$2,$3,$4)
 ON CONFLICT DO NOTHING RETURNING kind,id,revision,body
) INSERT INTO admin_control_history SELECT kind,id,revision,body FROM saved;

-- name: UpdateAdminControl
WITH saved AS (
 UPDATE admin_controls SET revision=$3,body=$4 WHERE kind=$1 AND id=$2 AND revision=$5
 RETURNING kind,id,revision,body
) INSERT INTO admin_control_history SELECT kind,id,revision,body FROM saved;

-- name: AdminProfileFlag
SELECT body FROM admin_controls WHERE kind='profile_flag' AND id=$1;

-- name: AdminCaseLoan
SELECT id::text FROM loans WHERE id::text=$1 AND user_id::text=$2 FOR UPDATE;

-- name: AdminCaseEvidence
WITH candidates AS (
SELECT 'anchor' AS kind,s.id::text,s.as_of::text || ' — ' || s.trust AS label
FROM loan_snapshots s JOIN loans l ON l.id=s.loan_id
WHERE l.user_id::text=$1 AND l.id::text=$2
AND (s.trust IN ('bank_confirmed','imported_verified') OR s.observed_event_seq IS NOT NULL)
AND s.id=(SELECT sn.id FROM loan_snapshots sn WHERE sn.loan_id=l.id ORDER BY sn.as_of DESC,sn.captured_at DESC,sn.id DESC LIMIT 1)
AND s.contract_version_id=(SELECT c.id FROM loan_contract_versions c WHERE c.loan_id=l.id ORDER BY c.version DESC LIMIT 1)
AND NOT EXISTS(SELECT 1 FROM loan_events e WHERE e.loan_id=l.id AND e.recorded_seq>coalesce(s.observed_event_seq,0))
UNION ALL
SELECT 'corrected_event',r.id::text,'Replacement event ' || r.recorded_seq::text
FROM loan_events r JOIN loans l ON l.id=r.loan_id
WHERE l.user_id::text=$1 AND l.id::text=$2
AND r.kind IN ('payment_reported','prepayment_reported','bank_fee_reported')
AND NOT EXISTS(SELECT 1 FROM loan_events later_void WHERE later_void.loan_id=r.loan_id AND later_void.voids_event_id=r.id)
AND EXISTS (
 SELECT 1 FROM loan_events v JOIN loan_events original ON original.id=v.voids_event_id AND original.loan_id=v.loan_id
 WHERE v.loan_id=r.loan_id AND v.kind='entry_voided'
 AND ((r.source_command_id IS NOT NULL AND v.source_command_id=r.source_command_id)
 OR (coalesce(r.fact_payload->>'request_hash','')<>''
 AND v.fact_payload->>'request_hash'=r.fact_payload->>'request_hash'
 AND v.idempotency_key=r.idempotency_key || ':void'))
 AND v.recorded_seq+1=r.recorded_seq
 AND original.kind IN ('payment_reported','prepayment_reported','bank_fee_reported')
)
UNION ALL
SELECT DISTINCT 'policy_conclusion',p.id,p.payload->>'Key' || ' v' || (p.payload->>'Version')
FROM loans l JOIN LATERAL(SELECT cv.allocation_policy_version_id FROM loan_contract_versions cv WHERE cv.loan_id=l.id ORDER BY cv.version DESC LIMIT 1)c ON true
JOIN allocation_policy_versions av ON av.id=c.allocation_policy_version_id
JOIN admin_policy_drafts p ON p.payload->>'Key'=av.policy_key
WHERE l.user_id::text=$1 AND l.id::text=$2 AND p.payload->>'State'='published'
AND p.payload->>'Reviewer'<>p.payload->>'Author'
AND (p.payload->>'Version')::integer>=av.version
) SELECT EXISTS(SELECT 1 FROM candidates WHERE kind=$3 AND id=$4);

-- name: AdminIdentityDirectory
SELECT id,username,roles,version,enabled,(totp_secret<>'') FROM admin_identities ORDER BY username LIMIT 200;

-- name: AdminPolicyDirectory
SELECT payload FROM admin_policy_drafts ORDER BY payload->>'Key', (payload->>'Version')::integer DESC LIMIT 200;

-- name: AdminControlDirectory
SELECT kind,id,revision,body FROM admin_controls WHERE kind=$1 ORDER BY id LIMIT 200;

-- name: AdminCaseEvidenceChoices
SELECT 'anchor' AS kind,s.id::text,s.as_of::text || ' — ' || s.trust AS label
FROM loan_snapshots s JOIN loans l ON l.id=s.loan_id
WHERE l.user_id::text=$1 AND l.id::text=$2
AND (s.trust IN ('bank_confirmed','imported_verified') OR s.observed_event_seq IS NOT NULL)
AND s.id=(SELECT sn.id FROM loan_snapshots sn WHERE sn.loan_id=l.id ORDER BY sn.as_of DESC,sn.captured_at DESC,sn.id DESC LIMIT 1)
AND s.contract_version_id=(SELECT c.id FROM loan_contract_versions c WHERE c.loan_id=l.id ORDER BY c.version DESC LIMIT 1)
AND NOT EXISTS(SELECT 1 FROM loan_events e WHERE e.loan_id=l.id AND e.recorded_seq>coalesce(s.observed_event_seq,0))
UNION ALL
SELECT 'corrected_event',r.id::text,'Replacement event ' || r.recorded_seq::text
FROM loan_events r JOIN loans l ON l.id=r.loan_id
WHERE l.user_id::text=$1 AND l.id::text=$2
AND r.kind IN ('payment_reported','prepayment_reported','bank_fee_reported')
AND NOT EXISTS(SELECT 1 FROM loan_events later_void WHERE later_void.loan_id=r.loan_id AND later_void.voids_event_id=r.id)
AND EXISTS (
 SELECT 1 FROM loan_events v JOIN loan_events original ON original.id=v.voids_event_id AND original.loan_id=v.loan_id
 WHERE v.loan_id=r.loan_id AND v.kind='entry_voided'
 AND ((r.source_command_id IS NOT NULL AND v.source_command_id=r.source_command_id)
 OR (coalesce(r.fact_payload->>'request_hash','')<>''
 AND v.fact_payload->>'request_hash'=r.fact_payload->>'request_hash'
 AND v.idempotency_key=r.idempotency_key || ':void'))
 AND v.recorded_seq+1=r.recorded_seq
 AND original.kind IN ('payment_reported','prepayment_reported','bank_fee_reported')
)
UNION ALL
SELECT DISTINCT 'policy_conclusion',p.id,p.payload->>'Key' || ' v' || (p.payload->>'Version')
FROM loans l JOIN LATERAL(SELECT cv.allocation_policy_version_id FROM loan_contract_versions cv WHERE cv.loan_id=l.id ORDER BY cv.version DESC LIMIT 1)c ON true
JOIN allocation_policy_versions av ON av.id=c.allocation_policy_version_id
JOIN admin_policy_drafts p ON p.payload->>'Key'=av.policy_key
WHERE l.user_id::text=$1 AND l.id::text=$2 AND p.payload->>'State'='published'
AND p.payload->>'Reviewer'<>p.payload->>'Author'
AND (p.payload->>'Version')::integer>=av.version LIMIT 200;
