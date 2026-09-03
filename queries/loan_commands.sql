-- name: LockLoanCommandUser
-- Serialize commands without blocking source-history foreign-key checks.
SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR NO KEY UPDATE;

-- name: LockLoanCommand
SELECT mutation_version FROM loans WHERE id=$1 AND user_id=$2 AND archived_at IS NULL FOR UPDATE;

-- name: LoanCommandVersion
SELECT mutation_version FROM loans WHERE id=$1 AND user_id=$2;

-- name: LoanCommandReceipt
SELECT loan_id,version,request_hash FROM loan_command_receipts WHERE user_id=$1 AND command_key=$2;

-- name: RecordLoanCommandReceipt
INSERT INTO loan_command_receipts(user_id,command_key,request_hash,loan_id,version) VALUES($1,$2,$3,$4,$5);

-- name: LoanCommandCurrency
-- Archived ownership is allowed only to decode a request for receipt replay.
SELECT l.currency FROM loans l JOIN users u ON u.id=l.user_id
WHERE l.id=$1 AND l.user_id=$2 AND u.deleted_at IS NULL;
