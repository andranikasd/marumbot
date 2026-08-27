-- +goose Up
-- +goose StatementBegin

-- The engine computes both repayment structures Armenian lenders offer, but the
-- schema only permitted one. Regulation 8/05 pt. 21(8) requires a contract to
-- state which of հավասարաչափ (անուիտետային) or ոչ հավասարաչափ (դիֆերենցված)
-- applies, so both must be storable.
ALTER TABLE loan_contract_versions
    DROP CONSTRAINT loan_contract_versions_repayment_type_check;

ALTER TABLE loan_contract_versions
    ADD CONSTRAINT loan_contract_versions_repayment_type_check
    CHECK (repayment_type IN ('annuity', 'declining'));

-- Every contract must name an allocation policy, and a loan filed by a borrower
-- who has not read their lender's small print cannot name a real one.
--
-- The default is the statutory fallback: RA Civil Code Article 358 orders a
-- short payment as the creditor's costs, then interest, then principal --
-- «եթե այլ համաձայնություն չի կայացվել», unless otherwise agreed. Armenian
-- banks do agree otherwise, and publish orders that put penalties ahead of
-- principal, so this is a starting point to be replaced per product rather than
-- a description of any particular lender.
--
-- excess_rule is requires_bank_request rather than reduce_principal on purpose.
-- Where a payment exceeds what is due, what the lender does with the remainder
-- is a term of that contract, and guessing produces a balance that silently
-- diverges from the bank's. Asking is the honest failure mode.
INSERT INTO allocation_policy_versions (
    id, policy_key, version, definition, definition_schema_version,
    excess_rule, source_reference
) VALUES (
    '00000000-0000-4000-8000-000000000358',
    'am-civil-code-358', 1,
    '{"order": ["costs", "interest", "principal"],
      "penalties": "unranked",
      "basis": "RA Civil Code Article 358, dispositive"}'::jsonb,
    1,
    'requires_bank_request',
    'https://www.arlis.am/hy/acts/32186'
) ON CONFLICT (policy_key, version) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DELETE FROM allocation_policy_versions WHERE policy_key = 'am-civil-code-358';

-- Reversible only while no declining contract exists, which is the point of
-- checking rather than dropping rows.
ALTER TABLE loan_contract_versions
    DROP CONSTRAINT loan_contract_versions_repayment_type_check;

ALTER TABLE loan_contract_versions
    ADD CONSTRAINT loan_contract_versions_repayment_type_check
    CHECK (repayment_type IN ('annuity'));

-- +goose StatementEnd
