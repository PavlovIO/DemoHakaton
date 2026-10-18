-- name: CreateAmlFlag :one
INSERT INTO aml_flags (
    organization_id, application_id, rule, severity, details
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: ListAmlFlags :many
SELECT * FROM aml_flags
WHERE (sqlc.narg('status')::aml_flag_status IS NULL OR status = sqlc.narg('status')::aml_flag_status)
  AND (sqlc.narg('severity')::aml_severity IS NULL OR severity = sqlc.narg('severity')::aml_severity)
  AND (sqlc.narg('organization_id')::uuid IS NULL OR organization_id = sqlc.narg('organization_id')::uuid)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ResolveAmlFlag :one
UPDATE aml_flags SET
    status = $2,
    resolved_by_user_id = $3,
    resolution_comment = $4,
    resolved_at = now()
WHERE id = $1 AND status = 'open'
RETURNING *;

-- name: SearchSanctions :many
-- Использует pg_trgm (оператор %). Требует установки pg_trgm.similarity_threshold = 0.4
SELECT * FROM sanctions_entries
WHERE name_normalized % $1
ORDER BY similarity(name_normalized, $1) DESC
LIMIT $2;

-- name: CreateAuditLog :one
INSERT INTO audit_log (user_id, action, entity_type, entity_id, ip, user_agent, meta)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListAuditLog :many
SELECT * FROM audit_log
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: GetAnalyticsSummaryByCurrency :many
SELECT currency, count(*) as count, sum(amount) as total_amount, round(avg(amount), 2) as average_amount
FROM applications
WHERE status NOT IN ('draft', 'cancelled')
GROUP BY currency;

-- name: GetTopDemandCountries :many
SELECT target_country_code as country_code, count(*)::int as count
FROM applications
WHERE status IN ('published', 'agreed')
GROUP BY target_country_code
ORDER BY count DESC
LIMIT $1;

-- name: GetTopSupplyCountries :many
SELECT org.country_code, count(DISTINCT o.id)::int as count
FROM offers o
JOIN organizations org ON org.id = o.lender_organization_id
GROUP BY org.country_code
ORDER BY count DESC
LIMIT $1;