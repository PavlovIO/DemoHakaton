# BE-14 · Аналитика и журнал аудита

Issue: #30 · Milestone: M3 · Зависит от: BE-03 (PR #67: `ClientMeta`, `Auth.audit`); ручки — BE-04 (`x-roles: [admin]`) · Блокирует: шаг 1 нужен BE-05, BE-07, BE-08, BE-09, BE-12 (они пишут аудит)

## Цель
Любое действие из списка issue пишет строку в `audit_log` с IP и User-Agent одним общим хелпером. Админ листает журнал с фильтрами (`GET /admin/audit-log`) и видит сводку за период (`GET /admin/analytics/summary`). Плюс: каждая строка лога сервера несёт `request_id`, который возвращается клиенту в `X-Request-ID`.

## Что уже есть
- Таблица `audit_log` (`backend/migrations/00001_init.sql`, `docs/db/tables/audit_log.md`): `user_id` (NULL — аноним), `action text`, `entity_type`, `entity_id text`, `ip inet`, `user_agent`, `meta jsonb`. Индексы `(created_at DESC)`, `(user_id, created_at DESC)`, `(entity_type, entity_id)`.
- `backend/queries/admin.sql`:
  - `CreateAuditLog :one`, `ListAuditLog :many` (без фильтров, без `Count`);
  - `GetAnalyticsSummaryByCurrency`, `GetTopDemandCountries`, `GetTopSupplyCountries` (без периода).
- Контракт: `adminAnalyticsSummary` (`created_from`, `created_to` — `date-time`, `top_n` 1..50, по умолчанию 10) и `adminListAuditLog` (`user_id`, `action`, `entity_type`, `entity_id`, `created_from`, `created_to`, пагинация) — `api/paths/admin.yaml`. Схемы `AnalyticsSummary`, `AuditLogEntry`, `AuditLogPage` — `api/schemas/analytics.yaml`. Запросы-образцы — `docs/db/schema.md`, «Аналитика».
- **PR #67 (`origin/be-03-auth`)**:
  - `service.ClientMeta{IP *netip.Addr; UserAgent string}` объявлен в `backend/internal/service/auth.go`;
  - `middleware.WithRequestMeta()` + `middleware.ClientMetaFromContext` — `backend/internal/http/middleware/request_meta.go`; User-Agent обрезается до 255 рун;
  - приватный метод `(*Auth).audit(ctx, userID *uuid.UUID, action, reason string, meta ClientMeta)` пишет `auth.login` и `auth.login_failed` (`meta.reason`: `unknown_email` / `bad_password` / `blocked`) через `AuthStore.CreateAuditLog` вне транзакции. Ошибку он только логирует (`slog.Warn` без контекста).
- Логи: `middleware.RequestLog` (`backend/internal/http/middleware/middleware.go`) пишет `slog.Log(r.Context(), …)`. `handler.WriteError` (`handler/errors.go`) пишет `slog.ErrorContext`/`DebugContext`. Связать эти две строки одного запроса сейчас нечем.
- ⚠️ `GetAnalyticsSummaryByCurrencyRow.TotalAmount` сгенерирован как **`int64`**: sqlc вывел тип `sum(numeric)` неверно. `AverageAmount` — `pgtype.Numeric`, а не `decimal.Decimal`, как остальные деньги (`backend/sqlc.yaml`, overrides). На суммах с копейками скан упадёт.
- ⚠️ `GetAnalyticsSummaryByCurrency` исключает `draft` **и** `cancelled`, а `docs/db/schema.md` — только `draft`. Периода нет ни в одном из трёх запросов (`docs/db/schema.md`, «Расхождения»).
- ⚠️ Нет запросов `applications_by_status`, `offers_total`, `deals_total` за период.
- ⚠️ `Register` в PR #67 аудит не пишет, хотя «регистрация» в списке issue.

