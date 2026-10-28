# BE-08 · Офферы: создание, просмотр, отклонение, отзыв

Issue: #24 · Milestone: M2 · Зависит от: BE-07 (`GetApplicationVisible`, `status_history`-хелпер, порядок блокировок, фикстуры интеграционных тестов), BE-04, BE-11 (`Notifier`) · Блокирует: BE-09, BE-10

## Цель

Операции `createOffer`, `listApplicationOffers`, `listOffers`, `getOffer`, `rejectOffer`, `withdrawOffer` отвечают по контракту вместо 501. Верифицированный кредитор делает оффер на опубликованную заявку, заёмщик-владелец его видит и отклоняет, кредитор отзывает; в ответе есть `unread_messages`. Оффер видят только автор, заёмщик-владелец заявки и admin, остальным — 404.

## Что уже есть

- Контракт: `api/paths/offers.yaml` (`listOffers`, `getOffer`, `acceptOffer`, `rejectOffer`, `withdrawOffer`), `api/paths/applications.yaml` (`listApplicationOffers`, `createOffer` — тег `offers`), `api/schemas/offers.yaml` (`OfferCreate` — **без** `currency`; `Offer` — обязательные `lender`, поле `unread_messages`, `application_number`). `rejectOffer` принимает необязательное тело `{comment ≤ 1000}`.
- Заглушки: `internal/http/handler/offers.go` — 7 методов, `AcceptOffer` реализует BE-09.
- Запросы `backend/queries/offers.sql`: `CreateOffer`, `GetOfferByID`, `ListOffersByApplication`, `ListOffers`, `CountOffers`, `AcceptOffer`, `RejectOtherOffers`, `RejectOffer` (причина `rejected_by_borrower` зашита), `WithdrawOffer`; после BE-07 — `RejectSentOffersByApplication`. В `chat.sql`: `CountUnreadMessages`.
- Схема (`backend/migrations/00001_init.sql`): `offers_one_active_per_lender_uq ON offers (application_id, lender_organization_id) WHERE status = 'sent'`; `offers_one_accepted_uq`; CHECK `offers_reject_reason_chk` (`rejected` ⇔ есть причина), `offers_decided_chk` (`sent` ⇔ `decided_at IS NULL`); `message_reads (offer_id, user_id, last_read_message_id)`.
- Из BE-07: `GetApplicationVisible`, `recordStatusChange` (`service/status_history.go`), фикстуры `service/fixtures_integration_test.go`.
- `pgerr.Map`: `23505` → `CONFLICT` с общим текстом «Запись уже существует».

⚠️ Ловушки в текущем коде:
- `GetOfferByID` — только по `id`: чужой оффер читается подстановкой UUID. Удаляется.
- `ListOffers` / `CountOffers`: `sqlc.narg('lender_org_id') IS NULL OR …` — без организации отдают **все** офферы. Нет области заёмщика (нужен `JOIN applications`), нет `borrower_organization_id`, `created_from/to`.
- `ListOffersByApplication` — только по `application_id`: кредитор получил бы чужие офферы по той же заявке.
- `RejectOffer`, `WithdrawOffer` — `WHERE id = $1 AND status = 'sent'`, без организации. Статус защищён, владелец — нет.
- `CountUnreadMessages` считает по одному офферу — в списке это N+1 запрос.
- Ни один запрос не возвращает имя кредитора и номер заявки, а `lender` в `Offer` обязателен.
- `CreateOffer` ничего не проверяет: сумма ≤ суммы заявки и валюта = валюте заявки — правило сервиса (`docs/db/schema.md`, таблица инвариантов).
- `valid_until` нигде не проверяется.

## Решения

