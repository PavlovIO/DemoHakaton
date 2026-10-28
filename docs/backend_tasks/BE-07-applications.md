# BE-07 · Заявки: CRUD, витрина, статусы, история

Issue: #23 · Milestone: M2 · Зависит от: BE-04 (`Principal` с `KycStatus`, роли, шаблон `*Visible`), BE-11 (каркас `Notifier`, шаг 1 его плана) · Блокирует: BE-08, BE-09, BE-12 (approve/reject заявки используют статусную машину и `status_history` отсюда)

## Цель

Восемь операций тега `applications` (`listApplications`, `createApplication`, `getApplication`, `updateApplication`, `submitApplication`, `cancelApplication`, `rejectAllOffers`, `getApplicationHistory`) отвечают по контракту вместо 501. Заёмщик создаёт заявку, редактирует, отправляет на модерацию и отменяет; кредитор видит витрину `published` и архив своих откликов; каждый переход пишется в `status_history` и порождает уведомления. Чужая или невидимая заявка → 404.

## Что уже есть

- Контракт: `api/paths/applications.yaml` (операции и параметры фильтров после `0acece7`), `api/schemas/applications.yaml` (`Application`, `ApplicationCreate`, `ApplicationUpdate`, `ApplicationPage`, `StatusHistoryEntry`). Сгенерированные типы: `internal/http/api/models.gen.go` → `ListApplicationsParams` (`Status []ApplicationStatus`, `TargetCountry []CountryCode`, `HasMyOffer *bool`, `AmountMin/AmountMax *string`, `BorrowerOrganizationID`, `CreatedFrom/CreatedTo`, `Sort` с 8 значениями, включая `published_at_*`), `ApplicationUpdate`.
- Заглушки: `internal/http/handler/applications.go` — все 8 методов возвращают `apperr.NotImplemented`. Образец операции: `handler/dictionaries.go` + `handler/dictionaries_test.go` + `service/dictionaries.go`.
- Запросы `backend/queries/applications.sql` (до BE-04; BE-04 заменяет `GetApplicationByID`, `ListApplications`, `CountApplications` на `*Visible`): `CreateApplication`, `GetApplicationByID`, `GetApplicationForUpdate`, `UpdateApplication`, `SubmitApplication`, `ApproveApplication`, `RejectApplication`, `CancelApplication`, `AgreeApplication`, `ListApplications`, `CountApplications`, `CreateStatusHistory`. В `offers.sql`: `RejectAllOffersForApplication`.
- Схема: `backend/migrations/00001_init.sql` — `applications` (CHECK `applications_agreed_chk`, индексы `applications_showcase_idx … WHERE status='published'`, `applications_status_created_idx`, `applications_borrower_idx`), `status_history` (`status_history_entity_idx (entity_type, entity_id, created_at)`), enum `notification_type` (есть `application_submitted`, `application_published`, `application_rejected`, `offer_rejected`; **нет** `application_cancelled`).
- Транзакции: `service/tx.go` — `TxManager.InTx(ctx, func(q repository.Querier) error)`.
- Деньги: `handler/money.go` выставляет `decimal.TrimTrailingZeros = false` в `init()` пакета `handler`.
- PR #67 (`origin/be-03-auth`): `service.Principal{UserID, OrgID *uuid.UUID, Role string}` (`service/principal.go`), `middleware.PrincipalFromContext`, `middleware.ClientMetaFromContext` → `service.ClientMeta`; паттерн сервиса — `AuthStore` + `AuthTx` в `service/auth.go`, фейк стора встраивает `repository.Querier` (`service/auth_test.go: fakeAuthStore`).

