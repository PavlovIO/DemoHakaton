-- Smoke-тест инвариантов схемы. Запускать на ЧИСТОЙ БД после `goose up`:
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f backend/migrations/testdata/invariants_test.sql
-- Всё выполняется в транзакции и откатывается в конце — данные не остаются.
-- Каждая проверка «должно упасть» реализована через DO-блок, который ловит ожидаемую ошибку
-- и бросает исключение, если ошибки НЕ было.
-- Результат — строки «NOTICE: ok …» в stderr; первая же провалившаяся проверка останавливает скрипт.

\set ON_ERROR_STOP on
\o /dev/null

BEGIN;

-- helper: выполнить SQL и убедиться, что он упал с нужным SQLSTATE
CREATE FUNCTION pg_temp.expect_fail(sql text, expected_state text, label text) RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
    BEGIN
        EXECUTE sql;
    EXCEPTION WHEN OTHERS THEN
        IF SQLSTATE <> expected_state THEN
            RAISE EXCEPTION 'FAIL %: expected %, got % (%)', label, expected_state, SQLSTATE, SQLERRM;
        END IF;
        RAISE NOTICE 'ok   %', label;
        RETURN;
    END;
    RAISE EXCEPTION 'FAIL %: statement succeeded but should have failed', label;
END;
$$;

-- ── фикстуры: сценарий из кейса ──
INSERT INTO organizations (id, type, name, country_code, registration_number, legal_address)
VALUES ('00000000-0000-0000-0000-00000000a001', 'borrower', 'ООО «Альфа»', 'RU', '7701234567', 'Москва'),
       ('00000000-0000-0000-0000-00000000b001', 'lender',   'Beta Corp',   'CY', 'HE123456',   'Limassol'),
       ('00000000-0000-0000-0000-00000000b002', 'lender',   'Gamma GmbH',  'DE', 'HRB 98765',  'Berlin');

INSERT INTO users (id, email, password_hash, role, organization_id) VALUES
    ('00000000-0000-0000-0000-0000000000a1', 'alpha@example.com', 'x', 'borrower', '00000000-0000-0000-0000-00000000a001'),
    ('00000000-0000-0000-0000-0000000000b1', 'beta@example.com',  'x', 'lender',   '00000000-0000-0000-0000-00000000b001'),
    ('00000000-0000-0000-0000-0000000000b2', 'gamma@example.com', 'x', 'lender',   '00000000-0000-0000-0000-00000000b002'),
    ('00000000-0000-0000-0000-0000000000ad', 'admin@example.com', 'x', 'admin',    NULL);

INSERT INTO applications (id, borrower_organization_id, created_by_user_id, status, amount, currency,
                          target_country_code, purpose, term_months, transfer_method)
VALUES ('00000000-0000-0000-0000-0000000a0001', '00000000-0000-0000-0000-00000000a001',
        '00000000-0000-0000-0000-0000000000a1', 'published', 500000.00, 'EUR', 'DE',
        'Закупка оборудования в Германии', 6, 'swift');

INSERT INTO offers (id, application_id, lender_organization_id, created_by_user_id, amount, currency,
                    interest_rate_annual, fee_percent, repayment_schedule)
VALUES ('00000000-0000-0000-0000-0000000f0001', '00000000-0000-0000-0000-0000000a0001',
        '00000000-0000-0000-0000-00000000b001', '00000000-0000-0000-0000-0000000000b1',
        500000.00, 'EUR', 8.0, 1.0, 'bullet'),
       ('00000000-0000-0000-0000-0000000f0002', '00000000-0000-0000-0000-0000000a0001',
        '00000000-0000-0000-0000-00000000b002', '00000000-0000-0000-0000-0000000000b2',
        400000.00, 'EUR', 9.5, NULL, 'annuity');

