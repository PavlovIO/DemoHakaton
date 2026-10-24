-- +goose Up
-- Ключ дедупликации AML-находок (BE-13). Сравнение по details ломалось на нестабильных
-- частях текста: дата FATF, score, имя выбранного алиаса. После обновления списка
-- закрытое как false_positive всплывало снова. Ключ строит RuleScreener из стабильных
-- полей; NULL — флаг не из скрининга (сид, ручная вставка), в дедупликации не участвует.
ALTER TABLE aml_flags ADD COLUMN match_key text;
UPDATE aml_flags SET match_key = details;

-- +goose Down
ALTER TABLE aml_flags DROP COLUMN match_key;
