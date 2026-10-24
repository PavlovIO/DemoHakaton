-- Вариант B сида BE-16 (по умолчанию) — сделка сразу: применяется после
-- seed_demo_common.sql и перед seed_demo_kyc.sql (make seed-demo, make db-reset). «Альфа» и Beta верифицированы,
-- заявка «Альфы» 500 000 EUR → DE с офферами Beta (8 % + 1 %) и Epsilon (9.5 %), чат и
-- история за ~3 месяца для аналитики. С вариантом A на одной БД несовместим.

-- 1. «Альфа» и Beta → verified. organizations_verified_chk требует kyc_status и
-- verified_at вместе — один UPDATE; охрана статуса делает его идемпотентным. Верификация —
-- раньше истории (~90 дней), иначе сделки были бы у неверифицированных компаний.
UPDATE organizations
SET kyc_status = 'verified', kyc_submitted_at = now() - interval '100 days', verified_at = now() - interval '99 days'
WHERE id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222')
  AND kyc_status = 'draft';

-- 2. Документы «Альфы» и Beta.
INSERT INTO kyc_documents (organization_id, doc_type, storage_key, file_name, mime_type, size_bytes, sha256,
                           uploaded_by_user_id, uploaded_at)
SELECT d.organization_id::uuid, d.doc_type::kyc_document_type, d.storage_key, d.file_name, 'application/pdf',
       f.size_bytes, decode(f.sha256, 'hex'), u.id, o.kyc_submitted_at - interval '1 hour'
FROM (VALUES
    ('11111111-1111-1111-1111-111111111111', 'charter', 'de000000000000000000000000000101', 'Устав_Альфа.pdf', 'demo_document.pdf'),
    ('11111111-1111-1111-1111-111111111111', 'registration_certificate', 'de000000000000000000000000000102', 'Выписка_ЕГРЮЛ_Альфа.pdf', 'demo_registry_extract.pdf'),
    ('11111111-1111-1111-1111-111111111111', 'address_proof', 'de000000000000000000000000000103', 'Подтверждение_адреса_Альфа.pdf', 'demo_document.pdf'),
    ('11111111-1111-1111-1111-111111111111', 'beneficiaries_register', 'de000000000000000000000000000104', 'Реестр_бенефициаров_Альфа.pdf', 'demo_document.pdf'),
    ('22222222-2222-2222-2222-222222222222', 'charter', 'de000000000000000000000000000201', 'Charter_Beta.pdf', 'demo_document.pdf'),
    ('22222222-2222-2222-2222-222222222222', 'registration_certificate', 'de000000000000000000000000000202', 'RegistryExtract_Beta.pdf', 'demo_registry_extract.pdf'),
    ('22222222-2222-2222-2222-222222222222', 'address_proof', 'de000000000000000000000000000203', 'AddressProof_Beta.pdf', 'demo_document.pdf'),
    ('22222222-2222-2222-2222-222222222222', 'beneficiaries_register', 'de000000000000000000000000000204', 'Beneficiaries_Beta.pdf', 'demo_document.pdf'),
    ('22222222-2222-2222-2222-222222222222', 'source_of_funds', 'de000000000000000000000000000205', 'SourceOfFunds_Beta.pdf', 'demo_document.pdf')
) AS d(organization_id, doc_type, storage_key, file_name, file)
JOIN demo_files f ON f.file = d.file
JOIN organizations o ON o.id = d.organization_id::uuid
JOIN users u ON u.organization_id = o.id
ON CONFLICT (storage_key) DO NOTHING;

-- 3. Снапшоты мок-реестра «Альфы» и Beta; историю KYC пишет seed_demo_kyc.sql.
INSERT INTO registry_checks (organization_id, source, query, found, name, registration_number, legal_address,
                             registry_status, raw_response, checked_at)
SELECT o.id, 'mock', coalesce(o.lei, o.registration_number), true, o.name, o.registration_number, o.legal_address,
       'active', '{}'::jsonb, o.kyc_submitted_at
FROM organizations o
WHERE o.id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222')
  AND NOT EXISTS (SELECT 1 FROM registry_checks r WHERE r.organization_id = o.id);