⚠️ Ловушки в текущем коде:
- `GetApplicationByID` (`queries/applications.sql`) ищет только по `id` — чужая заявка читается подстановкой UUID. Вызывающих нет (`db-security-plan.md`), удаляется.
- `ListApplications` / `CountApplications`: все фильтры на `sqlc.narg(...) IS NULL OR …`. Если сервис не передаст `borrower_org_id` и `lender_org_id`, запрос вернёт **все** заявки всех компаний. Плюс: `target_country_code` — одно значение (контракт — массив), нет `created_from/to`, `has_my_offer`, `borrower_organization_id`, сортировки по `published_at`, нет `JOIN organizations` (поле `borrower` обязательно в `Application`), нет `my_offer_id` и `offers_count`.
- Переходные `UPDATE` (`SubmitApplication`, `ApproveApplication`, `RejectApplication`, `CancelApplication`, `AgreeApplication`, `UpdateApplication`) фильтруют только по `id`, без условия на текущий статус (`docs/db/schema.md`, «Расхождения»).
- `RejectAllOffersForApplication` — `:exec` с зашитой причиной `application_cancelled`: для `reject-offers` нужна `rejected_by_borrower`, и без `RETURNING` некому слать уведомления и писать историю офферов.
- `pgerr.Map` (`repository/pgerr/pgerr.go`) превращает `pgx.ErrNoRows` в `NOT_FOUND`. Для `UPDATE … AND status = …` ноль строк означает конфликт статуса — переводи его в `apperr.InvalidTransition`, а не через `pgerr.Map`.
- `UpdateApplication` построен на `COALESCE(narg, col)`: `transfer_method_comment` нельзя сбросить в `NULL`, хотя в контракте поле `nullable`. Сгенерированный `ApplicationUpdate.TransferMethodComment` — `*string`, явный `null` от отсутствия поля не отличить. Решение — пустая строка (см. «Решения»).
- `decimal.TrimTrailingZeros = false` ставится только в пакете `handler`. В тестах пакета `service` `decimal.String()` вернёт `"500000"`; сравнивай суммы через `Equal`, а не строками.
- Тесты роутера (`handler/errors_test.go: TestRouterErrors`) собирают `handler.Deps` без сервиса заявок. Случаи `/api/v1/applications/not-a-uuid` и `?page=abc` отсекает валидатор до хендлера; любой новый тест, доходящий до реализованной операции, передаёт фейковый сервис в `Deps`.

## Решения

