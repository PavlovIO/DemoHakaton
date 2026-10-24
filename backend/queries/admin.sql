-- name: CreateAmlFlag :one
INSERT INTO aml_flags (
    organization_id, application_id, rule, severity, details, match_key
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: HasSimilarAmlFlag :one
-- Дедупликация находок (BE-13): у той же компании и заявки уже есть флаг с тем же правилом
-- и match_key в статусе open или false_positive. Повторный submit после rejected не плодит
-- копии, а закрытое как ложное срабатывание не всплывает снова; confirmed создаётся заново.
-- Ключ, а не details: в тексте есть дата FATF и score, они меняются при обновлении данных.
SELECT EXISTS (
    SELECT 1 FROM aml_flags
    WHERE organization_id = sqlc.arg('organization_id')::uuid
      AND application_id IS NOT DISTINCT FROM sqlc.narg('application_id')::uuid
      AND rule = sqlc.arg('rule')::aml_rule
      AND match_key = sqlc.arg('match_key')::text
      AND status IN ('open', 'false_positive')
);

-- name: ListAmlFlags :many
-- Админский список флагов (BE-13): фильтры через narg — «нет значения → всё». Название и
-- страна — для AmlFlag.organization (OrganizationShort).
SELECT sqlc.embed(f), o.name AS organization_name, o.country_code AS organization_country_code
FROM aml_flags f
JOIN organizations o ON o.id = f.organization_id
WHERE (sqlc.narg('status')::aml_flag_status IS NULL OR f.status = sqlc.narg('status')::aml_flag_status)
  AND (sqlc.narg('severity')::aml_severity IS NULL OR f.severity = sqlc.narg('severity')::aml_severity)
  AND (sqlc.narg('organization_id')::uuid IS NULL OR f.organization_id = sqlc.narg('organization_id')::uuid)
  AND (sqlc.narg('application_id')::uuid IS NULL OR f.application_id = sqlc.narg('application_id')::uuid)
ORDER BY f.created_at DESC, f.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountAmlFlags :one
-- Счётчик для ListAmlFlags: те же фильтры.
SELECT count(*) FROM aml_flags f
WHERE (sqlc.narg('status')::aml_flag_status IS NULL OR f.status = sqlc.narg('status')::aml_flag_status)
  AND (sqlc.narg('severity')::aml_severity IS NULL OR f.severity = sqlc.narg('severity')::aml_severity)
  AND (sqlc.narg('organization_id')::uuid IS NULL OR f.organization_id = sqlc.narg('organization_id')::uuid)
  AND (sqlc.narg('application_id')::uuid IS NULL OR f.application_id = sqlc.narg('application_id')::uuid);

-- name: GetAmlFlagByID :one
-- Флаг с организацией — ответ resolve и различение 404/409, когда ResolveAmlFlag не нашёл
-- открытый флаг.
SELECT sqlc.embed(f), o.name AS organization_name, o.country_code AS organization_country_code
FROM aml_flags f
JOIN organizations o ON o.id = f.organization_id
WHERE f.id = $1;

-- name: ResolveAmlFlag :one
UPDATE aml_flags SET
    status = $2,
    resolved_by_user_id = $3,
    resolution_comment = $4,
    resolved_at = now()
WHERE id = $1 AND status = 'open'
RETURNING *;

-- name: SearchSanctions :many
-- Кандидаты санкционного скрининга (BE-13). «%» — ради GIN-индекса, он отбирает строки с
-- similarity ≥ GUC pg_trgm.similarity_threshold; явное условие с @min_similarity делает
-- результат независимым от GUC. Инвариант: GUC роли (0.4 после DB-SEC, 0.3 по умолчанию)
-- ≤ aml.candidateMinSimilarity. SET/SET LOCAL не используем: на соединении из пула он
-- утекает в чужие запросы. entity_kind фильтруется до LIMIT: иначе на большом списке
-- топ кандидатов по названию компании могут занять персоны, и организацию не сравнят.
SELECT id, source, external_id, entity_kind, name, name_normalized,
       similarity(name_normalized, @query::text)::real AS trgm_score
FROM sanctions_entries
WHERE name_normalized % @query::text
  AND similarity(name_normalized, @query::text) >= @min_similarity::real
  AND entity_kind = ANY(@entity_kinds::text[])
ORDER BY trgm_score DESC, id
LIMIT @max_candidates;

-- name: ListCountriesFatf :many
-- Страны из списков FATF среди codes: скрининг компании и бенефициаров одним запросом.
SELECT code, fatf_list, fatf_as_of FROM countries
WHERE code = ANY(@codes::text[]) AND fatf_list IS NOT NULL;

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

-- name: ListAmlFlagsByOrganization :many
-- Флаги компании в карточке админа (BE-12): без пагинации, открытые сверху. Название и
-- страна — для AmlFlag.organization (OrganizationShort), aml_flags их не хранит.
SELECT sqlc.embed(f), o.name AS organization_name, o.country_code AS organization_country_code
FROM aml_flags f
JOIN organizations o ON o.id = f.organization_id
WHERE f.organization_id = $1
ORDER BY (f.status = 'open') DESC, f.created_at DESC, f.id DESC;

-- name: CountOpenHighAmlFlags :one
-- Открытые high-флаги блокируют approve KYC (409 CONFLICT, BE-12).
SELECT count(*) FROM aml_flags
WHERE organization_id = $1 AND status = 'open' AND severity = 'high';