- **Видимость в SQL** (`mds/db-security-plan.md`): `GetOfferByID`, `ListOffers`, `CountOffers`, `ListOffersByApplication` заменяются на `GetOfferVisible`, `ListOffersVisible`, `CountOffersVisible`, `ListApplicationOffersVisible` с обязательными `viewer_role`, `viewer_org`; старые удаляются. Правило: `admin` — все; `lender` — `o.lender_organization_id = viewer_org`; `borrower` — `a.borrower_organization_id = viewer_org`. Источник — `getOffer.summary`, `listOffers.description`.
- **Владелец в каждом `UPDATE`**: `RejectOffer` получает условие на организацию заёмщика (через `application_id IN (SELECT id FROM applications WHERE borrower_organization_id = …)`), `WithdrawOffer` — `AND lender_organization_id = …`. Видимость + роль уже дают владельца, условие в SQL — по правилу AGENTS.md «Доступ (IDOR)».
- **Ошибки reject/withdraw**: невидимый → 404; видимый, но роль не та (admin, или lender на `reject`, borrower на `withdraw`) → 403 `FORBIDDEN`; статус не `sent` → 409 `INVALID_STATUS_TRANSITION`. Ноль строк в `UPDATE` после проверки статуса в Go (гонка) → тоже 409.
- **`createOffer` берёт `FOR SHARE` на заявку** (новый запрос `LockApplicationForLender`) внутри транзакции. `FOR SHARE` конфликтует с `FOR UPDATE` из `cancel`/`reject-offers` (BE-07) и `accept` (BE-09): оффер не появится на заявке, которую в этот момент принимают или отменяют. Порядок блокировок — заявка, потом офферы (как в BE-07).
- **Заявка не `published` при создании оффера → 409 `CONFLICT`** («Заявка не принимает офферы») — по правилу 409 из DECISIONS п. 6: статус мешает действию, но перехода у заявки нет. В контракте у `createOffer` и объявлен ответ `409 Conflict`. Невидимая кредитору заявка → 404.
- **Сумма больше суммы заявки → 400 `VALIDATION_ERROR`**, `field: amount`. Валюта оффера берётся из заявки (в `OfferCreate` её нет) — правило «валюта = валюте заявки» выполняется построением.
- **Второй активный оффер компании → 409 `CONFLICT`** с текстом «У компании уже есть активный оффер по этой заявке»: сервис распознаёт `23505` с `ConstraintName = offers_one_active_per_lender_uq`. После `withdrawn`/`rejected` компания может сделать новый оффер — индекс частичный (`WHERE status = 'sent'`).
- **`valid_until` в прошлом при создании → 400**, `field: valid_until`. При принятии просроченного оффера BE-09 отвечает 409 `INVALID_STATUS_TRANSITION` (DECISIONS п. 6).
- **`unread_messages` — подзапрос в `*Visible`**, по `message_reads` текущего пользователя (`viewer_user`); собственные сообщения не считаются. Для admin поле не заполняется (`nil`): у admin нет указателя прочтения, он не сторона чата.
- **Статусная машина** — `service/offer_status.go`, одна таблица переходов (ADR-0001). Сервис возвращает свои типы `service.Offer`, `service.OfferStatus` (`mds/plan.md`, раздел 7).
- **Уведомления** (`notification_type`): создание → заёмщику `offer_received`; отклонение → кредитору `offer_rejected`; отзыв → заёмщику `offer_withdrawn`. `entity_type = offer`.
- **Аудит** (BE-14): `offer.create`, `offer.reject`, `offer.withdraw`.
- **Кредитор видит только свой оффер по заявке** (DECISIONS п. 11): `listApplicationOffers` отдаёт ему одну строку, суммы и ставки конкурентов скрыты. Заёмщик-владелец и admin видят все.
- **Админа о новом оффере не уведомляем** (DECISIONS п. 14): issue этого не требует, офферы видны в карточке заявки.
- **`'403'` в контракт добавляет BE-04** — у всех защищённых операций одним коммитом (DECISIONS п. 2). Если BE-04 ещё не влит, добавь `'403': Forbidden` к `rejectOffer` и `withdrawOffer` здесь и отметь в PR.

## Таблица переходов оффера

| Из | Действие | В | Кто | Побочные эффекты (в одной транзакции) |
|---|---|---|---|---|
| — | `create` | `sent` | lender, `kyc_status = verified`; заявка `published`; `amount ≤ application.amount` | `currency := application.currency`; `status_history` (`NULL → sent`); `offer_received` заёмщику; аудит `offer.create` |
| `sent` | `accept` | `accepted` | borrower-владелец заявки (операция BE-09) | см. план BE-09 |
| `sent` | `reject` | `rejected` (`rejected_by_borrower`) | borrower-владелец заявки | `reject_comment`, `decided_at`; `status_history` с `comment`; `offer_rejected` кредитору; аудит `offer.reject` |
| `sent` | `withdraw` | `withdrawn` | lender-автор | `decided_at`; `status_history`; `offer_withdrawn` заёмщику; аудит `offer.withdraw` |
| `sent` | `reject` (система) | `rejected` (`another_offer_accepted`) | принятие другого оффера (BE-09) | `offer_rejected` кредитору |
| `sent` | `reject` (система) | `rejected` (`application_cancelled`) | `cancelApplication` (BE-07) | `offer_rejected` кредитору |
| `sent` | `reject` (массово) | `rejected` (`rejected_by_borrower`) | `rejectAllOffers` (BE-07) | `offer_rejected` кредитору |

