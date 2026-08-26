-- name: CountsOverview
SELECT
  (SELECT count(*) FROM users WHERE deleted_at IS NULL)                      AS users,
  (SELECT count(*) FROM loans WHERE archived_at IS NULL)                     AS loans,
  (SELECT count(*) FROM loan_events)                                         AS events,
  (SELECT count(*) FROM loan_snapshots)                                      AS snapshots,
  (SELECT count(*) FROM allocation_policy_versions)                          AS policies,
  (SELECT count(*) FROM telegram_commands WHERE status = 'pending')          AS commands_pending,
  (SELECT count(*) FROM telegram_commands WHERE status = 'dead')             AS commands_dead,
  (SELECT count(*) FROM notification_deliveries WHERE status = 'pending')    AS deliveries_pending,
  (SELECT count(*) FROM notification_deliveries WHERE status = 'dead')       AS deliveries_dead,
  (SELECT count(*) FROM reminder_occurrences WHERE status = 'scheduled')     AS occurrences_scheduled,
  -- Age of the oldest work that is actually DUE. Something scheduled for
  -- tomorrow is not a backlog, and reporting it as a negative age makes the
  -- watchdog threshold meaningless.
  (SELECT coalesce(greatest(0, extract(epoch FROM now() - min(next_attempt_at))), 0)::bigint
     FROM telegram_commands
    WHERE status = 'pending' AND next_attempt_at <= now())                   AS oldest_command_age_s,
  (SELECT coalesce(greatest(0, extract(epoch FROM now() - min(next_attempt_at))), 0)::bigint
     FROM notification_deliveries
    WHERE status = 'pending' AND next_attempt_at <= now())                   AS oldest_delivery_age_s;

-- name: ListUsers
SELECT u.id, u.locale, u.timezone, u.access_state, u.trial_ends_at, u.created_at, u.deleted_at,
       (SELECT count(*) FROM loans l WHERE l.user_id = u.id AND l.archived_at IS NULL) AS loan_count
FROM users u
ORDER BY u.created_at DESC
LIMIT $1;

-- name: ListLoans
SELECT l.id, l.user_id, l.name, l.lender, l.currency, l.created_at, l.archived_at,
       (SELECT count(*) FROM loan_events e WHERE e.loan_id = l.id)    AS event_count,
       (SELECT count(*) FROM loan_snapshots s WHERE s.loan_id = l.id) AS snapshot_count,
       st.reliability_state, st.principal_minor, st.balance_as_of
FROM loans l
LEFT JOIN loan_state st ON st.loan_id = l.id
ORDER BY l.created_at DESC
LIMIT $1;

-- name: GetLoan
SELECT l.id, l.user_id, l.name, l.lender, l.currency, l.next_event_seq, l.created_at, l.archived_at
FROM loans l WHERE l.id = $1;

-- name: ListContractsForLoan
SELECT id, version, effective_from, effective_until, nominal_rate, day_count, repayment_type,
       start_date, maturity_date, payment_day, scheduled_payment_minor, rounding_mode,
       rounding_unit_minor, allocation_policy_version_id
FROM loan_contract_versions WHERE loan_id = $1 ORDER BY version;

-- name: ListSnapshotsForLoan
SELECT id, as_of, captured_at, trust, principal_minor, accrued_interest_minor,
       unpaid_interest_minor, current_fees_minor, overdue_fees_minor, penalties_minor,
       overdue_principal_minor, advance_credit_minor, next_installment_minor,
       next_due_date, remaining_installments, source_note
FROM loan_snapshots WHERE loan_id = $1 ORDER BY as_of DESC, captured_at DESC;

-- name: ListEventsForLoan
SELECT e.id, e.recorded_seq, e.kind, e.value_date, e.recorded_at, e.amount_minor,
       e.bank_order, e.bank_reference, e.voids_event_id, e.contract_version_id,
       (SELECT count(*) FROM snapshot_event_coverage c WHERE c.event_id = e.id) > 0 AS covered
FROM loan_events e WHERE e.loan_id = $1
ORDER BY e.value_date, e.bank_order NULLS LAST, e.recorded_seq;

-- name: ListPolicies
SELECT id, policy_key, version, excess_rule, definition, source_reference, created_at
FROM allocation_policy_versions ORDER BY policy_key, version;

-- name: InsertPolicy
INSERT INTO allocation_policy_versions
  (id, policy_key, version, definition, definition_schema_version, excess_rule, source_reference)
VALUES ($1, $2, $3, $4, 1, $5, $6);

-- name: ListCommands
SELECT id, telegram_update_id, user_id, command_kind, status, attempts,
       next_attempt_at, lease_owner, lease_until, received_at, completed_at, last_error_code
FROM telegram_commands ORDER BY received_at DESC LIMIT $1;

-- name: ListDeliveries
SELECT id, user_id, delivery_kind, status, priority, scheduled_at, next_attempt_at,
       attempts, telegram_message_id, sent_at, last_error_code
FROM notification_deliveries ORDER BY scheduled_at DESC LIMIT $1;

-- name: ListReconciliationRuns
SELECT r.id, r.loan_id, r.principal_drift_minor, r.interest_drift_minor,
       r.fee_drift_minor, r.penalty_drift_minor, r.engine_version, r.created_at
FROM reconciliation_runs r ORDER BY r.created_at DESC LIMIT $1;

-- name: GetLoanState
SELECT loan_id, state_version, anchor_snapshot_id, balance_as_of, principal_minor,
       accrued_interest_minor, unpaid_interest_minor, current_fees_minor, overdue_fees_minor,
       penalties_minor, overdue_principal_minor, advance_credit_minor,
       reliability_state, reliability_reasons, engine_version, rebuilt_at
FROM loan_state WHERE loan_id = $1;

-- name: CoveredEventIDs
SELECT event_id FROM snapshot_event_coverage c
JOIN loan_events e ON e.id = c.event_id WHERE e.loan_id = $1;
