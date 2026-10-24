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
-- EXISTS, а не JOIN: результат остаётся моделью Offer. Правило совпадает с ListOffersVisible.
SELECT * FROM offers
WHERE offers.id = sqlc.arg('id')
  AND (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND lender_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND EXISTS (SELECT 1 FROM applications a
                     WHERE a.id = offers.application_id
                       AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid))
  );

-- name: ListOffersByApplication :many
SELECT * FROM offers 
WHERE application_id = $1
  AND (sqlc.narg('statuses')::offer_status[] IS NULL OR status = ANY(sqlc.narg('statuses')::offer_status[]))
ORDER BY created_at DESC;

-- name: ListOffersVisible :many
-- Бизнес-фильтры — narg: они только сужают выборку и не могут её открыть.
SELECT * FROM offers
WHERE
  -- Блок видимости — символ в символ как в GetOfferVisible (sqlc не умеет общие фрагменты).
  (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND lender_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND EXISTS (SELECT 1 FROM applications a
                     WHERE a.id = offers.application_id
                       AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid))
  )
  AND (sqlc.narg('application_id')::uuid IS NULL OR application_id = sqlc.narg('application_id')::uuid)
  AND (sqlc.narg('statuses')::offer_status[] IS NULL OR status = ANY(sqlc.narg('statuses')::offer_status[]))
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountOffersVisible :one
SELECT count(*) FROM offers
WHERE
  -- Блок видимости — символ в символ как в GetOfferVisible (sqlc не умеет общие фрагменты).
  (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND lender_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND EXISTS (SELECT 1 FROM applications a
                     WHERE a.id = offers.application_id
                       AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid))
  )
  AND (sqlc.narg('application_id')::uuid IS NULL OR application_id = sqlc.narg('application_id')::uuid)
  AND (sqlc.narg('statuses')::offer_status[] IS NULL OR status = ANY(sqlc.narg('statuses')::offer_status[]));

-- name: AcceptOffer :one
-- Вызывать внутри транзакции после GetApplicationForUpdate
UPDATE offers SET
    status = 'accepted',
    decided_at = now()
WHERE id = $1 AND status = 'sent'
RETURNING *;

-- name: RejectOtherOffers :exec
-- Вызывать внутри транзакции после AcceptOffer
UPDATE offers SET
    status = 'rejected',
    reject_reason = 'another_offer_accepted',
    decided_at = now()
WHERE application_id = $1 AND id <> $2 AND status = 'sent';

-- name: RejectOffer :one
UPDATE offers SET
    status = 'rejected',
    reject_reason = 'rejected_by_borrower',
    reject_comment = sqlc.narg('comment')::text,
    decided_at = now()
WHERE id = $1 AND status = 'sent'
RETURNING *;

-- name: WithdrawOffer :one
UPDATE offers SET
    status = 'withdrawn',
    decided_at = now()
WHERE id = $1 AND status = 'sent'
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