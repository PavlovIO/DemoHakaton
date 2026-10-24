-- Демо-данные, общие для обоих вариантов (BE-16): 8 компаний, их бенефициары, admin и
-- 9 пользователей. Вариант — seed_demo_a.sql (KYC с нуля) или seed_demo_b.sql (сделка
-- сразу), применяется после этого файла, последним — seed_demo_kyc.sql (следы KYC по
-- итоговым статусам):
--   make seed-demo             # вариант B
--   make db-reset SEED=a       # вариант A
--
-- Идемпотентно: явные id + ON CONFLICT DO NOTHING / WHERE NOT EXISTS — повторный сид
-- строк не добавляет. Демо-идентификаторы — таблица шага 1 docs/backend_tasks/BE-16-demo-seed.md.
-- Пароль всех аккаунтов — Demo-Pass-2026 (README); хэш сторожит TestSeedDemoPasswords.
--
-- BEGIN/COMMIT в файлах нет: make seed-demo применяет все три файла одним
-- psql --single-transaction, а интеграционный тест — в своей откатываемой транзакции.
-- Временные view (demo_files, demo_kyc) живут до конца сессии и видны следующим файлам.

-- 1. Организации. «Альфа» и Beta здесь draft: вариант B переводит их в verified.
INSERT INTO organizations (id, type, name, country_code, registration_number, lei, legal_address,
                           kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at) VALUES
    ('11111111-1111-1111-1111-111111111111', 'borrower', 'ООО «Альфа»', 'RU', '7700000009', NULL,
     '125009, г. Москва, ул. Тверская, д. 1', 'draft', NULL, NULL, NULL),
    ('22222222-2222-2222-2222-222222222222', 'lender', 'Beta Corp Ltd', 'CY', 'CY-123456', 'DEMO00BETACORP000162',
     '1 Makariou Avenue, 1065, Nicosia, CY', 'draft', NULL, NULL, NULL),
    ('55555555-5555-5555-5555-555555555555', 'lender', 'Gamma GmbH', 'DE', 'HRB-555555', 'DEMO00GAMMAGMBH00148',
     'Friedrichstrasse 100, 10117, Berlin, DE', 'under_review', NULL, now() - interval '2 days', NULL),
    ('66666666-6666-6666-6666-666666666666', 'borrower', 'АО «Вектор»', 'RU', '7700000016', NULL,
     '115035, г. Москва, ул. Садовническая, д. 5', 'verified', NULL, now() - interval '100 days', now() - interval '99 days'),
    ('77777777-7777-7777-7777-777777777777', 'lender', 'Delta Trading', 'KW', 'KW-777777', 'DEMO00DELTATRADE0174',
     'Al Hamra Tower, 13001, Kuwait City, KW', 'under_review', NULL, now() - interval '3 days', NULL),
    ('88888888-8888-8888-8888-888888888888', 'lender', 'Epsilon Capital Pte', 'SG', '201988888E', 'DEMO00EPSILONCAP0155',
     '1 Raffles Place, 048616, Singapore, SG', 'verified', NULL, now() - interval '100 days', now() - interval '99 days'),
    ('99999999-9999-9999-9999-999999999999', 'borrower', 'ООО «Омега»', 'RU', '7700000023', NULL,
     '190000, г. Санкт-Петербург, Невский пр-т, д. 10', 'rejected', 'Компания ликвидирована по данным реестра',
     now() - interval '10 days', NULL),
    ('eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee', 'borrower', 'ООО «Сигма»', 'RU', '7700000030', NULL,
     '420111, г. Казань, ул. Баумана, д. 3', 'verified', NULL, now() - interval '100 days', now() - interval '99 days')
ON CONFLICT (id) DO NOTHING;

