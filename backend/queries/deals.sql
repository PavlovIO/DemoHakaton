-- name: CreateDeal :one
INSERT INTO deals (
    application_id, offer_id, borrower_organization_id, lender_organization_id,
    amount, currency, interest_rate_annual, fee_percent, fee_amount,
    term_months, target_country_code, transfer_method, repayment_schedule,
    collateral, additional_terms
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    CASE WHEN $8::numeric IS NOT NULL 
         THEN round($5::numeric * $8::numeric / 100, 2) 
         ELSE NULL 
    END,
    $9, $10, $11, $12, $13, $14
) RETURNING *;

-- name: GetDealVisible :one
-- Сделка, если пользователь (viewer_role, viewer_org) — её сторона или admin. Чужая и
-- несуществующая одинаково дают 0 строк → 404 (IDOR). Правило совпадает с ListDealsVisible.
SELECT * FROM deals
WHERE id = sqlc.arg('id')
  AND (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND borrower_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND lender_organization_id = sqlc.arg('viewer_org')::uuid)
  );

-- name: ListDealsVisible :many
SELECT * FROM deals
WHERE
  -- Блок видимости — символ в символ как в GetDealVisible (sqlc не умеет общие фрагменты).
  (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND borrower_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND lender_organization_id = sqlc.arg('viewer_org')::uuid)
  )
ORDER BY agreed_at DESC
LIMIT $1 OFFSET $2;

-- name: CountDealsVisible :one
SELECT count(*) FROM deals
WHERE
  -- Блок видимости — символ в символ как в GetDealVisible (sqlc не умеет общие фрагменты).
  (
       sqlc.arg('viewer_role')::user_role = 'admin'
    OR (sqlc.arg('viewer_role')::user_role = 'borrower'
        AND borrower_organization_id = sqlc.arg('viewer_org')::uuid)
    OR (sqlc.arg('viewer_role')::user_role = 'lender'
        AND lender_organization_id = sqlc.arg('viewer_org')::uuid)
  );