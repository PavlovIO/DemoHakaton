-- name: CreateDeal :one
-- Снапшот условий принятого оффера. fee_amount считает сервис (dealFee): правило
-- округления — в одном месте, в Go; NULL ⇔ fee_percent NULL (deals_fee_chk).
INSERT INTO deals (
    application_id, offer_id, borrower_organization_id, lender_organization_id,
    amount, currency, interest_rate_annual, fee_percent, fee_amount,
    term_months, target_country_code, transfer_method, repayment_schedule,
    collateral, additional_terms
) VALUES (
    sqlc.arg('application_id'), sqlc.arg('offer_id'),
    sqlc.arg('borrower_organization_id'), sqlc.arg('lender_organization_id'),
    sqlc.arg('amount'), sqlc.arg('currency'), sqlc.arg('interest_rate_annual'),
    sqlc.narg('fee_percent')::numeric, sqlc.narg('fee_amount')::numeric,
    sqlc.arg('term_months'), sqlc.arg('target_country_code'), sqlc.arg('transfer_method'),
    sqlc.arg('repayment_schedule'), sqlc.narg('collateral'), sqlc.narg('additional_terms')
) RETURNING *;

-- name: GetDealVisible :one
-- Сделка, если пользователь (viewer_role, viewer_org) — её сторона или admin. Чужая и
-- несуществующая одинаково дают 0 строк → 404 (IDOR). Правило совпадает с ListDealsVisible.
SELECT sqlc.embed(d),
       bo.name AS borrower_name, bo.country_code AS borrower_country_code,
       lo.name AS lender_name,   lo.country_code AS lender_country_code
FROM deals d
JOIN organizations bo ON bo.id = d.borrower_organization_id
JOIN organizations lo ON lo.id = d.lender_organization_id
WHERE d.id = sqlc.arg('id')
  AND
  (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND d.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND d.lender_organization_id = sqlc.arg('viewer_org')::uuid)
  );

-- name: ListDealsVisible :many
-- Видимость — на обязательных arg (как в GetDealVisible); бизнес-фильтры — narg:
-- они только сужают выборку и не могут её открыть.
SELECT sqlc.embed(d),
       bo.name AS borrower_name, bo.country_code AS borrower_country_code,
       lo.name AS lender_name,   lo.country_code AS lender_country_code
FROM deals d
JOIN organizations bo ON bo.id = d.borrower_organization_id
JOIN organizations lo ON lo.id = d.lender_organization_id
WHERE true
  -- Блок видимости — символ в символ как в GetDealVisible (sqlc не умеет общие фрагменты).
  AND
  (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND d.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND d.lender_organization_id = sqlc.arg('viewer_org')::uuid)
  )
  -- Сервис передаёт только для admin.
  AND (sqlc.narg('borrower_org_filter')::uuid IS NULL OR d.borrower_organization_id = sqlc.narg('borrower_org_filter')::uuid)
  AND (sqlc.narg('lender_org_filter')::uuid IS NULL OR d.lender_organization_id = sqlc.narg('lender_org_filter')::uuid)
  -- Период сделки — по agreed_at (контракт listDeals).
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR d.agreed_at >= sqlc.narg('created_from')::timestamptz)
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR d.agreed_at < sqlc.narg('created_to')::timestamptz)
ORDER BY d.agreed_at DESC, d.id DESC
LIMIT sqlc.arg('page_limit') OFFSET sqlc.arg('page_offset');

-- name: CountDealsVisible :one
SELECT count(*)
FROM deals d
WHERE true
  -- Блок видимости — символ в символ как в GetDealVisible (sqlc не умеет общие фрагменты).
  AND
  (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND d.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND d.lender_organization_id = sqlc.arg('viewer_org')::uuid)
  )
  -- Сервис передаёт только для admin.
  AND (sqlc.narg('borrower_org_filter')::uuid IS NULL OR d.borrower_organization_id = sqlc.narg('borrower_org_filter')::uuid)
  AND (sqlc.narg('lender_org_filter')::uuid IS NULL OR d.lender_organization_id = sqlc.narg('lender_org_filter')::uuid)
  -- Период сделки — по agreed_at (контракт listDeals).
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR d.agreed_at >= sqlc.narg('created_from')::timestamptz)
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR d.agreed_at < sqlc.narg('created_to')::timestamptz);