-- 2. Бенефициары: вымышленные люди, гражданство вне списков FATF. Имена далеко от
-- мок-санкций — сторожит TestSeedFlagDetails (флаг high заблокировал бы KYC в US-14).
INSERT INTO beneficiaries (id, organization_id, full_name, citizenship_country_code, ownership_percent) VALUES
    ('bbbbbbbb-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Петров Алексей Сергеевич', 'RU', 100.0000),
    ('bbbbbbbb-0000-0000-0000-000000000002', '22222222-2222-2222-2222-222222222222', 'Andreas Georgiou', 'CY', 100.0000),
    ('bbbbbbbb-0000-0000-0000-000000000003', '55555555-5555-5555-5555-555555555555', 'Klaus Becker', 'DE', 60.0000),
    ('bbbbbbbb-0000-0000-0000-000000000004', '55555555-5555-5555-5555-555555555555', 'Anna Weber', 'DE', 40.0000),
    ('bbbbbbbb-0000-0000-0000-000000000005', '66666666-6666-6666-6666-666666666666', 'Смирнова Ольга Викторовна', 'RU', 100.0000),
    ('bbbbbbbb-0000-0000-0000-000000000006', '77777777-7777-7777-7777-777777777777', 'Omar Haddad', 'AE', 100.0000),
    ('bbbbbbbb-0000-0000-0000-000000000007', '88888888-8888-8888-8888-888888888888', 'Tan Wei Ming', 'SG', 100.0000),
    ('bbbbbbbb-0000-0000-0000-000000000008', '99999999-9999-9999-9999-999999999999', 'Кузнецов Дмитрий Павлович', 'RU', 100.0000),
    ('bbbbbbbb-0000-0000-0000-000000000009', 'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee', 'Морозова Елена Андреевна', 'RU', 100.0000)
ON CONFLICT (id) DO NOTHING;

-- 3. Пользователи: хэш Demo-Pass-2026 (go -C backend run ./cmd/hashpw). «Сигма» заблокирована.
INSERT INTO users (id, email, password_hash, role, status, blocked_reason, organization_id) VALUES
    ('aaaaaaaa-0000-0000-0000-000000000001', 'admin@liquidity.local', '$argon2id$v=19$m=65536,t=3,p=4$ypr5anEEPt17PY6J91dNmg$B+sHGo1OE+KqnQO/WMtdfK1ynQzmf7S0rAEcUMrECLY', 'admin', 'active', NULL, NULL),
    ('aaaaaaaa-0000-0000-0000-000000000002', 'alpha@liquidity.local', '$argon2id$v=19$m=65536,t=3,p=4$ypr5anEEPt17PY6J91dNmg$B+sHGo1OE+KqnQO/WMtdfK1ynQzmf7S0rAEcUMrECLY', 'borrower', 'active', NULL, '11111111-1111-1111-1111-111111111111'),
    ('aaaaaaaa-0000-0000-0000-000000000003', 'beta@liquidity.local', '$argon2id$v=19$m=65536,t=3,p=4$ypr5anEEPt17PY6J91dNmg$B+sHGo1OE+KqnQO/WMtdfK1ynQzmf7S0rAEcUMrECLY', 'lender', 'active', NULL, '22222222-2222-2222-2222-222222222222'),
    ('aaaaaaaa-0000-0000-0000-000000000004', 'gamma@liquidity.local', '$argon2id$v=19$m=65536,t=3,p=4$ypr5anEEPt17PY6J91dNmg$B+sHGo1OE+KqnQO/WMtdfK1ynQzmf7S0rAEcUMrECLY', 'lender', 'active', NULL, '55555555-5555-5555-5555-555555555555'),
    ('aaaaaaaa-0000-0000-0000-000000000005', 'vector@liquidity.local', '$argon2id$v=19$m=65536,t=3,p=4$ypr5anEEPt17PY6J91dNmg$B+sHGo1OE+KqnQO/WMtdfK1ynQzmf7S0rAEcUMrECLY', 'borrower', 'active', NULL, '66666666-6666-6666-6666-666666666666'),
    ('aaaaaaaa-0000-0000-0000-000000000006', 'delta@liquidity.local', '$argon2id$v=19$m=65536,t=3,p=4$ypr5anEEPt17PY6J91dNmg$B+sHGo1OE+KqnQO/WMtdfK1ynQzmf7S0rAEcUMrECLY', 'lender', 'active', NULL, '77777777-7777-7777-7777-777777777777'),
    ('aaaaaaaa-0000-0000-0000-000000000007', 'epsilon@liquidity.local', '$argon2id$v=19$m=65536,t=3,p=4$ypr5anEEPt17PY6J91dNmg$B+sHGo1OE+KqnQO/WMtdfK1ynQzmf7S0rAEcUMrECLY', 'lender', 'active', NULL, '88888888-8888-8888-8888-888888888888'),
    ('aaaaaaaa-0000-0000-0000-000000000008', 'omega@liquidity.local', '$argon2id$v=19$m=65536,t=3,p=4$ypr5anEEPt17PY6J91dNmg$B+sHGo1OE+KqnQO/WMtdfK1ynQzmf7S0rAEcUMrECLY', 'borrower', 'active', NULL, '99999999-9999-9999-9999-999999999999'),
    ('aaaaaaaa-0000-0000-0000-000000000009', 'sigma@liquidity.local', '$argon2id$v=19$m=65536,t=3,p=4$ypr5anEEPt17PY6J91dNmg$B+sHGo1OE+KqnQO/WMtdfK1ynQzmf7S0rAEcUMrECLY', 'borrower', 'blocked',
     'Демо: блокировка по подозрению в передаче доступа третьим лицам', 'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee')
