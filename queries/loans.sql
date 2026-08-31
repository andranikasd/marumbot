-- name: CreateLoan
-- Records a loan, its first contract version, and the opening balance.
--
-- The balance enters as a SNAPSHOT, not as an event, because that is what it
-- is: a statement of what was owed on a date. Replay anchors on a snapshot and
-- applies events after it, so the opening figure is the anchor rather than a
-- fact competing with one.
--
-- The anchor date ($17) is the day the loan is FILED, not the day it started.
-- A loan that has been running is filed with what is owed now, and the schedule
-- must project from now: anchoring on the start date would re-accrue months
-- that have already been paid.
--
-- Its trust is 'user_entered', which is the honest grade for a number a
-- borrower typed off a piece of paper. Only a bank-confirmed snapshot resets
-- drift, so a loan filed this way is reported as indicative until the lender's
-- own figure arrives -- which is the whole reliability model working as
-- intended rather than an omission.
WITH new_loan AS (
    INSERT INTO loans (id, user_id, name, description, currency)
    SELECT $1, $2, $3, $4, $5
     WHERE (SELECT count(*) FROM loans WHERE user_id = $2 AND archived_at IS NULL) < $19
    RETURNING id
), new_contract AS (
    INSERT INTO loan_contract_versions (
        id, loan_id, version, effective_from,
        nominal_rate, day_count, repayment_type,
        start_date, maturity_date, payment_day,
        rounding_mode, rounding_unit_minor,
        allocation_policy_version_id,
        prepayment_policy, prepayment_schema_version
    )
    SELECT $6, id, 1, $7, $8, $9, $10, $7, $11, $12, $13, $14,
           (SELECT id FROM allocation_policy_versions
             WHERE policy_key = 'am-consumer-credit-prepayment' ORDER BY version DESC LIMIT 1),
           $18::jsonb, 1
      FROM new_loan
    RETURNING id AS contract_id, loan_id
), opening AS (
    INSERT INTO loan_snapshots (
        id, loan_id, contract_version_id, as_of, trust,
        principal_minor, source_note, idempotency_key
    )
    SELECT $15, loan_id, contract_id, $17, 'user_entered', $16,
           'opening balance as filed by the borrower', 'opening:' || loan_id::text
      FROM new_contract
    RETURNING loan_id
)
SELECT loan_id FROM opening;

-- name: ListLoansForUser
-- Everything the planner and the loan list need, newest first.
-- Dates and numerics are cast to text here rather than scanned as their own
-- types. They are only ever rendered into a message, and pgx will not scan a
-- date or a numeric into a string in binary format -- which failed at runtime,
-- on every /loans, with
--   cannot scan date (OID 1082) in binary format into *string
-- Casting in SQL states the intent once, where the shape is visible, instead of
-- converting in Go on the way back out.
SELECT l.id, l.name, coalesce(l.description, ''), l.currency,
       c.nominal_rate::text, c.repayment_type, c.day_count,
       c.start_date::text, c.maturity_date::text, c.payment_day,
       c.rounding_mode, c.rounding_unit_minor,
       s.principal_minor, s.as_of::text, s.trust,
       coalesce(p.excess_rule, 'unknown'),
       c.prepayment_policy::text
  FROM loans l
  JOIN LATERAL (
        SELECT * FROM loan_contract_versions v
         WHERE v.loan_id = l.id ORDER BY v.version DESC LIMIT 1
       ) c ON true
  LEFT JOIN allocation_policy_versions p ON p.id = c.allocation_policy_version_id
  LEFT JOIN LATERAL (
        SELECT * FROM loan_snapshots sn
         WHERE sn.loan_id = l.id ORDER BY sn.as_of DESC, sn.captured_at DESC LIMIT 1
       ) s ON true
 WHERE l.user_id = $1 AND l.archived_at IS NULL
 ORDER BY l.created_at DESC
 LIMIT $2;

-- name: SetBudget
-- One budget per user per currency. Allocating a dram budget across a dollar
-- loan needs an exchange rate and there is no validated source for one, so the
-- currency is part of the key rather than a conversion.
-- A pay day of zero means "not stated" and keeps whatever was stored, so the
-- text flow, which only asks for an amount, does not erase the form's answer.
INSERT INTO budgets (user_id, currency, monthly_amount_minor, overrides_schema_version, pay_day)
VALUES ($1, $2, $3, 1, $4)
ON CONFLICT (user_id, currency) DO UPDATE
   SET monthly_amount_minor = EXCLUDED.monthly_amount_minor,
       pay_day = CASE WHEN EXCLUDED.pay_day > 0 THEN EXCLUDED.pay_day ELSE budgets.pay_day END,
       updated_at = now()
RETURNING monthly_amount_minor;

-- name: GetBudget
SELECT currency, monthly_amount_minor, pay_day FROM budgets
 WHERE user_id = $1 ORDER BY monthly_amount_minor DESC LIMIT 1;

-- name: UpdateLoanForUser
-- The borrower's own edit. Scoped by user_id in the predicate rather than
-- checked beforehand: an ownership test that lives in a WHERE clause cannot be
-- forgotten by a caller, and a mismatched id updates nothing rather than
-- someone else's loan.
UPDATE loans SET name = $3, description = $4
 WHERE id = $1 AND user_id = $2 AND archived_at IS NULL
