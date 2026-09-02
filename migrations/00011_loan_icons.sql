-- +goose Up
ALTER TABLE loans ADD COLUMN IF NOT EXISTS icon text NOT NULL DEFAULT 'bank'
    CHECK (icon IN ('bank', 'car', 'home', 'phone', 'document', 'wallet')),
    ADD COLUMN IF NOT EXISTS optional_excluded boolean NOT NULL DEFAULT false;

-- +goose Down
SELECT 1;
