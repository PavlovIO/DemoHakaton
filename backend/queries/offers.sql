-- name: CreateOffer :one
INSERT INTO offers (
    application_id, lender_organization_id, created_by_user_id, status,
    amount, currency, interest_rate_annual, fee_percent, repayment_schedule,
    collateral, additional_terms, valid_until
) VALUES (
    $1, $2, $3, 'sent', $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: GetOfferVisible :one
-- Оффер, если пользователь (viewer_role, viewer_org) его видит: кредитор — свой, заёмщик — на
-- свою заявку, admin — любой. Чужой и несуществующий одинаково дают 0 строк → 404 (IDOR).
-- Правило и колонки совпадают с ListOffersVisible и ListApplicationOffersVisible.
-- borrower_organization_id — получатель уведомлений о судьбе оффера (BE-08, BE-10).
SELECT sqlc.embed(o),
       a.number AS application_number,
       a.borrower_organization_id,
       lo.name AS lender_name,
       lo.country_code AS lender_country_code,
       -- Непрочитанные viewer_user: чужие сообщения после его указателя message_reads.
       -- Подзапрос в выдаче, а не отдельный счётчик на каждую строку: список без N+1.
       (SELECT count(*) FROM messages m
         WHERE m.offer_id = o.id AND m.author_user_id <> sqlc.arg('viewer_user')::uuid
           AND m.id > COALESCE((SELECT r.last_read_message_id FROM message_reads r
                                 WHERE r.offer_id = o.id AND r.user_id = sqlc.arg('viewer_user')::uuid), 0)
       ) AS unread_messages
FROM offers o
JOIN applications a   ON a.id  = o.application_id
JOIN organizations lo ON lo.id = o.lender_organization_id
WHERE o.id = sqlc.arg('id')
  -- Блок видимости — символ в символ во всех *OffersVisible (sqlc не умеет общие фрагменты).
  AND (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
  );

-- name: ListApplicationOffersVisible :many
-- Офферы заявки глазами зрителя: кредитор видит только свой (DECISIONS п. 11), заёмщик-
-- владелец и admin — все. Без пагинации: контракт отдаёт массив.
SELECT sqlc.embed(o),
       a.number AS application_number,
       a.borrower_organization_id,
       lo.name AS lender_name,
       lo.country_code AS lender_country_code,
       -- Непрочитанные viewer_user: чужие сообщения после его указателя message_reads.
       -- Подзапрос в выдаче, а не отдельный счётчик на каждую строку: список без N+1.
       (SELECT count(*) FROM messages m
         WHERE m.offer_id = o.id AND m.author_user_id <> sqlc.arg('viewer_user')::uuid
           AND m.id > COALESCE((SELECT r.last_read_message_id FROM message_reads r
                                 WHERE r.offer_id = o.id AND r.user_id = sqlc.arg('viewer_user')::uuid), 0)
       ) AS unread_messages
FROM offers o
JOIN applications a   ON a.id  = o.application_id
JOIN organizations lo ON lo.id = o.lender_organization_id
WHERE o.application_id = sqlc.arg('application_id')
  AND (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
  )
  -- text[], а не offer_status[]: pgx не знает OID массива enum (как в ListApplicationsVisible).
  AND (sqlc.narg('statuses')::text[] IS NULL OR o.status = ANY(CAST(sqlc.narg('statuses')::text[] AS offer_status[])))
ORDER BY o.created_at DESC, o.id DESC;

-- name: ListOffersVisible :many
-- Бизнес-фильтры — narg: они только сужают выборку и не могут её открыть.
SELECT sqlc.embed(o),
       a.number AS application_number,
       a.borrower_organization_id,
       lo.name AS lender_name,
       lo.country_code AS lender_country_code,
       -- Непрочитанные viewer_user: чужие сообщения после его указателя message_reads.
       -- Подзапрос в выдаче, а не отдельный счётчик на каждую строку: список без N+1.
       (SELECT count(*) FROM messages m
         WHERE m.offer_id = o.id AND m.author_user_id <> sqlc.arg('viewer_user')::uuid
           AND m.id > COALESCE((SELECT r.last_read_message_id FROM message_reads r
                                 WHERE r.offer_id = o.id AND r.user_id = sqlc.arg('viewer_user')::uuid), 0)
       ) AS unread_messages
FROM offers o
JOIN applications a   ON a.id  = o.application_id
JOIN organizations lo ON lo.id = o.lender_organization_id
WHERE true
  AND (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
  )
  -- text[], а не offer_status[]: pgx не знает OID массива enum (как в ListApplicationsVisible).
  AND (sqlc.narg('statuses')::text[] IS NULL OR o.status = ANY(CAST(sqlc.narg('statuses')::text[] AS offer_status[])))
  AND (sqlc.narg('application_id')::uuid IS NULL OR o.application_id = sqlc.narg('application_id')::uuid)
  -- Сервис передаёт только для admin.
  AND (sqlc.narg('lender_org_filter')::uuid IS NULL OR o.lender_organization_id = sqlc.narg('lender_org_filter')::uuid)
  AND (sqlc.narg('borrower_org_filter')::uuid IS NULL OR a.borrower_organization_id = sqlc.narg('borrower_org_filter')::uuid)
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR o.created_at >= sqlc.narg('created_from')::timestamptz)
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR o.created_at < sqlc.narg('created_to')::timestamptz)
ORDER BY o.created_at DESC, o.id DESC
LIMIT sqlc.arg('page_limit') OFFSET sqlc.arg('page_offset');

