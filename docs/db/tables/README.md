# Таблицы БД

По файлу на таблицу: поля с типами и Go-типами sqlc, ограничения, связи (mermaid) и sqlc-запросы из `backend/queries`. Общая схема, ER-диаграмма и инварианты — в [schema.md](../schema.md).

Источники: `backend/migrations/*.sql`, `backend/queries/*.sql`, сгенерированный код `backend/internal/repository/sqlc` (пакет `repository`).

| Таблица | Группа | PK | sqlc-модель | Файл запросов | Запросов |
|---|---|---|---|---|---|
| [countries](countries.md) | справочники | `code char(2)` | `Country` | `dictionaries.sql` | 1 |
| [currencies](currencies.md) | справочники | `code char(3)` | `Currency` | `dictionaries.sql` | 1 |
| [transfer_methods](transfer_methods.md) | справочники | `code text` | `TransferMethod` | `dictionaries.sql` | 1 |
| [organizations](organizations.md) | организации и пользователи | `uuid` | `Organization` | `organizations.sql` | 8 |
| [beneficiaries](beneficiaries.md) | организации и пользователи | `uuid` | `Beneficiary` | — | **0** |
| [users](users.md) | организации и пользователи | `uuid` | `User` | `users.sql` | 8 |
| [sessions](sessions.md) | организации и пользователи | `token_hash bytea` | `Session` | `sessions.sql` | 6 |
| [kyc_documents](kyc_documents.md) | KYC | `uuid` | `KycDocument` | `kyc.sql` | 3 |
| [registry_checks](registry_checks.md) | KYC | `bigint` | `RegistryCheck` | `kyc.sql` | 2 |
| [applications](applications.md) | маркетплейс | `uuid` | `Application` | `applications.sql` | 11 (+ 3 аналитических в `admin.sql`) |
| [offers](offers.md) | маркетплейс | `uuid` | `Offer` | `offers.sql` | 10 |
| [deals](deals.md) | маркетплейс | `uuid` | `Deal` | `deals.sql` | 4 |
| [messages](messages.md) | чат | `bigint` | `Message` | `chat.sql` | 3 |
| [message_reads](message_reads.md) | чат | `(offer_id, user_id)` | `MessageRead` | `chat.sql` | 1 (+ чтение в `*OffersVisible`) |
| [notifications](notifications.md) | служебные | `bigint` | `Notification` | `notifications.sql` | 5 |
| [status_history](status_history.md) | служебные | `bigint` | `StatusHistory` | `applications.sql` | 1 (только запись) |
| [aml_flags](aml_flags.md) | AML | `bigint` | `AmlFlag` | `admin.sql` | 3 |
| [sanctions_entries](sanctions_entries.md) | AML | `bigint` | `SanctionsEntry` | `admin.sql` | 1 |
| [audit_log](audit_log.md) | служебные | `bigint` | `AuditLog` | `admin.sql` | 2 |

Ещё 3 аналитических запроса из `admin.sql` (`GetAnalyticsSummaryByCurrency`, `GetTopDemandCountries`, `GetTopSupplyCountries`) читают `applications`, `offers` и `organizations` — см. [schema.md](../schema.md#покрытие-sqlc).

## Соглашения о типах в Go

Из `backend/sqlc.yaml`:

| PostgreSQL | Go |
|---|---|
| `uuid` NOT NULL | `uuid.UUID` (google/uuid) |
| `uuid` NULL | `pgtype.UUID` |
| `numeric` NOT NULL / NULL | `decimal.Decimal` / `decimal.NullDecimal` (shopspring) |
| `citext` | `string` |
| `text` NULL | `pgtype.Text` |
| `timestamptz` | `pgtype.Timestamptz` |
| `inet` | `*netip.Addr` |
| `jsonb`, `bytea` | `[]byte` |
| enum `x` NOT NULL / NULL | `X` / `NullX` |
