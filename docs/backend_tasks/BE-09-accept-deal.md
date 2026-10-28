# BE-09 · Принятие оффера и фиксация сделки

Issue: #25 · Milestone: M2 · Зависит от: BE-08 (`GetOfferVisible`, `NextOfferStatus`, `RejectSentOffersByApplication`), BE-07 (`NextApplicationStatus`, `recordStatusChange`, `AgreeApplication` с `from_status`, фикстуры), BE-11 (`Notifier`), BE-14 (`audit.go`; если не влит — минимальная версия) · Блокирует: BE-16 (сид сделок), демо

## Цель

`POST /offers/{offerId}/accept` в одной транзакции принимает оффер, отклоняет остальные активные офферы заявки, переводит заявку в `agreed` и создаёт сделку-снапшот; ответ `200` с `Deal`. Повторный или параллельный `accept` по той же заявке отвечает 409, вторая сделка не появляется. `GET /deals` и `GET /deals/{dealId}` отдают сделки сторонам и admin, остальным — пусто / 404.

## Что уже есть

- Контракт: `api/paths/offers.yaml` → `acceptOffer` (ответы 200 `Deal`, 401, 404, 409 `InvalidTransition`); `api/paths/deals.yaml` → `listDeals` (`borrower_organization_id`, `lender_organization_id` — только admin; `created_from/to` — по `agreed_at`; пагинация), `getDeal`; `api/schemas/deals.yaml` → `Deal` (обязательные `borrower`, `lender` — `OrganizationShort`; `fee_amount` «рассчитано бэкендом»).
- Заглушки: `handler/offers.go: AcceptOffer`, `handler/deals.go: ListDeals, GetDeal`.
- Запросы: `queries/applications.sql: GetApplicationForUpdate`, `AgreeApplication`; `queries/offers.sql: AcceptOffer` (`WHERE id = $1 AND status = 'sent'`); `queries/deals.sql: CreateDeal`, `GetDealByID`, `ListDeals`, `CountDeals`.
- SQL-схема транзакции — `docs/db/schema.md`, «Принятие оффера». Та же транзакция руками проверяется в `backend/migrations/testdata/invariants_test.sql` (блок «принятие оффера»).
- БД-страховки (`00001_init.sql`): `offers_one_accepted_uq ON offers (application_id) WHERE status = 'accepted'`; `deals.application_id UNIQUE`, `deals.offer_id UNIQUE`; `applications_agreed_chk` (`agreed` ⇔ `accepted_offer_id`); `deals_fee_chk` (`fee_percent IS NULL` ⇔ `fee_amount IS NULL`); `deals_parties_chk`.
- `service/tx.go: TxManager.InTx` — `READ COMMITTED` (уровень по умолчанию pgx), откат при ошибке и панике.
- Архитектура (`mds/plan.md`, «Сделано»): `service/` — один пакет, `AcceptOffer` живёт в `service/deals.go` (домен-результат) и выполняется в одной транзакции через `TxManager`.
- DB-SEC: у `lp_app` на `deals` только `SELECT, INSERT` — сделка неизменяема на уровне прав.

