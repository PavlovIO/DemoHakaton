# `registry_checks`

Снапшоты ответов реестра. В MVP реестр — только мок (`source = 'mock'`); значения `egrul_dadata` и `gleif` остаются в enum на будущее. Это доказательная база проверки, админ видит её в `GET /admin/organizations/{id}` (поле `registry_check`).

- Группа: KYC
- sqlc-модель: `repository.RegistryCheck`
- API: пишется при `POST /organizations/me/kyc/submit`, читается в `GET /admin/organizations/{id}`. Публичный `GET /registry/lookup` в таблицу не пишет: запись с ручки без логина позволила бы забить таблицу, а снапшот без компании ни к чему не привязать.

## Поля

| Поле | Тип | NULL | По умолчанию | Go (sqlc) | Описание |
|---|---|---|---|---|---|
| `id` | `bigint` identity PK | нет | — | `int64` | |
| `organization_id` | `uuid` FK → `organizations.id` ON DELETE CASCADE | да | — | `pgtype.UUID` | Компания; приложение всегда заполняет (снапшот пишется только при `kyc/submit`) |
| `source` | `registry_source` (`egrul_dadata` / `gleif` / `mock`) | нет | — | `RegistrySource` | |
| `query` | `text` | нет | — | `string` | Что искали: ИНН или LEI |
| `found` | `boolean` | нет | — | `bool` | Нашлось ли |
| `name` | `text` | да | — | `pgtype.Text` | Название по реестру |
| `registration_number` | `text` | да | — | `pgtype.Text` | |
| `legal_address` | `text` | да | — | `pgtype.Text` | |
| `registry_status` | `text` | да | — | `pgtype.Text` | Статус в реестре (действующая, ликвидирована…) |
| `raw_response` | `jsonb` | да | — | `[]byte` | Сырой ответ |
| `checked_at` | `timestamptz` | нет | `now()` | `pgtype.Timestamptz` | |

## Ограничения и индексы

- `registry_checks_org_idx (organization_id, checked_at DESC)` — последняя проверка компании.

## Связи

```mermaid
erDiagram
    organizations |o--o{ registry_checks : "organization_id (nullable)"
```

- Ссылается на: [`organizations`](organizations.md) (необязательно).

## sqlc-запросы (`queries/kyc.sql`)

| Запрос | Тип | Что делает |
|---|---|---|
| `CreateRegistryCheck` | `:one` | Сохраняет снапшот ответа |
| `GetLatestRegistryCheck` | `:one` | Последняя проверка компании |

Запрос привязки проверки к компании (`AttachRegistryCheck`) не нужен: lookup до регистрации в БД не пишет, снапшот сохраняется уже с `organization_id` (BE-06).
