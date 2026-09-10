-- +goose Up
CREATE INDEX plan_versions_history_page ON plan_versions(user_id,created_at DESC,id DESC);
-- +goose Down
DROP INDEX plan_versions_history_page;
