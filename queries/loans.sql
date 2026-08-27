-- name: CreateLoan
-- Records a loan, its first contract version, and the opening balance.
--
-- The balance enters as a SNAPSHOT, not as an event, because that is what it
-- is: a statement of what was owed on a date. Replay anchors on a snapshot and
-- applies events after it, so the opening figure is the anchor rather than a
-- fact competing with one.
--
-- Its trust is 'user_entered', which is the honest grade for a number a
-- borrower typed off a piece of paper. Only a bank-confirmed snapshot resets
-- drift, so a loan filed this way is reported as indicative until the lender's
-- own figure arrives -- which is the whole reliability model working as
-- intended rather than an omission.
WITH new_loan AS (
    INSERT INTO loans (id, user_id, name, lender, currency)
    VALUES ($1, $2, $3, $4, $5)
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
             WHERE policy_key = 'am-civil-code-358' ORDER BY version DESC LIMIT 1),
           '{}'::jsonb, 1
      FROM new_loan
    RETURNING id AS contract_id, loan_id
), opening AS (
    INSERT INTO loan_snapshots (
        id, loan_id, contract_version_id, as_of, trust,
        principal_minor, source_note, idempotency_key
    )
    SELECT $15, loan_id, contract_id, $7, 'user_entered', $16,
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
SELECT l.id, l.name, coalesce(l.lender, ''), l.currency,
       c.nominal_rate::text, c.repayment_type, c.day_count,
       c.start_date::text, c.maturity_date::text, c.payment_day,
       c.rounding_mode, c.rounding_unit_minor,
       s.principal_minor, s.as_of::text, s.trust
  FROM loans l
  JOIN LATERAL (
        SELECT * FROM loan_contract_versions v
         WHERE v.loan_id = l.id ORDER BY v.version DESC LIMIT 1
       ) c ON true
  LEFT JOIN LATERAL (
        SELECT * FROM loan_snapshots sn
         WHERE sn.loan_id = l.id ORDER BY sn.as_of DESC, sn.captured_at DESC LIMIT 1
       ) s ON true
 WHERE l.user_id = $1 AND l.archived_at IS NULL
 ORDER BY l.created_at DESC
 LIMIT $2;