ON CONFLICT (id) DO NOTHING;

-- 4. AML-флаги — как будто скрининг BE-13 уже прошёл при submitKyc (DECISIONS п. 45).
-- details и match_key — ровно вывод RuleScreener (сторожит TestSeedFlagDetails): по
-- match_key повторная подача KYC узнаёт флаг и не создаёт дубль. Gamma — нечёткое
-- совпадение с GAMA GmbH из мок-списка (approve KYC → 409); Delta — серый список FATF;
-- «Омега» — ликвидирована по реестру, флаг разобран при отказе в KYC.
INSERT INTO aml_flags (organization_id, rule, severity, details, match_key, status,
                       resolved_by_user_id, resolution_comment, resolved_at, created_at)
SELECT v.organization_id::uuid, v.rule::aml_rule, v.severity::aml_severity, v.details, v.match_key,
       v.status::aml_flag_status, v.resolved_by_user_id::uuid, v.resolution_comment,
       now() - v.resolved_ago::interval, now() - v.created_ago::interval
FROM (VALUES
    ('55555555-5555-5555-5555-555555555555', 'sanctions_match', 'high',
     'Совпадение компании «Gamma GmbH» со списком mock (запись demo-gama-001 «GAMA GmbH»), score 0.95',
     'компании «Gamma GmbH» ~ mock/demo-gama-001, high',
     'open', NULL, NULL, NULL, '2 days'),
    ('77777777-7777-7777-7777-777777777777', 'high_risk_jurisdiction', 'medium',
     'Страна регистрации KW — серый список FATF (на 2026-06-19)',
     'Страна регистрации KW — серый список FATF',
     'open', NULL, NULL, NULL, '3 days'),
    ('99999999-9999-9999-9999-999999999999', 'registry_mismatch', 'high',
     'Название по реестру «ООО «Омега» / Общество с ограниченной ответственностью «Омега»», статус liquidated',
     'Название по реестру «ООО «Омега» / Общество с ограниченной ответственностью «Омега»», статус liquidated',
     'confirmed', 'aaaaaaaa-0000-0000-0000-000000000001', 'Ликвидация подтверждена выпиской ЕГРЮЛ', '9 days', '10 days')
) AS v(organization_id, rule, severity, details, match_key, status,
       resolved_by_user_id, resolution_comment, resolved_ago, created_ago)
WHERE NOT EXISTS (
    SELECT 1 FROM aml_flags f
    WHERE f.organization_id = v.organization_id::uuid AND f.rule = v.rule::aml_rule AND f.match_key = v.match_key
);

