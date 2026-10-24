-- name: CreateApplication :one
INSERT INTO applications (
    borrower_organization_id, created_by_user_id, status,
    amount, currency, target_country_code, purpose, term_months,
    transfer_method, transfer_method_comment
) VALUES (
    $1, $2, 'draft', $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetApplicationVisible :one
-- Заявка, если пользователь (viewer_role, viewer_org) её видит. Чужая и несуществующая
-- одинаково дают 0 строк → 404 (IDOR). Правило совпадает с ListApplicationsVisible.
-- Неизвестная роль не попадает ни в одну ветку; у admin viewer_org = uuid.Nil.
SELECT sqlc.embed(a),
       org.name AS borrower_name,
       org.country_code AS borrower_country_code,
       -- Активный (sent) оффер компании зрителя: не больше одного (offers_one_active_per_lender_uq).
       (SELECT o.id FROM offers o
         WHERE o.application_id = a.id AND o.status = 'sent'
           AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid)::uuid AS my_offer_id,
       -- Все офферы в любом статусе (DECISIONS п. 9): счётчик не обнуляется после agreed и reject-offers.
       (SELECT count(*) FROM offers o WHERE o.application_id = a.id) AS offers_count
FROM applications a
JOIN organizations org ON org.id = a.borrower_organization_id
WHERE a.id = sqlc.arg('id')
  AND (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND (a.status = 'published'
             OR EXISTS (SELECT 1 FROM offers o
                         WHERE o.application_id = a.id
                           AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid)))
  );

-- name: LockApplicationByOfferForBorrower :one
-- Первый шаг acceptOffer: блокирует заявку оффера, если она принадлежит заёмщику.
-- Статус намеренно не фильтруется: сервис отличает 404 (чужая) от 409 (уже agreed/cancelled).
SELECT a.* FROM applications a
  JOIN offers o ON o.application_id = a.id
 WHERE o.id = sqlc.arg('offer_id') AND a.borrower_organization_id = sqlc.arg('borrower_org')::uuid
   FOR UPDATE OF a;

-- name: UpdateApplication :one
UPDATE applications SET
    amount = COALESCE(sqlc.narg('amount')::numeric, amount),
    currency = COALESCE(sqlc.narg('currency')::char(3), currency),
    target_country_code = COALESCE(sqlc.narg('target_country_code')::char(2), target_country_code),
    purpose = COALESCE(sqlc.narg('purpose')::text, purpose),
    term_months = COALESCE(sqlc.narg('term_months')::integer, term_months),
    transfer_method = COALESCE(sqlc.narg('transfer_method')::text, transfer_method),
    -- NULL — не трогать, "" — очистить (DECISIONS п. 7).
    transfer_method_comment = CASE WHEN sqlc.narg('transfer_method_comment')::text IS NULL THEN transfer_method_comment
                                   ELSE NULLIF(sqlc.narg('transfer_method_comment')::text, '') END,
    updated_at = now()
WHERE id = $1 AND status = sqlc.arg('from_status')::application_status
RETURNING *;

-- name: SubmitApplication :one
UPDATE applications SET
    status = 'moderation',
    submitted_at = now(),
    moderation_comment = NULL,
    updated_at = now()
WHERE id = $1 AND status = sqlc.arg('from_status')::application_status
RETURNING *;

-- name: ApproveApplication :one
UPDATE applications SET
    status = 'published',
    published_at = now(),
    updated_at = now()
WHERE id = $1 AND status = sqlc.arg('from_status')::application_status
RETURNING *;

-- name: RejectApplication :one
UPDATE applications SET
    status = 'rejected',
    moderation_comment = $2,
    updated_at = now()
WHERE id = $1 AND status = sqlc.arg('from_status')::application_status
RETURNING *;

-- name: CancelApplication :one
UPDATE applications SET
    status = 'cancelled',
    updated_at = now()
WHERE id = $1 AND status = sqlc.arg('from_status')::application_status
RETURNING *;

-- name: AgreeApplication :one
UPDATE applications SET
    status = 'agreed',
    accepted_offer_id = $2,
    updated_at = now()
WHERE id = $1 AND status = sqlc.arg('from_status')::application_status
RETURNING *;

-- name: ListApplicationsVisible :many
-- Видимость — на обязательных arg (как в GetApplicationVisible); бизнес-фильтры — narg:
-- они только сужают выборку и не могут её открыть.
SELECT sqlc.embed(a),
       org.name AS borrower_name,
       org.country_code AS borrower_country_code,
       -- Активный (sent) оффер компании зрителя: не больше одного (offers_one_active_per_lender_uq).
       (SELECT o.id FROM offers o
         WHERE o.application_id = a.id AND o.status = 'sent'
           AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid)::uuid AS my_offer_id,
       -- Все офферы в любом статусе (DECISIONS п. 9): счётчик не обнуляется после agreed и reject-offers.
       (SELECT count(*) FROM offers o WHERE o.application_id = a.id) AS offers_count
