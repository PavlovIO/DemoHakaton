-- +goose Up
-- Лента сортируется по created_at DESC, id DESC (уведомления одной транзакции получают
-- одинаковый now()). Без id в индексе Postgres досортировывает каждую страницу.
CREATE INDEX notifications_feed_idx ON notifications (user_id, created_at DESC, id DESC);
DROP INDEX notifications_user_idx;

-- +goose Down
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
DROP INDEX notifications_feed_idx;
