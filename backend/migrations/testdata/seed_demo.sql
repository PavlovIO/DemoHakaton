-- Демо-данные для user story из кейса.
-- Запуск: make seed-demo или
--   Get-Content backend\migrations\testdata\seed_demo.sql | docker exec -i lp_postgres psql -U lp -d liquidity_platform -v ON_ERROR_STOP=1
--
-- Пароли — заглушки. Реальные argon2id-хеши появятся, когда Backend 1 сделает auth-хелпер.

BEGIN;

-- 1. Организации
INSERT INTO organizations (id, type, name, country_code, registration_number, legal_address, kyc_status, verified_at) VALUES
    ('11111111-1111-1111-1111-111111111111', 'borrower', 'Альфа',
     'RU', '7700000001', 'г. Москва, ул. Тверская, д. 1', 'verified', now() - interval '1 day'),
    ('22222222-2222-2222-2222-222222222222', 'lender', 'Beta Corp',
     'CY', 'CY-123456', 'Limassol, 1 Example Street', 'verified', now() - interval '1 day')
ON CONFLICT (id) DO NOTHING;

-- 2. Бенефициары (идемпотентно благодаря явному UUID)
INSERT INTO beneficiaries (id, organization_id, full_name, citizenship_country_code, ownership_percent) VALUES
    ('bbbbbbbb-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Иванов Иван Иванович', 'RU', 100.0000),
    ('bbbbbbbb-0000-0000-0000-000000000002', '22222222-2222-2222-2222-222222222222', 'John Doe', 'CY', 100.0000)
ON CONFLICT (id) DO NOTHING;

-- 3. Пользователи (пароли-заглушки)
INSERT INTO users (id, email, password_hash, role, organization_id) VALUES
    ('aaaaaaaa-0000-0000-0000-000000000001', 'admin@liquidity.local', '$argon2id$v=19$PLACEHOLDER_REPLACE_ME_ADMIN',    'admin',    NULL),
    ('aaaaaaaa-0000-0000-0000-000000000002', 'alpha@liquidity.local', '$argon2id$v=19$PLACEHOLDER_REPLACE_ME_BORROWER', 'borrower', '11111111-1111-1111-1111-111111111111'),
    ('aaaaaaaa-0000-0000-0000-000000000003', 'beta@liquidity.local',  '$argon2id$v=19$PLACEHOLDER_REPLACE_ME_LENDER',   'lender',   '22222222-2222-2222-2222-222222222222')
ON CONFLICT (id) DO NOTHING;

-- 4. KYC документы (добавлено)
INSERT INTO kyc_documents (organization_id, doc_type, storage_key, file_name, mime_type, size_bytes, sha256, uploaded_by_user_id) VALUES
    ('11111111-1111-1111-1111-111111111111', 'charter', 'alpha-charter.pdf', 'Устав_Альфа.pdf', 'application/pdf', 102400, decode('0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', 'hex'), 'aaaaaaaa-0000-0000-0000-000000000002'),
    ('11111111-1111-1111-1111-111111111111', 'registration_certificate', 'alpha-reg.pdf', 'ОГРН_Альфа.pdf', 'application/pdf', 102400, decode('0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', 'hex'), 'aaaaaaaa-0000-0000-0000-000000000002'),
    ('11111111-1111-1111-1111-111111111111', 'address_proof', 'alpha-address.pdf', 'Адрес_Альфа.pdf', 'application/pdf', 102400, decode('0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', 'hex'), 'aaaaaaaa-0000-0000-0000-000000000002'),
    ('11111111-1111-1111-1111-111111111111', 'beneficiaries_register', 'alpha-benef.pdf', 'Бенефициары_Альфа.pdf', 'application/pdf', 102400, decode('0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', 'hex'), 'aaaaaaaa-0000-0000-0000-000000000002'),
    ('22222222-2222-2222-2222-222222222222', 'charter', 'beta-charter.pdf', 'Charter_Beta.pdf', 'application/pdf', 102400, decode('0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', 'hex'), 'aaaaaaaa-0000-0000-0000-000000000003'),
    ('22222222-2222-2222-2222-222222222222', 'source_of_funds', 'beta-source.pdf', 'SourceOfFunds_Beta.pdf', 'application/pdf', 102400, decode('0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', 'hex'), 'aaaaaaaa-0000-0000-0000-000000000003')
ON CONFLICT (storage_key) DO NOTHING;

-- 5. Заявка на ликвидность
INSERT INTO applications (
    id, borrower_organization_id, created_by_user_id, status,
    amount, currency, target_country_code, purpose, term_months,
    transfer_method, published_at, submitted_at
) VALUES (
    '33333333-3333-3333-3333-333333333333',
    '11111111-1111-1111-1111-111111111111',
    'aaaaaaaa-0000-0000-0000-000000000002',
    'published', 500000.00, 'EUR', 'DE',
    'Закупка промышленного оборудования в Германии', 6, 'swift', now() - interval '1 day', now() - interval '2 days'
) ON CONFLICT (id) DO NOTHING;

