-- +goose Up
-- +goose StatementBegin

-- A loan is identified by what the borrower calls it, not by which bank issued
-- it. Marum does not need the lender's name to do arithmetic, and a name it
-- does not need is a name it should not hold: the less that sits beside a
-- balance, the less a leak of that balance reveals.
--
-- Expand only. The lender column keeps its existing rows and stops being
-- written; a later migration removes it once nothing reads it.
ALTER TABLE loans ADD COLUMN IF NOT EXISTS description text;

COMMENT ON COLUMN loans.name IS
    'What the borrower calls this loan. Their words, not the lender''s.';
COMMENT ON COLUMN loans.description IS
    'Optional note from the borrower. Never a bank name or an account number.';
COMMENT ON COLUMN loans.lender IS
    'DEPRECATED. Marum no longer collects the lender. Retained for existing '
    'rows only; drop once nothing reads it.';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE loans DROP COLUMN IF EXISTS description;

-- +goose StatementEnd