-- name: CountOffersVisible :one
SELECT count(*)
FROM offers o
JOIN applications a ON a.id = o.application_id
WHERE true
  AND (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
  )
  -- text[], а не offer_status[]: pgx не знает OID массива enum (как в ListApplicationsVisible).
  AND (sqlc.narg('statuses')::text[] IS NULL OR o.status = ANY(CAST(sqlc.narg('statuses')::text[] AS offer_status[])))
  AND (sqlc.narg('application_id')::uuid IS NULL OR o.application_id = sqlc.narg('application_id')::uuid)
  -- Сервис передаёт только для admin.
  AND (sqlc.narg('lender_org_filter')::uuid IS NULL OR o.lender_organization_id = sqlc.narg('lender_org_filter')::uuid)
  AND (sqlc.narg('borrower_org_filter')::uuid IS NULL OR a.borrower_organization_id = sqlc.narg('borrower_org_filter')::uuid)
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR o.created_at >= sqlc.narg('created_from')::timestamptz)
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR o.created_at < sqlc.narg('created_to')::timestamptz);

-- name: LockApplicationForLender :one
-- FOR SHARE на заявку при создании оффера: конфликтует с FOR UPDATE из cancel, reject-offers
-- (BE-07) и accept (BE-09) — оффер не появится на заявке, которую в этот момент отменяют или
-- принимают. Условие совпадает с видимостью заявки для lender в GetApplicationVisible:
-- невидимая — 0 строк → 404. Порядок блокировок общий: сначала заявка, потом офферы.
SELECT a.id, a.number, a.status, a.amount, a.currency, a.borrower_organization_id
FROM applications a
WHERE a.id = sqlc.arg('id')
  AND (a.status = 'published'
       OR EXISTS (SELECT 1 FROM offers f
                   WHERE f.application_id = a.id
                     AND f.lender_organization_id = sqlc.arg('lender_org')::uuid))
FOR SHARE OF a;

-- name: AcceptOffer :one
-- Вызывать внутри транзакции после GetApplicationForUpdate
UPDATE offers SET
    status = 'accepted',
    decided_at = now()
WHERE id = $1 AND status = 'sent'
RETURNING *;

-- name: RejectOffer :one
-- 0 строк после проверки статуса в сервисе — гонка с другим решением → 409.
UPDATE offers SET
    status = 'rejected',
    reject_reason = 'rejected_by_borrower',
    reject_comment = sqlc.narg('comment')::text,
    decided_at = now()
WHERE offers.id = sqlc.arg('id') AND offers.status = 'sent'
  -- Владелец в самом UPDATE (AGENTS.md, «Доступ»): только заёмщик заявки оффера.
  AND offers.application_id IN (SELECT ap.id FROM applications ap WHERE ap.borrower_organization_id = sqlc.arg('borrower_org')::uuid)
RETURNING *;

-- name: WithdrawOffer :one
UPDATE offers SET
    status = 'withdrawn',
    decided_at = now()
WHERE id = sqlc.arg('id') AND status = 'sent'
  AND lender_organization_id = sqlc.arg('lender_org')::uuid
RETURNING *;

-- name: RejectSentOffersByApplication :many
-- Каскадное отклонение активных офферов заявки (cancel, reject-offers; BE-09 — после AcceptOffer).
-- Вызывать после LockApplicationForBorrower в той же транзакции: сначала заявка, потом офферы.
-- RETURNING — для истории офферов и уведомлений их кредиторам.
UPDATE offers SET
    status = 'rejected',
    reject_reason = sqlc.arg('reason')::offer_reject_reason,
    decided_at = now()
WHERE application_id = sqlc.arg('application_id') AND status = 'sent'
RETURNING id, lender_organization_id;