-- 6. Оффер от кредитора
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

-- 7. История статусов (добавлено)
INSERT INTO status_history (entity_type, entity_id, from_status, to_status, actor_user_id, actor_role, created_at)
SELECT 'application', '33333333-3333-3333-3333-333333333333', 'draft', 'moderation', 'aaaaaaaa-0000-0000-0000-000000000002', 'borrower', now() - interval '2 days'
WHERE NOT EXISTS (SELECT 1 FROM status_history WHERE entity_type = 'application' AND entity_id = '33333333-3333-3333-3333-333333333333' AND to_status = 'moderation');

INSERT INTO status_history (entity_type, entity_id, from_status, to_status, actor_user_id, actor_role, created_at)
SELECT 'application', '33333333-3333-3333-3333-333333333333', 'moderation', 'published', 'aaaaaaaa-0000-0000-0000-000000000001', 'admin', now() - interval '1 day'
WHERE NOT EXISTS (SELECT 1 FROM status_history WHERE entity_type = 'application' AND entity_id = '33333333-3333-3333-3333-333333333333' AND to_status = 'published');

INSERT INTO status_history (entity_type, entity_id, from_status, to_status, actor_user_id, actor_role, created_at)
SELECT 'offer', '44444444-4444-4444-4444-444444444444', NULL, 'sent', 'aaaaaaaa-0000-0000-0000-000000000003', 'lender', now()
WHERE NOT EXISTS (SELECT 1 FROM status_history WHERE entity_type = 'offer' AND entity_id = '44444444-4444-4444-4444-444444444444');

-- 8. Уведомления (добавлено)
INSERT INTO notifications (user_id, type, title, entity_type, entity_id)
SELECT 'aaaaaaaa-0000-0000-0000-000000000001', 'kyc_submitted', 'Новая заявка на KYC от Альфа', 'organization', '11111111-1111-1111-1111-111111111111'
WHERE NOT EXISTS (SELECT 1 FROM notifications WHERE user_id = 'aaaaaaaa-0000-0000-0000-000000000001' AND entity_id = '11111111-1111-1111-1111-111111111111');

INSERT INTO notifications (user_id, type, title, entity_type, entity_id)
SELECT 'aaaaaaaa-0000-0000-0000-000000000001', 'kyc_submitted', 'Новая заявка на KYC от Beta Corp', 'organization', '22222222-2222-2222-2222-222222222222'
WHERE NOT EXISTS (SELECT 1 FROM notifications WHERE user_id = 'aaaaaaaa-0000-0000-0000-000000000001' AND entity_id = '22222222-2222-2222-2222-222222222222');

INSERT INTO notifications (user_id, type, title, entity_type, entity_id)
SELECT 'aaaaaaaa-0000-0000-0000-000000000002', 'application_published', 'Ваша заявка APP-000001 опубликована', 'application', '33333333-3333-3333-3333-333333333333'
WHERE NOT EXISTS (SELECT 1 FROM notifications WHERE user_id = 'aaaaaaaa-0000-0000-0000-000000000002' AND entity_id = '33333333-3333-3333-3333-333333333333');

INSERT INTO notifications (user_id, type, title, entity_type, entity_id)
SELECT 'aaaaaaaa-0000-0000-0000-000000000002', 'offer_received', 'Новое предложение по заявке APP-000001', 'offer', '44444444-4444-4444-4444-444444444444'
WHERE NOT EXISTS (SELECT 1 FROM notifications WHERE user_id = 'aaaaaaaa-0000-0000-0000-000000000002' AND entity_id = '44444444-4444-4444-4444-444444444444');

-- 9. Чат по офферу
INSERT INTO messages (offer_id, author_user_id, text)
SELECT '44444444-4444-4444-4444-444444444444', 'aaaaaaaa-0000-0000-0000-000000000003', 'Добрый день! Готовы предоставить финансирование на указанных условиях.'
WHERE NOT EXISTS (SELECT 1 FROM messages WHERE offer_id = '44444444-4444-4444-4444-444444444444' AND text = 'Добрый день! Готовы предоставить финансирование на указанных условиях.');

INSERT INTO messages (offer_id, author_user_id, text)
SELECT '44444444-4444-4444-4444-444444444444', 'aaaaaaaa-0000-0000-0000-000000000002', 'Здравствуйте! Возможен ли график с промежуточными платежами?'
WHERE NOT EXISTS (SELECT 1 FROM messages WHERE offer_id = '44444444-4444-4444-4444-444444444444' AND text = 'Здравствуйте! Возможен ли график с промежуточными платежами?');

INSERT INTO messages (offer_id, author_user_id, text)
SELECT '44444444-4444-4444-4444-444444444444', 'aaaaaaaa-0000-0000-0000-000000000003', 'Рассмотрим индивидуально. Основные условия — в оффере.'
WHERE NOT EXISTS (SELECT 1 FROM messages WHERE offer_id = '44444444-4444-4444-4444-444444444444' AND text = 'Рассмотрим индивидуально. Основные условия — в оффере.');

COMMIT;