`accepted`, `rejected`, `withdrawn` — конечные. Системные и массовые отклонения идут через SQL-условие `status = 'sent'` в `RejectSentOffersByApplication` и используют тот же переход `reject`.

## Шаги

### 1. SQL

`backend/queries/offers.sql`:
1. BE-04 (шаг 3) уже заменил `GetOfferByID`, `ListOffers`, `CountOffers` на `GetOfferVisible`, `ListOffersVisible`, `CountOffersVisible` (`SELECT *`). Здесь удалить `ListOffersByApplication` и `RejectOtherOffers` (его заменяет `RejectSentOffersByApplication` из BE-07; BE-09 на это рассчитывает).
2. Переписать `GetOfferVisible :one` — блок видимости BE-04 без изменений, плюс колонки:
   ```sql
   SELECT sqlc.embed(o),
          a.number AS application_number, a.borrower_organization_id,
          lo.name AS lender_name, lo.country_code AS lender_country_code,
          (SELECT count(*) FROM messages m
            WHERE m.offer_id = o.id AND m.author_user_id <> sqlc.arg('viewer_user')::uuid
              AND m.id > COALESCE((SELECT r.last_read_message_id FROM message_reads r
                                    WHERE r.offer_id = o.id AND r.user_id = sqlc.arg('viewer_user')::uuid), 0)
          ) AS unread_messages
     FROM offers o
     JOIN applications a   ON a.id  = o.application_id
     JOIN organizations lo ON lo.id = o.lender_organization_id
    WHERE o.id = sqlc.arg('id')
      AND (   sqlc.arg('viewer_role')::user_role = 'admin'
           OR (sqlc.arg('viewer_role')::user_role = 'lender'   AND o.lender_organization_id   = sqlc.arg('viewer_org')::uuid)
           OR (sqlc.arg('viewer_role')::user_role = 'borrower' AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid));
   ```
   `a.borrower_organization_id` в выдаче нужен для уведомлений (BE-08, BE-10).
3. `ListOffersVisible :many` / `CountOffersVisible :one`: тот же `FROM … JOIN` и блок видимости; фильтры `sqlc.narg('statuses')::offer_status[]`, `application_id`, `lender_org_filter`, `borrower_org_filter`, `created_from`/`created_to` по `o.created_at`; `ORDER BY o.created_at DESC, o.id DESC`; `LIMIT/OFFSET`.
4. `ListApplicationOffersVisible :many`: тот же `SELECT`, `WHERE o.application_id = sqlc.arg('application_id')` + видимость + `statuses`, без пагинации (контракт отдаёт массив).
5. `LockApplicationForLender :one`:
   ```sql
   SELECT a.id, a.status, a.amount, a.currency, a.borrower_organization_id
     FROM applications a
    WHERE a.id = sqlc.arg('id')
      AND (a.status = 'published' OR EXISTS (SELECT 1 FROM offers f WHERE f.application_id = a.id
                                                AND f.lender_organization_id = sqlc.arg('lender_org')::uuid))
      FOR SHARE OF a;
   ```
   Условие совпадает с видимостью заявки для lender в `GetApplicationVisible`.
6. `RejectOffer`: добавить `AND application_id IN (SELECT id FROM applications WHERE borrower_organization_id = sqlc.arg('borrower_org')::uuid)`; `WithdrawOffer`: `AND lender_organization_id = sqlc.arg('lender_org')::uuid`.
7. `CreateOffer` — без изменений.

`backend/queries/chat.sql`: удалить `CountUnreadMessages` (счётчик теперь в `*Visible`).
`make gen-sqlc`.
**Готово:** `make gen-check` без диффа; `grep -rn "GetOfferByID\|ListOffersByApplication\|RejectOtherOffers\|CountUnreadMessages" backend/` — пусто; `go -C backend build ./...` зелёный.

### 2. Статусная машина оффера

`backend/internal/service/offer_status.go`: `type OfferStatus string` (`sent`, `accepted`, `rejected`, `withdrawn`), `type OfferAction string` (`accept`, `reject`, `withdraw`), таблица по разделу «Таблица переходов», `func NextOfferStatus(from OfferStatus, a OfferAction) (OfferStatus, error)` → `apperr.InvalidTransition`.
**Готово:** `TestNextOfferStatus` — все 4 статуса × 3 действия (12 случаев): разрешены только 3 из `sent`, 9 → `INVALID_STATUS_TRANSITION`; зелёный.

### 3. Сервис `Offers`

