# BE-10 · Чат по офферу

Issue: #26 · Milestone: M3 · Зависит от: BE-08 (`GetOfferVisible` с `borrower_organization_id` и `unread_messages`), BE-03 (`service.RateLimiter`, `apperr.RateLimitedAfter`, `Retry-After` в `errors.go` — PR #67), BE-04 (роли), BE-11 (`Notifier`) · Блокирует: —

## Цель

`GET /offers/{offerId}/messages?after_id=&limit=` отдаёт новые сообщения по возрастанию `id` и сдвигает указатель «прочитано» текущего пользователя; `POST /offers/{offerId}/messages` пишет сообщение стороны оффера (оффер `sent` или `accepted`) и уведомляет другую сторону. Admin читает, но не пишет; чужие видят 404; частая отправка → 429 с `Retry-After`.

## Что уже есть

- Контракт: `api/paths/chat.yaml` — `listMessages` (`after_id` int64, `limit` 1–200, default 50; ответы 200, 401, 404), `sendMessage` (тело `MessageCreate{text 1–4000}`; ответы 201, 400, 401, 404, 409 `InvalidTransition`, 429 `RateLimited` — **без 403**). `api/schemas/chat.yaml` → `Message` (обязательные `author_role`, `author_organization` — `OrganizationShort`; `is_mine`).
- Сгенерированное: `ListMessagesParams{AfterID *int64, Limit *int}` (`models.gen.go`) — default 50 кодоген **не подставляет**, `nil` обрабатывает код.
- Заглушки: `internal/http/handler/chat.go` — `ListMessages`, `SendMessage` → 501.
- Запросы `backend/queries/chat.sql`: `CreateMessage`, `ListMessages` (`WHERE offer_id = $1 AND id > $2 ORDER BY id LIMIT $3`), `MarkMessagesRead` (upsert, указатель растёт через `GREATEST`), `CountUnreadMessages` (BE-08 удаляет — счётчик переехал в `GetOfferVisible`).
- Схема: `messages` (`id bigint identity` — курсор; CHECK `length(text) BETWEEN 1 AND 4000`; индекс `messages_offer_idx (offer_id, id)`), `message_reads (offer_id, user_id) PK`.
- PR #67 (`origin/be-03-auth`): `service/ratelimit.go` — интерфейс `RateLimiter{Check(key, limit, window) (retryAfter, allowed); Hit(key, window); Reset(key)}` и `MemoryRateLimiter` (скользящее окно, в памяти процесса); BE-03 (шаг 3) добавляет атомарный `Allow(key, limit, window) (retryAfter, ok)`; `apperr.RateLimitedAfter(d)`; `handler/errors.go: WriteError` ставит `Retry-After` в секундах.
- Из BE-08: `GetOfferVisible(id, viewer_role, viewer_org, viewer_user)` — 404 для не-сторон, отдаёт `status`, `lender_organization_id`, `borrower_organization_id`.

⚠️ Ловушки в текущем коде:
- `ListMessages` фильтрует только по `offer_id`: без предварительной проверки доступа читается любой чат. Не возвращает роль и организацию автора (обязательны в `Message`).
- Комментарий у `ListMessages` («если передать 0, вернёт последние сообщения») неверен: `id > 0 ORDER BY id ASC LIMIT n` отдаёт **первые** n (`docs/db/tables/messages.md`). Параметр сгенерирован как `ListMessagesParams.ID` — имя ничего не говорит.
- `CreateMessage` и `MarkMessagesRead` пишут по `offer_id` без проверки, что пользователь — сторона оффера; безопасны только после `GetOfferVisible` в том же запросе.
- `GET` меняет состояние (`message_reads`) — это требование контракта (`listMessages.description`), а AGENTS.md запрещает менять состояние в GET. См. «Решения».
- `MemoryRateLimiter` локален для процесса: при двух инстансах лимит удваивается (для MVP с одним инстансом приемлемо).

## Решения

