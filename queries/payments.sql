-- name: LockPaymentLoan
SELECT next_event_seq - 1, currency FROM loans
WHERE id = $1 AND user_id = $2 AND archived_at IS NULL FOR UPDATE;

-- name: PaymentReceipt
SELECT id::text, recorded_seq,
 CASE WHEN kind = 'entry_voided' THEN 'voided'
      WHEN value_date IS NULL THEN 'pending_bank_posting'
      ELSE 'needs_reconciliation' END,
 fact_payload->>'request_hash'
FROM loan_events WHERE loan_id = $1 AND idempotency_key = $2;

-- name: ActivePaymentEvent
SELECT EXISTS (SELECT 1 FROM loan_events e
 WHERE e.loan_id = $1 AND e.id = $2
 AND e.kind IN ('payment_reported','prepayment_reported')
 AND NOT EXISTS (SELECT 1 FROM loan_events v WHERE v.voids_event_id = e.id));

-- name: DuplicatePayment
SELECT EXISTS (SELECT 1 FROM loan_events e
 WHERE e.loan_id = $1 AND e.amount_minor = $2
 AND e.fact_payload->>'transaction_date' = $3
 AND e.value_date IS NOT DISTINCT FROM $4::date
 AND e.kind = $5 AND e.id::text <> $6
 AND NOT EXISTS (SELECT 1 FROM loan_events v WHERE v.voids_event_id = e.id));

-- name: AppendPayment
-- The application holds the loan row lock across all entries in a correction.
WITH sequence AS (
 UPDATE loans SET next_event_seq = next_event_seq + 1 WHERE id = $1
 RETURNING next_event_seq - 1 AS seq
), contract AS (
 SELECT id FROM loan_contract_versions WHERE loan_id = $1
 AND effective_from <= $5::date ORDER BY effective_from DESC, version DESC LIMIT 1
)
INSERT INTO loan_events (id, loan_id, contract_version_id, recorded_seq, kind,
 value_date, amount_minor, voids_event_id, idempotency_key, fact_payload, fact_schema_version)
SELECT $2::uuid, $1::uuid, contract.id, sequence.seq, $3, $4::date, $6,
 $7::uuid, $8, $9::jsonb, 2 FROM sequence, contract
RETURNING id::text, recorded_seq;

-- name: PaymentContext
SELECT id::text, name, currency, next_event_seq - 1 FROM loans
WHERE id = $1 AND user_id = $2 AND archived_at IS NULL;

-- name: ReconciliationReceipt
SELECT id::text, observed_event_seq, reconciliation_hash FROM loan_snapshots
WHERE loan_id=$1 AND idempotency_key=$2;

-- name: ReconciliationEligibility
SELECT NOT EXISTS(SELECT 1 FROM loan_events e WHERE e.loan_id=$1
 AND e.kind IN ('payment_reported','prepayment_reported')
 AND NOT EXISTS(SELECT 1 FROM loan_events v WHERE v.voids_event_id=e.id)
 AND (e.value_date IS NULL OR e.value_date>$2::date)),
 EXISTS(SELECT 1 FROM loan_contract_versions c WHERE c.loan_id=$1
 AND c.id=(SELECT id FROM loan_contract_versions WHERE loan_id=$1 AND effective_from<=$2::date ORDER BY effective_from DESC,version DESC LIMIT 1)
 AND c.effective_from<=$2::date AND ($3::date IS NULL OR (c.maturity_date>=$3::date
 AND extract(day from $3::date)=least(c.payment_day,extract(day from date_trunc('month',$3::date)+interval '1 month - 1 day')))));

-- name: ReconciliationBudget
-- The borrower states actual remaining cash and total period spending.
-- No payment amount is subtracted again. Explicit funding must already exist.
UPDATE budgets SET opening_cash_minor=$4, opening_as_of=$3::date,
 funding=jsonb_set(jsonb_set(jsonb_set(funding,'{spent_minor}',to_jsonb($5::bigint)),'{cash_through}',to_jsonb($3::text)),'{spent_period_start}',to_jsonb($7::text)),updated_at=now()