- **Видимость в SQL, а не в сервисе.** `GetApplicationByID`, `ListApplications`, `CountApplications` заменяются на `GetApplicationVisible`, `ListApplicationsVisible`, `CountApplicationsVisible` с обязательными `sqlc.arg('viewer_role')::user_role` и `sqlc.arg('viewer_org')::uuid`; старые удаляются. Без организации (`uuid.Nil`) заёмщик и кредитор получают пусто, а не всё. Источник: `mds/db-security-plan.md` (Context), шаблон — BE-04.
- **Правило видимости заявки** (`api/paths/applications.yaml`, `getApplication.description`): `admin` — любая; `borrower` — `borrower_organization_id = viewer_org`; `lender` — `status = 'published'` или есть оффер его компании в любом статусе. Невидимая → 404.
- **Мутации — только владелец-заёмщик**, блокировка строки: новый запрос `LockApplicationForBorrower` (`… WHERE id = $1 AND borrower_organization_id = $2 FOR UPDATE`). 0 строк → 404. Статус проверяется в Go по заблокированной строке → 409. Почему: блокировка сериализует `cancel`/`reject-offers` с `acceptOffer` (BE-09) и `createOffer` (BE-08).
- **Порядок блокировок: сначала строка `applications`, потом строки `offers`.** Любая операция, меняющая несколько офферов заявки (`cancel`, `reject-offers`, `accept` в BE-09), сначала берёт `FOR UPDATE` на заявку. Иначе `cancel` и `accept` блокируют офферы в разном порядке и ловят deadlock (`40P01` → 500).
- **Страховка статусом в `WHERE`.** Все переходные `UPDATE applications` получают `AND status = sqlc.arg('from_status')::application_status`; 0 строк → `apperr.InvalidTransition`. Закрывает пункт «Расхождения» в `docs/db/schema.md`.
- **Статусная машина — одна таблица переходов** в `service/application_status.go`, по ADR-0001 «Статусные машины». Действие имеет ровно один целевой статус; функция проверяет, что текущий статус входит в разрешённые для действия.
- **Сервис возвращает свои типы** (`service.Application`, `service.ApplicationStatus`, `service.StatusHistoryEntry`), а не строки sqlc — `mds/plan.md`, раздел 7. Хендлер переводит их в `api.Application`.
- **Ответ мутации — перечитанная заявка.** В конце транзакции сервис вызывает `GetApplicationVisible` на том же `q`, чтобы вернуть `borrower` (имя и страну организации), `offers_count` и остальные вычисляемые поля одной функцией конвертации.
- **KYC и роль проверяет сервис**, middleware BE-04 — ранний отказ (DECISIONS п. 36: проверка повторяется намеренно, как защита в глубину).
- **`status_history` пишет общий хелпер** `service/status_history.go` (владелец — эта задача). Создание заявки тоже пишется: `from_status = NULL → draft`.
- **Причины отклонения офферов:** `cancel` → `application_cancelled`, `reject-offers` → `rejected_by_borrower` (enum `offer_reject_reason`).
- **Уведомления:** `submit` → всем админам `application_submitted` (BE-11: админам — `kyc_submitted`, `application_submitted`, `aml_flag_created`); `cancel`/`reject-offers` → кредитору каждого отклонённого оффера `offer_rejected` (`entity_type = offer`). Отдельного типа `application_cancelled` в enum нет, новый не добавляем.
- **Аудит** (BE-14: «создание / отправка заявки»): `application.create`, `application.submit`, плюс `application.cancel`. Через хелпер `service/audit.go` с `service.ClientMeta`; хендлер передаёт `middleware.ClientMetaFromContext(ctx)`.
- **AML при `submit`** — вызов `aml.Screener` (BE-13) внутри той же транзакции. Пока BE-13 не влит — интерфейс в сервисе и no-op реализация в `cmd/server/main.go`.
- **`offers_count` — все офферы заявки в любом статусе** (DECISIONS п. 9): счётчик не обнуляется после `agreed` и `reject-offers`. Записать в `description` поля `Application.offers_count` в `api/schemas/applications.yaml`.
- **Кредитор видит историю заявки без комментариев модерации** (DECISIONS п. 10): `moderation_comment` в `Application` виден только владельцу и admin, а в `status_history` та же причина лежит в `comment`. Для `p.Role == lender` сервис обнуляет `comment` у записей `entity_type = application`; записи офферов не трогает.
- **`transfer_method_comment` очищается пустой строкой: `""` → `NULL`** (DECISIONS п. 7, то же правило, что `lei` в BE-05). Отсутствующее поле или `null` — не трогать. В SQL — `NULLIF(…, '')`, правило — в `description` поля `ApplicationUpdate.transfer_method_comment`.
- **`actor` у офферов, отклонённых каскадом (`cancel`, `reject-offers`), — заёмщик** (DECISIONS п. 12): его действие вызвало отклонение; причина — в `comment`. Так же BE-09 пишет `another_offer_accepted`.
- **Об отмене заявки админов не уведомляем**, даже из `moderation` (DECISIONS п. 14): типа нет, отмена видна в истории и в очереди модерации.
- **Способы перевода `spfs` и `cips`** появляются в справочнике и в enum `TransferMethod` в BE-02 (DECISIONS п. 24); здесь отдельной работы не нужно — `transfer_method` валидируется по enum контракта и FK `transfer_methods`.

## Таблица переходов заявки

| Из | Действие | В | Кто | Побочные эффекты (в одной транзакции) |
|---|---|---|---|---|
| — | `create` | `draft` | borrower, `kyc_status = verified` | `status_history` (`NULL → draft`), аудит `application.create` |
| `draft`, `rejected` | `submit` | `moderation` | borrower-владелец | `submitted_at = now()`, `moderation_comment = NULL`; AML-скрининг; уведомление админам `application_submitted`; `status_history`; аудит `application.submit` |
| `moderation` | `approve` | `published` | admin (операция BE-12) | `published_at = now()`; уведомление заёмщику `application_published`; `status_history` |
| `moderation` | `reject` | `rejected` | admin (операция BE-12) | `moderation_comment = причина`; уведомление `application_rejected`; `status_history` с `comment` |
| `draft`, `moderation`, `rejected`, `published` | `cancel` | `cancelled` | borrower-владелец | все `sent`-офферы → `rejected (application_cancelled)` + их `status_history` + `offer_rejected` кредиторам; `status_history` заявки; аудит `application.cancel` |
| `published` | `agree` (принятие оффера) | `agreed` | borrower-владелец (операция BE-09) | см. план BE-09 |

Не переходы, но проверяют статус по той же таблице (409 `INVALID_STATUS_TRANSITION`, если статус не подходит):

| Действие | Разрешено в | Эффект |
|---|---|---|
| `update` (PATCH) | `draft`, `rejected` | поля меняются, статус тот же; `status_history` не пишется |
| `reject-offers` | `published` | все `sent`-офферы → `rejected (rejected_by_borrower)` + их `status_history` + `offer_rejected` кредиторам; заявка остаётся `published`; ответ `{rejected_count}` (0 — тоже 200) |