## Решения
- **Один хелпер `writeAudit` в `internal/service/audit.go`.** `Auth.audit` из PR #67 становится тонкой обёрткой над ним, а не второй реализацией (README, «Общие куски кода»). `ClientMeta` переезжает из `auth.go` в `audit.go` без изменения имени.
- **Аудит бизнес-действия пишется querier'ом транзакции действия, и его ошибка откатывает действие.** Действие без следа в журнале хуже отказа. **Исключение — `auth.login_failed`:** транзакции нет, а ответ клиенту — ошибка входа, поэтому запись best-effort (`slog.WarnContext`), как в PR #67.
- **Формат `action` — `<сущность>.<глагол>`** (`auth.login`, `offer.accept`), только константы из `audit.go`. Список — в шаге 1. В `meta` — только идентификаторы и причины, без паролей, токенов и содержимого документов (AGENTS.md, «Безопасность»).
- **`ClientMeta` передаётся в метод сервиса явным аргументом** (как `Register`/`Login` в PR #67): сервис не читает контекст запроса (README, «Общие куски кода»).
- **Период аналитики — полуинтервал `[created_from, created_to)`**, без границ — всё время:
  - заявки, `applications_total`, `applications_by_status`, суммы и топ спроса — по `applications.created_at`;
  - офферы и топ предложения — по `offers.created_at`;
  - сделки — по `deals.agreed_at`.
- **`amounts_by_currency` — заявки во всех статусах, кроме `draft`** (так в `docs/db/schema.md`): `cancelled` и `rejected` тоже спрос, который был подан. **`top_demand_countries` — только `published`/`agreed`** (как в схеме).
- **`created_from > created_to` → 400 `VALIDATION_ERROR`, `field: created_to`.**
- **`request_id` — своё middleware плюс обёртка `slog.Handler`** (`mds/plan.md`, п. 4); внешних библиотек нет.
- **`user_id`/`org_id` в логах — через атрибуты контекста** (DECISIONS п. 42): `middleware.WithLogAttrs(ctx, attrs...)` дописывает атрибуты, `ContextHandler` добавляет их в каждую запись. `Authenticate` после успешной проверки сессии кладёт `user_id` и `org_id`. Пакет логгера не зависит от `service.Principal`.

## Шаги

### 1. Хелпер аудита (отдельный PR, сразу после мержа PR #67)
1. `backend/internal/service/audit.go`:
   ```go
   // ClientMeta — IP и User-Agent клиента (перенесено из auth.go).
   type ClientMeta struct { IP *netip.Addr; UserAgent string }

   // AuditEvent — одно действие для audit_log.
   type AuditEvent struct {
       Action     string               // константа Audit*
       UserID     *uuid.UUID           // nil — аноним
       EntityType repository.EntityType // "" — без сущности
       EntityID   string
       Meta       map[string]any        // только id и причины
   }

   type AuditWriter interface {
       CreateAuditLog(ctx context.Context, arg repository.CreateAuditLogParams) (repository.AuditLog, error)
   }

   func writeAudit(ctx context.Context, w AuditWriter, meta ClientMeta, e AuditEvent) error
   ```
   `Meta == nil` → `{}` (колонка `NOT NULL DEFAULT '{}'`, а `json.Marshal(nil)` даёт `null`).
2. Константы действий. Если задача-владелец ещё не готова, её константа всё равно объявляется здесь:

   | Константа | `action` | `entity_type` | Пишет | `meta` |
   |---|---|---|---|---|
   | `AuditAuthLogin` | `auth.login` | `user` | BE-03 (есть в PR #67) | — |
   | `AuditAuthLoginFailed` | `auth.login_failed` | `user` или нет | BE-03 (есть) | `reason` |
   | `AuditAuthRegister` | `auth.register` | `user` | BE-03 → добавить здесь | `organization_id` |
   | `AuditDocumentUpload` | `document.upload` | `document` | BE-05 | `doc_type`, `organization_id` |
   | `AuditDocumentDownload` | `document.download` | `document` | BE-05, только скачивание админом (DECISIONS п. 16); транзакции нет — запись до отдачи файла, ошибка → 500 | `organization_id` |
   | `AuditKycSubmit` | `kyc.submit` | `organization` | BE-05 | — |
   | `AuditKycApprove` / `AuditKycReject` | `kyc.approve` / `kyc.reject` | `organization` | BE-12 | `comment` / `reason` |
   | `AuditApplicationCreate` / `AuditApplicationSubmit` / `AuditApplicationCancel` | `application.create` / `application.submit` / `application.cancel` | `application` | BE-07 | — |
   | `AuditApplicationApprove` / `AuditApplicationReject` | `application.approve` / `application.reject` | `application` | BE-12 | `reason` |
   | `AuditOfferCreate` / `AuditOfferReject` / `AuditOfferWithdraw` | `offer.create` / `offer.reject` / `offer.withdraw` | `offer` | BE-08 | — |
   | `AuditOfferAccept` | `offer.accept` | `offer` | BE-09 | `deal_id` |
   | `AuditUserBlock` / `AuditUserUnblock` | `user.block` / `user.unblock` | `user` | BE-12 | `reason` |
   | `AuditAmlFlagResolve` | `aml_flag.resolve` | `aml_flag` | BE-13 (вне списка issue) | `resolution` |

   Не пишутся (DECISIONS п. 16): `auth.logout`; email при `auth.login_failed` с `reason = unknown_email` — это ПДн чужого человека, `user_id` и `meta` без email, как в PR #67.
3. В `auth.go`:
   - удалить `ClientMeta`;
   - `(*Auth).audit` переписать на `writeAudit(ctx, a.store, meta, AuditEvent{…})` с `slog.WarnContext(ctx, …)` при ошибке;
   - в `Register` внутри `InTx` добавить `writeAudit(ctx, q, meta, AuditEvent{Action: AuditAuthRegister, UserID: &userRow.ID, …})` с возвратом ошибки.
4. `backend/internal/service/audit_test.go`: фейковый `AuditWriter` проверяет, что `Ip`, `UserAgent` (`Valid=false` при пустой строке), `EntityType` (`Valid=false` при пустом), `Meta` (`{}` при nil) переданы корректно.

**Готово:**
- `grep -rn "CreateAuditLog(" backend/internal/service --include=*.go | grep -v _test` находит только `audit.go`;
- `TestWriteAudit` и существующие тесты `auth_test.go` зелёные;
- новый кейс в `auth_test.go`: `Register` вызывает `CreateAuditLog` с `action = "auth.register"`.

### 2. SQL журнала
`backend/queries/admin.sql`. Условие фильтра общее для `ListAuditLog` и нового `CountAuditLog :one`:
```sql
WHERE (sqlc.narg('user_id')::uuid IS NULL OR user_id = sqlc.narg('user_id')::uuid)
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action')::text)
  AND (sqlc.narg('entity_type')::entity_type IS NULL OR entity_type = sqlc.narg('entity_type')::entity_type)
  AND (sqlc.narg('entity_id')::text IS NULL OR entity_id = sqlc.narg('entity_id')::text)
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR created_at >= sqlc.narg('created_from')::timestamptz)
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR created_at < sqlc.narg('created_to')::timestamptz)
```
`ListAuditLog`: `ORDER BY created_at DESC, id DESC LIMIT … OFFSET …`. Обновить `docs/db/tables/audit_log.md` (фраза «пока без фильтров»).

**Готово:** `make gen-check`; в `querier.go` есть `CountAuditLog`, у `ListAuditLogParams` 6 фильтров.

### 3. SQL аналитики
`backend/queries/admin.sql`. Все запросы принимают `narg created_from` / `created_to` (тот же шаблон полуинтервала):
- `GetAnalyticsSummaryByCurrency`: `WHERE status <> 'draft'` + период по `created_at`; колонки `count(*)::bigint AS count, sum(amount)::numeric AS total_amount, round(avg(amount), 2)::numeric AS average_amount`. Явный `::numeric` даёт `decimal.Decimal` по override из `backend/sqlc.yaml` — проверить в `admin.sql.go`.
- `CountApplicationsByStatus :many` — `SELECT status, count(*)::bigint FROM applications WHERE <период> GROUP BY status`.
- `CountOffersInPeriod :one` (по `offers.created_at`), `CountDealsInPeriod :one` (по `deals.agreed_at`).
- `GetTopDemandCountries`, `GetTopSupplyCountries`: добавить период (`a.created_at`, `o.created_at`), `LIMIT sqlc.arg('top_n')`, тай-брейк `ORDER BY count DESC, country_code`.

Обновить блок «Аналитика» и «Расхождения» в `docs/db/schema.md`.

**Готово:** `make gen-check`; `GetAnalyticsSummaryByCurrencyRow.TotalAmount` и `.AverageAmount` — `decimal.Decimal`.

### 4. Сервис и ручки
- `backend/internal/service/analytics.go`: `Summary(ctx, p Principal, from, to *time.Time, topN int32) (AnalyticsSummary, error)` — 6 запросов шага 3.
  - `applications_total` = сумма по `CountApplicationsByStatus`; статусы без заявок в `applications_by_status` не включать.
  - `from > to` → `apperr.Validation(…, FieldError{Field: "created_to", …})`.
- `backend/internal/service/audit_log.go` (чтение): `List(ctx, p, AuditFilter, limit, offset)` → `ListAuditLog` + `CountAuditLog`.
- `handler/admin.go`: `AdminAnalyticsSummary`, `AdminListAuditLog`. Маппинг:
  - `ip` — `addr.String()`, `nil` → `null`;
  - `meta` — `json.Unmarshal` в `map[string]any`;
  - суммы — `decimal.Decimal` (`api.Money`), масштаб сохраняет `handler/money.go`.
- Роли (`x-roles`, BE-04): `[admin]`.

**Готово:**
- юнит `TestAnalyticsSummary` на фейке: `applications_total` = сумма статусов, `from > to` → `CodeValidation`;
- `httptest`: `GET /api/v1/admin/analytics/summary?top_n=0` → 400 (`field: top_n`), `GET /api/v1/admin/audit-log?entity_type=offer&page_size=5` → 200 с `total`.

### 5. Интеграционные тесты на заселённой БД
`make db` грузит демо-сид, поэтому тесты не рассчитывают на пустые таблицы:
- `backend/internal/service/analytics_integration_test.go`: в транзакции с `Rollback` вставить фикстуры с `created_at`/`agreed_at` в 2001 году (организации со случайным рег. номером, заявки в 3 статусах и 2 валютах, офферы, сделку) и вызвать `Summary` с `from=2001-01-01`, `to=2002-01-01` на querier транзакции. Ожидание:
  - точные числа;
  - `average_amount` для EUR (100.00 + 200.50) = `150.25`;
  - `draft` не входит в `amounts_by_currency`, но входит в `applications_by_status`.
- `audit_log_integration_test.go`: записать 3 события с уникальным `action` (`test.<uuid>`), фильтр по `action` → `total = 3`; по `user_id` + `created_from` → ожидаемое подмножество.

**Готово:** оба теста зелёные в `make test-all` после `make db`.

### 6. Логи — `request_id`
Из `mds/plan.md`, п. 4: связать строку `RequestLog` со строкой ошибки `WriteError`.
1. `backend/internal/http/middleware/request_id.go`:
   - `RequestID()` берёт `X-Request-ID` из запроса, если он подходит под `^[A-Za-z0-9._-]{1,64}$`, иначе генерирует (`uuid.NewString()`). Проверка защищает от инъекций в лог: переводы строк и произвольная длина не проходят.
   - Кладёт id в контекст и ставит заголовок ответа **до** `next.ServeHTTP`.
   - Плюс `RequestIDFromContext(ctx) (string, bool)`.
2. `backend/internal/http/middleware/log_handler.go`: `ContextHandler{slog.Handler}`. Его `Handle(ctx, rec)` добавляет `request_id` из контекста. `WithAttrs`/`WithGroup` возвращают обёрнутый `ContextHandler`, иначе `logger.With(...)` потеряет обёртку.
3. `backend/cmd/server/main.go`, `newLogger`: оборачивать оба хендлера (`text`/`json`) в `middleware.ContextHandler`.
4. `backend/internal/http/handler/router.go`: `middleware.RequestID()` — первым в `middleware.Chain`, до `RequestLog`.
4a. `log_handler.go`: `WithLogAttrs(ctx, attrs ...slog.Attr) context.Context` — добавляет атрибуты к уже лежащим в контексте (новый срез, без мутации). `ContextHandler.Handle` пишет их после `request_id`. `middleware.Authenticate` после успешной сессии: `ctx = WithLogAttrs(ctx, slog.String("user_id", …), slog.String("org_id", …))` (у admin `org_id` не пишется).
5. Вызовы `slog.X(...)` без контекста в коде запросов заменить на `slog.XContext(ctx, ...)`: в PR #67 это `slog.Warn` в `Auth.audit` и `Auth.Authenticate`. Проверка — `grep -rnE "slog\.(Info|Warn|Error|Debug)\(" backend/internal`, вне `cmd/` попаданий быть не должно.
6. Обновить doc пакета `middleware` и путь запроса в `backend/README.md` («Как устроен код»: `RequestID → RequestLog → …`).

**Готово:** тест `TestRequestIDCorrelatesLogLines` в `handler`:
- логгер `slog.New(middleware.ContextHandler{slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})})` через `slog.SetDefault` с восстановлением в `t.Cleanup`;
- запрос `GET /api/v1/nope` → 404, заголовок `X-Request-ID` непуст;
- в `buf` две JSON-строки (`request rejected` и `http request`) с тем же `request_id`;
- запрос с `X-Request-ID: abc-123` → тот же id в ответе;
- с `X-Request-ID: "a\nb"` → сгенерирован новый.

### 7. (Опционально, после M3) Мониторинг
Только если останется время (`mds/plan.md`, п. 5). Новая зависимость `prometheus/client_golang` — по AGENTS.md нужен ADR.
- Метрики на **отдельном** листенере `METRICS_ADDR` (пусто — выключено): `/metrics` вне `/api/v1` валидатор спеки отбил бы 404, и метрики не должны быть публичными.
- `http_requests_total{route,method,status}` и `http_request_duration_seconds{route,method}`. `route` = `r.Pattern` (шаблон ServeMux, а не сырой путь — иначе кардинальность взорвётся на UUID).
- Пул: `pool.Stat()` → `AcquiredConns`, `IdleConns`, `EmptyAcquireCount` через `GaugeFunc`.
- Grafana в `docker-compose` — только для демо.

**Готово:** `curl localhost:$METRICS_PORT/metrics | grep http_requests_total` показывает счётчик с `route="GET /api/v1/healthz"`; `METRICS_ADDR` есть в `.env.example` и README.

## Тесты
| Случай | Ожидание |
|---|---|
| `POST /auth/login` с неверным паролем | строка `auth.login_failed`, `meta.reason = bad_password`, `ip`, `user_agent` заполнены |
| `POST /auth/register` | строка `auth.register`, `user_id` = новый пользователь |
| ошибка `CreateAuditLog` внутри бизнес-`InTx` | действие откатывается, ответ 500 |
| ошибка `CreateAuditLog` при неудачном логине | ответ 401 как обычно, в логе `audit write failed` с `request_id` |
| `GET /admin/audit-log?action=auth.login&page=1&page_size=2` | 200, ≤ 2 записи, `total` — все `auth.login` |
| `GET /admin/audit-log?entity_type=bogus` | 400 `VALIDATION_ERROR`, `field: entity_type` |
| `GET /admin/audit-log` под borrower | 403 `FORBIDDEN` |
| summary без периода | 200, все 7 полей присутствуют, массивы не `null` |
| summary `created_from=2026-10-10T00:00:00Z&created_to=2026-10-01T00:00:00Z` | 400, `field: created_to` |
| summary с `top_n=2` при 3 странах спроса | `top_demand_countries` — 2 записи, по убыванию `count` |
| сумма 100.00 + 200.50 EUR | `total_amount = "300.50"`, `average_amount = "150.25"` (строки с 2 знаками) |
| запрос с `X-Request-ID: abc-123` | в ответе `X-Request-ID: abc-123`, во всех строках лога запроса `request_id=abc-123` |
| запрос с валидной сессией | строка `http request` содержит `user_id` и `org_id`; без сессии — их нет |

## Готово, когда
- [ ] Сводка за период: заявки всего и по статусам, офферы, сделки, суммы и средние по валютам, топ стран спроса и предложения; период работает для всех полей (интеграционный тест с фикстурами 2001 года).
- [ ] `audit_log` пишется для: логина, неудачного логина, регистрации, KYC submit/approve/reject, загрузки документа, создания/отправки заявки, оффера create/accept/reject/withdraw, блокировки — с IP и User-Agent. По каждому действию есть тест у задачи-владельца; все вызовы идут через `writeAudit`.
- [ ] `GET /admin/audit-log` фильтрует по всем 6 параметрам контракта и возвращает `total`.
- [ ] Ответ любого запроса содержит `X-Request-ID`, строки `RequestLog` и `WriteError` одного запроса несут одинаковый `request_id` (`TestRequestIDCorrelatesLogLines`).
- [ ] И общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: закрыты в [DECISIONS.md](DECISIONS.md) — IP за прокси (п. 4); аудит `document.download`, `auth.logout`, email при `unknown_email` (п. 16); `user_id`/`org_id` в логах (п. 42).