-- 4. Главная заявка US-14: 500 000 EUR → DE, 6 мес. В фильтре «Германия» вместе с заявкой
-- «Вектора» (250 000 EUR, 12 мес.) — нужная находится по сумме и сроку.
INSERT INTO applications (id, borrower_organization_id, created_by_user_id, status, amount, currency,
                          target_country_code, purpose, term_months, transfer_method,
                          created_at, submitted_at, published_at)
VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111',
        'aaaaaaaa-0000-0000-0000-000000000002', 'published', 500000.00, 'EUR', 'DE',
        'Закупка промышленного оборудования в Германии', 6, 'swift',
        now() - interval '2 days 3 hours', now() - interval '2 days', now() - interval '1 day')
ON CONFLICT (id) DO NOTHING;

-- 5. Офферы к главной заявке. Beta: 8 % + комиссия 1 % → при принятии сделка с
-- fee_amount = 5 000.00 EUR (BE-09). Epsilon: 9.5 % без комиссии — после принятия оффера
-- Beta автоматически rejected/another_offer_accepted (BE-09).
INSERT INTO offers (id, application_id, lender_organization_id, created_by_user_id, status, amount, currency,
                    interest_rate_annual, fee_percent, repayment_schedule, collateral, additional_terms, created_at)
VALUES
    ('44444444-4444-4444-4444-444444444444', '33333333-3333-3333-3333-333333333333',
     '22222222-2222-2222-2222-222222222222', 'aaaaaaaa-0000-0000-0000-000000000003', 'sent', 500000.00, 'EUR',
     8.0000, 1.0000, 'bullet', 'Залог оборудования', 'Единый платёж в конце срока', now() - interval '20 hours'),
    ('44444444-4444-4444-4444-444444444445', '33333333-3333-3333-3333-333333333333',
     '88888888-8888-8888-8888-888888888888', 'aaaaaaaa-0000-0000-0000-000000000007', 'sent', 500000.00, 'EUR',
     9.5000, NULL, 'bullet', NULL, 'Без комиссии, погашение в конце срока', now() - interval '18 hours')
ON CONFLICT (id) DO NOTHING;

INSERT INTO status_history (entity_type, entity_id, from_status, to_status, actor_user_id, actor_role, created_at)
SELECT v.entity_type::entity_type, v.entity_id::uuid, v.from_status, v.to_status, v.actor::uuid, v.role::user_role,
       now() - v.ago::interval
FROM (VALUES
    ('application', '33333333-3333-3333-3333-333333333333', 'draft', 'moderation',
     'aaaaaaaa-0000-0000-0000-000000000002', 'borrower', '2 days'),
    ('application', '33333333-3333-3333-3333-333333333333', 'moderation', 'published',
     'aaaaaaaa-0000-0000-0000-000000000001', 'admin', '1 day'),
    ('offer', '44444444-4444-4444-4444-444444444444', NULL, 'sent',
     'aaaaaaaa-0000-0000-0000-000000000003', 'lender', '20 hours'),
    ('offer', '44444444-4444-4444-4444-444444444445', NULL, 'sent',
     'aaaaaaaa-0000-0000-0000-000000000007', 'lender', '18 hours')
) AS v(entity_type, entity_id, from_status, to_status, actor, role, ago)
WHERE NOT EXISTS (SELECT 1 FROM status_history h
                  WHERE h.entity_id = v.entity_id::uuid AND h.to_status = v.to_status);

-- 6. Уведомления «Альфе». Номер заявки — из applications.number (identity): на БД, где
-- заявки уже создавались, это не APP-000001. Формат заголовков — как у сервисов.
INSERT INTO notifications (user_id, type, title, entity_type, entity_id, created_at)
SELECT 'aaaaaaaa-0000-0000-0000-000000000002', v.type::notification_type,
       v.title || 'APP-' || lpad(a.number::text, 6, '0'), v.entity_type::entity_type, v.entity_id, now() - v.ago::interval
