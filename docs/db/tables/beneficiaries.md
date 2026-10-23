# `beneficiaries`

Конечные бенефициары компании, данные для KYC из `PATCH /organizations/me`. Список обновляется заменой целиком: delete + insert в одной транзакции.

- Группа: организации и пользователи
- sqlc-модель: `repository.Beneficiary`
- API: `GET/PATCH /organizations/me` (поле `beneficiaries`)

## Поля

| Поле | Тип | NULL | По умолчанию | Go (sqlc) | Описание |
|---|---|---|---|---|---|
| `id` | `uuid` PK | нет | `gen_random_uuid()` | `uuid.UUID` | |
| `organization_id` | `uuid` FK → `organizations.id` ON DELETE CASCADE | нет | — | `uuid.UUID` | Компания |
| `full_name` | `text` | нет | — | `string` | ФИО, 1–300 символов |
| `citizenship_country_code` | `char(2)` FK → `countries.code` | нет | — | `string` | Гражданство |
| `ownership_percent` | `numeric(7,4)` | нет | — | `decimal.Decimal` | Доля владения, `(0; 100]` |
| `created_at` | `timestamptz` | нет | `now()` | `pgtype.Timestamptz` | |

## Ограничения и индексы

- `CHECK (ownership_percent > 0 AND ownership_percent <= 100)`.
- `beneficiaries_org_idx (organization_id)`.
- Сумма долей ≤ 100 % в БД не проверяется — это правило сервиса.

## Связи

```mermaid
erDiagram
    organizations ||--o{ beneficiaries : "organization_id"
    countries ||--o{ beneficiaries : "citizenship_country_code"
```

- Ссылается на: [`organizations`](organizations.md), [`countries`](countries.md).

## sqlc-запросы (`queries/organizations.sql`)

| Запрос | Тип | Что делает |
|---|---|---|
| `ListBeneficiariesByOrg` | `:many` | Бенефициары компании, `ORDER BY created_at, id` |
| `DeleteBeneficiariesByOrg` | `:exec` | Очистка списка перед заменой |
| `CreateBeneficiary` | `:one` | Вставка одного (до 50 в цикле, `:copyfrom` не нужен) |