- **Доступ** — через `GetOfferVisible` (BE-08): `admin` — все офферы, `lender` — свои, `borrower` — офферы на свои заявки; невидимый → 404 и для чтения, и для записи.
- **`ListMessages` заменяется на `ListMessagesVisible`** с обязательными `viewer_role`, `viewer_org` — правило видимости оффера в `WHERE`, как у остальных `*Visible` (`mds/db-security-plan.md`); старый удаляется. `CreateMessage` и `MarkMessagesRead` остаются: автор и читатель — всегда `p.UserID`, оффер проверен `GetOfferVisible` перед ними.
- **Admin не пишет** → 403 `FORBIDDEN` (роль не подходит; issue: «admin — только чтение»). Ответ `'403'` в контракт `sendMessage` добавляет BE-04 вместе с остальными защищёнными операциями (DECISIONS п. 2); здесь — только текст в `summary`.
- **Писать можно в оффер `sent` или `accepted`**, иначе 409 `INVALID_STATUS_TRANSITION` с текстом «Чат закрыт: оффер в статусе X» (`apperr.New(apperr.CodeInvalidTransition, …)` — это не переход, `InvalidTransition(from, to)` здесь не подходит).
- **Отметка прочтения в `GET`** — по контракту. Допустимо: меняется только указатель самого читателя, он монотонный (`GREATEST`) и идемпотентный, бизнес-состояние не меняется. Admin указатель не пишет — он не сторона чата и не имеет счётчика `unread_messages`.
- **Polling:** `after_id` по умолчанию `0`, `limit` по умолчанию `50` (`api/paths/chat.yaml`), сортировка `id ASC`. Первая загрузка отдаёт 50 самых старых сообщений; фронт дочитывает, пока пришло ровно `limit` (DECISIONS п. 15). Запрос «последние N» (`before_id`, `ORDER BY id DESC`) в MVP не делаем — понадобится, будет отдельной задачей.
- **Текст хранится как есть** (plain text): без trim и санитизации — экранирует фронт (AGENTS.md, «XSS»). Сообщение из одних пробелов принимается (DECISIONS п. 15, issue: «хранится как есть»). Длину 1–4000 проверяет валидатор по спеке и CHECK в БД.
- **Rate limit на отправку — 20 сообщений в минуту на пользователя** (DECISIONS п. 15): с запасом для живой переписки на демо. Ключ `chat:send:<user_id>`, константы `chatSendLimit = 20`, `chatSendWindow = time.Minute`. `Allow` до транзакции: проверяет и учитывает попытку атомарно. Попытка, упавшая в транзакции (оффер закрыт, 404), тоже расходует лимит — для 20/мин это незаметно, а гонки `Check`/`Hit` нет. Превышение → `apperr.RateLimitedAfter(retryAfter)` → 429 + `Retry-After`.
- **Уведомление — одно на каждое сообщение** (DECISIONS п. 14), без склейки: `message_received` (`entity_type = offer`, `entity_id = offer_id`) организации другой стороны в той же транзакции: автор lender → `borrower_organization_id`, автор borrower → `lender_organization_id`.
- **Ответ `sendMessage`** — созданное сообщение, перечитанное через `ListMessagesVisible(after_id = id − 1, limit = 1)` на том же `q`: одна функция конвертации для GET и POST.
- **Сервис возвращает `service.Message`** (`ID`, `OfferID`, `AuthorUserID`, `AuthorRole`, `AuthorOrg OrgShort`, `Text`, `IsMine`, `CreatedAt`) — `mds/plan.md`, раздел 7. `is_mine = author_user_id == p.UserID`.

## Шаги

### 1. Контракт: 403 у `sendMessage`

`api/paths/chat.yaml`: у `sendMessage` в `summary` — «admin — только чтение (403)». Ответ `'403'` должен уже стоять (BE-04, шаг 1); если BE-04 ещё не влит — добавь `'403': { $ref: '../components.yaml#/components/responses/Forbidden' }` здесь и отметь в PR. `make gen-api`, `make lint-api`, `make diff-api`.
**Готово:** `make diff-api` не показывает ломающих изменений; `make gen-check` без диффа после коммита.

### 2. SQL