⚠️ Ловушки в текущем коде:
- `GetApplicationForUpdate` блокирует заявку по `offer_id`, но **не проверяет владельца** и возвращает только `id, borrower_organization_id, status` — снапшоту сделки нужны `term_months`, `target_country_code`, `transfer_method`.
- `CreateDeal` считает `fee_amount` в SQL (`CASE WHEN $8 … round($5 * $8 / 100, 2)`), а `docs/db/schema.md` и контракт требуют расчёт в бэкенде. Позиционный `$8` используется дважды с приведением типа — хрупко для sqlc.
- `RejectOtherOffers` — `:exec` без `RETURNING`: кредиторам отклонённых офферов не из чего отправить уведомления (BE-08 удаляет его; используется `RejectSentOffersByApplication`).
- `GetDealByID` — только по `id`; `ListDeals`/`CountDeals` на `sqlc.narg(...) IS NULL OR …` — без организаций отдают **все** сделки; нет `created_from/to`; нет имён организаций для `borrower`/`lender`.
- Пример в `docs/db/schema.md` фильтрует `a.status = 'published'` прямо в `SELECT … FOR UPDATE`: тогда «заявка уже `agreed`» и «чужая заявка» обе дают 0 строк, и 404 от 409 не отличить.
- `pgerr.Map` превращает `23505` в `CONFLICT`, а контракт `acceptOffer` объявляет 409 `INVALID_STATUS_TRANSITION`.
- `handler/errors_test.go: TestRouterErrors`, случай `"not implemented yet"` ходит в `/api/v1/deals` и ждёт 501 — после этой задачи он упадёт.
- `decimal.TrimTrailingZeros = false` выставлен только в пакете `handler` (`handler/money.go`): в тестах `service` строка `5000.00` получится только через `StringFixed(2)`.

## Решения

- **Одна транзакция, порядок шагов и блокировок фиксирован** (`docs/db/schema.md`, `mds/plan.md`):
  1. `LockApplicationByOfferForBorrower` — `FOR UPDATE OF a` на заявку, фильтр по владельцу, **без** фильтра по статусу. 0 строк → 404 (оффера нет или заявка чужая).
  2. Статус заявки проверяется в Go по заблокированной строке: `NextApplicationStatus(cur, agree)` → не `published` → 409.
  3. `GetOfferVisible` на том же `q` → `NextOfferStatus(cur, accept)` → не `sent` → 409. Затем `valid_until` задан и `< now()` → 409 `INVALID_STATUS_TRANSITION` «Срок действия оффера истёк»; статус оффера не меняется — отдельного статуса `expired` нет (DECISIONS п. 6).
  4. `AcceptOffer` (`AND status = 'sent' AND application_id = …`) — 0 строк → 409.
  5. `RejectSentOffersByApplication(reason = another_offer_accepted)` → `id, lender_organization_id` отклонённых.
  6. `AgreeApplication(id, accepted_offer_id, from_status = published)`.
  7. `CreateDeal` со снапшотом и `fee_amount`, посчитанным в Go.
  8. `status_history`: оффер `sent→accepted`, каждый отклонённый `sent→rejected` (`comment = another_offer_accepted`), заявка `published→agreed`. `actor` у всех записей, включая отклонённые офферы, — заёмщик `p` (DECISIONS п. 12): отклонение вызвало его действие; так же пишет `cancel` в BE-07.
  9. Уведомления: кредитору принятого — `offer_accepted` (`entity_type = deal`, `entity_id = deal.id`); кредиторам отклонённых — `offer_rejected` (`entity_type = offer`). Заёмщику ничего (DECISIONS п. 14): типа `deal_created` нет, сделку он получает в ответе `accept`.
  10. `audit_log`: `offer.accept`, `entity_type = offer`, в `meta` — `deal_id`.
  11. `GetDealVisible` на том же `q` → ответ.
  Почему так: `FOR UPDATE` на заявке сериализует параллельные `accept`; в `READ COMMITTED` вторая транзакция после ожидания получает **новую** версию строки (`status = agreed`) и отвечает 409 на шаге 2. Блокировка заявки до офферов — общий порядок с `cancel`/`reject-offers` (BE-07) и `createOffer` (`FOR SHARE`, BE-08): без него параллельные `cancel` и `accept` ловят deadlock.