RETURNING id;

-- name: ArchiveLoanForUser
-- Removing a loan hides it; it does not destroy the ledger behind it. A balance
-- is only checkable because its events can be replayed, and a borrower who
-- deletes a loan by mistake would otherwise lose that permanently.
UPDATE loans SET archived_at = now()
 WHERE id = $1 AND user_id = $2 AND archived_at IS NULL
RETURNING id;

-- name: GetLoanForUser
SELECT l.id, l.name, coalesce(l.description, ''), l.currency,
       c.nominal_rate::text, c.repayment_type,
       c.start_date::text, c.maturity_date::text, c.payment_day,
       s.principal_minor
  FROM loans l
  JOIN LATERAL (
        SELECT * FROM loan_contract_versions v
         WHERE v.loan_id = l.id ORDER BY v.version DESC LIMIT 1
       ) c ON true
  LEFT JOIN LATERAL (
        SELECT * FROM loan_snapshots sn
         WHERE sn.loan_id = l.id ORDER BY sn.as_of DESC, sn.captured_at DESC LIMIT 1
       ) s ON true
 WHERE l.id = $1 AND l.user_id = $2 AND l.archived_at IS NULL;

-- name: EnsureDefaultReminders
-- Every loan gets reminders when it is filed. Three days before, and on the
-- day: enough warning to move money, and a nudge when it is actually due.
--
-- The unique key on (loan_id, offset_days) makes this idempotent, so running it
-- again for a loan that already has rules changes nothing.
INSERT INTO reminder_rules (id, loan_id, offset_days, send_at_local)
VALUES (gen_random_uuid(), $1, -3, '10:00'),
       (gen_random_uuid(), $1,  0, '10:00')
ON CONFLICT (loan_id, offset_days) DO NOTHING;

-- name: ScheduleReminders
-- Generates the occurrences due in the next fortnight.
--
-- The idempotency key is the loan, the due date and the offset, so a tick that
-- runs twice -- or two ticks racing -- produce one occurrence rather than two
-- reminders about the same payment. That property lives in the schema rather
-- than in the scheduler's memory, which is what makes it survive a restart.
INSERT INTO reminder_occurrences (
    id, user_id, loan_id, due_date, offset_days, target_send_at, idempotency_key
)
SELECT gen_random_uuid(), l.user_id, l.id, $1::date, r.offset_days,
       (($1::date + r.offset_days * interval '1 day')
         + r.send_at_local) AT TIME ZONE u.timezone,
       l.id::text || ':' || $1::text || ':' || r.offset_days::text
  FROM loans l
  JOIN users u ON u.id = l.user_id
  JOIN reminder_rules r ON r.loan_id = l.id AND r.enabled
 WHERE l.id = $2 AND l.archived_at IS NULL AND u.deleted_at IS NULL
ON CONFLICT (idempotency_key) DO NOTHING;

-- name: DueReminders
-- Occurrences whose moment has arrived and that nothing has taken yet.
SELECT o.id, o.user_id, o.loan_id, o.due_date::text, o.offset_days,
       l.name, l.currency
  FROM reminder_occurrences o
  JOIN loans l ON l.id = o.loan_id
 WHERE o.status = 'scheduled' AND o.target_send_at <= now()
   AND l.archived_at IS NULL
 ORDER BY o.target_send_at
 LIMIT $1;

-- name: MarkReminderSatisfied
UPDATE reminder_occurrences SET status = 'satisfied'
 WHERE id = $1 AND status = 'scheduled' RETURNING id;

-- name: CancelRemindersForLoan
UPDATE reminder_occurrences SET status = 'canceled'
 WHERE loan_id = $1 AND status = 'scheduled' RETURNING id;

-- name: ActiveLoanUsers
-- Every account that still owes something, for the reminder tick. Bounded
-- because the tick is bounded; the next tick is minutes away.
SELECT DISTINCT l.user_id
  FROM loans l
  JOIN users u ON u.id = l.user_id
 WHERE l.archived_at IS NULL AND u.deleted_at IS NULL AND u.access_state <> 'paused'
 LIMIT $1;

-- name: ApprovePlan
INSERT INTO approved_plans (user_id, goal, cap_minor, policy, engine, payoff_date, months, interest_minor)
VALUES ($1, $2, $3, $4, $5, $6::date, $7, $8)
ON CONFLICT (user_id) DO UPDATE
   SET goal = EXCLUDED.goal, cap_minor = EXCLUDED.cap_minor, policy = EXCLUDED.policy,
       engine = EXCLUDED.engine, payoff_date = EXCLUDED.payoff_date,
       months = EXCLUDED.months, interest_minor = EXCLUDED.interest_minor,
       approved_at = now()
RETURNING user_id;

-- name: ApprovedPlan
SELECT goal, cap_minor, policy, engine, payoff_date::text, months, interest_minor, approved_at
  FROM approved_plans WHERE user_id = $1;

-- name: ClearApprovedPlan
DELETE FROM approved_plans WHERE user_id = $1 RETURNING user_id;
