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

-- name: GetOrganizationForUpdate :one
-- Блокирует строку организации до конца транзакции: правка профиля, загрузка и удаление
-- документа и submit KYC сериализуются, и проверка статуса в сервисе не устаревает.
SELECT id, type, name, country_code, registration_number, lei, legal_address,
       kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, created_at, updated_at
FROM organizations
WHERE id = $1
FOR UPDATE;

-- name: UpdateOrganization :one
-- lei: NULL — не трогать, пустая строка — очистить (DECISIONS п. 7), иначе — записать.
-- Охрана статуса в WHERE — вторая линия после блокировки и проверки в сервисе.
UPDATE organizations SET
    name = COALESCE(sqlc.narg('name')::text, name),
    registration_number = COALESCE(sqlc.narg('registration_number')::text, registration_number),
    legal_address = COALESCE(sqlc.narg('legal_address')::text, legal_address),
    lei = CASE WHEN sqlc.narg('lei')::text IS NULL THEN lei
               ELSE NULLIF(sqlc.narg('lei')::text, '')::char(20) END,
    updated_at = now()
WHERE id = $1 AND kyc_status IN ('draft', 'rejected')
RETURNING id, type, name, country_code, registration_number, lei, legal_address,
          kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, created_at, updated_at;

-- name: SubmitKycForReview :one
UPDATE organizations SET
    kyc_status = 'under_review',
    kyc_submitted_at = now(),
    kyc_reject_reason = NULL,
    verified_at = NULL,
    updated_at = now()
WHERE id = $1 AND kyc_status IN ('draft', 'rejected')
RETURNING id, type, name, country_code, registration_number, lei, legal_address,
          kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, created_at, updated_at;

-- name: ApproveKyc :one
-- Решение админа (BE-12). Охрана статуса в WHERE — страховка к FOR UPDATE в сервисе:
-- 0 строк значит «уже не under_review» (409), а не «нет компании».
UPDATE organizations SET
    kyc_status = 'verified',
    verified_at = now(),
    kyc_reject_reason = NULL,
    updated_at = now()
WHERE id = $1 AND kyc_status = 'under_review'
RETURNING id, type, name, country_code, registration_number, lei, legal_address,
          kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, created_at, updated_at;

-- name: RejectKyc :one
-- Причина видна компании: kyc_reject_reason отдаёт getMyOrganization и /auth/me.
UPDATE organizations SET
    kyc_status = 'rejected',
    kyc_reject_reason = $2,
    verified_at = NULL,
    updated_at = now()
WHERE id = $1 AND kyc_status = 'under_review'
RETURNING id, type, name, country_code, registration_number, lei, legal_address,
          kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, created_at, updated_at;

-- name: ListOrganizations :many
-- Админский список (BE-12). kyc_statuses — text[], а не kyc_status[]: pgx v5 не кодирует
-- массив незарегистрированного enum. q экранирует сервис (service.EscapeLike, DECISIONS п. 40).
SELECT id, type, name, country_code, registration_number, lei, legal_address,
       kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, created_at, updated_at
FROM organizations
WHERE (sqlc.narg('kyc_statuses')::text[] IS NULL OR kyc_status::text = ANY(sqlc.narg('kyc_statuses')::text[]))
  AND (sqlc.narg('type')::organization_type IS NULL OR type = sqlc.narg('type')::organization_type)
  AND (sqlc.narg('country_code')::char(2) IS NULL OR country_code = sqlc.narg('country_code')::char(2))
  AND (sqlc.narg('q')::text IS NULL
       OR name ILIKE '%' || sqlc.narg('q')::text || '%' ESCAPE '\'
       OR registration_number ILIKE '%' || sqlc.narg('q')::text || '%' ESCAPE '\')
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountOrganizations :one
SELECT count(*) FROM organizations
WHERE (sqlc.narg('kyc_statuses')::text[] IS NULL OR kyc_status::text = ANY(sqlc.narg('kyc_statuses')::text[]))
  AND (sqlc.narg('type')::organization_type IS NULL OR type = sqlc.narg('type')::organization_type)
  AND (sqlc.narg('country_code')::char(2) IS NULL OR country_code = sqlc.narg('country_code')::char(2))
  AND (sqlc.narg('q')::text IS NULL
       OR name ILIKE '%' || sqlc.narg('q')::text || '%' ESCAPE '\'
       OR registration_number ILIKE '%' || sqlc.narg('q')::text || '%' ESCAPE '\');

-- name: ListBeneficiariesByOrg :many
SELECT * FROM beneficiaries
WHERE organization_id = $1
ORDER BY created_at, id;

-- name: DeleteBeneficiariesByOrg :exec
-- Список бенефициаров заменяется целиком: delete + insert в одной транзакции.
DELETE FROM beneficiaries
WHERE organization_id = $1;

-- name: CreateBeneficiary :one
-- clock_timestamp(), а не DEFAULT now(): список вставляется одной транзакцией, now() у всех
-- строк одинаковый, и ORDER BY created_at в ListBeneficiariesByOrg потерял бы порядок из PATCH.
INSERT INTO beneficiaries (organization_id, full_name, citizenship_country_code, ownership_percent, created_at)
VALUES ($1, $2, $3, $4, clock_timestamp())
RETURNING *;
