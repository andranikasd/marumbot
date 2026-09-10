-- A digest only: no financial amounts or identifiers leave this regression.
SELECT md5(
 coalesce((SELECT jsonb_agg(to_jsonb(t) ORDER BY id)::text FROM users t),'[]') ||
 coalesce((SELECT jsonb_agg(to_jsonb(t) ORDER BY id)::text FROM loans t),'[]') ||
 coalesce((SELECT jsonb_agg(to_jsonb(t) ORDER BY id)::text FROM loan_events t),'[]') ||
 coalesce((SELECT jsonb_agg(to_jsonb(t) ORDER BY id)::text FROM billing_events t),'[]') ||
 coalesce((SELECT jsonb_agg(to_jsonb(t) ORDER BY id)::text FROM loan_snapshots t),'[]') ||
 coalesce((SELECT jsonb_agg(to_jsonb(t) ORDER BY id)::text FROM plan_versions t),'[]')
);
