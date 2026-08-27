-- +goose Up
-- +goose StatementBegin

-- A fourth day-count convention. Ardshinbank uses 365/366 for non-annuity
-- loans and Fast Bank names it in its general provisions, so a contract must
-- be able to record it. The engine already computes it; this lets it be
-- stored.
ALTER TABLE loan_contract_versions
    DROP CONSTRAINT loan_contract_versions_day_count_check;
ALTER TABLE loan_contract_versions
    ADD CONSTRAINT loan_contract_versions_day_count_check
    CHECK (day_count IN ('act365', 'act360', '30_360', 'act_act'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Reversible only while no contract names act_act, which the constraint
-- itself will refuse rather than silently strand a row.
ALTER TABLE loan_contract_versions
    DROP CONSTRAINT loan_contract_versions_day_count_check;
ALTER TABLE loan_contract_versions
    ADD CONSTRAINT loan_contract_versions_day_count_check
    CHECK (day_count IN ('act365', 'act360', '30_360'));

-- +goose StatementEnd
