-- Демо-данные для user story из кейса.
-- Запуск: make seed-demo (docker cp + psql -f; pipe из PowerShell ломает кириллицу).
-- Скрипт идемпотентен — повторный запуск не дублирует данные.
--
-- Пароли — заглушки. Реальные argon2id-хеши появятся, когда Backend 1 сделает auth-хелпер.
-- Для просмотра данных / отладки SQL этого достаточно; для реального логина — заменить.

BEGIN;

-- 1. Организации
INSERT INTO organizations (id, type, name, country_code, registration_number, legal_address, kyc_status, verified_at) VALUES
    ('11111111-1111-1111-1111-111111111111', 'borrower', 'Альфа',
     'RU', '7700000001', 'г. Москва, ул. Тверская, д. 1', 'verified', now()),
    ('22222222-2222-2222-2222-222222222222', 'lender', 'Beta Corp',
     'CY', 'CY-123456', 'Limassol, 1 Example Street', 'verified', now())
ON CONFLICT (id) DO NOTHING;

-- 2. Бенефициары (уникального ключа нет, поэтому идемпотентность через NOT EXISTS)
INSERT INTO beneficiaries (organization_id, full_name, citizenship_country_code, ownership_percent)
SELECT v.organization_id, v.full_name, v.citizenship_country_code, v.ownership_percent
FROM (VALUES
    ('11111111-1111-1111-1111-111111111111'::uuid, 'Иванов Иван Иванович', 'RU', 100.0000),
    ('22222222-2222-2222-2222-222222222222'::uuid, 'John Doe',             'CY', 100.0000)
) AS v(organization_id, full_name, citizenship_country_code, ownership_percent)
WHERE NOT EXISTS (
    SELECT 1 FROM beneficiaries b
    WHERE b.organization_id = v.organization_id AND b.full_name = v.full_name
);

-- 3. Пользователи (пароли-заглушки)
INSERT INTO users (id, email, password_hash, role, organization_id) VALUES
    ('aaaaaaaa-0000-0000-0000-000000000001', 'admin@liquidity.local', '$argon2id$v=19$PLACEHOLDER_REPLACE_ME_ADMIN',    'admin',    NULL),
    ('aaaaaaaa-0000-0000-0000-000000000002', 'alpha@liquidity.local', '$argon2id$v=19$PLACEHOLDER_REPLACE_ME_BORROWER', 'borrower', '11111111-1111-1111-1111-111111111111'),
    ('aaaaaaaa-0000-0000-0000-000000000003', 'beta@liquidity.local',  '$argon2id$v=19$PLACEHOLDER_REPLACE_ME_LENDER',   'lender',   '22222222-2222-2222-2222-222222222222')
ON CONFLICT (id) DO NOTHING;

-- 4. Заявка на ликвидность (number — auto identity, created_by_user_id — alpha)
INSERT INTO applications (
    id, borrower_organization_id, created_by_user_id, status,
    amount, currency, target_country_code, purpose, term_months,
    transfer_method, published_at
) VALUES (
    '33333333-3333-3333-3333-333333333333',
    '11111111-1111-1111-1111-111111111111',
    'aaaaaaaa-0000-0000-0000-000000000002',
    'published', 500000.00, 'EUR', 'DE',
    'Закупка промышленного оборудования в Германии', 6, 'swift', now()
) ON CONFLICT (id) DO NOTHING;

-- 5. Оффер от кредитора (created_by_user_id — beta)
INSERT INTO offers (
    id, application_id, lender_organization_id, created_by_user_id, status,
    amount, currency, interest_rate_annual, fee_percent, repayment_schedule,
    collateral, additional_terms
) VALUES (
    '44444444-4444-4444-4444-444444444444',
    '33333333-3333-3333-3333-333333333333',
    '22222222-2222-2222-2222-222222222222',
    'aaaaaaaa-0000-0000-0000-000000000003',
    'sent', 500000.00, 'EUR', 8.0000, 1.0000, 'bullet',
    'Залог оборудования', 'Единый платёж в конце срока'
) ON CONFLICT (id) DO NOTHING;

-- 6. Чат по офферу (id — identity, поэтому идемпотентность через NOT EXISTS)
INSERT INTO messages (offer_id, author_user_id, text)
SELECT '44444444-4444-4444-4444-444444444444'::uuid, v.author_user_id, v.text
FROM (VALUES
    (1, 'aaaaaaaa-0000-0000-0000-000000000003'::uuid,
        'Добрый день! Готовы предоставить финансирование на указанных условиях.'),
    (2, 'aaaaaaaa-0000-0000-0000-000000000002'::uuid,
        'Здравствуйте! Возможен ли график с промежуточными платежами?'),
    (3, 'aaaaaaaa-0000-0000-0000-000000000003'::uuid,
        'Рассмотрим индивидуально. Основные условия — в оффере.')
) AS v(ord, author_user_id, text)
WHERE NOT EXISTS (
    SELECT 1 FROM messages WHERE offer_id = '44444444-4444-4444-4444-444444444444'
)
ORDER BY v.ord;

COMMIT;