-- 5. KYC-документы компаний, прошедших submitKyc («Альфа» и Beta — в вариантах). Файлы —
-- demo_files/, make seed-files кладёт их в DOCUMENT_STORAGE_DIR под storage_key
-- (manifest.txt); size_bytes и sha256 — реальные значения файлов (сторожит
-- TestSeedDemoFiles в cmd/seedfiles). storage_key — 32 hex, как у BE-05. demo_files —
-- временный view: его же читают документы вариантов.
CREATE OR REPLACE TEMP VIEW demo_files (file, size_bytes, sha256) AS VALUES
    ('demo_document.pdf', 650, '4513de1471055f7cf73816afbda9431f4b770f77fb4e571f943de1cd1c3e3cf5'),
    ('demo_registry_extract.pdf', 654, 'a00c31d0603ca87f363482d29c8ea539c93b96ce9e472db0a5a56128dd1c86b4');

INSERT INTO kyc_documents (organization_id, doc_type, storage_key, file_name, mime_type, size_bytes, sha256,
                           uploaded_by_user_id, uploaded_at)
SELECT d.organization_id::uuid, d.doc_type::kyc_document_type, d.storage_key, d.file_name, 'application/pdf',
       f.size_bytes, decode(f.sha256, 'hex'), u.id, o.kyc_submitted_at - interval '1 hour'
FROM (VALUES
    ('55555555-5555-5555-5555-555555555555', 'charter', 'de000000000000000000000000000301', 'Charter_Gamma.pdf', 'demo_document.pdf'),
    ('55555555-5555-5555-5555-555555555555', 'registration_certificate', 'de000000000000000000000000000302', 'RegistryExtract_Gamma.pdf', 'demo_registry_extract.pdf'),
    ('55555555-5555-5555-5555-555555555555', 'address_proof', 'de000000000000000000000000000303', 'AddressProof_Gamma.pdf', 'demo_document.pdf'),
    ('55555555-5555-5555-5555-555555555555', 'beneficiaries_register', 'de000000000000000000000000000304', 'Beneficiaries_Gamma.pdf', 'demo_document.pdf'),
    ('55555555-5555-5555-5555-555555555555', 'source_of_funds', 'de000000000000000000000000000305', 'SourceOfFunds_Gamma.pdf', 'demo_document.pdf'),
    ('66666666-6666-6666-6666-666666666666', 'charter', 'de000000000000000000000000000401', 'Устав_Вектор.pdf', 'demo_document.pdf'),
    ('66666666-6666-6666-6666-666666666666', 'registration_certificate', 'de000000000000000000000000000402', 'Выписка_ЕГРЮЛ_Вектор.pdf', 'demo_registry_extract.pdf'),
    ('66666666-6666-6666-6666-666666666666', 'address_proof', 'de000000000000000000000000000403', 'Подтверждение_адреса_Вектор.pdf', 'demo_document.pdf'),
    ('66666666-6666-6666-6666-666666666666', 'beneficiaries_register', 'de000000000000000000000000000404', 'Реестр_бенефициаров_Вектор.pdf', 'demo_document.pdf'),
    ('77777777-7777-7777-7777-777777777777', 'charter', 'de000000000000000000000000000501', 'Charter_Delta.pdf', 'demo_document.pdf'),
    ('77777777-7777-7777-7777-777777777777', 'registration_certificate', 'de000000000000000000000000000502', 'RegistryExtract_Delta.pdf', 'demo_registry_extract.pdf'),
    ('77777777-7777-7777-7777-777777777777', 'address_proof', 'de000000000000000000000000000503', 'AddressProof_Delta.pdf', 'demo_document.pdf'),
    ('77777777-7777-7777-7777-777777777777', 'beneficiaries_register', 'de000000000000000000000000000504', 'Beneficiaries_Delta.pdf', 'demo_document.pdf'),
    ('77777777-7777-7777-7777-777777777777', 'source_of_funds', 'de000000000000000000000000000505', 'SourceOfFunds_Delta.pdf', 'demo_document.pdf'),
    ('88888888-8888-8888-8888-888888888888', 'charter', 'de000000000000000000000000000601', 'Charter_Epsilon.pdf', 'demo_document.pdf'),
    ('88888888-8888-8888-8888-888888888888', 'registration_certificate', 'de000000000000000000000000000602', 'RegistryExtract_Epsilon.pdf', 'demo_registry_extract.pdf'),
    ('88888888-8888-8888-8888-888888888888', 'address_proof', 'de000000000000000000000000000603', 'AddressProof_Epsilon.pdf', 'demo_document.pdf'),
    ('88888888-8888-8888-8888-888888888888', 'beneficiaries_register', 'de000000000000000000000000000604', 'Beneficiaries_Epsilon.pdf', 'demo_document.pdf'),
    ('88888888-8888-8888-8888-888888888888', 'source_of_funds', 'de000000000000000000000000000605', 'SourceOfFunds_Epsilon.pdf', 'demo_document.pdf'),
    ('99999999-9999-9999-9999-999999999999', 'charter', 'de000000000000000000000000000701', 'Устав_Омега.pdf', 'demo_document.pdf'),
    ('99999999-9999-9999-9999-999999999999', 'registration_certificate', 'de000000000000000000000000000702', 'Выписка_ЕГРЮЛ_Омега.pdf', 'demo_registry_extract.pdf'),
    ('99999999-9999-9999-9999-999999999999', 'address_proof', 'de000000000000000000000000000703', 'Подтверждение_адреса_Омега.pdf', 'demo_document.pdf'),
    ('99999999-9999-9999-9999-999999999999', 'beneficiaries_register', 'de000000000000000000000000000704', 'Реестр_бенефициаров_Омега.pdf', 'demo_document.pdf'),
    ('eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee', 'charter', 'de000000000000000000000000000801', 'Устав_Сигма.pdf', 'demo_document.pdf'),
    ('eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee', 'registration_certificate', 'de000000000000000000000000000802', 'Выписка_ЕГРЮЛ_Сигма.pdf', 'demo_registry_extract.pdf'),
    ('eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee', 'address_proof', 'de000000000000000000000000000803', 'Подтверждение_адреса_Сигма.pdf', 'demo_document.pdf'),
    ('eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee', 'beneficiaries_register', 'de000000000000000000000000000804', 'Реестр_бенефициаров_Сигма.pdf', 'demo_document.pdf')
) AS d(organization_id, doc_type, storage_key, file_name, file)
JOIN demo_files f ON f.file = d.file
JOIN organizations o ON o.id = d.organization_id::uuid
JOIN users u ON u.organization_id = o.id
ON CONFLICT (storage_key) DO NOTHING;

