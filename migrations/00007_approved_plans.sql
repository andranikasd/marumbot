-- +goose Up
-- +goose StatementBegin

-- The plan a borrower said yes to. One per account: approving a new one
-- replaces the old, because two active plans is a contradiction, not a
-- history. The figures are a snapshot for display; the engine recomputes
-- the living plan from the loans every time, and the snapshot lets the bot
-- say when reality has drifted from what was approved.
CREATE TABLE approved_plans (
    user_id       uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    goal          text NOT NULL CHECK (goal IN ('least_interest','fastest','relief','first_win')),
    cap_minor     bigint NOT NULL DEFAULT 0,
    policy        text NOT NULL,
    engine        text NOT NULL,
    payoff_date   date NOT NULL,
    months        integer NOT NULL,
    interest_minor bigint NOT NULL,
    approved_at   timestamptz NOT NULL DEFAULT now()
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE approved_plans;
-- +goose StatementEnd
