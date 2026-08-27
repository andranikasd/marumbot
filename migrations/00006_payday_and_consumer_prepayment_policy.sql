-- +goose Up
-- +goose StatementBegin

-- The day of the month the borrower's money arrives. The planner uses it to
-- decide whether paying the surplus before the due date is worth anything:
-- under daily accrual it saves amount × rate × days, but only if the day is
-- known. Zero means not stated, which disables that part of the search.
ALTER TABLE budgets ADD COLUMN pay_day smallint NOT NULL DEFAULT 0
    CHECK (pay_day BETWEEN 0 AND 31);

-- Consumer credit in Armenia has a statutory prepayment rule. The Law on
-- Consumer Crediting, Article 10, lets the borrower prepay at any time with
-- no penalty, and the Central Bank's circular of 2 March 2015 requires an
-- early payment to be applied to the loan on the day it is made -- «հենց
-- վճարման կատարման օրն ուղղել վարկերի մարմանը» -- unless the borrower has
-- given a different written instruction. That is reduce_principal, by law,
-- for every consumer loan a bank or credit organisation issues.
--
-- Civil Code 358 stays as the fallback for anything that is not consumer
-- credit. New loans default to this policy because a borrower using a
-- Telegram bot to plan repayments is, overwhelmingly, a consumer.
INSERT INTO allocation_policy_versions (
    id, policy_key, version, definition, definition_schema_version,
    excess_rule, source_reference
) VALUES (
    '00000000-0000-4000-8000-000000002015',
    'am-consumer-credit-prepayment', 1,
    '{"order": ["costs", "interest", "principal"],
      "penalties": "unranked",
      "basis": "RA Law on Consumer Crediting Art. 10; CBA circular 2015-03-02"}'::jsonb,
    1,
    'reduce_principal',
    'https://www.cba.am/hy/circulars-notices/492/'
) ON CONFLICT DO NOTHING;

-- Loans recorded so far were created under the statutory fallback only
-- because nothing better existed. They are consumer loans; move them.
UPDATE loan_contract_versions
   SET allocation_policy_version_id = '00000000-0000-4000-8000-000000002015'
 WHERE allocation_policy_version_id = '00000000-0000-4000-8000-000000000358';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

UPDATE loan_contract_versions
   SET allocation_policy_version_id = '00000000-0000-4000-8000-000000000358'
 WHERE allocation_policy_version_id = '00000000-0000-4000-8000-000000002015';
DELETE FROM allocation_policy_versions WHERE id = '00000000-0000-4000-8000-000000002015';
ALTER TABLE budgets DROP COLUMN pay_day;

-- +goose StatementEnd