- **Страховки БД переводятся в 409 `INVALID_STATUS_TRANSITION`**, а не в `CONFLICT`: `23505` на `offers_one_accepted_uq`, `deals_application_id_key`, `deals_offer_id_key`. При правильной блокировке не срабатывают, но если сработают — клиент получает тот же ответ, что и при проигранной гонке.
- **Комиссия считается в Go**: `fee_amount = round(amount × fee_percent / 100, 2)`, `amount.Mul(fee).Shift(-2).Round(2)` — `Shift` точен (без деления), `Round` у shopspring округляет половину от нуля, как `round(numeric)` в Postgres. `fee_percent = NULL` → `fee_amount = NULL` (`deals_fee_chk`). Кейс из issue: `500000.00 × 1% = 5000.00`.
- **Снапшот:** из оффера — `amount`, `currency`, `interest_rate_annual`, `fee_percent`, `repayment_schedule`, `collateral`, `additional_terms`, `lender_organization_id`; из заявки — `term_months`, `target_country_code`, `transfer_method`, `borrower_organization_id`. Сумма сделки — сумма **оффера** (может быть меньше суммы заявки).
- **Видимость сделок в SQL** (`mds/db-security-plan.md`): `GetDealByID`, `ListDeals`, `CountDeals` → `GetDealVisible`, `ListDealsVisible`, `CountDealsVisible` с обязательными `viewer_role`, `viewer_org`; старые удаляются. Правило: `admin` — все; `borrower` — `d.borrower_organization_id = viewer_org`; `lender` — `d.lender_organization_id = viewer_org`.
- **Сервис возвращает `service.Deal`** (Go-типы, `Borrower`/`Lender OrgShort`, `Number int64`), хендлер переводит в `api.Deal` (`mds/plan.md`, раздел 7). Номер — `func DealNumber(n int64) string` (`fmt.Sprintf("DEAL-%06d", n)`) в `service/deals.go`, рядом с `ApplicationNumber` из BE-07; им пользуются хендлер и заголовок уведомления `offer_accepted`.
- **Роль:** не borrower → `apperr.Forbidden` в сервисе (middleware BE-04 по `x-roles` отсекает раньше). Lender и admin не принимают офферы. `'403'` у `acceptOffer` в контракт добавляет BE-04 вместе с остальными защищёнными операциями (DECISIONS п. 2); если BE-04 ещё не влит — добавь здесь и отметь в PR.
- **Просрочка — 409, а не 400** (DECISIONS п. 6): данные запроса верны, но оффер больше нельзя перевести в `accepted`. Время берётся из часов сервиса `now func() time.Time`, чтобы тест не зависел от реального времени.
- **Тест конкурентности — интеграционный** (`mds/plan.md`): реальные транзакции Postgres, коммиты, без отката.

## Шаги

### 1. SQL

`backend/queries/applications.sql`: заменить `GetApplicationForUpdate` на
```sql
-- name: LockApplicationByOfferForBorrower :one
-- Первый шаг acceptOffer: блокирует заявку оффера, если она принадлежит заёмщику.
-- Статус намеренно не фильтруется: сервис отличает 404 (чужая) от 409 (уже agreed/cancelled).
SELECT a.* FROM applications a
  JOIN offers o ON o.application_id = a.id
 WHERE o.id = sqlc.arg('offer_id') AND a.borrower_organization_id = sqlc.arg('borrower_org')::uuid
   FOR UPDATE OF a;
```
`backend/queries/offers.sql`: в `AcceptOffer` добавить `AND application_id = sqlc.arg('application_id')`.
`backend/queries/deals.sql`:
- `CreateDeal` — именованные `sqlc.arg(...)`, `fee_amount = sqlc.narg('fee_amount')::numeric`, без `CASE`/`round`.
- BE-04 (шаг 3) уже заменил `GetDealByID`, `ListDeals`, `CountDeals` на `*Visible` (`SELECT *`). Переписать их под ответ `Deal` — блок видимости без изменений, плюс JOIN организаций:
  ```sql
  -- name: GetDealVisible :one
  SELECT sqlc.embed(d),
         bo.name AS borrower_name, bo.country_code AS borrower_country_code,
         lo.name AS lender_name,   lo.country_code AS lender_country_code
    FROM deals d
    JOIN organizations bo ON bo.id = d.borrower_organization_id
    JOIN organizations lo ON lo.id = d.lender_organization_id
   WHERE d.id = sqlc.arg('id')
     AND (   sqlc.arg('viewer_role')::user_role = 'admin'
          OR (sqlc.arg('viewer_role')::user_role = 'borrower' AND d.borrower_organization_id = sqlc.arg('viewer_org')::uuid)
          OR (sqlc.arg('viewer_role')::user_role = 'lender'   AND d.lender_organization_id   = sqlc.arg('viewer_org')::uuid));
  ```
  `ListDealsVisible :many` / `CountDealsVisible :one` — то же + `sqlc.narg('borrower_org_filter')`, `sqlc.narg('lender_org_filter')`, `created_from`/`created_to` по `d.agreed_at`; `ORDER BY d.agreed_at DESC, d.id DESC`; `LIMIT/OFFSET`.
