-- name: CreateNotification :one
INSERT INTO notifications (user_id, type, title, entity_type, entity_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateNotificationsForAdmins :execrows
-- Уведомление каждому активному админу (BE-11, шаг 1). Явные ::type обязательны:
-- в INSERT … SELECT sqlc не выводит тип параметра из колонки.
INSERT INTO notifications (user_id, type, title, entity_type, entity_id)
SELECT u.id, sqlc.arg('type')::notification_type, sqlc.arg('title')::text,
       sqlc.arg('entity_type')::entity_type, sqlc.arg('entity_id')::text
FROM users u
WHERE u.role = 'admin' AND u.status = 'active';

-- name: CreateNotificationsForOrganization :execrows
-- Уведомление каждому активному пользователю компании (BE-11, шаг 1).
INSERT INTO notifications (user_id, type, title, entity_type, entity_id)
SELECT u.id, sqlc.arg('type')::notification_type, sqlc.arg('title')::text,
       sqlc.arg('entity_type')::entity_type, sqlc.arg('entity_id')::text
FROM users u
WHERE u.organization_id = sqlc.arg('organization_id')::uuid AND u.status = 'active';

-- name: ListNotifications :many
SELECT * FROM notifications
WHERE user_id = $1
  AND (sqlc.narg('unread_only')::boolean IS NOT TRUE OR is_read = false)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications
WHERE user_id = $1 AND is_read = false;

-- name: MarkNotificationRead :exec
UPDATE notifications
SET is_read = true
WHERE id = $1 AND user_id = $2;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications
SET is_read = true
WHERE user_id = $1 AND is_read = false;