FROM applications a
JOIN organizations org ON org.id = a.borrower_organization_id
WHERE true
  -- Блок видимости — символ в символ как в GetApplicationVisible и CountApplicationsVisible (sqlc не умеет общие фрагменты).
  AND (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND (a.status = 'published'
             OR EXISTS (SELECT 1 FROM offers o
                         WHERE o.application_id = a.id
                           AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid)))
  )
  -- text[], а не application_status[]: pgx не знает OID массива enum и не кодирует его
  -- без регистрации типа. Приведение в SQL оставляет сравнение по колонке (индекс по status).
  AND (sqlc.narg('statuses')::text[] IS NULL OR a.status = ANY(CAST(sqlc.narg('statuses')::text[] AS application_status[])))
  AND (sqlc.narg('target_countries')::text[] IS NULL OR a.target_country_code = ANY(sqlc.narg('target_countries')::text[]))
  AND (sqlc.narg('transfer_methods')::text[] IS NULL OR a.transfer_method = ANY(sqlc.narg('transfer_methods')::text[]))
  AND (sqlc.narg('currency')::char(3) IS NULL OR a.currency = sqlc.narg('currency')::char(3))
  AND (sqlc.narg('amount_min')::numeric IS NULL OR a.amount >= sqlc.narg('amount_min')::numeric)
  AND (sqlc.narg('amount_max')::numeric IS NULL OR a.amount <= sqlc.narg('amount_max')::numeric)
  AND (sqlc.narg('term_min')::int IS NULL OR a.term_months >= sqlc.narg('term_min')::int)
  AND (sqlc.narg('term_max')::int IS NULL OR a.term_months <= sqlc.narg('term_max')::int)
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR a.created_at >= sqlc.narg('created_from')::timestamptz)
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR a.created_at < sqlc.narg('created_to')::timestamptz)
  -- Сервис передаёт только для admin.
  AND (sqlc.narg('borrower_org_filter')::uuid IS NULL OR a.borrower_organization_id = sqlc.narg('borrower_org_filter')::uuid)
  -- Сервис передаёт только для lender: архив откликов (true) или заявки без моего оффера (false).
  AND (sqlc.narg('has_my_offer')::boolean IS NULL
       OR EXISTS (SELECT 1 FROM offers o
                   WHERE o.application_id = a.id
                     AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid) = sqlc.narg('has_my_offer')::boolean)
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'amount_asc' THEN a.amount END ASC,
  CASE WHEN sqlc.arg('sort')::text = 'amount_desc' THEN a.amount END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'term_asc' THEN a.term_months END ASC,
  CASE WHEN sqlc.arg('sort')::text = 'term_desc' THEN a.term_months END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'published_at_asc' THEN a.published_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort')::text = 'published_at_desc' THEN a.published_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort')::text = 'created_at_asc' THEN a.created_at END ASC,
  a.created_at DESC,
  -- Уникальный последний ключ — стабильная пагинация при равных значениях.
  a.id DESC
LIMIT sqlc.arg('page_limit') OFFSET sqlc.arg('page_offset');

-- name: CountApplicationsVisible :one
SELECT count(*) FROM applications a
WHERE true
  -- Блок видимости — символ в символ как в GetApplicationVisible и ListApplicationsVisible.
  AND (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND (a.status = 'published'
             OR EXISTS (SELECT 1 FROM offers o
                         WHERE o.application_id = a.id
                           AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid)))
  )
  -- text[], а не application_status[]: pgx не знает OID массива enum и не кодирует его
  -- без регистрации типа. Приведение в SQL оставляет сравнение по колонке (индекс по status).
  AND (sqlc.narg('statuses')::text[] IS NULL OR a.status = ANY(CAST(sqlc.narg('statuses')::text[] AS application_status[])))
  AND (sqlc.narg('target_countries')::text[] IS NULL OR a.target_country_code = ANY(sqlc.narg('target_countries')::text[]))
  AND (sqlc.narg('transfer_methods')::text[] IS NULL OR a.transfer_method = ANY(sqlc.narg('transfer_methods')::text[]))
  AND (sqlc.narg('currency')::char(3) IS NULL OR a.currency = sqlc.narg('currency')::char(3))
  AND (sqlc.narg('amount_min')::numeric IS NULL OR a.amount >= sqlc.narg('amount_min')::numeric)
  AND (sqlc.narg('amount_max')::numeric IS NULL OR a.amount <= sqlc.narg('amount_max')::numeric)
  AND (sqlc.narg('term_min')::int IS NULL OR a.term_months >= sqlc.narg('term_min')::int)
  AND (sqlc.narg('term_max')::int IS NULL OR a.term_months <= sqlc.narg('term_max')::int)
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR a.created_at >= sqlc.narg('created_from')::timestamptz)
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR a.created_at < sqlc.narg('created_to')::timestamptz)
  -- Сервис передаёт только для admin.
  AND (sqlc.narg('borrower_org_filter')::uuid IS NULL OR a.borrower_organization_id = sqlc.narg('borrower_org_filter')::uuid)
  -- Сервис передаёт только для lender: архив откликов (true) или заявки без моего оффера (false).
  AND (sqlc.narg('has_my_offer')::boolean IS NULL
       OR EXISTS (SELECT 1 FROM offers o
                   WHERE o.application_id = a.id
                     AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid) = sqlc.narg('has_my_offer')::boolean);

-- name: LockApplicationForBorrower :one
-- Блокировка заявки заёмщиком-владельцем для мутаций (PATCH, submit, cancel, reject-offers).
-- Чужая и несуществующая — 0 строк → 404. Порядок блокировок: сначала заявка, потом её
-- офферы — так же в acceptOffer (BE-09), иначе cancel и accept ловят deadlock.
SELECT * FROM applications
WHERE id = $1 AND borrower_organization_id = $2
FOR UPDATE;
