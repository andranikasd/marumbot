-- name: RecordPaidMonthStatement
-- A source statement anchors the post-payment principal without fabricating
-- payment events, cash spending or lender allocation. Existing event coverage
-- is untouched; the use case refuses unresolved payment facts under the lock.
WITH owned AS (
 SELECT id,next_event_seq-1 AS observed_event_seq FROM loans
 WHERE id=$1 AND user_id=$2 AND archived_at IS NULL
), contract AS (
 SELECT v.id FROM loan_contract_versions v JOIN owned ON owned.id=v.loan_id
 WHERE v.effective_from<=$4::date ORDER BY v.version DESC LIMIT 1
)
INSERT INTO loan_snapshots(id,loan_id,contract_version_id,as_of,trust,principal_minor,
 next_due_date,next_installment_minor,source_note,idempotency_key,observed_event_seq)
SELECT $3::uuid,owned.id,contract.id,$4::date,'user_entered',$5,$6::date,$7,
 'Required payments reported complete through '||$8::text,
 'paid-months:'||$3::text,owned.observed_event_seq FROM owned,contract
RETURNING id::text;
