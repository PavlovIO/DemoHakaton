-- name: CreateKycDocument :one
INSERT INTO kyc_documents (
    organization_id, doc_type, storage_key, file_name, mime_type, size_bytes, sha256, uploaded_by_user_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: ListKycDocumentsByOrg :many
SELECT * FROM kyc_documents
WHERE organization_id = $1;

-- name: DeleteKycDocument :exec
DELETE FROM kyc_documents
WHERE id = $1 AND organization_id = $2;

-- name: CreateRegistryCheck :one
INSERT INTO registry_checks (
    organization_id, source, query, found, name, registration_number, legal_address, registry_status, raw_response
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetLatestRegistryCheck :one
SELECT * FROM registry_checks
WHERE organization_id = $1
ORDER BY checked_at DESC
LIMIT 1;