-- 6. Снапшоты мок-реестра (BE-06) — их пишет submitKyc; карточка админа (BE-12) их
-- показывает. Название и номер — из фикстур мок-реестра (= таблица шага 1).
INSERT INTO registry_checks (organization_id, source, query, found, name, registration_number, legal_address,
                             registry_status, raw_response, checked_at)
SELECT o.id, 'mock', coalesce(o.lei, o.registration_number), true, o.name, o.registration_number, o.legal_address,
       v.registry_status, '{}'::jsonb, o.kyc_submitted_at
FROM (VALUES
    ('55555555-5555-5555-5555-555555555555', 'active'),
    ('66666666-6666-6666-6666-666666666666', 'active'),
    ('77777777-7777-7777-7777-777777777777', 'active'),
    ('88888888-8888-8888-8888-888888888888', 'active'),
    ('99999999-9999-9999-9999-999999999999', 'liquidated'),
    ('eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee', 'active')
) AS v(organization_id, registry_status)
JOIN organizations o ON o.id = v.organization_id::uuid
WHERE NOT EXISTS (SELECT 1 FROM registry_checks r WHERE r.organization_id = o.id);

-- 7. Блокировка «Сигмы» — как её пишет BlockUser (BE-12): история пользователя и аудит
-- user.block от admin с причиной в meta.
INSERT INTO status_history (entity_type, entity_id, from_status, to_status, actor_user_id, actor_role, comment, created_at)
SELECT 'user', u.id, 'active', 'blocked', 'aaaaaaaa-0000-0000-0000-000000000001', 'admin', u.blocked_reason, now() - interval '5 days'
FROM users u
WHERE u.id = 'aaaaaaaa-0000-0000-0000-000000000009'
  AND NOT EXISTS (SELECT 1 FROM status_history h WHERE h.entity_type = 'user' AND h.entity_id = u.id);

