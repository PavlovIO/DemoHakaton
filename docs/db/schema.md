# Схема БД

PostgreSQL 16. Миграции лежат в `backend/migrations` в формате [goose](https://github.com/pressly/goose). Схема соответствует контракту `api/openapi.yaml` v0.1.0. Решения, на которых она построена, описаны в [ADR-0001](../adr/0001-architecture-and-api-contract.md).

| Файл | Что делает |
|---|---|
| `00001_init.sql` | Расширения, enum-типы, все таблицы, индексы, триггеры `updated_at` |
| `00002_seed_dictionaries.sql` | Справочники: страны (с FATF и «недружественностью»), валюты, способы перевода |
| `testdata/invariants_test.sql` | Smoke-тест ограничений: 19 проверок, в том числе транзакция принятия оффера |

## Как запустить

```bash
# применить
goose -dir backend/migrations postgres "$DATABASE_URL" up
# проверить инварианты (всё в транзакции, откатывается)
psql "$DATABASE_URL" -f backend/migrations/testdata/invariants_test.sql
# откатить всё
goose -dir backend/migrations postgres "$DATABASE_URL" reset
```

sqlc умеет читать goose-миграции напрямую. В `sqlc.yaml` достаточно указать `schema: backend/migrations`.

## ER-диаграмма

```mermaid
erDiagram
    countries ||--o{ organizations : "country_code"
    currencies ||--o{ applications : "currency"
    transfer_methods ||--o{ applications : "transfer_method"

    organizations ||--o| users : "1 аккаунт на компанию (MVP)"
    organizations ||--o{ beneficiaries : ""
    organizations ||--o{ kyc_documents : ""
    organizations ||--o{ registry_checks : "снапшоты реестров"
    organizations ||--o{ aml_flags : ""

    users ||--o{ sessions : ""
    users ||--o{ notifications : ""
    users ||--o{ audit_log : ""

    organizations ||--o{ applications : "borrower"
    applications ||--o{ offers : ""
    organizations ||--o{ offers : "lender"
    applications |o--o| offers : "accepted_offer_id"
    applications ||--o| deals : ""
    offers ||--o| deals : ""
    offers ||--o{ messages : "чат = оффер"
    offers ||--o{ message_reads : ""

    organizations {
        uuid id PK
        organization_type type "borrower | lender"
        text name
        char2 country_code FK
        text registration_number "ИНН / рег. номер"
        char20 lei "опционально"
        kyc_status kyc_status
        timestamptz verified_at
    }
    users {
        uuid id PK
        citext email UK
        text password_hash "argon2id"
        user_role role
        user_status status
        uuid organization_id FK "NULL для admin"
    }
    applications {
        uuid id PK
        bigint number UK "APP-000123"
        uuid borrower_organization_id FK
        application_status status
        numeric amount "17,2"
        char3 currency FK
        char2 target_country_code FK
        int term_months
        uuid accepted_offer_id FK
    }
    offers {
        uuid id PK
        uuid application_id FK
        uuid lender_organization_id FK
        offer_status status
        numeric amount
        numeric interest_rate_annual "7,4"
        numeric fee_percent "nullable"
        repayment_schedule repayment_schedule
    }
    deals {
        uuid id PK
        bigint number UK "DEAL-000045"
        uuid application_id UK
        uuid offer_id UK
        numeric amount
        numeric fee_amount
        timestamptz agreed_at
    }
```

Таблицы без связей на диаграмме:
- `status_history` — ссылается полиморфно через `entity_type` + `entity_id`;
- `sanctions_entries` — загруженные санкционные списки.

## Таблицы

### Справочники

| Таблица | Назначение | API |
|---|---|---|
| `countries` | ISO 3166-1 alpha-2, названия ru/en, `fatf_list` (`black`/`grey`/NULL) + `fatf_as_of`, `is_unfriendly` | `GET /dictionaries/countries`. Поле `fatf_high_risk = fatf_list IS NOT NULL` |
| `currencies` | ISO 4217 | `GET /dictionaries/currencies` |
| `transfer_methods` | swift / sepa / other | `GET /dictionaries/transfer-methods` |

В сиде не полный ISO, а около 30 стран, нужных для демо и типичных юрисдикций. Чёрный список FATF взят по итогам пленара июня 2026. Серый список и перечень недружественных стран (распоряжение № 430-р) нужно сверить с первоисточниками: это отдельная задача.

### Организации и пользователи

- **`organizations`** — юрлицо, которое проходит KYC. Статус `kyc_status` хранится здесь, а не у пользователя.
- **`beneficiaries`** — бенефициары из `PATCH /organizations/me`. Обновлять заменой списка целиком: delete + insert в одной транзакции.
- **`users`** — учётные записи.
  - `email` типа `citext`, поэтому `Alpha@x.com` и `alpha@x.com` — один пользователь.
  - У админа `organization_id IS NULL`.
- **`sessions`** — серверные сессии.
  - В cookie лежит случайный токен (32 байта), в БД хранится только `sha256(token)`. Утечка БД не даёт войти.
  - Блокировка пользователя: `DELETE FROM sessions WHERE user_id = $1`.
  - Просроченные сессии чистит периодическая задача: `DELETE … WHERE expires_at < now()`.

### KYC

- **`kyc_documents`** — метаданные файлов.
  - Сам файл лежит на диске или в S3 под `storage_key` (случайное имя), вне веб-корня.
  - `file_name` нужен только для показа: экранируется и в путях не используется.
  - MIME проверяется по содержимому файла. CHECK в БД — вторая линия защиты.
- **`registry_checks`** — сырые ответы DaData и GLEIF (или мока) с датой. Это доказательная база проверки, её видит админ в `GET /admin/organizations/{id}` (`registry_check`).

### Маркетплейс

- **`applications`** — заявки заёмщиков. Номер `number` выдаётся автоматически (identity), фронт показывает его как `APP-%06d`.
- **`offers`** — предложения кредиторов. Чат привязан к офферу, то есть к паре «заёмщик ↔ кредитор» по конкретной заявке.
- **`deals`** — неизменяемый снапшот условий на момент принятия. `fee_amount = round(amount × fee_percent / 100, 2)` считает бэкенд.
- **`messages`** + **`message_reads`** — чат с polling по `id > after_id`. По указателю «прочитано до» считается `unread_messages`.

### Служебные

- **`notifications`** пишутся в той же транзакции, что и событие, без брокера.
- **`status_history`** — каждый переход статуса заявки, оффера или KYC. Отдаётся в `GET /applications/{id}/history`.
- **`aml_flags`** — результаты rule-based скрининга, админ их разбирает.
- **`sanctions_entries`** — загруженные списки OFAC/EU/UN/UK (или мок), с GIN-индексом `pg_trgm` для нечёткого поиска.
- **`audit_log`** — журнал действий: логин, неудачный логин, загрузка документа, решения админа и т. д. Только INSERT.

## Где проверяется каждый инвариант

Правило такое: если нарушение инварианта означает испорченные данные, он защищён в БД. Если это бизнес-правило, которое может поменяться, или проверка, требующая данных из другой таблицы, он проверяется в сервисе и покрывается модульными тестами.

| Инвариант | Где |
|---|---|
| Заёмщик только из RU, кредитор не из RU | БД `organizations_type_country_chk` |
| Рег. номер уникален в пределах страны, LEI уникален | БД |
| `verified` ⇔ `verified_at` заполнен | БД |
| Админ без компании, остальные с компанией | БД `users_admin_org_chk` |
| Один аккаунт на компанию (решение для MVP) | БД `users_one_per_org_uq`. Чтобы снять — удалить индекс |
| Роль пользователя совпадает с типом организации | **Сервис** регистрации |
| Создавать заявки и офферы можно только при `kyc_status = verified` | **Сервис** |
| Разрешённые переходы статусов | **Сервис** (таблица переходов + табличные тесты) |
| `agreed` ⇔ есть `accepted_offer_id` | БД `applications_agreed_chk` |
| Один активный оффер компании на заявку | БД `offers_one_active_per_lender_uq` |
| Не больше одного принятого оффера на заявку | БД `offers_one_accepted_uq` — страховка от гонки |
| Одна сделка на заявку и на оффер | БД `UNIQUE` |
| Сумма оффера ≤ суммы заявки, валюта оффера = валюта заявки | **Сервис** (нужны данные другой таблицы) |
| `rejected` ⇔ указана причина; `sent` ⇔ нет `decided_at` | БД |
| Деньги > 0, проценты в разумных пределах, длины строк | БД `CHECK` + валидация по OpenAPI |
| Только PDF/PNG/JPEG, файлы ≤ 10 MB | **Сервис** (по содержимому) + БД |
| Закрытый AML-флаг ⇔ есть кто и когда закрыл | БД |
| Доступ только к своим сущностям (IDOR) | **Репозиторий**: фильтр по `organization_id` в каждом запросе |

## Ключевые запросы

### Принятие оффера (`POST /offers/{offerId}/accept`)

Всё выполняется в одной транзакции. `FOR UPDATE` на заявке сериализует параллельные клики. Если гонка всё же прорвётся, второй коммит упадёт на `offers_one_accepted_uq`.

```sql
BEGIN;
-- 1. Блокируем заявку и одновременно проверяем владельца и статус
SELECT a.* FROM applications a
  JOIN offers o ON o.application_id = a.id
 WHERE o.id = $offer_id
   AND a.borrower_organization_id = $my_org_id     -- IDOR: чужая заявка → 0 строк → 404
   AND a.status = 'published'
   FOR UPDATE OF a;                                -- 0 строк при верном владельце → 409

-- 2. Принимаем оффер (должна обновиться ровно 1 строка, иначе 409)
UPDATE offers SET status = 'accepted', decided_at = now()
 WHERE id = $offer_id AND status = 'sent';

-- 3. Остальные активные офферы отклоняем
UPDATE offers SET status = 'rejected', reject_reason = 'another_offer_accepted', decided_at = now()
 WHERE application_id = $app_id AND status = 'sent'
RETURNING id, lender_organization_id;              -- им нужны уведомления

-- 4. Заявка → agreed
UPDATE applications SET status = 'agreed', accepted_offer_id = $offer_id WHERE id = $app_id;

-- 5. Снапшот сделки, 6. status_history, 7. notifications, 8. audit_log
INSERT INTO deals (...) SELECT ... RETURNING *;
COMMIT;
```

### Витрина кредитора (`GET /applications` под ролью lender)

```sql
SELECT a.*, o.name, o.country_code,
       (SELECT id FROM offers WHERE application_id = a.id
          AND lender_organization_id = $my_org_id AND status = 'sent') AS my_offer_id
  FROM applications a
  JOIN organizations o ON o.id = a.borrower_organization_id
 WHERE (a.status = 'published'
        OR EXISTS (SELECT 1 FROM offers f WHERE f.application_id = a.id
                     AND f.lender_organization_id = $my_org_id))      -- архив своих откликов
   AND ($country::char(2) IS NULL OR a.target_country_code = $country)
   AND ($currency::char(3) IS NULL OR a.currency = $currency)
   AND ($amount_min::numeric IS NULL OR a.amount >= $amount_min)
   AND ($amount_max::numeric IS NULL OR a.amount <= $amount_max)
   AND ($term_min::int IS NULL OR a.term_months >= $term_min)
   AND ($term_max::int IS NULL OR a.term_months <= $term_max)
 ORDER BY a.published_at DESC NULLS LAST
 LIMIT $page_size OFFSET ($page - 1) * $page_size;
```

Индекс `applications_showcase_idx (target_country_code, currency, amount) WHERE status = 'published'` покрывает основной фильтр.

### Аналитика (`GET /admin/analytics/summary`)

```sql
-- Средний размер по каждой валюте отдельно (без конвертации)
SELECT currency, count(*), sum(amount), round(avg(amount), 2)
  FROM applications WHERE status NOT IN ('draft') AND created_at BETWEEN $from AND $to
 GROUP BY currency;

-- Топ стран спроса (куда хотят деньги)
SELECT target_country_code, count(*) FROM applications
 WHERE status IN ('published', 'agreed') GROUP BY 1 ORDER BY 2 DESC LIMIT $n;

-- Топ стран предложения (откуда кредиторы, сделавшие офферы)
SELECT org.country_code, count(DISTINCT o.id) FROM offers o
  JOIN organizations org ON org.id = o.lender_organization_id
 GROUP BY 1 ORDER BY 2 DESC LIMIT $n;
```

### Санкционный скрининг

```sql
SET pg_trgm.similarity_threshold = 0.4;            -- широкий отбор кандидатов
SELECT id, source, name, similarity(name_normalized, $q) AS score
  FROM sanctions_entries
 WHERE name_normalized % $q
 ORDER BY score DESC LIMIT 20;
-- Кандидатов досчитать Jaro-Winkler в Go. ≥ 0.90 → флаг high, 0.85–0.90 → medium (ручная проверка).
```

Перед поиском имя нормализуется одинаково для списка и для запроса:
- нижний регистр;
- транслитерация кириллицы в латиницу;
- удаление орг. форм (ООО, LLC, Ltd, GmbH);
- схлопывание пробелов.

## Что сознательно не делаем в MVP

- **Мягкое удаление и версионирование заявок.** Достаточно `status_history`.
- **Партиционирование `audit_log` и `messages`.** Объёмы демо этого не требуют.
- **Row-Level Security.** IDOR закрываем в репозитории. RLS — хороший следующий шаг, если останется время.
- **Конвертация валют.** Суммы и аналитика считаются по каждой валюте отдельно.