`make gen-sqlc`.
**Готово:** `make gen-check` без диффа; `grep -rn "GetApplicationForUpdate\|GetDealByID\|ListDeals\b\|CountDeals\b" backend/` — пусто; `go -C backend build ./...` зелёный.

### 2. Расчёт комиссии

`backend/internal/service/deals.go`: `func dealFee(amount decimal.Decimal, feePercent decimal.NullDecimal) decimal.NullDecimal` с комментарием-формулой и ссылкой на правило округления.
**Готово:** `TestDealFee` зелёный (случаи — в «Тесты»).

### 3. `Deals.AcceptOffer`

`backend/internal/service/deals.go`:
```go
// Deals — сделки и принятие оффера (операция нескольких доменов живёт в домене-результате, mds/plan.md).
type Deals struct { store DealStore; tx TxRunner; notifier Notifier; audit AuditWriter; now func() time.Time }

func (s *Deals) AcceptOffer(ctx context.Context, p Principal, offerID uuid.UUID, meta ClientMeta) (Deal, error)
```
Тело — шаги 1–11 из «Решения», всё внутри одного `s.tx.InTx(ctx, func(q repository.Querier) error { … })`; все вызовы — через `q`, ни одного через `s.store` (иначе запрос уйдёт мимо транзакции). Ошибки:
- `LockApplicationByOfferForBorrower` → `pgx.ErrNoRows` → `apperr.NotFound("Оффер не найден")`;
- `AcceptOffer` / `AgreeApplication` → `pgx.ErrNoRows` → `apperr.InvalidTransition(...)`;
- `*pgconn.PgError` `23505` с `ConstraintName` из списка (`offers_one_accepted_uq`, `deals_application_id_key`, `deals_offer_id_key`) → `apperr.InvalidTransition("published", "agreed")`; имена — константы.
Комментарий над функцией: порядок блокировок (заявка → офферы) и почему статус проверяется после блокировки.
**Готово:** юнит-тесты `AcceptOffer` из «Тесты» зелёные на фейках.

### 4. `Deals.List` / `Deals.Get`

`List(ctx, p, f DealFilter) (DealPage, error)`: `borrower_org_filter`/`lender_org_filter` — только для admin, иначе `NULL`; страница по умолчанию 1/20. `Get(ctx, p, id)`: `GetDealVisible` → `pgerr.Map(err, "Сделка не найдена")`.
**Готово:** юнит-тест «borrower передал `LenderOrgFilter` → в запрос ушёл `NULL`» зелёный.

### 5. Хендлеры

`handler/handler.go`: интерфейс `DealService` (`AcceptOffer`, `List`, `Get`), поле в `Deps`; `cmd/server/main.go` — `service.NewDeals(...)` с тем же `TxManager`.
`handler/offers.go: AcceptOffer` → `offersapi.AcceptOffer200JSONResponse(toAPIDeal(d))`, `meta` — `middleware.ClientMetaFromContext(ctx)`.
`handler/deals.go: ListDeals`, `GetDeal`.
`handler/errors_test.go: TestRouterErrors`: случай `"not implemented yet"` перевести на операцию, которая к моменту PR ещё отвечает 501 (`grep -l NotImplemented backend/internal/http/handler/*.go`); если таких не осталось — удалить случай.
**Готово:** `handler/deals_test.go` и `TestRouterErrors` зелёные; сквозной сценарий из `backend/README.md` (шаг 6) даёт `200` с `fee_amount` и `GET /deals` возвращает сделку обеим сторонам.

