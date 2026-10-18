-- name: CreateOffer :one
INSERT INTO offers (
    application_id, lender_organization_id, created_by_user_id, status,
    amount, currency, interest_rate_annual, fee_percent, repayment_schedule,
    collateral, additional_terms, valid_until
) VALUES (
    $1, $2, $3, 'sent', $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: GetOfferByID :one
SELECT * FROM offers WHERE id = $1;

-- name: ListOffersByApplication :many
SELECT * FROM offers 
WHERE application_id = $1
  AND (sqlc.narg('statuses')::offer_status[] IS NULL OR status = ANY(sqlc.narg('statuses')::offer_status[]))
ORDER BY created_at DESC;

-- name: ListOffers :many
SELECT * FROM offers
WHERE
  (sqlc.narg('lender_org_id')::uuid IS NULL OR lender_organization_id = sqlc.narg('lender_org_id')::uuid)
  AND (sqlc.narg('application_id')::uuid IS NULL OR application_id = sqlc.narg('application_id')::uuid)
  AND (sqlc.narg('statuses')::offer_status[] IS NULL OR status = ANY(sqlc.narg('statuses')::offer_status[]))
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountOffers :one
SELECT count(*) FROM offers
WHERE
  (sqlc.narg('lender_org_id')::uuid IS NULL OR lender_organization_id = sqlc.narg('lender_org_id')::uuid)
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

-- name: RejectAllOffersForApplication :exec
UPDATE offers SET
    status = 'rejected',
    reject_reason = 'application_cancelled',
    decided_at = now()
WHERE application_id = $1 AND status = 'sent';