-- ── организации ──
SELECT pg_temp.expect_fail(
  $q$INSERT INTO organizations (type, name, country_code, registration_number, legal_address)
     VALUES ('borrower', 'Foreign borrower', 'CY', 'X1', 'a')$q$, '23514', 'заёмщик не из РФ');
SELECT pg_temp.expect_fail(
  $q$INSERT INTO organizations (type, name, country_code, registration_number, legal_address)
     VALUES ('lender', 'RU lender', 'RU', 'X2', 'a')$q$, '23514', 'кредитор из РФ');
SELECT pg_temp.expect_fail(
  $q$INSERT INTO organizations (type, name, country_code, registration_number, legal_address)
     VALUES ('borrower', 'Дубль', 'RU', '7701234567', 'a')$q$, '23505', 'дубль рег. номера');
SELECT pg_temp.expect_fail(
  $q$UPDATE organizations SET kyc_status = 'verified'
     WHERE id = '00000000-0000-0000-0000-00000000a001'$q$, '23514', 'verified без verified_at');

-- ── пользователи ──
SELECT pg_temp.expect_fail(
  $q$INSERT INTO users (email, password_hash, role, organization_id)
     VALUES ('ALPHA@example.com', 'x', 'borrower', NULL)$q$, '23514', 'borrower без компании');
SELECT pg_temp.expect_fail(
  $q$INSERT INTO users (email, password_hash, role, organization_id)
     VALUES ('ALPHA@EXAMPLE.COM', 'x', 'admin', NULL)$q$, '23505', 'email регистронезависимо уникален');
SELECT pg_temp.expect_fail(
  $q$INSERT INTO users (email, password_hash, role, organization_id)
     VALUES ('alpha2@example.com', 'x', 'borrower', '00000000-0000-0000-0000-00000000a001')$q$,
  '23505', 'один аккаунт на компанию');

-- ── заявки ──
SELECT pg_temp.expect_fail(
  $q$UPDATE applications SET amount = 0 WHERE number = 1$q$, '23514', 'сумма заявки > 0');
SELECT pg_temp.expect_fail(
  $q$UPDATE applications SET status = 'agreed'
     WHERE id = '00000000-0000-0000-0000-0000000a0001'$q$, '23514', 'agreed без accepted_offer_id');
SELECT pg_temp.expect_fail(
  $q$UPDATE applications SET currency = 'XXX'
     WHERE id = '00000000-0000-0000-0000-0000000a0001'$q$, '23503', 'валюта из справочника');

-- ── офферы ──
SELECT pg_temp.expect_fail(
  $q$INSERT INTO offers (application_id, lender_organization_id, created_by_user_id, amount, currency,
                         interest_rate_annual, repayment_schedule)
     VALUES ('00000000-0000-0000-0000-0000000a0001', '00000000-0000-0000-0000-00000000b001',
             '00000000-0000-0000-0000-0000000000b1', 100, 'EUR', 5, 'bullet')$q$,
  '23505', 'второй активный оффер той же компании');
SELECT pg_temp.expect_fail(
  $q$UPDATE offers SET status = 'rejected', decided_at = now()
     WHERE id = '00000000-0000-0000-0000-0000000f0002'$q$, '23514', 'rejected без причины');

-- ── принятие оффера: транзакция как в сервисе acceptOffer ──
SELECT id FROM applications WHERE id = '00000000-0000-0000-0000-0000000a0001' FOR UPDATE;

UPDATE offers SET status = 'accepted', decided_at = now()
 WHERE id = '00000000-0000-0000-0000-0000000f0001' AND status = 'sent';

UPDATE offers SET status = 'rejected', reject_reason = 'another_offer_accepted', decided_at = now()
 WHERE application_id = '00000000-0000-0000-0000-0000000a0001' AND status = 'sent';

UPDATE applications SET status = 'agreed', accepted_offer_id = '00000000-0000-0000-0000-0000000f0001'
 WHERE id = '00000000-0000-0000-0000-0000000a0001' AND status = 'published';

