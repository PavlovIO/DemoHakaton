-- name: CreateApplication :one
INSERT INTO applications (
    borrower_organization_id, created_by_user_id, status,
    amount, currency, target_country_code, purpose, term_months,
    transfer_method, transfer_method_comment
) VALUES (
    $1, $2, 'draft', $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetApplicationByID :one
SELECT * FROM applications WHERE id = $1;

-- name: GetApplicationForUpdate :one
-- Блокировка строки для транзакции acceptOffer. Вызывать первым шагом в pgx.Tx.
SELECT a.id, a.borrower_organization_id, a.status 
FROM applications a
WHERE a.id = (SELECT o.application_id FROM offers o WHERE o.id = $1) 
FOR UPDATE OF a;

-- name: UpdateApplication :one
UPDATE applications SET
    amount = COALESCE(sqlc.narg('amount')::numeric, amount),
    currency = COALESCE(sqlc.narg('currency')::char(3), currency),
    target_country_code = COALESCE(sqlc.narg('target_country_code')::char(2), target_country_code),
    purpose = COALESCE(sqlc.narg('purpose')::text, purpose),
    term_months = COALESCE(sqlc.narg('term_months')::integer, term_months),
    transfer_method = COALESCE(sqlc.narg('transfer_method')::text, transfer_method),
    transfer_method_comment = COALESCE(sqlc.narg('transfer_method_comment')::text, transfer_method_comment),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SubmitApplication :one
UPDATE applications SET
    status = 'moderation',
    submitted_at = now(),
    moderation_comment = NULL,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ApproveApplication :one
UPDATE applications SET
    status = 'published',
    published_at = now(),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: RejectApplication :one
UPDATE applications SET
    status = 'rejected',
    moderation_comment = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CancelApplication :one
UPDATE applications SET
    status = 'cancelled',
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: AgreeApplication :one
UPDATE applications SET
    status = 'agreed',
    accepted_offer_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListApplications :many
SELECT * FROM applications
WHERE
  (sqlc.narg('borrower_org_id')::uuid IS NULL OR borrower_organization_id = sqlc.narg('borrower_org_id')::uuid)
  AND (sqlc.narg('lender_org_id')::uuid IS NULL OR 
       (status = 'published' OR EXISTS (
           SELECT 1 FROM offers o 
           WHERE o.application_id = applications.id AND o.lender_organization_id = sqlc.narg('lender_org_id')::uuid
       )))
  AND (sqlc.narg('statuses')::application_status[] IS NULL OR status = ANY(sqlc.narg('statuses')::application_status[]))
  AND (sqlc.narg('target_country_code')::char(2) IS NULL OR target_country_code = sqlc.narg('target_country_code')::char(2))
  AND (sqlc.narg('currency')::char(3) IS NULL OR currency = sqlc.narg('currency')::char(3))
  AND (sqlc.narg('amount_min')::numeric IS NULL OR amount >= sqlc.narg('amount_min')::numeric)
  AND (sqlc.narg('amount_max')::numeric IS NULL OR amount <= sqlc.narg('amount_max')::numeric)
  AND (sqlc.narg('term_min')::int IS NULL OR term_months >= sqlc.narg('term_min')::int)
  AND (sqlc.narg('term_max')::int IS NULL OR term_months <= sqlc.narg('term_max')::int)
ORDER BY 
  CASE WHEN sqlc.arg('sort_by')::text = 'amount_asc' THEN amount END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'amount_desc' THEN amount END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'term_asc' THEN term_months END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'term_desc' THEN term_months END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'created_at_asc' THEN created_at END ASC,
  created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountApplications :one
SELECT count(*) FROM applications
WHERE
  (sqlc.narg('borrower_org_id')::uuid IS NULL OR borrower_organization_id = sqlc.narg('borrower_org_id')::uuid)
  AND (sqlc.narg('lender_org_id')::uuid IS NULL OR 
       (status = 'published' OR EXISTS (
           SELECT 1 FROM offers o 
           WHERE o.application_id = applications.id AND o.lender_organization_id = sqlc.narg('lender_org_id')::uuid
       )))
  AND (sqlc.narg('statuses')::application_status[] IS NULL OR status = ANY(sqlc.narg('statuses')::application_status[]))
  AND (sqlc.narg('target_country_code')::char(2) IS NULL OR target_country_code = sqlc.narg('target_country_code')::char(2))
  AND (sqlc.narg('currency')::char(3) IS NULL OR currency = sqlc.narg('currency')::char(3))
  AND (sqlc.narg('amount_min')::numeric IS NULL OR amount >= sqlc.narg('amount_min')::numeric)
  AND (sqlc.narg('amount_max')::numeric IS NULL OR amount <= sqlc.narg('amount_max')::numeric)
  AND (sqlc.narg('term_min')::int IS NULL OR term_months >= sqlc.narg('term_min')::int)
  AND (sqlc.narg('term_max')::int IS NULL OR term_months <= sqlc.narg('term_max')::int);

-- name: CreateStatusHistory :one
INSERT INTO status_history (
    entity_type, entity_id, from_status, to_status, actor_user_id, actor_role, comment
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;