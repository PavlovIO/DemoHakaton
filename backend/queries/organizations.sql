-- name: CreateOrganization :one
INSERT INTO organizations (type, name, country_code, registration_number, lei, legal_address, kyc_status)
VALUES ($1, $2, $3, $4, $5, $6, 'draft')
RETURNING id, type, name, country_code, registration_number, lei, legal_address,
          kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, created_at, updated_at;

-- name: GetOrganizationByID :one
SELECT id, type, name, country_code, registration_number, lei, legal_address,
       kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, created_at, updated_at
FROM organizations
WHERE id = $1;

-- name: UpdateOrganization :one
UPDATE organizations SET
    name = COALESCE(sqlc.narg('name')::text, name),
    registration_number = COALESCE(sqlc.narg('registration_number')::text, registration_number),
    legal_address = COALESCE(sqlc.narg('legal_address')::text, legal_address),
    lei = COALESCE(sqlc.narg('lei')::char(20), lei),
    updated_at = now()
WHERE id = $1
RETURNING id, type, name, country_code, registration_number, lei, legal_address,
          kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, created_at, updated_at;

-- name: SubmitKycForReview :one
UPDATE organizations SET
    kyc_status = 'under_review',
    kyc_submitted_at = now(),
    kyc_reject_reason = NULL,
    verified_at = NULL,
    updated_at = now()
WHERE id = $1
RETURNING id, type, name, kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, updated_at;

-- name: ApproveKyc :one
UPDATE organizations SET
    kyc_status = 'verified',
    verified_at = now(),
    kyc_reject_reason = NULL,
    updated_at = now()
WHERE id = $1
RETURNING id, type, name, kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, updated_at;

-- name: RejectKyc :one
UPDATE organizations SET
    kyc_status = 'rejected',
    kyc_reject_reason = $2,
    verified_at = NULL,
    updated_at = now()
WHERE id = $1
RETURNING id, type, name, kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, updated_at;

-- name: ListOrganizations :many
SELECT id, type, name, country_code, registration_number, lei, legal_address,
       kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, created_at, updated_at
FROM organizations
WHERE (sqlc.narg('kyc_status')::kyc_status IS NULL OR kyc_status = sqlc.narg('kyc_status')::kyc_status)
  AND (sqlc.narg('type')::organization_type IS NULL OR type = sqlc.narg('type')::organization_type)
  AND (sqlc.narg('country_code')::char(2) IS NULL OR country_code = sqlc.narg('country_code')::char(2))
  AND (sqlc.narg('q')::text IS NULL
       OR name ILIKE '%' || sqlc.narg('q')::text || '%'
       OR registration_number ILIKE '%' || sqlc.narg('q')::text || '%')
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountOrganizations :one
SELECT count(*) FROM organizations
WHERE (sqlc.narg('kyc_status')::kyc_status IS NULL OR kyc_status = sqlc.narg('kyc_status')::kyc_status)
  AND (sqlc.narg('type')::organization_type IS NULL OR type = sqlc.narg('type')::organization_type)
  AND (sqlc.narg('country_code')::char(2) IS NULL OR country_code = sqlc.narg('country_code')::char(2))
  AND (sqlc.narg('q')::text IS NULL
       OR name ILIKE '%' || sqlc.narg('q')::text || '%'
       OR registration_number ILIKE '%' || sqlc.narg('q')::text || '%');