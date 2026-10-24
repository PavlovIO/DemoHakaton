-- name: LockOfferChat :exec
-- Транзакционная блокировка чата оффера перед CreateMessage. id из IDENTITY выдаётся до коммита,
-- и без блокировки сообщение 10 может закоммититься после 11: клиент, уже получивший 11,
-- опрашивает с after_id = 11 и 10 не увидит никогда (а MarkMessagesRead сочтёт его прочитанным).
-- Под блокировкой сообщения одного оффера коммитятся в порядке id. Первый ключ — пространство
-- «чат», чтобы не пересекаться с другими advisory-блокировками.
SELECT pg_advisory_xact_lock(hashtext('chat'), hashtext(sqlc.arg('offer_id')::uuid::text));

-- name: CreateMessage :one
-- Только после GetOfferVisible в той же транзакции: запрос не проверяет, что автор — сторона оффера.
INSERT INTO messages (offer_id, author_user_id, text)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListMessagesVisible :many
-- Polling: сообщения с id > after_id по возрастанию; after_id = 0 → самые старые max_rows штук
-- (не последние: «последние N» потребовали бы ORDER BY id DESC, в MVP не нужно).
-- Правило видимости — то же, что у GetOfferVisible: чужой чат даёт 0 строк, но сервис
-- всё равно сначала зовёт GetOfferVisible, чтобы отличить 404 от пустого чата.
-- JOIN organizations внутренний: автор — всегда borrower или lender (admin не пишет).
SELECT sqlc.embed(m),
       u.role AS author_role,
       ao.id AS author_org_id, ao.name AS author_org_name, ao.country_code AS author_org_country_code
  FROM messages m
  JOIN offers o         ON o.id  = m.offer_id
  JOIN applications a   ON a.id  = o.application_id
  JOIN users u          ON u.id  = m.author_user_id
  JOIN organizations ao ON ao.id = u.organization_id
 WHERE m.offer_id = sqlc.arg('offer_id') AND m.id > sqlc.arg('after_id')::bigint
   AND (   sqlc.arg('viewer_role')::user_role = 'admin'
        OR (sqlc.arg('viewer_role')::user_role = 'lender'   AND o.lender_organization_id   = sqlc.arg('viewer_org')::uuid)
        OR (sqlc.arg('viewer_role')::user_role = 'borrower' AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid))
 ORDER BY m.id
 LIMIT sqlc.arg('max_rows');

-- name: MarkMessagesRead :exec
-- Только после GetOfferVisible: запрос не проверяет, что читатель — сторона оффера.
-- Upsert: обновляем last_read_message_id, только если новое значение больше старого
INSERT INTO message_reads (offer_id, user_id, last_read_message_id)
VALUES ($1, $2, $3)
ON CONFLICT (offer_id, user_id) DO UPDATE
SET last_read_message_id = GREATEST(message_reads.last_read_message_id, EXCLUDED.last_read_message_id);
