-- +goose Up
-- +goose StatementBegin
-- Справочники для MVP. Это не полный ISO 3166 — только страны, нужные для демо и типичных
-- юрисдикций трансграничного финансирования. Расширять отдельной миграцией.
--
-- FATF: «чёрный» список по итогам пленара июня 2026 (Иран, КНДР, Мьянма).
--   «Серый» список заполняется отдельной задачей — сверить с fatf-gafi.org.
-- is_unfriendly: перечень по распоряжению Правительства РФ № 430-р — сверить с актуальной редакцией.

INSERT INTO countries (code, name_ru, name_en, fatf_list, fatf_as_of, is_unfriendly) VALUES
    ('RU', 'Россия',            'Russia',               NULL,    NULL,         false),
    ('CY', 'Кипр',              'Cyprus',               NULL,    NULL,         true),
    ('DE', 'Германия',          'Germany',              NULL,    NULL,         true),
    ('NL', 'Нидерланды',        'Netherlands',          NULL,    NULL,         true),
    ('FR', 'Франция',           'France',               NULL,    NULL,         true),
    ('IT', 'Италия',            'Italy',                NULL,    NULL,         true),
    ('AT', 'Австрия',           'Austria',              NULL,    NULL,         true),
    ('LU', 'Люксембург',        'Luxembourg',           NULL,    NULL,         true),
    ('IE', 'Ирландия',          'Ireland',              NULL,    NULL,         true),
    ('CH', 'Швейцария',         'Switzerland',          NULL,    NULL,         true),
    ('GB', 'Великобритания',    'United Kingdom',       NULL,    NULL,         true),
    ('US', 'США',               'United States',        NULL,    NULL,         true),
    ('SG', 'Сингапур',          'Singapore',            NULL,    NULL,         true),
    ('AE', 'ОАЭ',               'United Arab Emirates', NULL,    NULL,         false),
    ('KZ', 'Казахстан',         'Kazakhstan',           NULL,    NULL,         false),
    ('KG', 'Киргизия',          'Kyrgyzstan',           NULL,    NULL,         false),
    ('UZ', 'Узбекистан',        'Uzbekistan',           NULL,    NULL,         false),
    ('AM', 'Армения',           'Armenia',              NULL,    NULL,         false),
    ('AZ', 'Азербайджан',       'Azerbaijan',           NULL,    NULL,         false),
    ('GE', 'Грузия',            'Georgia',              NULL,    NULL,         false),
    ('BY', 'Беларусь',          'Belarus',              NULL,    NULL,         false),
    ('RS', 'Сербия',            'Serbia',               NULL,    NULL,         false),
    ('TR', 'Турция',            'Turkey',               NULL,    NULL,         false),
    ('CN', 'Китай',             'China',                NULL,    NULL,         false),
    ('HK', 'Гонконг',           'Hong Kong',            NULL,    NULL,         false),
    ('IN', 'Индия',             'India',                NULL,    NULL,         false),
    ('TH', 'Таиланд',           'Thailand',             NULL,    NULL,         false),
    ('VN', 'Вьетнам',           'Vietnam',              NULL,    NULL,         false),
    ('EG', 'Египет',            'Egypt',                NULL,    NULL,         false),
    ('IR', 'Иран',              'Iran',                 'black', '2026-06-19', false),
    ('KP', 'КНДР',              'North Korea',          'black', '2026-06-19', false),
    ('MM', 'Мьянма',            'Myanmar',              'black', '2026-06-19', false);

INSERT INTO currencies (code, name, minor_units) VALUES
    ('RUB', 'Российский рубль',     2),
    ('EUR', 'Евро',                 2),
    ('USD', 'Доллар США',           2),
    ('CNY', 'Китайский юань',       2),
    ('AED', 'Дирхам ОАЭ',           2),
    ('KZT', 'Казахстанский тенге',  2),
    ('TRY', 'Турецкая лира',        2),
    ('HKD', 'Гонконгский доллар',   2),
    ('CHF', 'Швейцарский франк',    2),
    ('GBP', 'Фунт стерлингов',      2),
    ('INR', 'Индийская рупия',      2),
    ('AMD', 'Армянский драм',       2),
    ('BYN', 'Белорусский рубль',    2);

INSERT INTO transfer_methods (code, name) VALUES
    ('swift', 'SWIFT-перевод'),
    ('sepa',  'SEPA-перевод'),
    ('other', 'Другое (указать в комментарии)');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM transfer_methods;
DELETE FROM currencies;
DELETE FROM countries;
-- +goose StatementEnd
