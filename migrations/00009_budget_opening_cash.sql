-- +goose Up
-- +goose StatementBegin

-- Money on hand that can go to loans, as the borrower stated it, with the
-- day they said it. Cash on hand decays -- it gets spent -- so the statement
-- carries its date and the planner only trusts it within the month it was
-- made; a January figure says nothing about March.
--
-- The per-month budget overrides live in the overrides column this table has
-- carried since 00002 ({"2026-12": 40000000}, minor units); this migration
-- only adds the opening-cash pair. Expand only.
ALTER TABLE budgets
    ADD COLUMN IF NOT EXISTS opening_cash_minor bigint NOT NULL DEFAULT 0
        CHECK (opening_cash_minor >= 0),
    ADD COLUMN IF NOT EXISTS opening_as_of date;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE budgets
    DROP COLUMN IF EXISTS opening_cash_minor,
    DROP COLUMN IF EXISTS opening_as_of;
-- +goose StatementEnd
