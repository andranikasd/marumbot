-- name: ActiveActualBaselines
SELECT DISTINCT ON (p.currency) p.id::text,p.currency,a.created_at
FROM plan_activation_events a JOIN plan_versions p ON p.id=a.plan_id AND p.user_id=a.user_id
WHERE a.user_id=$1 ORDER BY p.currency,a.revision DESC;

-- name: PlanActualFacts
-- One selected calendar month; activation is pinned to an immutable event.
-- Includes pending facts solely for coverage; only posted dates are compared.
SELECT e.id::text,e.loan_id::text,
 coalesce(e.fact_payload->>'transaction_date',''),coalesce(e.value_date::text,''),
 e.amount_minor,e.fact_payload->'allocation',e.recorded_at >= $4::timestamptz
FROM loan_events e JOIN loans l ON l.id=e.loan_id
WHERE l.user_id=$1 AND l.currency=$2
AND e.kind IN ('payment_reported','prepayment_reported')
AND coalesce(e.value_date::text,e.fact_payload->>'transaction_date')>=to_char($3::date,'YYYY-MM-DD')
AND coalesce(e.value_date::text,e.fact_payload->>'transaction_date')<to_char($3::date+interval '1 month','YYYY-MM-DD')
AND NOT EXISTS(SELECT 1 FROM loan_events v WHERE v.voids_event_id=e.id)
ORDER BY e.recorded_seq,e.id LIMIT 10001;