`backend/queries/chat.sql`: удалить `ListMessages`, добавить
```sql
-- name: ListMessagesVisible :many
-- Polling: сообщения с id > after_id по возрастанию. after_id = 0 → самые старые limit штук.
SELECT sqlc.embed(m),
       u.role AS author_role,
       ao.id AS author_org_id, ao.name AS author_org_name, ao.country_code AS author_org_country_code
  FROM messages m
  JOIN offers o         ON o.id  = m.offer_id
  JOIN applications a   ON a.id  = o.application_id
  JOIN users u          ON u.id  = m.author_user_id
  JOIN organizations ao ON ao.id = u.organization_id
 WHERE m.offer_id = sqlc.arg('offer_id') AND m.id > sqlc.arg('after_id')::bigint
   AND (   sqlc.arg('viewer_role')::user_role = 'admin'
        OR (sqlc.arg('viewer_role')::user_role = 'lender'   AND o.lender_organization_id   = sqlc.arg('viewer_org')::uuid)
        OR (sqlc.arg('viewer_role')::user_role = 'borrower' AND a.borrower_organization_id = sqlc.arg('viewer_org')::uuid))
 ORDER BY m.id
 LIMIT sqlc.arg('max_rows');
```
`JOIN organizations` внутренний: автор — всегда borrower или lender (admin не пишет, у него `organization_id IS NULL`). Если `CountUnreadMessages` ещё не удалён в BE-08 — удалить здесь.
`make gen-sqlc`.
**Готово:** `make gen-check` без диффа; `grep -rn "ListMessages\b\|CountUnreadMessages" backend/` — пусто.

### 3. Сервис `Chat`

`backend/internal/service/chat.go`:
```go
type Chat struct {
    store   ChatStore        // GetOfferVisible, ListMessagesVisible, MarkMessagesRead
    tx      TxRunner
    notify  Notifier
    limiter RateLimiter
}
func (s *Chat) List(ctx context.Context, p Principal, offerID uuid.UUID, afterID int64, limit int32) ([]Message, error)
func (s *Chat) Send(ctx context.Context, p Principal, offerID uuid.UUID, text string) (Message, error)
```
- `List`: `GetOfferVisible` → `pgerr.Map(err, "Оффер не найден")` → `ListMessagesVisible` → если `p.Role != admin` и сообщения есть — `MarkMessagesRead(offer, p.UserID, last.ID)`. Пустой результат — `[]`, не `nil` (в JSON `[]`).
- `Send`:
  1. `p.Role` не `borrower`/`lender` → `apperr.Forbidden("Администратор не пишет в чат")`;
  2. `limiter.Allow(key, chatSendLimit, chatSendWindow)` → `!ok` → `apperr.RateLimitedAfter(retryAfter)`;
  3. `InTx`: `GetOfferVisible` (404) → статус не `sent`/`accepted` → 409 → `CreateMessage(offer, p.UserID, text)` → `Notifier`: `message_received` другой стороне → `ListMessagesVisible(after_id = msg.ID-1, max_rows = 1)`;
  4. отдельного `Hit` нет — попытку учёл `Allow`.
- Константы `chatSendLimit`, `chatSendWindow`, `defaultMessagesLimit = 50` — с комментарием, откуда значения.
**Готово:** юнит-тесты сервиса из «Тесты» зелёные.

### 4. Хендлеры и сборка

`handler/handler.go`: интерфейс `ChatService`, поле в `Deps`. `handler/chat.go`: `AfterID == nil` → `0`; `Limit == nil` → `50`; ответы `ListMessages200JSONResponse`, `SendMessage201JSONResponse`; `author_role` → `api.Role`. `cmd/server/main.go`: `service.NewChat(..., limiter)` — тот же экземпляр `service.NewMemoryRateLimiter()`, что у auth, если он уже создан; ключи не пересекаются благодаря префиксу `chat:send:`.
**Готово:** `handler/chat_test.go` зелёный; на запущенном сервере 21 `curl -X POST …/messages` подряд от одного пользователя → 21-й отвечает `429` с заголовком `Retry-After` ≥ 1.

### 5. Интеграционные тесты

`service/chat_integration_test.go`, фикстуры BE-07/08 (заявка, оффер L1, второй оффер L2).
**Готово:** `make db-reset && make test-all` зелёный.

### 6. Документация

`backend/README.md`: `listMessages`, `sendMessage` в «Статус реализации»; в «chat» — 403 для admin на `POST`, 429 + `Retry-After` и лимит 20/мин, первая загрузка отдаёт старейшие `limit` сообщений; в «Ошибки» строку `RATE_LIMITED` дополнить отправкой сообщений. `docs/db/tables/messages.md` и `docs/db/schema.md` «Покрытие sqlc» — `ListMessagesVisible`.
**Готово:** README и `api/paths/chat.yaml` описывают одинаковые коды ответов.