WHERE user_id=$1 AND currency=$2 AND version=$6 AND funding IS NOT NULL
RETURNING version;

-- name: ReconcilePaymentSnapshot
WITH sequence AS (
 UPDATE loans SET next_event_seq=next_event_seq+1 WHERE id=$1 RETURNING next_event_seq-1 AS seq
), contract AS (
 SELECT id FROM loan_contract_versions WHERE loan_id=$1 AND effective_from<=$3::date
 ORDER BY effective_from DESC,version DESC LIMIT 1
), snapshot AS (
 INSERT INTO loan_snapshots(id,loan_id,contract_version_id,as_of,trust,principal_minor,
 next_due_date,next_installment_minor,source_note,idempotency_key,observed_event_seq,reconciliation_hash)
 SELECT $2::uuid,$1::uuid,contract.id,$3::date,'user_entered',$4,$5::date,$6,
 'Borrower confirms posted payments are included; cash and spending restated',$7,sequence.seq,$8
 FROM sequence,contract RETURNING id,observed_event_seq
), coverage AS (
 INSERT INTO snapshot_event_coverage(snapshot_id,event_id)
 SELECT snapshot.id,e.id FROM snapshot,loan_events e WHERE e.loan_id=$1
 AND e.kind IN ('payment_reported','prepayment_reported') AND e.value_date<=$3::date
 AND NOT EXISTS(SELECT 1 FROM loan_events v WHERE v.voids_event_id=e.id)
 ON CONFLICT(event_id) DO NOTHING
)
SELECT id::text,observed_event_seq FROM snapshot;

-- name: PeriodReportedSpending
SELECT coalesce(sum(e.amount_minor),0)::bigint FROM loan_events e JOIN loans l ON l.id=e.loan_id
WHERE l.user_id=$1 AND l.currency=$2 AND e.kind IN ('payment_reported','prepayment_reported')
AND coalesce(e.fact_payload->>'transaction_date',e.value_date::text)>=$4::text
AND coalesce(e.fact_payload->>'transaction_date',e.value_date::text)<=$3::text
AND NOT EXISTS(SELECT 1 FROM loan_events v WHERE v.voids_event_id=e.id);

-- name: PaymentAllocations
SELECT e.id::text, e.fact_payload->'allocation'
FROM loan_events e JOIN loans l ON l.id=e.loan_id
WHERE l.user_id=$1 AND e.id::text=ANY($2::text[])
AND e.kind IN ('payment_reported','prepayment_reported')
AND e.fact_payload->'allocation' IS NOT NULL;

-- name: MonthlyPaymentActuals
-- Source transfers by transaction month, including pending posting. Never infer
-- missing splits or mix currencies. Voids/corrections apply across all months.
WITH facts AS (
 SELECT l.currency,e.amount_minor,e.value_date,
 CASE WHEN e.value_date IS NOT NULL THEN nullif(e.fact_payload->'allocation','null'::jsonb) END AS allocation
 FROM loan_events e JOIN loans l ON l.id=e.loan_id
 WHERE l.user_id=$1 AND e.kind IN ('payment_reported','prepayment_reported')
 AND coalesce(e.fact_payload->>'transaction_date',e.value_date::text)>=to_char($2::date,'YYYY-MM-DD')
 AND coalesce(e.fact_payload->>'transaction_date',e.value_date::text)<to_char($2::date+interval '1 month','YYYY-MM-DD')
 AND NOT EXISTS(SELECT 1 FROM loan_events v WHERE v.voids_event_id=e.id)
)
SELECT currency,count(*),count(allocation),count(*)-count(allocation),
 count(*) FILTER(WHERE value_date IS NULL),sum(amount_minor)::text,
 sum((allocation->>'principal_minor')::bigint)::text,
 sum((allocation->>'interest_minor')::bigint)::text,
 sum((allocation->>'fees_minor')::bigint)::text,
 coalesce(sum(amount_minor) FILTER(WHERE allocation IS NULL),0)::text
FROM facts GROUP BY currency ORDER BY currency;
