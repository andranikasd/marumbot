-- name: RecordLoanSetupSource
INSERT INTO loan_setup_sources(loan_id,interest_known,original_principal_minor,source_date,projection_terms_confirmed,accrued_interest_minor,snapshot_id)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: RecordLoanSetupStatement
INSERT INTO loan_snapshots(id,loan_id,contract_version_id,as_of,trust,principal_minor,next_due_date,next_installment_minor,source_note,idempotency_key,observed_event_seq,captured_at)
SELECT $3,l.id,c.id,$4,'user_entered',$5,$6::date,$7,
 'Current bank figures reported during setup','setup:'||l.id::text,l.next_event_seq-1,
 GREATEST(clock_timestamp(), (SELECT max(s.captured_at)+interval '1 microsecond' FROM loan_snapshots s WHERE s.loan_id=l.id))
FROM loans l JOIN loan_contract_versions c ON c.loan_id=l.id AND c.version=1
WHERE l.id=$1 AND l.user_id=$2
RETURNING id::text;

-- name: ConfirmLoanProjectionTerms
INSERT INTO loan_setup_sources(loan_id,interest_known,original_principal_minor,source_date,projection_terms_confirmed)
SELECT id,true,$3,$4,$5 FROM loans WHERE id=$1 AND user_id=$2 AND archived_at IS NULL
ON CONFLICT(loan_id) DO UPDATE SET projection_terms_confirmed=EXCLUDED.projection_terms_confirmed;

-- name: RecordLoanOpeningInterest
INSERT INTO loan_setup_sources(loan_id,interest_known,original_principal_minor,source_date,accrued_interest_minor,snapshot_id)
SELECT l.id,true,first.principal_minor,$4,$3,latest.id
FROM loans l
JOIN LATERAL(SELECT principal_minor FROM loan_snapshots WHERE loan_id=l.id ORDER BY as_of,captured_at LIMIT 1) first ON true
JOIN LATERAL(SELECT id FROM loan_snapshots WHERE loan_id=l.id AND as_of=$4::date ORDER BY captured_at DESC LIMIT 1) latest ON true
WHERE l.id=$1 AND l.user_id=$2 AND l.archived_at IS NULL
ON CONFLICT(loan_id) DO UPDATE SET source_date=EXCLUDED.source_date,accrued_interest_minor=EXCLUDED.accrued_interest_minor,snapshot_id=EXCLUDED.snapshot_id;