INSERT INTO audit_log (user_id, action, entity_type, entity_id, meta, created_at)
SELECT 'aaaaaaaa-0000-0000-0000-000000000001', 'user.block', 'user', u.id::text,
       jsonb_build_object('reason', u.blocked_reason), now() - interval '5 days'
FROM users u
WHERE u.id = 'aaaaaaaa-0000-0000-0000-000000000009'
  AND NOT EXISTS (SELECT 1 FROM audit_log a WHERE a.action = 'user.block' AND a.entity_id = u.id::text);

-- 8. Заявки «Вектора» для фильтров витрины: 5 опубликованных разных стран, валют, сроков и
-- способов, одна на модерации (очередь админа) и один черновик. «Вектор» верифицирован в
-- обоих вариантах, поэтому витрина не пуста и в варианте A.
INSERT INTO applications (id, borrower_organization_id, created_by_user_id, status, amount, currency,
                          target_country_code, purpose, term_months, transfer_method,
                          created_at, submitted_at, published_at)
SELECT v.id::uuid, '66666666-6666-6666-6666-666666666666', 'aaaaaaaa-0000-0000-0000-000000000005',
       v.status::application_status, v.amount, v.currency, v.country, v.purpose, v.term_months, v.method,
       now() - v.age::interval,
       CASE WHEN v.status <> 'draft' THEN now() - v.age::interval + interval '1 hour' END,
       CASE WHEN v.status = 'published' THEN now() - v.age::interval + interval '1 day' END
FROM (VALUES
    ('cccccccc-0000-0000-0000-000000000001', 'published', 250000.00, 'EUR', 'DE', 12, 'sepa',
     'Пополнение оборотных средств для импорта станков из Германии', '20 days'),
    ('cccccccc-0000-0000-0000-000000000002', 'published', 1200000.00, 'USD', 'AE', 6, 'swift',
     'Предоплата поставщику электроники в ОАЭ', '15 days'),
    ('cccccccc-0000-0000-0000-000000000003', 'published', 3000000.00, 'CNY', 'CN', 3, 'cips',
     'Оплата партии комплектующих из Китая', '12 days'),
    ('cccccccc-0000-0000-0000-000000000004', 'published', 150000.00, 'EUR', 'NL', 24, 'sepa',
     'Лизинг логистического оборудования в Нидерландах', '9 days'),
    ('cccccccc-0000-0000-0000-000000000005', 'published', 800000.00, 'USD', 'TR', 9, 'spfs',
     'Финансирование контракта на поставку текстиля из Турции', '6 days'),
    ('cccccccc-0000-0000-0000-000000000006', 'moderation', 400000.00, 'EUR', 'AT', 12, 'swift',
     'Закупка медицинского оборудования в Австрии', '1 day'),
    ('cccccccc-0000-0000-0000-000000000007', 'draft', 90000000.00, 'RUB', 'KZ', 18, 'swift',
     'Расширение сети складов в Казахстане', '3 hours')
) AS v(id, status, amount, currency, country, term_months, method, purpose, age)
ON CONFLICT (id) DO NOTHING;

-- История заявок «Вектора»: подача — заёмщик, публикация — admin (BE-12).
INSERT INTO status_history (entity_type, entity_id, from_status, to_status, actor_user_id, actor_role, created_at)
SELECT 'application', a.id, v.from_status, v.to_status, v.actor::uuid, v.role::user_role, v.created_at
FROM applications a
JOIN LATERAL (VALUES
    ('draft', 'moderation', 'aaaaaaaa-0000-0000-0000-000000000005', 'borrower', a.submitted_at),
    ('moderation', 'published', 'aaaaaaaa-0000-0000-0000-000000000001', 'admin', a.published_at)
) AS v(from_status, to_status, actor, role, created_at) ON v.created_at IS NOT NULL
WHERE a.id::text LIKE 'cccccccc-%'
  AND NOT EXISTS (SELECT 1 FROM status_history h
                  WHERE h.entity_type = 'application' AND h.entity_id = a.id AND h.to_status = v.to_status);

