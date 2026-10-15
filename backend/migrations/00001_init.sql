-- +goose Up
-- +goose StatementBegin
-- Liquidity Platform — начальная схема БД.
-- Соответствует api/openapi.yaml v0.1.0. Описание и ER-диаграмма: docs/db/schema.md
--
-- Соглашения:
--   * PK бизнес-сущностей — uuid (не угадываются в URL, защита от перебора);
--     служебные/журнальные таблицы — bigint identity.
--   * Деньги — NUMERIC(17,2), проценты — NUMERIC(7,4). Никаких float.
--   * Все timestamp — timestamptz (UTC).
--   * Перечисления — PostgreSQL ENUM (sqlc генерирует из них Go-типы).
--   * Проверка владения (IDOR) делается в SQL-запросах репозитория, а не здесь;
--     здесь — только инварианты, которые нельзя нарушить ни при каком коде.

CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;    -- регистронезависимый email
CREATE EXTENSION IF NOT EXISTS pg_trgm;   -- нечёткий поиск по санкционным спискам

-- ───────────────────────── enums ─────────────────────────
CREATE TYPE user_role          AS ENUM ('admin', 'borrower', 'lender');
CREATE TYPE user_status        AS ENUM ('active', 'blocked');
CREATE TYPE organization_type  AS ENUM ('borrower', 'lender');
CREATE TYPE kyc_status         AS ENUM ('draft', 'under_review', 'verified', 'rejected');
CREATE TYPE kyc_document_type  AS ENUM ('charter', 'registration_certificate', 'address_proof',
                                        'beneficiaries_register', 'source_of_funds', 'other');
CREATE TYPE application_status AS ENUM ('draft', 'moderation', 'rejected', 'published', 'agreed', 'cancelled');
CREATE TYPE offer_status       AS ENUM ('sent', 'accepted', 'rejected', 'withdrawn');
CREATE TYPE offer_reject_reason AS ENUM ('rejected_by_borrower', 'another_offer_accepted', 'application_cancelled');
CREATE TYPE repayment_schedule AS ENUM ('bullet', 'annuity', 'interest_monthly_principal_bullet', 'custom');
CREATE TYPE entity_type        AS ENUM ('user', 'organization', 'document', 'application', 'offer',
                                        'deal', 'message', 'aml_flag');
CREATE TYPE aml_rule           AS ENUM ('sanctions_match', 'high_risk_jurisdiction', 'amount_threshold',
                                        'registry_mismatch', 'other');
CREATE TYPE aml_severity       AS ENUM ('low', 'medium', 'high');
CREATE TYPE aml_flag_status    AS ENUM ('open', 'false_positive', 'confirmed');
CREATE TYPE fatf_list          AS ENUM ('black', 'grey');
CREATE TYPE registry_source    AS ENUM ('egrul_dadata', 'gleif', 'mock');
CREATE TYPE notification_type  AS ENUM (
    'kyc_submitted', 'kyc_approved', 'kyc_rejected',
    'application_submitted', 'application_published', 'application_rejected',
    'offer_received', 'offer_accepted', 'offer_rejected', 'offer_withdrawn',
    'message_received', 'aml_flag_created'
);

-- Общий триггер для updated_at
CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

-- ───────────────────────── справочники ─────────────────────────
CREATE TABLE countries (
    code         char(2)  PRIMARY KEY CHECK (code ~ '^[A-Z]{2}$'),   -- ISO 3166-1 alpha-2
    name_ru      text     NOT NULL,
    name_en      text     NOT NULL,
    fatf_list    fatf_list,                                          -- NULL = не в списках FATF
    fatf_as_of   date,                                               -- дата пленара FATF
    is_unfriendly boolean NOT NULL DEFAULT false                     -- перечень недружественных стран РФ
);

CREATE TABLE currencies (
    code        char(3)  PRIMARY KEY CHECK (code ~ '^[A-Z]{3}$'),    -- ISO 4217
    name        text     NOT NULL,
    minor_units smallint NOT NULL DEFAULT 2
);

CREATE TABLE transfer_methods (
    code text PRIMARY KEY,                                           -- swift, sepa, other
    name text NOT NULL
);

