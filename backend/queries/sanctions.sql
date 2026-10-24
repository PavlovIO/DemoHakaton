-- Загрузчик санкционных списков (cmd/loadsanctions, BE-13): источник заменяется атомарно —
-- DELETE и COPY в одной транзакции. Под lp_app (только SELECT) запросы падают с 42501.

-- name: DeleteSanctionsBySource :execrows
DELETE FROM sanctions_entries WHERE source = $1;

-- name: InsertSanctionsEntries :copyfrom
INSERT INTO sanctions_entries (source, external_id, entity_kind, name, name_normalized, country_code, raw)
VALUES ($1, $2, $3, $4, $5, $6, $7);
