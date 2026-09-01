-- +goose Up
-- +goose StatementBegin

-- Shadow mode (DK.1): the engine's recommendation, computed for a real
-- account and stored WITHOUT being acted on, so later bank cycles can be
-- compared row-by-row against what the engine said at the time.
--
-- This looks like persisting something recomputable, which the house rules
-- forbid -- it is not. Recomputing later runs a different engine version
-- against different inputs; the whole point of the row is to freeze what
-- THIS engine said on THIS day with THESE inputs, as evidence for the field
-- gates. The sheet is the full month-by-month projection, in minor units.
--
-- One row per account, day and goal: the walk runs hourly but the answer
-- can only change when the inputs do, and the unique key makes every rerun
-- after the first a no-op.
CREATE TABLE shadow_recommendations (
    id                uuid PRIMARY KEY,
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    computed_on       date NOT NULL,
    goal              text NOT NULL,
    engine            text NOT NULL,
    input_fingerprint text NOT NULL,
    sheet             jsonb NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, computed_on, goal)
);

CREATE INDEX shadow_recommendations_user_idx
    ON shadow_recommendations (user_id, computed_on DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE shadow_recommendations;
-- +goose StatementEnd