-- ───────────────────────── организации и пользователи ─────────────────────────
CREATE TABLE organizations (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type                organization_type NOT NULL,
    name                text NOT NULL CHECK (length(name) BETWEEN 2 AND 300),
    country_code        char(2) NOT NULL REFERENCES countries(code),
    registration_number text NOT NULL CHECK (length(registration_number) BETWEEN 1 AND 50), -- ИНН для RU
    lei                 char(20) CHECK (lei ~ '^[A-Z0-9]{20}$'),
    legal_address       text NOT NULL CHECK (length(legal_address) <= 1000),
    kyc_status          kyc_status NOT NULL DEFAULT 'draft',
    kyc_reject_reason   text,
    kyc_submitted_at    timestamptz,
    verified_at         timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    -- Правило кейса: заёмщик зарегистрирован в РФ, кредитор — за её пределами.
    CONSTRAINT organizations_type_country_chk CHECK (
        (type = 'borrower' AND country_code = 'RU') OR
        (type = 'lender'   AND country_code <> 'RU')
    ),
    CONSTRAINT organizations_verified_chk CHECK ((kyc_status = 'verified') = (verified_at IS NOT NULL)),
    CONSTRAINT organizations_regnum_uq UNIQUE (country_code, registration_number)
);
CREATE UNIQUE INDEX organizations_lei_uq ON organizations (lei) WHERE lei IS NOT NULL;
CREATE INDEX organizations_kyc_status_idx ON organizations (kyc_status, created_at);
CREATE TRIGGER organizations_updated_at BEFORE UPDATE ON organizations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE beneficiaries (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id          uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    full_name                text NOT NULL CHECK (length(full_name) BETWEEN 1 AND 300),
    citizenship_country_code char(2) NOT NULL REFERENCES countries(code),
    ownership_percent        numeric(7,4) NOT NULL CHECK (ownership_percent > 0 AND ownership_percent <= 100),
    created_at               timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX beneficiaries_org_idx ON beneficiaries (organization_id);

CREATE TABLE users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email           citext NOT NULL UNIQUE CHECK (length(email) <= 254),
    password_hash   text NOT NULL,                                   -- argon2id, PHC-строка
    role            user_role NOT NULL,
    status          user_status NOT NULL DEFAULT 'active',
    organization_id uuid REFERENCES organizations(id) ON DELETE RESTRICT,
    blocked_reason  text,
    last_login_at   timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    -- admin не привязан к компании, остальные — обязательно.
    -- Соответствие role ↔ organizations.type проверяется в сервисе регистрации.
    CONSTRAINT users_admin_org_chk CHECK ((role = 'admin') = (organization_id IS NULL))
);
-- MVP: один аккаунт на компанию. Чтобы разрешить нескольких сотрудников — удалить этот индекс.
CREATE UNIQUE INDEX users_one_per_org_uq ON users (organization_id) WHERE organization_id IS NOT NULL;
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Серверные сессии. В cookie лежит случайный токен, в БД — только его SHA-256.
CREATE TABLE sessions (
    token_hash   bytea PRIMARY KEY CHECK (length(token_hash) = 32),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    ip           inet,
    user_agent   text
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

-- ───────────────────────── KYC ─────────────────────────
CREATE TABLE kyc_documents (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    doc_type            kyc_document_type NOT NULL,
    storage_key         text NOT NULL UNIQUE,                        -- случайное имя файла вне веб-корня
    file_name           text NOT NULL CHECK (length(file_name) <= 255), -- оригинальное имя, только для показа
    mime_type           text NOT NULL CHECK (mime_type IN ('application/pdf', 'image/png', 'image/jpeg')),
    size_bytes          bigint NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 10485760), -- ≤ 10 MB
    sha256              bytea NOT NULL CHECK (length(sha256) = 32),
    uploaded_by_user_id uuid NOT NULL REFERENCES users(id),
    uploaded_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX kyc_documents_org_idx ON kyc_documents (organization_id, doc_type);

-- Снапшоты ответов внешних реестров (доказательная база проверки).
CREATE TABLE registry_checks (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id     uuid REFERENCES organizations(id) ON DELETE CASCADE, -- NULL для lookup до регистрации
    source              registry_source NOT NULL,
    query               text NOT NULL,                               -- ИНН / LEI
    found               boolean NOT NULL,
    name                text,
    registration_number text,
    legal_address       text,
    registry_status     text,
    raw_response        jsonb,
    checked_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX registry_checks_org_idx ON registry_checks (organization_id, checked_at DESC);

-- ───────────────────────── заявки ─────────────────────────
CREATE TABLE applications (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number                   bigint GENERATED ALWAYS AS IDENTITY UNIQUE, -- показывается как APP-000123
    borrower_organization_id uuid NOT NULL REFERENCES organizations(id),
    created_by_user_id       uuid NOT NULL REFERENCES users(id),
    status                   application_status NOT NULL DEFAULT 'draft',
    amount                   numeric(17,2) NOT NULL CHECK (amount > 0),
    currency                 char(3) NOT NULL REFERENCES currencies(code),
    target_country_code      char(2) NOT NULL REFERENCES countries(code),
    purpose                  text NOT NULL CHECK (length(purpose) BETWEEN 10 AND 2000),
    term_months              integer NOT NULL CHECK (term_months BETWEEN 1 AND 120),
    transfer_method          text NOT NULL REFERENCES transfer_methods(code),
    transfer_method_comment  text CHECK (length(transfer_method_comment) <= 500),
    moderation_comment       text,
    accepted_offer_id        uuid,                                   -- FK добавляется ниже (цикл с offers)
    submitted_at             timestamptz,
    published_at             timestamptz,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT applications_agreed_chk CHECK ((status = 'agreed') = (accepted_offer_id IS NOT NULL))
);
-- Витрина кредитора: только published, фильтры по стране/валюте+сумме/сроку.
CREATE INDEX applications_showcase_idx ON applications (target_country_code, currency, amount)
    WHERE status = 'published';
CREATE INDEX applications_status_created_idx ON applications (status, created_at DESC);
CREATE INDEX applications_borrower_idx ON applications (borrower_organization_id, created_at DESC);
CREATE TRIGGER applications_updated_at BEFORE UPDATE ON applications
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ───────────────────────── офферы ─────────────────────────
CREATE TABLE offers (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id         uuid NOT NULL REFERENCES applications(id),
    lender_organization_id uuid NOT NULL REFERENCES organizations(id),
    created_by_user_id     uuid NOT NULL REFERENCES users(id),
    status                 offer_status NOT NULL DEFAULT 'sent',
    reject_reason          offer_reject_reason,
    reject_comment         text CHECK (length(reject_comment) <= 1000),
    amount                 numeric(17,2) NOT NULL CHECK (amount > 0),   -- ≤ суммы заявки — проверяет сервис
    currency               char(3) NOT NULL REFERENCES currencies(code), -- = валюте заявки — проверяет сервис
    interest_rate_annual   numeric(7,4) NOT NULL CHECK (interest_rate_annual >= 0 AND interest_rate_annual <= 1000),
    fee_percent            numeric(7,4) CHECK (fee_percent >= 0 AND fee_percent <= 100),
    repayment_schedule     repayment_schedule NOT NULL,
    collateral             text CHECK (length(collateral) <= 2000),
    additional_terms       text CHECK (length(additional_terms) <= 4000),
    valid_until            timestamptz,
    created_at             timestamptz NOT NULL DEFAULT now(),
    decided_at             timestamptz,

    CONSTRAINT offers_reject_reason_chk CHECK ((status = 'rejected') = (reject_reason IS NOT NULL)),
    CONSTRAINT offers_decided_chk CHECK ((status = 'sent') = (decided_at IS NULL))
);
-- Одна компания — не более одного активного оффера на заявку.
CREATE UNIQUE INDEX offers_one_active_per_lender_uq ON offers (application_id, lender_organization_id)
    WHERE status = 'sent';
-- Страховка от гонки двойного принятия: не более одного принятого оффера на заявку.
CREATE UNIQUE INDEX offers_one_accepted_uq ON offers (application_id) WHERE status = 'accepted';
CREATE INDEX offers_application_idx ON offers (application_id, status);
CREATE INDEX offers_lender_idx ON offers (lender_organization_id, created_at DESC);

ALTER TABLE applications
    ADD CONSTRAINT applications_accepted_offer_fk
    FOREIGN KEY (accepted_offer_id) REFERENCES offers(id);

-- ───────────────────────── сделки ─────────────────────────
-- Снапшот условий на момент принятия оффера. После создания не меняется.
CREATE TABLE deals (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number                   bigint GENERATED ALWAYS AS IDENTITY UNIQUE, -- DEAL-000045
    application_id           uuid NOT NULL UNIQUE REFERENCES applications(id),
    offer_id                 uuid NOT NULL UNIQUE REFERENCES offers(id),
    borrower_organization_id uuid NOT NULL REFERENCES organizations(id),
    lender_organization_id   uuid NOT NULL REFERENCES organizations(id),
    amount                   numeric(17,2) NOT NULL CHECK (amount > 0),
    currency                 char(3) NOT NULL REFERENCES currencies(code),
    interest_rate_annual     numeric(7,4) NOT NULL,
    fee_percent              numeric(7,4),
    fee_amount               numeric(17,2),                            -- amount × fee_percent / 100
    term_months              integer NOT NULL,
    target_country_code      char(2) NOT NULL REFERENCES countries(code),
    transfer_method          text NOT NULL REFERENCES transfer_methods(code),
    repayment_schedule       repayment_schedule NOT NULL,
    collateral               text,
    additional_terms         text,
    agreed_at                timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT deals_parties_chk CHECK (borrower_organization_id <> lender_organization_id),
    CONSTRAINT deals_fee_chk CHECK ((fee_percent IS NULL) = (fee_amount IS NULL))
);
CREATE INDEX deals_borrower_idx ON deals (borrower_organization_id, agreed_at DESC);
CREATE INDEX deals_lender_idx ON deals (lender_organization_id, agreed_at DESC);

-- ───────────────────────── чат ─────────────────────────
-- Чат привязан к офферу = к паре «заёмщик–кредитор» по конкретной заявке.
CREATE TABLE messages (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    offer_id       uuid NOT NULL REFERENCES offers(id),
    author_user_id uuid NOT NULL REFERENCES users(id),
    text           text NOT NULL CHECK (length(text) BETWEEN 1 AND 4000), -- plain text, без HTML
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX messages_offer_idx ON messages (offer_id, id); -- polling: WHERE offer_id = $1 AND id > $after

-- Указатель «прочитано до» — для счётчика непрочитанных.
CREATE TABLE message_reads (
    offer_id             uuid NOT NULL REFERENCES offers(id) ON DELETE CASCADE,
    user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_read_message_id bigint NOT NULL,
    PRIMARY KEY (offer_id, user_id)
);

-- ───────────────────────── уведомления ─────────────────────────
-- Пишутся в той же транзакции, что и смена статуса (без брокера).
CREATE TABLE notifications (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        notification_type NOT NULL,
    title       text NOT NULL,
    entity_type entity_type NOT NULL,
    entity_id   text NOT NULL,
    is_read     boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE NOT is_read;

-- ───────────────────────── история статусов ─────────────────────────
CREATE TABLE status_history (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entity_type   entity_type NOT NULL,
    entity_id     uuid NOT NULL,
    from_status   text,
    to_status     text NOT NULL,
    actor_user_id uuid REFERENCES users(id),                         -- NULL = система
    actor_role    user_role,
    comment       text,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX status_history_entity_idx ON status_history (entity_type, entity_id, created_at);

-- ───────────────────────── AML ─────────────────────────
CREATE TABLE aml_flags (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    application_id      uuid REFERENCES applications(id) ON DELETE CASCADE,
    rule                aml_rule NOT NULL,
    severity            aml_severity NOT NULL,
    details             text NOT NULL,
    status              aml_flag_status NOT NULL DEFAULT 'open',
    resolved_by_user_id uuid REFERENCES users(id),
    resolution_comment  text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    resolved_at         timestamptz,

    CONSTRAINT aml_flags_resolution_chk CHECK (
        (status = 'open') = (resolved_at IS NULL AND resolved_by_user_id IS NULL)
    )
);
CREATE INDEX aml_flags_org_idx ON aml_flags (organization_id, status);
CREATE INDEX aml_flags_open_idx ON aml_flags (severity, created_at DESC) WHERE status = 'open';

-- Загруженные санкционные списки (OFAC/EU/UN/UK/…), обновляются загрузчиком.
CREATE TABLE sanctions_entries (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source          text NOT NULL,                                   -- ofac_sdn, eu_fsf, un_sc, uk_fcdo, mock …
    external_id     text NOT NULL,
    entity_kind     text NOT NULL CHECK (entity_kind IN ('organization', 'person', 'other')),
    name            text NOT NULL,
    name_normalized text NOT NULL,                                   -- lower + транслит + без орг. форм
    country_code    char(2),
    raw             jsonb,
    loaded_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source, external_id, name_normalized)
);
CREATE INDEX sanctions_entries_name_trgm_idx ON sanctions_entries USING gin (name_normalized gin_trgm_ops);

-- ───────────────────────── аудит ─────────────────────────
CREATE TABLE audit_log (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    action      text NOT NULL,                                       -- 'offer.accept', 'auth.login_failed' …
    entity_type entity_type,
    entity_id   text,
    ip          inet,
    user_agent  text,
    meta        jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_created_idx ON audit_log (created_at DESC);
CREATE INDEX audit_log_user_idx ON audit_log (user_id, created_at DESC);
CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS audit_log, sanctions_entries, aml_flags, status_history, notifications,
    message_reads, messages, deals CASCADE;
ALTER TABLE IF EXISTS applications DROP CONSTRAINT IF EXISTS applications_accepted_offer_fk;
DROP TABLE IF EXISTS offers, applications, registry_checks, kyc_documents, sessions, users,
    beneficiaries, organizations, transfer_methods, currencies, countries CASCADE;
DROP FUNCTION IF EXISTS set_updated_at();
DROP TYPE IF EXISTS notification_type, registry_source, fatf_list, aml_flag_status, aml_severity,
    aml_rule, entity_type, repayment_schedule, offer_reject_reason, offer_status, application_status,
    kyc_document_type, kyc_status, organization_type, user_status, user_role;
-- +goose StatementEnd
