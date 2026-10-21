-- name: ListCountries :many
-- Справочники для фронта (GET /dictionaries/*): публичные, без проверки владения.
-- fatf_high_risk: страна в чёрном или сером списке FATF.
SELECT code, name_ru, name_en, (fatf_list IS NOT NULL)::boolean AS fatf_high_risk
FROM countries
ORDER BY name_ru;

-- name: ListCurrencies :many
SELECT code, name
FROM currencies
ORDER BY code;

-- name: ListTransferMethods :many
SELECT code, name
FROM transfer_methods
ORDER BY code;
