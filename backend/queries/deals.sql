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

-- name: GetDealByID :one
SELECT * FROM deals WHERE id = $1;

-- name: ListDeals :many
SELECT * FROM deals
WHERE
  (sqlc.narg('borrower_org_id')::uuid IS NULL OR borrower_organization_id = sqlc.narg('borrower_org_id')::uuid)
  AND (sqlc.narg('lender_org_id')::uuid IS NULL OR lender_organization_id = sqlc.narg('lender_org_id')::uuid)
ORDER BY agreed_at DESC
LIMIT $1 OFFSET $2;

-- name: CountDeals :one
SELECT count(*) FROM deals
WHERE
  (sqlc.narg('borrower_org_id')::uuid IS NULL OR borrower_organization_id = sqlc.narg('borrower_org_id')::uuid)
  AND (sqlc.narg('lender_org_id')::uuid IS NULL OR lender_organization_id = sqlc.narg('lender_org_id')::uuid);