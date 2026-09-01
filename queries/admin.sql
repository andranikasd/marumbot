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
       (u.deletion_requested_at IS NOT NULL) AS deletion_requested,
       (SELECT count(*) FROM loans l WHERE l.user_id = u.id AND l.archived_at IS NULL) AS loan_count
FROM users u
ORDER BY u.created_at DESC
LIMIT $1;

-- name: ListLoans
-- No l.lender: deprecated by 00004, retained only until a drop migration.
SELECT l.id, l.user_id, l.name, l.currency, l.created_at, l.archived_at,
       (SELECT count(*) FROM loan_events e WHERE e.loan_id = l.id)    AS event_count,
       (SELECT count(*) FROM loan_snapshots s WHERE s.loan_id = l.id) AS snapshot_count,
       st.reliability_state, st.principal_minor, st.balance_as_of
FROM loans l
LEFT JOIN loan_state st ON st.loan_id = l.id
ORDER BY l.created_at DESC
LIMIT $1;

-- name: GetLoan
SELECT l.id, l.user_id, l.name, coalesce(l.description, ''), l.currency, l.next_event_seq, l.created_at, l.archived_at
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

-- name: SetUserAccess
-- Pause or restore an account.
--
-- Paused rather than deleted: a suspended account keeps its ledger, so the
-- decision is reversible and the evidence for it survives. Deleting to
-- discipline someone destroys the record of why.
UPDATE users SET access_state = $2
 WHERE id = $1 AND deleted_at IS NULL
RETURNING id, access_state;

-- name: RequestUserDeletion
-- Marks an account for erasure without erasing it yet.
--
-- Two steps on purpose. Erasure is irreversible and the request is not, so a
-- mistake is recoverable for as long as the tombstone has not been honoured.
-- deletion_tombstones is what stops a restored backup resurrecting the account.
UPDATE users SET deletion_requested_at = now()
 WHERE id = $1 AND deleted_at IS NULL
RETURNING id;

-- name: DeleteUser
-- Honours a deletion request. Cascades remove the loans, events, identities
-- and everything else keyed to the account.
WITH tombstone AS (
    INSERT INTO deletion_tombstones (subject_hmac)
    SELECT encode(sha256(id::text::bytea), 'hex') FROM users WHERE id = $1
    ON CONFLICT DO NOTHING
    RETURNING subject_hmac
)
DELETE FROM users WHERE id = $1 RETURNING id;

-- name: ArchiveLoan
-- Hides a loan without destroying its ledger. A loan whose events are gone
-- cannot be replayed, and replay is the only thing that makes a balance
-- checkable.
UPDATE loans SET archived_at = now()
 WHERE id = $1 AND archived_at IS NULL
RETURNING id;

-- name: RestoreLoan
UPDATE loans SET archived_at = NULL WHERE id = $1 RETURNING id;

-- name: RenameLoan
UPDATE loans SET name = $2, description = $3 WHERE id = $1 RETURNING id;

-- name: ListCommandsDetailed
-- The command queue with enough detail to explain a stuck one: how many times
-- it has been tried, when it will be tried again, who holds it, and why it
-- last failed.
SELECT c.id, c.telegram_update_id, coalesce(c.user_id::text, ''), c.command_kind,
       c.status, c.attempts, c.received_at, c.next_attempt_at,
       coalesce(c.lease_owner, ''), coalesce(c.last_error_code, ''),
       c.completed_at,
       greatest(0, extract(epoch FROM now() - c.next_attempt_at))::bigint AS due_age_s
  FROM telegram_commands c
 WHERE ($1 = '' OR c.status = $1)
 ORDER BY c.received_at DESC
 LIMIT $2;

-- name: RetryCommand
-- Puts a dead command back in the queue. The attempt count is reset because the
-- operator is asserting the cause was fixed; leaving it would make one retry
-- exhaust the budget again immediately.
UPDATE telegram_commands
   SET status = 'pending', attempts = 0, next_attempt_at = now(),
       lease_owner = NULL, lease_token = NULL, lease_until = NULL
 WHERE id = $1 AND status IN ('dead', 'pending', 'leased')
RETURNING id;

-- name: PurgeDeadCommands
-- Removes every command that has exhausted its retries.
--
-- Dead is normally kept: a command that could not be processed is evidence
-- about a bug, and deleting it destroys the only record of what the user sent.
-- Purging is therefore an operator's decision, taken once the evidence has
-- been read -- which is why it is a button in the inbox and not a scheduled
-- job. Only 'dead' rows are touched; a pending or leased command is live work.
DELETE FROM telegram_commands WHERE status = 'dead'
RETURNING id;

-- name: UsersByDay
-- Sign-ups per day over the last two weeks, for the dashboard trend. Days
-- with no sign-up are absent; the read model fills them so the chart has a
-- bar per day rather than a gap that reads as missing data.
SELECT created_at::date AS day, count(*)
  FROM users
 WHERE created_at >= (now()::date - 13)
 GROUP BY 1 ORDER BY 1;

-- name: LoansByDay
SELECT created_at::date AS day, count(*)
  FROM loans
 WHERE created_at >= (now()::date - 13)
 GROUP BY 1 ORDER BY 1;

-- name: GetUserAdmin
SELECT u.id, u.locale, u.timezone, u.access_state, u.trial_ends_at, u.created_at, u.deleted_at,
       (u.deletion_requested_at IS NOT NULL) AS deletion_requested,
       (SELECT count(*) FROM loans l WHERE l.user_id = u.id AND l.archived_at IS NULL) AS loan_count
  FROM users u WHERE u.id = $1;

-- name: ListLoansByUser
SELECT l.id, l.user_id, l.name, l.currency, l.created_at, l.archived_at,
       (SELECT count(*) FROM loan_events e WHERE e.loan_id = l.id)    AS event_count,
       (SELECT count(*) FROM loan_snapshots s WHERE s.loan_id = l.id) AS snapshot_count,
       st.reliability_state, st.principal_minor, st.balance_as_of
  FROM loans l
  LEFT JOIN loan_state st ON st.loan_id = l.id
 WHERE l.user_id = $1
 ORDER BY l.archived_at NULLS FIRST, l.created_at DESC;

-- name: ListBudgetsForUser
SELECT currency, monthly_amount_minor, pay_day, updated_at
  FROM budgets WHERE user_id = $1 ORDER BY monthly_amount_minor DESC;

-- name: GetConversationState
SELECT state_name, updated_at FROM conversation_states WHERE user_id = $1;

-- name: CountCommandsByStatus
SELECT status, count(*) FROM telegram_commands GROUP BY status ORDER BY status;

-- name: CountDeliveriesByStatus
SELECT status, count(*) FROM notification_deliveries GROUP BY status ORDER BY status;

-- name: ListCommandsForUser
-- The latest commands one account sent, for the activity panel on its page.
SELECT id, telegram_update_id, user_id, command_kind, status, attempts,
       next_attempt_at, lease_owner, lease_until, received_at, completed_at, last_error_code
  FROM telegram_commands WHERE user_id = $1
 ORDER BY received_at DESC LIMIT $2;

-- name: ListDeliveriesForUser
-- The latest messages queued or sent to one account.
SELECT id, user_id, delivery_kind, status, priority, scheduled_at, next_attempt_at,
       attempts, telegram_message_id, sent_at, last_error_code
  FROM notification_deliveries WHERE user_id = $1
 ORDER BY scheduled_at DESC LIMIT $2;