`agreed` и `cancelled` — конечные: из них нет ни одного действия.

## Шаги

### 1. SQL: видимость, блокировка, статус в WHERE

Файл `backend/queries/applications.sql`:

1. BE-04 (шаг 3) уже удалил `GetApplicationByID` и создал `GetApplicationVisible`, `ListApplicationsVisible`, `CountApplicationsVisible` (`SELECT *`, старые фильтры). Если BE-04 ещё не влит — сначала сделай его шаг 3 для заявок.
2. Переписать `GetApplicationVisible :one` — блок видимости BE-04 без изменений, плюс колонки организации, `my_offer_id`, `offers_count`:
   ```sql
   SELECT sqlc.embed(a),
          org.name AS borrower_name, org.country_code AS borrower_country_code,
          (SELECT f.id FROM offers f
            WHERE f.application_id = a.id AND f.status = 'sent'
              AND f.lender_organization_id = sqlc.arg('viewer_org')::uuid) AS my_offer_id,
          (SELECT count(*) FROM offers f WHERE f.application_id = a.id) AS offers_count
     FROM applications a
     JOIN organizations org ON org.id = a.borrower_organization_id
    WHERE a.id = sqlc.arg('id')
      AND (   sqlc.arg('viewer_role')::user_role = 'admin'
           OR (sqlc.arg('viewer_role')::user_role = 'borrower' AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
           OR (sqlc.arg('viewer_role')::user_role = 'lender'
               AND (a.status = 'published' OR EXISTS (
                     SELECT 1 FROM offers f WHERE f.application_id = a.id
                        AND f.lender_organization_id = sqlc.arg('viewer_org')::uuid))));
   ```
   `sqlc.embed(a)` даёт поле `Application repository.Application` в строке результата — одна функция конвертации для всех запросов. Если sqlc (`versions.env`: 1.31.1) не примет алиас в `embed`, пиши `sqlc.embed(applications)` и `FROM applications`.
3. Переписать `ListApplicationsVisible :many` и `CountApplicationsVisible :one` с тем же блоком видимости и фильтрами:
   - `sqlc.narg('statuses')::application_status[]` → `status = ANY(…)`;
   - `sqlc.narg('target_countries')::char(2)[]` → `target_country_code = ANY(…)` (мультивыбор, ИЛИ);
   - `sqlc.narg('transfer_methods')::text[]` → `transfer_method = ANY(…)` (мультивыбор, ИЛИ; DECISIONS п. 37);
   - `currency`, `amount_min`, `amount_max`, `term_min`, `term_max` — как в старом `ListApplications`;
   - `sqlc.narg('created_from')::timestamptz` / `created_to` → `a.created_at >= / <`;
   - `sqlc.narg('borrower_org_filter')::uuid` → `a.borrower_organization_id = …` (сервис передаёт только для admin);
   - `sqlc.narg('has_my_offer')::boolean` → `IS NULL OR (EXISTS(оффер viewer_org) = has_my_offer)` (сервис передаёт только для lender);
   - сортировка `sqlc.arg('sort')::text`: ветки `CASE` для всех 8 значений `ListApplicationsParamsSort`, `published_at` — `NULLS LAST`; последним ключом `a.id DESC` для стабильной пагинации;
   - `LIMIT sqlc.arg('page_limit') OFFSET sqlc.arg('page_offset')`.
   `offers_count` в списке считается так же, как в `GetApplicationVisible`, — по всем статусам.
   Фильтры здесь остаются `narg`: они только сужают выборку внутри видимости, а видимость — на обязательных `arg`.