INSERT INTO deals (application_id, offer_id, borrower_organization_id, lender_organization_id, amount,
                   currency, interest_rate_annual, fee_percent, fee_amount, term_months,
                   target_country_code, transfer_method, repayment_schedule)
SELECT a.id, o.id, a.borrower_organization_id, o.lender_organization_id, o.amount, o.currency,
       o.interest_rate_annual, o.fee_percent, round(o.amount * o.fee_percent / 100, 2), a.term_months,
       a.target_country_code, a.transfer_method, o.repayment_schedule
  FROM applications a JOIN offers o ON o.id = a.accepted_offer_id
 WHERE a.id = '00000000-0000-0000-0000-0000000a0001';

DO $$
DECLARE d deals;
BEGIN
    SELECT * INTO d FROM deals;
    IF d.fee_amount <> 5000.00 THEN RAISE EXCEPTION 'FAIL fee_amount = %', d.fee_amount; END IF;
    IF (SELECT status FROM offers WHERE id = '00000000-0000-0000-0000-0000000f0002') <> 'rejected'
        THEN RAISE EXCEPTION 'FAIL второй оффер не отклонён'; END IF;
    RAISE NOTICE 'ok   принятие оффера: сделка создана, комиссия 1%% = 5000.00 EUR, второй оффер отклонён';
END $$;

-- после принятия нельзя принять ещё один оффер и нельзя создать вторую сделку
SELECT pg_temp.expect_fail(
  $q$UPDATE offers SET status = 'accepted', reject_reason = NULL
     WHERE id = '00000000-0000-0000-0000-0000000f0002'$q$, '23505', 'второй accepted на заявку');
SELECT pg_temp.expect_fail(
  $q$INSERT INTO deals (application_id, offer_id, borrower_organization_id, lender_organization_id,
                        amount, currency, interest_rate_annual, term_months, target_country_code,
                        transfer_method, repayment_schedule)
     VALUES ('00000000-0000-0000-0000-0000000a0001', '00000000-0000-0000-0000-0000000f0002',
             '00000000-0000-0000-0000-00000000a001', '00000000-0000-0000-0000-00000000b002',
             1, 'EUR', 1, 1, 'DE', 'swift', 'bullet')$q$, '23505', 'вторая сделка по заявке');

-- ── чат, AML, документы ──
SELECT pg_temp.expect_fail(
  $q$INSERT INTO messages (offer_id, author_user_id, text)
     VALUES ('00000000-0000-0000-0000-0000000f0001', '00000000-0000-0000-0000-0000000000a1', '')$q$,
  '23514', 'пустое сообщение');
SELECT pg_temp.expect_fail(
  $q$INSERT INTO aml_flags (organization_id, rule, severity, details, status)
     VALUES ('00000000-0000-0000-0000-00000000b001', 'sanctions_match', 'high', 'x', 'confirmed')$q$,
  '23514', 'закрытый AML-флаг без resolved_at');
SELECT pg_temp.expect_fail(
  $q$INSERT INTO kyc_documents (organization_id, doc_type, storage_key, file_name, mime_type,
                                size_bytes, sha256, uploaded_by_user_id)
     VALUES ('00000000-0000-0000-0000-00000000a001', 'charter', 'k1', 'evil.html', 'text/html', 10,
             sha256('x'), '00000000-0000-0000-0000-0000000000a1')$q$, '23514', 'запрещённый MIME');

-- ── нечёткий поиск по санкционному списку ──
INSERT INTO sanctions_entries (source, external_id, entity_kind, name, name_normalized)
VALUES ('mock', '1', 'organization', 'Evil Trading LLC', 'evil trading');
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM sanctions_entries WHERE name_normalized % 'evill tradng') THEN
        RAISE EXCEPTION 'FAIL pg_trgm не нашёл похожее имя';
    END IF;
    RAISE NOTICE 'ok   pg_trgm: «evill tradng» ≈ «evil trading»';
END $$;

ROLLBACK;