### 6. Интеграционные тесты, включая конкурентность

`backend/internal/service/deals_integration_test.go` (package `service_test`, `newPool` из `dictionaries_integration_test.go`, фикстуры BE-07/08). Каждый тест создаёт свою заявку и офферы со случайными реквизитами: данные коммитятся и остаются в БД (у `lp_app` нет `DELETE`).

Тест конкурентности `TestAcceptOfferConcurrent`:
```go
for i := 0; i < 20; i++ {            // повтор повышает шанс настоящего пересечения
    app, offerA, offerB := newAppWithTwoOffers(t, pool)
    start := make(chan struct{})
    errs := make([]error, 2)
    var wg sync.WaitGroup
    for j, id := range []uuid.UUID{offerA, offerB} {
        wg.Add(1)
        go func() { defer wg.Done(); <-start; _, errs[j] = svc.AcceptOffer(ctx, borrower, id, service.ClientMeta{}) }()
    }
    close(start); wg.Wait()
    // ровно один nil и один INVALID_STATUS_TRANSITION; deals по app — 1 строка;
    // заявка agreed, accepted_offer_id = победитель; проигравший оффер rejected / another_offer_accepted
}
```
Тот же тест для двух параллельных `accept` **одного** оффера. Пул `pgxpool` должен иметь ≥ 2 соединений (по умолчанию `max(4, NumCPU)` — достаточно).
**Готово:** `make db-reset && go -C backend test -count=1 -run 'TestAcceptOffer' ./internal/service/` зелёный 5 запусков подряд.

### 7. Документация

`backend/README.md`: `acceptOffer`, `listDeals`, `getDeal` — в «Статус реализации»; в «Статусы» абзац о принятии — добавить, что сумма сделки = сумма оффера, `fee_amount` округляется до копеек половиной от нуля. `docs/db/schema.md`: «Принятие оффера» — шаг 1 без фильтра статуса (статус проверяет сервис после блокировки), убрать из «Расхождения» пункты про `GetApplicationForUpdate`, `RejectOtherOffers`, `fee_amount в SQL`; «Покрытие sqlc» и `docs/db/tables/{deals,applications,offers}.md` — новые имена запросов.
**Готово:** имена запросов в `docs/db/` совпадают с `internal/repository/sqlc/querier.go`.

## Тесты

Юнит, `service/deals_test.go`:
- `TestDealFee` (сравнение через `Equal` и `StringFixed(2)`):

  | amount | fee_percent | fee_amount |
  |---|---|---|
  | `500000.00` | `1` | `5000.00` (кейс issue) |
  | `500000.00` | `1.2500` | `6250.00` |
  | `1234.56` | `0.3333` | `4.11` |
  | `0.50` | `1` | `0.01` (ровно половина `0.005` → вверх) |
  | `100.00` | `0` | `0.00` |
  | `100.00` | `NULL` | `NULL` |