4. Добавить `LockApplicationForBorrower :one`: `SELECT * FROM applications WHERE id = $1 AND borrower_organization_id = $2 FOR UPDATE;`.
5. В `UpdateApplication`, `SubmitApplication`, `ApproveApplication`, `RejectApplication`, `CancelApplication`, `AgreeApplication` добавить `AND status = sqlc.arg('from_status')::application_status`. В `UpdateApplication` — `transfer_method_comment = CASE WHEN sqlc.narg('transfer_method_comment')::text IS NULL THEN transfer_method_comment ELSE NULLIF(sqlc.narg('transfer_method_comment')::text, '') END` вместо `COALESCE`. Сообщить в PR: BE-09 и BE-12 вызывают `AgreeApplication`/`Approve…`/`Reject…` с новым параметром.
6. Перенести `CreateStatusHistory` в новый `backend/queries/status_history.sql` и добавить `ListApplicationHistory :many`:
   ```sql
   SELECT h.* FROM status_history h
    WHERE (h.entity_type = 'application' AND h.entity_id = sqlc.arg('application_id'))
       OR (h.entity_type = 'offer' AND h.entity_id IN (
             SELECT o.id FROM offers o
              WHERE o.application_id = sqlc.arg('application_id')
                AND (sqlc.arg('viewer_role')::user_role <> 'lender'
                     OR o.lender_organization_id = sqlc.arg('viewer_org')::uuid)))
    ORDER BY h.created_at, h.id;
   ```
   Доступ к самой заявке проверяется до этого через `GetApplicationVisible`; фильтр по офферам не даёт кредитору увидеть историю чужих офферов.

В `backend/queries/offers.sql`: заменить `RejectAllOffersForApplication` на
```sql
-- name: RejectSentOffersByApplication :many
UPDATE offers SET status = 'rejected', reject_reason = sqlc.arg('reason')::offer_reject_reason, decided_at = now()
 WHERE application_id = sqlc.arg('application_id') AND status = 'sent'
RETURNING id, lender_organization_id;
```
BE-09 переиспользует его вместо `RejectOtherOffers` (после `AcceptOffer` принятый оффер уже не `sent`).

`make gen-sqlc`.
**Готово:** `make gen-check` без диффа после коммита; `grep -rn "GetApplicationByID\|ListApplications\b\|RejectAllOffersForApplication" backend/` — пусто; `go -C backend build ./...` зелёный.

### 2. Хелпер `status_history`

`backend/internal/service/status_history.go`:
```go
// StatusChange — один переход статуса сущности для журнала status_history.
type StatusChange struct {
    EntityType repository.EntityType
    EntityID   uuid.UUID
    From       string     // "" — сущность создана (from_status = NULL)
    To         string
    Actor      *Principal // nil — переход сделала система
    Comment    string     // "" → NULL
}

// StatusHistoryWriter — подмножество Querier для записи истории.
type StatusHistoryWriter interface {
    CreateStatusHistory(ctx context.Context, arg repository.CreateStatusHistoryParams) (repository.StatusHistory, error)
}

func recordStatusChange(ctx context.Context, q StatusHistoryWriter, c StatusChange) error
```
`Actor.Role` → `repository.NullUserRole{UserRole: repository.UserRole(role), Valid: true}`, `Actor.UserID` → `pgtype.UUID`. Хелпер вызывается только внутри `InTx` с тем же `q`, что и переход.
**Готово:** `TestRecordStatusChange` (табличный: создание с `From=""` → `FromStatus.Valid=false`; системный переход `Actor=nil` → `ActorUserID.Valid=false`, `ActorRole.Valid=false`; комментарий) зелёный.

### 3. Статусная машина

`backend/internal/service/application_status.go`:
- `type ApplicationStatus string` с константами `draft`, `moderation`, `rejected`, `published`, `agreed`, `cancelled`;
- `type ApplicationAction string`: `submit`, `approve`, `reject`, `cancel`, `agree`;
- таблица `applicationTransitions map[ApplicationAction]struct{ from []ApplicationStatus; to ApplicationStatus }` по разделу «Таблица переходов»;
- `func NextApplicationStatus(from ApplicationStatus, a ApplicationAction) (ApplicationStatus, error)` → `apperr.InvalidTransition(string(from), string(to))`;
- `func canEditApplication(s ApplicationStatus) bool` и `canRejectAllOffers` — `draft|rejected` и `published`.
**Готово:** `TestNextApplicationStatus` перебирает **все** 6 статусов × 5 действий (30 случаев), ожидаемый результат взят из таблицы; `go -C backend test -short -run TestNextApplicationStatus ./internal/service/` зелёный.

### 4. Сервис `Applications`