`backend/internal/service/offers.go`. Типы: `Offer` (Go-типы; `Lender OrgShort`; `ApplicationNumber int64`; `UnreadMessages *int64`; `RejectReason *string`), `OfferFilter`, `OfferPage`, `CreateOfferInput`. Зависимости — маленькие интерфейсы стора, `TxRunner`, `Notifier`, `AuditWriter`, часы `now func() time.Time` (для `valid_until`).

- `Create(ctx, p, applicationID, in, meta)`:
  1. `p.Role != lender` → `apperr.Forbidden`; `p.KycStatus != verified` → `apperr.KYCNotVerified()`;
  2. `valid_until` задан и `≤ now()` → `apperr.Validation(…, {Field: "valid_until"})`;
  3. `InTx`: `LockApplicationForLender` (`ErrNoRows` → 404) → `status != published` → `apperr.Conflict("Заявка не принимает офферы")` → `in.Amount.GreaterThan(app.Amount)` → `apperr.Validation(…, {Field: "amount", Message: "больше суммы заявки"})` → `CreateOffer(currency = app.Currency)` (`23505` + `offers_one_active_per_lender_uq` → `apperr.Conflict("У компании уже есть активный оффер по этой заявке")`) → `recordStatusChange(offer, "" → sent)` → `Notifier`: заёмщику (`app.BorrowerOrganizationID`) `offer_received` → аудит `offer.create` → `GetOfferVisible` → результат.
- `Get(ctx, p, id)`: `GetOfferVisible` → `pgerr.Map(err, "Оффер не найден")`; для admin `UnreadMessages = nil`.
- `List(ctx, p, f)`: `lender_org_filter`/`borrower_org_filter` — только для admin, иначе `NULL`; пагинация по умолчанию 1/20.
- `ListByApplication(ctx, p, applicationID, statuses)`: `GetApplicationVisible` (404) → `ListApplicationOffersVisible`. Кредитор видит заявку, но только свой оффер; без оффера — пустой массив.
- `Reject(ctx, p, id, comment, meta)`: `InTx`: `GetOfferVisible` (404) → `p.Role != borrower` → 403 → `NextOfferStatus(cur, reject)` (409) → `RejectOffer(id, borrower_org, comment)` (`ErrNoRows` → 409) → история (`comment` = текст отказа) → `offer_rejected` кредитору → аудит → `GetOfferVisible`.
- `Withdraw(ctx, p, id, meta)`: то же для `lender` → `WithdrawOffer(id, lender_org)` → `offer_withdrawn` заёмщику.
- Проверка `23505` по имени ограничения: `errors.As(err, &pgErr) && pgErr.ConstraintName == "offers_one_active_per_lender_uq"`; имя — константа рядом с проверкой.
**Готово:** юнит-тесты сервиса из «Тесты» зелёные.

### 4. Хендлеры

`handler/handler.go`: интерфейс `OfferService`, поле в `Deps`. `handler/offers.go`: `CreateOffer`, `ListApplicationOffers`, `ListOffers`, `GetOffer`, `RejectOffer`, `WithdrawOffer` (`AcceptOffer` оставить заглушкой для BE-09). `application_number` → `service.ApplicationNumber(n)` (BE-07); `RejectOffer` — тело может отсутствовать (`request.Body == nil` → `comment = nil`). `cmd/server/main.go` собирает `service.NewOffers(...)`.
**Готово:** `handler/offers_test.go` зелёный; `curl -X POST $API/applications/$APP_ID/offers …` дважды от одного кредитора → второй `409 CONFLICT`.

### 5. Интеграционные тесты

`service/offers_integration_test.go`, фикстуры из BE-07 (`fixtures_integration_test.go`, расширить `newOffer(...)`).
**Готово:** `make db-reset && make test-all` зелёный.

### 6. Документация

`backend/README.md`: 6 операций в «Статус реализации»; в таблице `offers` — 409 `CONFLICT` для второго активного оффера и для заявки не в `published`, 400 для суммы больше заявки, кто получает 403 на `reject`/`withdraw`. `docs/db/schema.md` «Покрытие sqlc» и `docs/db/tables/offers.md` — новые имена запросов, убрать пункты «Расхождения», которые закрыты.
**Готово:** имена запросов в `docs/db/` совпадают с `internal/repository/sqlc/querier.go`.

## Тесты

Юнит, `service/offer_status_test.go`: `TestNextOfferStatus` — 12 случаев (см. шаг 2).

