-- name: CreateMessage :one
INSERT INTO messages (offer_id, author_user_id, text)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListMessages :many
-- Для polling используем after_id. Если передать 0, вернет последние сообщения.
SELECT * FROM messages
WHERE offer_id = $1 AND id > $2
ORDER BY id ASC
LIMIT $3;

-- name: MarkMessagesRead :exec
-- Upsert: обновляем last_read_message_id, только если новое значение больше старого
INSERT INTO message_reads (offer_id, user_id, last_read_message_id)
VALUES ($1, $2, $3)
ON CONFLICT (offer_id, user_id) DO UPDATE
SET last_read_message_id = GREATEST(message_reads.last_read_message_id, EXCLUDED.last_read_message_id);

-- name: CountUnreadMessages :one
SELECT count(*) FROM messages m
LEFT JOIN message_reads r ON r.offer_id = m.offer_id AND r.user_id = $2
WHERE m.offer_id = $1
  AND m.author_user_id <> $2
  AND (r.last_read_message_id IS NULL OR m.id > r.last_read_message_id);