`backend/internal/service/applications.go`:
- Типы: `Application` (поля `api.Application` в Go-типах: `decimal.Decimal`, `time.Time`, `*string`, `*uuid.UUID`; `Borrower OrgShort{ID, Name, CountryCode}`; `Number int64`; `OffersCount *int64`; `MyOfferID *uuid.UUID`), `ApplicationFilter`, `ApplicationPage{Items, Total, Page, PageSize}`, `CreateApplicationInput`, `UpdateApplicationInput`, `StatusHistoryEntry`.
- Зависимости — маленькие интерфейсы рядом: `ApplicationStore` (только нужные методы `Querier` для чтения вне транзакции), `TxRunner` (`InTx`), `Notifier` (BE-11), `AuditWriter` (BE-14), `aml.Screener` (BE-13).
- Методы (первый аргумент после `ctx` — `p Principal`):
  - `List(ctx, p, f ApplicationFilter) (ApplicationPage, error)`: `amount_min`/`amount_max` без `currency` → `apperr.Validation(…, FieldError{Field: "amount_min"|"amount_max", Message: "только вместе с currency"})`; `HasMyOffer` обнуляется не для lender, `BorrowerOrgID` — не для admin; страница по умолчанию 1/20.
  - `Get(ctx, p, id)`: `GetApplicationVisible` → `pgerr.Map(err, "Заявка не найдена")`.
  - `Create(ctx, p, in)`: роль `borrower` иначе `apperr.Forbidden`; `p.KycStatus != "verified"` → `apperr.KYCNotVerified()`; в `InTx`: `CreateApplication` → `recordStatusChange(NULL→draft)` → аудит → `GetApplicationVisible`.
  - `Update(ctx, p, id, in)`: `InTx`: `LockApplicationForBorrower` (0 строк → 404) → `canEditApplication` иначе 409 → `UpdateApplication(from_status=текущий)` → `GetApplicationVisible`.
  - `Submit`: `InTx`: lock → `NextApplicationStatus(cur, submit)` → `SubmitApplication(from_status)` → `aml.Screener` → `recordStatusChange` → `Notifier` админам `application_submitted` → аудит.
  - `Cancel`: `InTx`: lock → `NextApplicationStatus(cur, cancel)` → `RejectSentOffersByApplication(reason=application_cancelled)` → по каждому офферу `recordStatusChange(offer, sent→rejected, Actor=p, Comment="application_cancelled")` + `offer_rejected` его кредитору → `CancelApplication(from_status)` → `recordStatusChange(application)` → аудит.
  - `RejectAllOffers`: `InTx`: lock → `published` иначе 409 → `RejectSentOffersByApplication(reason=rejected_by_borrower)` → история и уведомления как в `Cancel` → вернуть `len(rows)`.
  - `History(ctx, p, id)`: `GetApplicationVisible` (404) → `ListApplicationHistory` → для lender `comment = nil` у записей `entity_type = application`.
  - Переходы модерации `approve`/`reject` реализует BE-12 в этом же пакете через `NextApplicationStatus` и `recordStatusChange`; эта задача даёт им только строки `approve`/`reject` в таблице переходов.
- `func ApplicationNumber(n int64) string` — `fmt.Sprintf("APP-%06d", n)`; единственное место формата номера заявки: им пользуются хендлеры (BE-07, BE-08) и заголовки уведомлений (BE-11).
- Конвертация: одна функция `applicationFromVisible(row)`; `moderation_comment` — только владельцу и admin; `offers_count` → `nil` для lender; `my_offer_id` → `nil` не для lender.
- ⚠️ `UPDATE … AND status = from_status` вернул `pgx.ErrNoRows` → `apperr.InvalidTransition`, а не `pgerr.Map`.
**Готово:** юнит-тесты из раздела «Тесты» (сервис) зелёные на фейках; `go vet ./...` чистый.

### 5. Хендлеры

`internal/http/handler/handler.go`: интерфейс `ApplicationService` (методы шага 4), поле в `Deps` и `Handler`; `cmd/server/main.go` собирает `service.NewApplications(...)`.
`internal/http/handler/applications.go`: в каждом методе `p, err := currentPrincipal(ctx)` (BE-04, `handler/principal.go`: нет principal → `apperr.Unauthorized`), перевод `api` ⇄ `service`:
- `AmountMin/AmountMax` — `decimal.NewFromString` (формат уже проверен валидатором);
- `TargetCountry []CountryCode` → `[]string`;
- `number` → `service.ApplicationNumber(n)`;
- успешные ответы — `ListApplications200JSONResponse`, `CreateApplication201JSONResponse`, `RejectAllOffers200JSONResponse{RejectedCount: n}` и т. д.; ошибки — `return nil, err`.
**Готово:** `handler/applications_test.go` (см. «Тесты») зелёный; `curl -b b.txt $API/applications/<id>` для чужой заявки → `404 NOT_FOUND`.

