-- +goose Up
ALTER TABLE loans ADD COLUMN icon text NOT NULL DEFAULT 'bank'
    CHECK (icon IN ('bank', 'car', 'home', 'phone', 'document', 'wallet')),
    ADD COLUMN optional_excluded boolean NOT NULL DEFAULT false;

-- +goose Down
SELECT 1;