FROM applications a
CROSS JOIN (VALUES
    ('application_published', 'Заявка опубликована: ', 'application', '33333333-3333-3333-3333-333333333333', '1 day'),
    ('offer_received', 'Новый оффер по заявке ', 'offer', '44444444-4444-4444-4444-444444444444', '20 hours'),
    ('offer_received', 'Новый оффер по заявке ', 'offer', '44444444-4444-4444-4444-444444444445', '18 hours')
) AS v(type, title, entity_type, entity_id, ago)
WHERE a.id = '33333333-3333-3333-3333-333333333333'
  AND NOT EXISTS (SELECT 1 FROM notifications n
                  WHERE n.user_id = 'aaaaaaaa-0000-0000-0000-000000000002' AND n.entity_id = v.entity_id);

-- 7. Чат по офферу Beta.
INSERT INTO messages (offer_id, author_user_id, text, created_at)
SELECT '44444444-4444-4444-4444-444444444444', v.author::uuid, v.text, now() - v.ago::interval
FROM (VALUES
    ('aaaaaaaa-0000-0000-0000-000000000003', 'Добрый день! Готовы предоставить финансирование на указанных условиях.', '19 hours'),
    ('aaaaaaaa-0000-0000-0000-000000000002', 'Здравствуйте! Возможен ли график с промежуточными платежами?', '17 hours'),
    ('aaaaaaaa-0000-0000-0000-000000000003', 'Рассмотрим индивидуально. Основные условия — в оффере.', '16 hours')
) AS v(author, text, ago)
WHERE NOT EXISTS (SELECT 1 FROM messages m
                  WHERE m.offer_id = '44444444-4444-4444-4444-444444444444' AND m.text = v.text);

-- 8. История для аналитики (BE-14): 45 заявок за ~3 месяца (от 94 до 6 дней назад, все даты
-- в прошлом), 60 офферов, 15 сделок.
-- Детерминированно (DECISIONS п. 28): id — md5('demo-hist-…' || i), всё остальное —
-- функции от i, даты — от now(); random() нет, поэтому повторный сид строк не добавляет,
-- а графики одинаковы на каждом стенде. Статусы по i % 3: 0 — agreed, 1 — cancelled,
-- 2 — rejected модерацией. Ни одна заявка истории не остаётся published — витрину не
-- засоряют. status_history не пишется: аналитика читает только applications, offers, deals.
-- Участники — только verified-компании: заёмщики «Альфа», «Вектор», «Сигма», кредиторы
-- Beta и Epsilon.

-- 8.1. Заявки. Будущие agreed вставляются published: applications_agreed_chk требует
-- accepted_offer_id, а оффера ещё нет (статус меняет 8.3).
INSERT INTO applications (id, borrower_organization_id, created_by_user_id, status, amount, currency,
                          target_country_code, purpose, term_months, transfer_method, moderation_comment,
                          created_at, submitted_at, published_at)
SELECT md5('demo-hist-app-' || i)::uuid,
       (ARRAY['11111111-1111-1111-1111-111111111111', '66666666-6666-6666-6666-666666666666',
              'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee'])[1 + (i / 3) % 3]::uuid,
       (ARRAY['aaaaaaaa-0000-0000-0000-000000000002', 'aaaaaaaa-0000-0000-0000-000000000005',
              'aaaaaaaa-0000-0000-0000-000000000009'])[1 + (i / 3) % 3]::uuid,
       (ARRAY['published', 'cancelled', 'rejected'])[1 + i % 3]::application_status,
       -- Порядок суммы одинаков в пересчёте на EUR: CNY ×7, RUB ×90.
       (100000 + (i * 37 % 20) * 50000) * CASE c.currency WHEN 'CNY' THEN 7 WHEN 'RUB' THEN 90 ELSE 1 END,
       c.currency,
       (ARRAY['DE', 'AE', 'CN', 'TR', 'NL', 'KZ'])[1 + i % 6],
       'Демо-история: ' || (ARRAY['импорт оборудования', 'оплата поставки сырья', 'предоплата по контракту',
                                  'пополнение оборотных средств', 'финансирование логистики'])[1 + i % 5],
       (ARRAY[3, 6, 9, 12, 18, 24])[1 + (i * 5) % 6],
       CASE c.currency WHEN 'EUR' THEN 'sepa' WHEN 'USD' THEN 'swift' WHEN 'CNY' THEN 'cips' ELSE 'spfs' END,
       CASE WHEN i % 3 = 2 THEN 'Демо: цель финансирования описана недостаточно' END,
       now() - make_interval(days => 96 - 2 * i),
       now() - make_interval(days => 96 - 2 * i) + interval '2 hours',
       CASE WHEN i % 3 <> 2 THEN now() - make_interval(days => 96 - 2 * i) + interval '1 day' END