### 6. Интеграционные тесты и фикстуры

`backend/internal/service/fixtures_integration_test.go` (package `service_test`): хелперы `newBorrower(t, pool, kyc)`/`newLender(...)` — `CreateOrganization` + `CreateUser` + `SubmitKycForReview` + `ApproveKyc` (после BE-05 и BE-12 переходы KYC охраняются статусом в `WHERE`, из `draft` сразу в `verified` нельзя); `newPublishedApplication(...)` — `CreateApplication` → `SubmitApplication` → `ApproveApplication`. Регистрационные номера и email — со случайным суффиксом: `lp_app` (DB-SEC) не имеет `DELETE`, данные тестов остаются в БД. BE-08/09/10 расширяют этот файл.
`backend/internal/service/applications_integration_test.go` — случаи из «Тесты» (интеграционные).
**Готово:** `make db-reset && make test-all` зелёный.

### 7. Документация

`api/paths/applications.yaml`, `listApplications`: параметр `transfer_method` — `in: query`, массив `TransferMethod`, `style: form`, `explode: true`, описание «способ перевода, мультивыбор (ИЛИ)», по образцу `target_country`; хендлер передаёт его как `[]string` в `ApplicationFilter.TransferMethods`. `make diff-api` — новый необязательный параметр не ломает клиентов.
`api/schemas/applications.yaml`: `description` у `Application.offers_count` («все офферы заявки в любом статусе; только владельцу и admin») и у `ApplicationUpdate.transfer_method_comment` («`""` очищает; отсутствие поля или `null` — оставить как есть»); `make gen-api`, `make lint-api`.
`backend/README.md`: 8 операций — в «Статус реализации»; в «Ошибки»/«applications» отразить: `amount_*` без `currency` → 400; `reject-offers` вне `published` → 409; причины `application_cancelled` / `rejected_by_borrower`.
**Готово:** в README нет расхождений с кодом по кодам ошибок этих операций (сверка по таблице «Тесты»).

## Тесты

Юнит, `service/application_status_test.go`:
- `TestNextApplicationStatus` — 30 случаев (6 статусов × 5 действий). Разрешены ровно: `draft/rejected + submit → moderation`; `moderation + approve → published`; `moderation + reject → rejected`; `draft/moderation/rejected/published + cancel → cancelled`; `published + agree → agreed`. Остальные 21 → `INVALID_STATUS_TRANSITION`.
- `TestCanEditApplication`, `TestCanRejectAllOffers` — по 6 статусов.

Юнит, `service/applications_test.go` (фейк стора с встроенным `repository.Querier`, фейковые `TxRunner`, `Notifier`, `AuditWriter`, `Screener`):
- `Create` lender → `FORBIDDEN`; borrower с `KycStatus=under_review` → `KYC_NOT_VERIFIED`; verified → вызваны `CreateApplication` и `CreateStatusHistory{FromStatus: NULL, ToStatus: draft}`.
- `List` с `AmountMin` без `Currency` → `VALIDATION_ERROR`, `details[0].field = amount_min`.
- `List` borrower: в запрос ушли `viewer_role=borrower`, `viewer_org=p.OrgID`, `borrower_org_filter=NULL`, даже если в фильтре передан чужой `BorrowerOrgID`; lender с `HasMyOffer=true` → `has_my_offer=true`; borrower с `HasMyOffer=true` → `NULL`.
- `Submit` из `draft` → `SubmitApplication(from_status=draft)`, вызван `Screener`, уведомление `application_submitted`, история `draft→moderation`; из `published` → 409, ни одной записи.
- `Cancel` из `published` с двумя `sent`-офферами → `RejectSentOffersByApplication(reason=application_cancelled)`, 3 записи истории (2 оффера + заявка), 2 уведомления `offer_rejected`; из `agreed` → 409.
- `RejectAllOffers` из `published` → `reason=rejected_by_borrower`, результат = числу строк; из `moderation` → 409.
- `Update` в `published` → 409; `LockApplicationForBorrower` вернул `ErrNoRows` → 404.
- `Cancel` из `moderation` → ни одного уведомления админам.
- `History` для lender: у записей заявки `Comment == nil`, у записей его оффера `comment` сохранён; для borrower и admin `comment` заявки на месте.
- `UPDATE … from_status` вернул `ErrNoRows` → `INVALID_STATUS_TRANSITION`, не `NOT_FOUND`.