Юнит, `service/offers_test.go` (фейки):
- `Create` borrower → `FORBIDDEN`; lender `under_review` → `KYC_NOT_VERIFIED`.
- `Create` на заявку `moderation` (лок вернул строку) → `CONFLICT`; лок вернул `ErrNoRows` → `NOT_FOUND`.
- `Create` `amount = 500000.01` при заявке `500000.00` → `VALIDATION_ERROR`, `field = amount`; `500000.00` → ок, в `CreateOffer` ушла `currency` заявки.
- `Create` с `valid_until` = `now - 1s` → `VALIDATION_ERROR`, `field = valid_until`.
- `Create`: `CreateOffer` вернул `PgError{Code: 23505, ConstraintName: offers_one_active_per_lender_uq}` → `CONFLICT` с текстом про активный оффер.
- `Create` успешно → `CreateStatusHistory{FromStatus: NULL, ToStatus: sent}`, уведомление `offer_received` организации заёмщика.
- `Reject` admin → `FORBIDDEN`; lender → `FORBIDDEN`; borrower на `withdrawn` → `INVALID_STATUS_TRANSITION`; `RejectOffer` вернул `ErrNoRows` → `INVALID_STATUS_TRANSITION`; успех → история с `comment`, `offer_rejected` кредитору.
- `Withdraw` borrower → `FORBIDDEN`; lender из `accepted` → `INVALID_STATUS_TRANSITION`; успех → `offer_withdrawn` заёмщику.
- `Get` admin → `UnreadMessages == nil`; lender → значение из строки.
- `List` borrower с `LenderOrgFilter` → в запрос ушёл `NULL`.

Хендлер, `handler/offers_test.go`:
- `POST /applications/{id}/offers` с `{"amount":"100.00","interest_rate_annual":"8.00","repayment_schedule":"annuity"}` → `201`, `currency` из ответа сервиса, `amount = "100.00"`.
- То же без `repayment_schedule` → `400 VALIDATION_ERROR`.
- `POST /offers/{id}/reject` без тела → `200`, сервис получил `comment = nil`; с `{"comment":"дорого"}` → `comment = "дорого"`.
- `GET /offers?status=sent&status=accepted` → сервис получил оба статуса.
- Ошибка `NotFound` → `404`, `InvalidTransition` → `409 INVALID_STATUS_TRANSITION`, `Conflict` → `409 CONFLICT`.

Интеграционные, `service/offers_integration_test.go`:
- Видимость `getOffer`: кредитор L1 — свой → ок, оффер L2 → 404; заёмщик-владелец → ок; другой заёмщик → 404; admin → ок.
- `listApplicationOffers` заявки с офферами L1 и L2: заёмщик → 2; L1 → 1 (свой); заёмщик B → 404.
- `listOffers` без фильтров: L1 видит только свои; заёмщик — только по своим заявкам; `viewer_org = uuid.Nil` → `total = 0`.
- Двойное создание оффера L1 на одну заявку → второй `CONFLICT`; после `withdraw` первого — новый оффер создаётся.
- `unread_messages`: L1 пишет 3 сообщения (вставка через `CreateMessage`), заёмщик `MarkMessagesRead` до первого → у заёмщика `2`, у L1 `0`.
- `reject` → `status = rejected`, `reject_reason = rejected_by_borrower`, `decided_at` заполнен, строка в `status_history`, строка `offer_rejected` в `notifications`.
- `createOffer` на заявку `cancelled`, где у L1 был оффер (заявка видна) → `CONFLICT`; на `draft` чужой заявки → 404.

## Готово, когда

- [ ] Создание оффера: только верифицированный lender (`403 KYC_NOT_VERIFIED` иначе), заявка `published` (иначе 409), сумма ≤ суммы заявки (иначе 400), валюта = валюте заявки (берётся из заявки), второй активный оффер компании → 409 — покрыто тестами выше.
- [ ] Оффер видят автор, заёмщик-владелец заявки и admin; остальным — 404 (интеграционный тест видимости).
- [ ] `TestNextOfferStatus` покрывает 12 пар «статус × действие».
- [ ] `unread_messages` в `getOffer`/`listOffers`/`listApplicationOffers` считается по `message_reads` текущего пользователя (интеграционный тест).
- [ ] Создание, отклонение и отзыв создают уведомления заёмщику / кредитору и строку `status_history`.
- [ ] Общий критерий готовности из `docs/backend_tasks/README.md`.

## Открытые вопросы

Нет: закрыты в [DECISIONS.md](DECISIONS.md) — код 409 для «заявка не принимает офферы» (п. 6), `403` в контракте (п. 2), видимость чужих офферов кредитору (п. 11), уведомление админа (п. 14).
