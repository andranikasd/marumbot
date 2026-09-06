-- +goose Up
-- The loan list asks, per loan, whether any reported payment is still
-- unreconciled. Without these the planner walked every event the loan has ever
-- had and filtered by kind afterwards, so the cost of opening the loan list
-- grew with a borrower's payment history rather than with their loan count.
--
-- Both are partial: the two kinds below are a small minority of loan_events,
-- and a partial index keeps the write cost off every other event.
CREATE INDEX IF NOT EXISTS loan_events_reported_payments
ON loan_events (loan_id)
WHERE kind IN ('payment_reported', 'prepayment_reported');

CREATE INDEX IF NOT EXISTS loan_events_voids_by_loan
ON loan_events (loan_id, recorded_seq)
WHERE kind = 'entry_voided';

-- +goose Down
DROP INDEX IF EXISTS loan_events_voids_by_loan;
DROP INDEX IF EXISTS loan_events_reported_payments;