Хендлер, `handler/applications_test.go` (настоящий роутер, фейковый сервис, `Principal` в контексте):
- `GET /applications?target_country=DE&target_country=AE&status=published&sort=published_at_desc` → сервис получил `[DE AE]`, `[published]`, сорт; ответ `200`, `items[0].number = "APP-000123"`, `amount = "500000.00"`.
- `GET /applications?amount_min=abc` → `400`, `field = amount_min`.
- `POST /applications` → `201`; сервис вернул `KYCNotVerified` → `403 KYC_NOT_VERIFIED`.
- `POST /applications/{id}/reject-offers` → `200 {"rejected_count":2}`; `InvalidTransition` → `409 INVALID_STATUS_TRANSITION`.
- `GET /applications/{id}` с ошибкой `NotFound` → `404 NOT_FOUND`.

Интеграционные, `service/applications_integration_test.go`:
- Видимость `GetApplicationVisible`: заёмщик A — своя → ок, заявка заёмщика B → 404; кредитор — `draft` чужая → 404, `published` → ок, `agreed` со своим оффером → ок, `agreed` без оффера → 404; admin → любая.
- Список: заёмщик видит только свои (заявки другого заёмщика в БД есть); кредитор без фильтров — только `published` + со своим оффером; `has_my_offer=true` → только со своим оффером; `target_country=[DE,AE]` — обе страны; `transfer_method=[sepa,spfs]` — только заявки с этими способами; `created_from` в будущем → `total = 0`; admin с `borrower_organization_id` → только этой компании.
- `viewer_org = uuid.Nil` для `borrower` и `lender` → `total = 0`.
- `cancel` на `published` с офферами двух кредиторов → оба `rejected`, `reject_reason = application_cancelled`, в `status_history` 3 новые строки, в `notifications` 2 строки `offer_rejected`.
- `reject-offers` → `reject_reason = rejected_by_borrower`, заявка `published`.
- `history` для кредитора L1 не содержит записей оффера L2 и комментариев модерации; для заёмщика — содержит.
- `PATCH` с `transfer_method_comment: ""` → в БД `NULL`; `PATCH` без поля → значение сохранилось.
- `offers_count` после `reject-offers` с двумя офферами = 2.

## Готово, когда

- [ ] `TestNextApplicationStatus` покрывает все 30 пар «статус × действие» и зелёный.
- [ ] Заёмщик видит только свои заявки, кредитор — `published` и заявки со своим оффером (`my_offer_id` заполнен для активного), admin — все; интеграционные тесты видимости зелёные.
- [ ] Фильтры `status`, `target_country` (несколько значений), `currency` + `amount_min/max` (без `currency` → 400), `term_min/max`, `created_from/to`, `has_my_offer`, `borrower_organization_id`, 8 сортировок и пагинация работают — покрыто интеграционным тестом списка.
- [ ] `cancel` и `reject-offers` переводят все `sent`-офферы в `rejected` с причиной `application_cancelled` / `rejected_by_borrower` в одной транзакции с изменением заявки.
- [ ] Каждый переход (включая создание и отклонение офферов) пишет строку в `status_history`; `submit`, `cancel`, `reject-offers` создают уведомления.
- [ ] Чекпоинт: `curl` из «Сквозной сценарий» (`backend/README.md`, шаг 4) создаёт заявку, `GET /applications/{id}` её возвращает; фронт открывает её карточку.
- [ ] Общий критерий готовности из `docs/backend_tasks/README.md`.

## Открытые вопросы

Нет: закрыты в [DECISIONS.md](DECISIONS.md) — `offers_count` (п. 9), история для кредитора (п. 10), сброс `transfer_method_comment` (п. 7), уведомление об отмене (п. 14), фильтр по способу перевода (п. 37). Сигнатура `Notifier` — шаг 1 плана BE-11 (`NotifyUser`, `NotifyOrganization`, `NotifyAdmins`); если BE-11 не влит, этот шаг делается первым отдельным PR.