- `AcceptOffer` (фейк `TxRunner` вызывает `fn` с фейковым `Querier`, фиксирует порядок вызовов):
  - lender / admin → `FORBIDDEN`, ни одного вызова стора;
  - лок вернул `ErrNoRows` → `NOT_FOUND`;
  - заявка `agreed` / `cancelled` / `moderation` → `INVALID_STATUS_TRANSITION`, `AcceptOffer` не вызван;
  - оффер `withdrawn` → `INVALID_STATUS_TRANSITION`;
  - `valid_until` = `now − 1s` → `INVALID_STATUS_TRANSITION` «Срок действия оффера истёк», `AcceptOffer` не вызван; `valid_until` = `now + 1h` или `NULL` → принят;
  - отклонённые офферы: в `CreateStatusHistory` `ActorUserID` = заёмщик, `Comment` = `another_offer_accepted`; уведомлений заёмщику нет;
  - `AcceptOffer` вернул `ErrNoRows` → `INVALID_STATUS_TRANSITION`;
  - `CreateDeal` вернул `23505 offers_one_accepted_uq` → `INVALID_STATUS_TRANSITION`, не `CONFLICT`;
  - успех: порядок вызовов ровно `Lock → GetOfferVisible → AcceptOffer → RejectSentOffersByApplication(another_offer_accepted) → AgreeApplication(from_status=published) → CreateDeal → CreateStatusHistory×(2 + N) → уведомления (1 offer_accepted + N offer_rejected) → CreateAuditLog(offer.accept) → GetDealVisible`; в `CreateDeal` — `term_months`/`target_country_code`/`transfer_method` из заявки, `amount`/`currency`/ставка/`fee_percent` из оффера, `fee_amount` = `dealFee`.

Хендлер, `handler/deals_test.go`, `handler/offers_test.go`:
- `POST /offers/{id}/accept` → `200`, тело `Deal`: `number = "DEAL-000045"`, `amount = "500000.00"`, `fee_amount = "5000.00"`, `borrower.name`, `lender.name`.
- Сервис вернул `InvalidTransition` → `409 INVALID_STATUS_TRANSITION`; `NotFound` → `404`.
- `GET /deals?created_from=2026-10-01T00:00:00Z&page=2` → сервис получил период и страницу; `GET /deals/{id}` → `200`.

Интеграционные, `service/deals_integration_test.go`:
- Сценарий issue: заявка `500000.00 EUR`, оффер `500000.00`, `fee_percent = 1` → сделка `fee_amount = 5000.00`, `currency = EUR`; заявка `agreed`, `accepted_offer_id` = оффер; второй оффер `rejected` / `another_offer_accepted`; в `status_history` 3 новые строки; в `notifications` `offer_accepted` и `offer_rejected`; в `audit_log` `offer.accept`.
- Повторный `accept` того же оффера → `INVALID_STATUS_TRANSITION`, сделок по заявке — 1.
- Принятие чужого оффера (заёмщик B, оффер на заявку A) → `NOT_FOUND`; несуществующий `offerId` → `NOT_FOUND`.
- `TestAcceptOfferConcurrent` — два разных оффера и один оффер (шаг 6).
- Видимость сделок: заёмщик и кредитор сделки → `getDeal` ок; другой кредитор → 404; `listDeals` другого кредитора → `total = 0`; `viewer_org = uuid.Nil` → `total = 0`; admin с `lender_organization_id` → только сделки этого кредитора; `created_from` после `agreed_at` → пусто.

## Готово, когда

- [ ] Принятие — одна транзакция: блокировка заявки с проверкой владельца → оффер `accepted` → остальные `sent` → `rejected (another_offer_accepted)` → заявка `agreed` → сделка-снапшот → `status_history` → уведомления → `audit_log` (юнит-тест порядка вызовов + интеграционный сценарий).
- [ ] `fee_amount = round(amount × fee_percent / 100, 2)` считается в Go; `500000.00 × 1% = 5000.00 EUR` (`TestDealFee` и интеграционный сценарий).
- [ ] `TestAcceptOfferConcurrent`: два параллельных `accept` разных офферов одной заявки → ровно одна сделка, второй — 409; стабилен 5 прогонов подряд.
- [ ] Принятие чужого оффера → 404 (интеграционный тест).
- [ ] Просроченный оффер (`valid_until` в прошлом) не принимается: 409, статус оффера `sent`.
- [ ] `listDeals`/`getDeal` отдают сделку только сторонам и admin.
- [ ] Общий критерий готовности из `docs/backend_tasks/README.md`.

## Открытые вопросы

Нет: закрыты в [DECISIONS.md](DECISIONS.md) — просроченный оффер (п. 6), `actor` автоотклонения (п. 12), уведомление заёмщику (п. 14), `403` в контракте (п. 2).
