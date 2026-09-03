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