## Тесты

Юнит, `service/chat_test.go` (фейк стора, фейковый `TxRunner`, `Notifier`, `RateLimiter` с управляемым `allowed`):
- `Send` от admin → `FORBIDDEN`, лимитер и стор не вызваны.
- `Send`, лимитер `allowed=false, retryAfter=12s` → `RATE_LIMITED`, `RetryAfter = 12s`, `CreateMessage` не вызван.
- `Send` в оффер `rejected` / `withdrawn` → `INVALID_STATUS_TRANSITION`; `sent` и `accepted` → ок.
- `Send` от lender → уведомление `message_received` организации `borrower_organization_id`; от borrower → `lender_organization_id`; два сообщения подряд → два уведомления.
- `Send` с текстом `"   "` → ок, текст сохранён без изменений.
- `Send` → `Allow` вызван ровно 1 раз и до `InTx`; 21-й вызов за минуту (фейковые часы) → `RATE_LIMITED`, `InTx` не вызван.
- `List` от admin → `MarkMessagesRead` не вызван; от lender с 3 сообщениями (id 5, 6, 7) → `MarkMessagesRead(last_read = 7)`; пустой результат → `MarkMessagesRead` не вызван, ответ `[]`.
- `GetOfferVisible` вернул `ErrNoRows` → `NOT_FOUND` и для `List`, и для `Send`.
- `is_mine` = `true` только для сообщений `p.UserID`.

Хендлер, `handler/chat_test.go`:
- `GET /offers/{id}/messages` без параметров → сервис получил `afterID = 0`, `limit = 50`; `?after_id=10&limit=200` → `10`, `200`; `?limit=201` → `400 VALIDATION_ERROR`, `field = limit`.
- `POST` с `{"text":""}` → `400`; с текстом из 4001 символа → `400`; `{"text":"<script>alert(1)</script>"}` → `201`, `text` в ответе байт в байт как отправлен.
- Сервис вернул `RateLimitedAfter(1500ms)` → `429`, `Retry-After: 2`.
- Сервис вернул `Forbidden` → `403 FORBIDDEN`; `InvalidTransition` → `409`.

Интеграционные, `service/chat_integration_test.go`:
- Заёмщик пишет в оффер L1 → L1 читает `after_id=0` → 1 сообщение, `is_mine=false`, `author_role=borrower`, `author_organization.name` = имя заёмщика; у L1 появилась строка `message_reads`, `unread_messages` у L1 в `getOffer` = 0.
- Polling: 3 сообщения, `after_id` = id второго → ровно третье.
- Кредитор L2 читает и пишет в чат оффера L1 → 404 оба раза; заёмщик другой компании → 404.
- Admin читает → 200, строк `message_reads` для admin нет; admin пишет → `FORBIDDEN`.
- Оффер `withdrawn` → `Send` → `INVALID_STATUS_TRANSITION`; оффер `accepted` (после BE-09) → `Send` ок.
- `Send` создаёт строку `notifications` типа `message_received` у другой стороны.

## Готово, когда

- [ ] Стороны оффера читают и пишут, admin только читает (`POST` → 403), остальным — 404 (интеграционные тесты доступа).
- [ ] Polling: `after_id` + `limit` (по умолчанию 50, максимум 200), сортировка по `id` — тест polling зелёный.
- [ ] Запись в оффер не в статусе `sent`/`accepted` → 409.
- [ ] Чтение сдвигает `message_reads` читателя (не admin); отправка создаёт уведомление `message_received` другой стороне.
- [ ] Текст сохраняется как есть, длина 1–4000 (400 вне диапазона); превышение лимита отправки → 429 с `Retry-After`.
- [ ] Общий критерий готовности из `docs/backend_tasks/README.md`.

## Открытые вопросы

Нет: закрыты в [DECISIONS.md](DECISIONS.md) — лимит 20/мин, сообщения из пробелов, первая загрузка (п. 15), уведомление на каждое сообщение (п. 14), `403` в контракте (п. 2).