FROM generate_series(1, 45) AS i
CROSS JOIN LATERAL (SELECT (ARRAY['EUR', 'USD', 'CNY', 'RUB'])[1 + i % 4] AS currency) AS c
ON CONFLICT (id) DO NOTHING;

-- 8.2. По два оффера к каждой опубликованной заявке (k = 1, 2 — от Beta и Epsilon по
-- очереди). У agreed: k=1 accepted, k=2 rejected/another_offer_accepted (как при accept,
-- BE-09); у cancelled: k=1 rejected/application_cancelled, k=2 withdrawn.
INSERT INTO offers (id, application_id, lender_organization_id, created_by_user_id, status, reject_reason,
                    amount, currency, interest_rate_annual, fee_percent, repayment_schedule, created_at, decided_at)
SELECT md5('demo-hist-offer-' || i || '-' || k)::uuid, a.id,
       (ARRAY['22222222-2222-2222-2222-222222222222', '88888888-8888-8888-8888-888888888888'])[1 + (i + k) % 2]::uuid,
       (ARRAY['aaaaaaaa-0000-0000-0000-000000000003', 'aaaaaaaa-0000-0000-0000-000000000007'])[1 + (i + k) % 2]::uuid,
       s.status::offer_status, s.reject_reason::offer_reject_reason,
       a.amount, a.currency,
       6 + (i % 7) * 0.5 + k * 0.25,
       CASE WHEN i % 4 = 0 THEN NULL ELSE 1 + (i % 3) * 0.25 END,
       (ARRAY['bullet', 'annuity', 'interest_monthly_principal_bullet'])[1 + (i + k) % 3]::repayment_schedule,
       a.published_at + make_interval(hours => 6 * k),
       a.published_at + interval '2 days'
FROM generate_series(1, 45) AS i
CROSS JOIN generate_series(1, 2) AS k
JOIN applications a ON a.id = md5('demo-hist-app-' || i)::uuid
JOIN LATERAL (VALUES
    (0, 1, 'accepted', NULL),
    (0, 2, 'rejected', 'another_offer_accepted'),
    (1, 1, 'rejected', 'application_cancelled'),
    (1, 2, 'withdrawn', NULL)
) AS s(app_kind, offer_no, status, reject_reason) ON s.app_kind = i % 3 AND s.offer_no = k
ON CONFLICT (id) DO NOTHING;

-- 8.3. Заявки с принятым оффером → agreed (accepted_offer_id вместе со статусом).
UPDATE applications a
SET status = 'agreed', accepted_offer_id = md5('demo-hist-offer-' || i || '-1')::uuid
FROM generate_series(1, 45) AS i
WHERE a.id = md5('demo-hist-app-' || i)::uuid AND i % 3 = 0 AND a.status = 'published';

-- 8.4. Сделки — снапшот условий принятого оффера; комиссия — как считает BE-09:
-- fee_amount = round(amount × fee_percent / 100, 2), без комиссии — оба NULL.
INSERT INTO deals (id, application_id, offer_id, borrower_organization_id, lender_organization_id, amount, currency,
                   interest_rate_annual, fee_percent, fee_amount, term_months, target_country_code, transfer_method,
                   repayment_schedule, agreed_at)
SELECT md5('demo-hist-deal-' || i)::uuid, a.id, o.id, a.borrower_organization_id, o.lender_organization_id,
       o.amount, o.currency, o.interest_rate_annual, o.fee_percent, round(o.amount * o.fee_percent / 100, 2),
       a.term_months, a.target_country_code, a.transfer_method, o.repayment_schedule, o.decided_at
FROM generate_series(1, 45) AS i
JOIN applications a ON a.id = md5('demo-hist-app-' || i)::uuid
JOIN offers o ON o.id = a.accepted_offer_id
ON CONFLICT DO NOTHING;
