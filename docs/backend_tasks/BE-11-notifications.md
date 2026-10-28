# BE-11 · Уведомления

Issue: #27 · Milestone: M3 · Зависит от: шаг 1 — ни от чего; шаги 2–4 — BE-04 (`Principal`, `x-roles`) · Блокирует: BE-05, BE-07, BE-08, BE-09, BE-10, BE-12, BE-13 (им нужен `Notifier`)

## Цель
Бизнес-события пишут уведомления в `notifications` в той же транзакции, что и смену статуса. Пользователь опрашивает `GET /notifications` и получает свою ленту с `unread_total`. `POST /notifications/{id}/read` и `POST /notifications/read-all` отмечают уведомления прочитанными; чужое уведомление → 404.

## Что уже есть
- Таблица `notifications` (`backend/migrations/00001_init.sql`): `id bigint identity`, `user_id → users ON DELETE CASCADE`, `type notification_type`, `title`, `entity_type`, `entity_id text`, `is_read`, `created_at`. Индексы `notifications_user_idx (user_id, created_at DESC)` и `notifications_unread_idx (user_id) WHERE NOT is_read`. Описание — `docs/db/tables/notifications.md`.
- Запросы в `backend/queries/notifications.sql`: `CreateNotification :one`, `ListNotifications :many` (`user_id`, `narg unread_only`, `LIMIT/OFFSET`), `CountUnreadNotifications :one`, `MarkNotificationRead :exec` (`WHERE id = $1 AND user_id = $2`), `MarkAllNotificationsRead :exec`.
- Контракт: `api/paths/notifications.yaml` (`listNotifications`: `unread_only`, `page`, `page_size`; `markNotificationRead`: `notificationId` — `int64`; `markAllNotificationsRead`), схемы `Notification` и `NotificationPage` (обязательное поле `unread_total`) — `api/schemas/notifications.yaml`.
- Типы `NotificationType` и `EntityType` лежат в `api/schemas/enums.yaml`, а не в `notifications.yaml`. Значения enum в БД (`notification_type`) совпадают со спекой.
- Заглушки 501: `backend/internal/http/handler/notifications.go`.
- Транзакции: `service.TxManager.InTx(ctx, func(q repository.Querier) error)` — `backend/internal/service/tx.go`.
- ⚠️ `MarkNotificationRead` объявлен как `:exec`, поэтому сервис не узнает, обновилась ли строка, и не сможет ответить 404 на чужой или несуществующий id. Нужен `:execrows`.
- ⚠️ Для `NotificationPage.total` нет запроса: `CountUnreadNotifications` считает только непрочитанные, а при `unread_only=false` `total` ≠ `unread_total`.
- ⚠️ Нет запросов «получатели события» — все админы, все пользователи организации.
- ⚠️ `RejectOtherOffers` и `RejectAllOffersForApplication` (`backend/queries/offers.sql`) — `:exec` без `RETURNING`, поэтому кредиторам автоматически отклонённых офферов не из чего отправить `offer_rejected`. Это отмечено и в `docs/db/schema.md` («Расхождения»). Исправляют владельцы запросов (BE-07, BE-09) по контракту из шага 1.
- ⚠️ Сид `backend/migrations/testdata/seed_demo.sql` пишет уведомления с жёстко заданным текстом `APP-000001`. Это задача BE-16.

## Решения
- **Notifier пишет через querier транзакции вызывающего**, а не через свой пул. Уведомление атомарно с событием: откат события откатывает и уведомление (ADR-0001, «таблица вместо брокера»).
- **Один SQL на событие:** `INSERT … SELECT` по получателям (`CreateNotificationsForOrganization`, `CreateNotificationsForAdmins`) вместо цикла `CreateNotification`. Нет N+1, и состав получателей определяет SQL, а не сервис.
- Отдельный запрос «id пользователей организации» (`ListActiveUserIDsByOrganization`) не нужен: его заменяет `INSERT … SELECT`.
- **Получатели — только `status = 'active'`:** и пользователи организации, и админы. Заблокированный пользователь уведомлений не получает.
- **`title` формирует вызывающий сервис**, Notifier его не сочиняет: тексты привязаны к событию и сущности (номер заявки, название компании). Это снимок на момент события, при переименовании компании он не меняется.
- **Ошибка записи уведомления — ошибка всей транзакции**, без «best effort». Иначе событие пройдёт без уведомления, а инвариант ADR-0001 нарушится молча.
- **Старые уведомления в MVP не чистим** (DECISIONS п. 39): объёмы демо малы, ленту покрывает индекс `notifications_user_idx`.
- **`markNotificationRead` идемпотентен:** повторная отметка своего уведомления → 204. Чужое или несуществующее → 404 (README, «Роли и доступ»).

## Шаги

### 1. Каркас `Notifier` (отдельный PR, до BE-07)
Зависимостей нет — делается сразу.

1. В `backend/queries/notifications.sql` добавить:
   ```sql
   -- name: CreateNotificationsForOrganization :execrows
   INSERT INTO notifications (user_id, type, title, entity_type, entity_id)
   SELECT u.id, sqlc.arg('type')::notification_type, sqlc.arg('title')::text,
          sqlc.arg('entity_type')::entity_type, sqlc.arg('entity_id')::text
   FROM users u
   WHERE u.organization_id = sqlc.arg('organization_id')::uuid AND u.status = 'active';

   -- name: CreateNotificationsForAdmins :execrows
   INSERT INTO notifications (user_id, type, title, entity_type, entity_id)
   SELECT u.id, sqlc.arg('type')::notification_type, sqlc.arg('title')::text,
          sqlc.arg('entity_type')::entity_type, sqlc.arg('entity_id')::text
   FROM users u
   WHERE u.role = 'admin' AND u.status = 'active';
   ```
   Явные `::type` у `sqlc.arg` обязательны: в `INSERT … SELECT` sqlc не выводит тип параметра из колонки. `make gen-sqlc`.
2. `backend/internal/service/notifier.go`:
   ```go
   // Notification — одно уведомление; получателей выбирает метод Notifier.
   type Notification struct {
       Type       repository.NotificationType
       Title      string
       EntityType repository.EntityType
       EntityID   string // uuid.String() или strconv.FormatInt для bigint-сущностей
   }

   // NotificationWriter — часть repository.Querier, которая нужна Notifier.
   type NotificationWriter interface {
       CreateNotification(ctx context.Context, arg repository.CreateNotificationParams) (repository.Notification, error)
       CreateNotificationsForOrganization(ctx context.Context, arg repository.CreateNotificationsForOrganizationParams) (int64, error)
       CreateNotificationsForAdmins(ctx context.Context, arg repository.CreateNotificationsForAdminsParams) (int64, error)
   }

   // Notifier пишет уведомления querier'ом транзакции бизнес-события.
   type Notifier struct{}

   func (Notifier) NotifyUser(ctx context.Context, q NotificationWriter, userID uuid.UUID, n Notification) error
   func (Notifier) NotifyOrganization(ctx context.Context, q NotificationWriter, orgID uuid.UUID, n Notification) error
   func (Notifier) NotifyAdmins(ctx context.Context, q NotificationWriter, n Notification) error
   ```
   Ошибки оборачивать: `fmt.Errorf("notify organization %s: %w", orgID, err)`. Doc-комментарий пакета или типа объясняет инвариант: вызывать только внутри `InTx`, передавая `q` из колбэка.
   Пример использования для владельцев событий — в doc-комментарии `Notifier`:
   ```go
   err := s.tx.InTx(ctx, func(q repository.Querier) error {
       app, err := q.SubmitApplication(ctx, id)
       if err != nil { return err }
       return s.notifier.NotifyAdmins(ctx, q, service.Notification{
           Type: repository.NotificationTypeApplicationSubmitted, Title: "…",
           EntityType: repository.EntityTypeApplication, EntityID: app.ID.String(),
       })
   })
   ```
3. Unit-тест `backend/internal/service/notifier_test.go` с фейком `NotificationWriter`: каждый метод передаёт в запрос ровно `Type/Title/EntityType/EntityID`; ошибка фейка возвращается обёрнутой (`errors.Is` находит исходную).
4. Интеграционный тест `backend/internal/service/notifier_integration_test.go` (пропуск по образцу `newPool` из `dictionaries_integration_test.go`). Всё — в одной транзакции с `Rollback`: создать организацию и пользователя со случайными email и рег. номером, плюс второго админа. `NotifyOrganization` → у пользователя ровно одна строка; `NotifyAdmins` → строка у каждого активного админа; заблокированный админ (`BlockUser`) строки не получает.
5. В PR отметить «каркас Notifier из BE-11, шаг 1».

**Готово:** `make gen-check` без диффа; `go -C backend test -short -run TestNotifier ./internal/service/` зелёный; `make test-all` проходит `TestNotifierIntegration`.

### 2. Запросы для ручек
В `backend/queries/notifications.sql`:
- `MarkNotificationRead` перевести с `:exec` на `:execrows` (тело не меняется).
- Добавить `CountNotifications :one` с тем же `WHERE`, что у `ListNotifications`: `user_id = $1 AND (sqlc.narg('unread_only')::boolean IS NOT TRUE OR is_read = false)`.
- В `ListNotifications` добавить тай-брейк `ORDER BY created_at DESC, id DESC`: уведомления одной транзакции получают одинаковый `now()`, без тай-брейка пагинация нестабильна.

**Готово:** `make gen-sqlc`; в `internal/repository/sqlc/querier.go` у `MarkNotificationRead` возврат `(int64, error)`, есть `CountNotifications`.

### 3. Сервис `service.Notifications`
`backend/internal/service/notifications.go`, зависимость — интерфейс `NotificationStore` рядом (по образцу `AuthStore` из PR #67):
- `List(ctx, p Principal, unreadOnly bool, limit, offset int32) (NotificationPage, error)` — `ListNotifications` + `CountNotifications` + `CountUnreadNotifications`. `unread_total` не зависит от `unread_only`.
- `MarkRead(ctx, p Principal, id int64) error` — `rows == 0` → `apperr.NotFound("Уведомление не найдено")`.
- `MarkAllRead(ctx, p Principal) error`.

Фильтр всегда по `p.UserID`, `user_id` из запроса не принимается: в контракте его нет.

**Готово:** табличный тест `TestNotificationsMarkRead` (rows 1 → nil; rows 0 → `CodeNotFound`) и `TestNotificationsList` (`unread_total` берётся из `CountUnreadNotifications`, `total` — из `CountNotifications`) зелёные.

### 4. Хендлеры
`backend/internal/http/handler/notifications.go`: заменить три заглушки. `Principal` — через `currentPrincipal(ctx)` (BE-04, `handler/principal.go`). `page`/`page_size` → `limit/offset` — общим хелпером пагинации; если его ещё нет, создать `handler/pagination.go` (дефолты 1/20 из `api/components.yaml`). `Deps` — новое поле `Notifications NotificationService` (интерфейс в `handler.go`). Подключить в `backend/cmd/server/main.go`. Роли (`x-roles`, BE-04): все три операции — `[borrower, lender, admin]`.

**Готово:** тесты хендлера через `httptest` с фейковым сервисом (образец — `handler/dictionaries_test.go`): `GET /api/v1/notifications?unread_only=true` → 200, JSON с `unread_total`; `POST …/notifications/999/read` при `NotFound` от фейка → 404 `NOT_FOUND`; `POST …/notifications/read-all` → 204 без тела.

### 5. Подключение событий (по мере готовности владельцев)
Каждая задача-владелец вызывает `Notifier` в своей транзакции. BE-11 отвечает за то, чтобы таблица ниже совпадала с кодом: после мержа BE-07…BE-13 пройти `grep -rn "NotificationType" backend/internal/service` и сверить.

| Тип | Задача | Событие (операция) | Получатель | `entity_type` / `entity_id` | Пример `title` |
|---|---|---|---|---|---|
| `kyc_submitted` | BE-05 | `submitKyc` (`draft`/`rejected` → `under_review`) | админы (`NotifyAdmins`) | `organization` / org id | `KYC на проверке: «Альфа»` |
| `kyc_approved` | BE-12 | `adminApproveKyc` | организация | `organization` / org id | `Компания верифицирована` |
| `kyc_rejected` | BE-12 | `adminRejectKyc` | организация | `organization` / org id | `Верификация отклонена — причина в профиле` |
| `application_submitted` | BE-07 | `submitApplication` (→ `moderation`) | админы | `application` / app id | `Заявка APP-000123 на модерации` |
| `application_published` | BE-12 | `adminApproveApplication` | организация-заёмщик | `application` / app id | `Заявка APP-000123 опубликована` |
| `application_rejected` | BE-12 | `adminRejectApplication` | организация-заёмщик | `application` / app id | `Заявка APP-000123 возвращена на доработку` |
| `offer_received` | BE-08 | `createOffer` | организация-заёмщик | `offer` / offer id | `Новое предложение по заявке APP-000123` |
| `offer_accepted` | BE-09 | `acceptOffer` | организация-кредитор принятого оффера | `deal` / deal id | `Ваше предложение по APP-000123 принято — сделка DEAL-000045` |
| `offer_rejected` | BE-08 | `rejectOffer` (`rejected_by_borrower`) | организация-кредитор | `offer` / offer id | `Предложение по APP-000123 отклонено` |
| `offer_rejected` | BE-09 | `acceptOffer` → прочие `sent`-офферы (`another_offer_accepted`) | кредитор каждого отклонённого | `offer` / offer id | `Заёмщик выбрал другое предложение по APP-000123` |
| `offer_rejected` | BE-07 | `cancelApplication`, `rejectAllOffers` | кредитор каждого отклонённого | `offer` / offer id | `Заявка APP-000123 отменена` / `…предложения отклонены` |
| `offer_withdrawn` | BE-08 | `withdrawOffer` | организация-заёмщик | `offer` / offer id | `Кредитор отозвал предложение по APP-000123` |
| `message_received` | BE-10 | `sendMessage` | организация другой стороны оффера; админ не получает | `offer` / offer id | `Новое сообщение по APP-000123` |
| `aml_flag_created` | BE-13 | скрининг из `submitKyc` / `submitApplication` создал ≥ 1 флаг — **одно уведомление на прогон**, не на флаг (DECISIONS п. 14) | админы | `organization` или `application` / id | `AML: 2 флага по «Gamma GmbH» (high)` |

Не уведомляем (DECISIONS п. 14), тесты владельцев проверяют, что вызова нет:
- заёмщика о сделке — типа `deal_created` нет, сделка приходит в ответе `acceptOffer` (BE-09);
- админов о новом оффере (BE-08) и об отмене заявки, в том числе из `moderation` (BE-07);
- склейку `message_received` не делаем: одно уведомление на каждое сообщение чата (BE-10).

Для строк BE-07/BE-09 с массовым отклонением BE-07 вводит `RejectSentOffersByApplication :many … RETURNING id, lender_organization_id` вместо `RejectOtherOffers`/`RejectAllOffersForApplication` (`BE-07-applications.md`, `BE-09-accept-deal.md`). Сервис вызывает `NotifyOrganization` на каждую возвращённую строку.

**Готово:** для каждой строки таблицы есть тест сервиса-владельца, где фейковый querier фиксирует вызов `CreateNotificationsFor*` с нужным `Type` и получателем; таблица в этом плане и в `backend/README.md` («notifications») совпадает с кодом.

### 6. Документация
`backend/README.md`: в разделе «notifications» перечислить, какое событие кому шлёт уведомление (короткая версия таблицы шага 5), добавить `unread_total` и 404 для чужого id; в «Статус реализации» отметить три операции. `docs/db/schema.md`, «Что сознательно не делаем»: «старые уведомления не удаляются».

**Готово:** раздел обновлён в том же PR, что шаг 4.

## Тесты
| Случай | Ожидание |
|---|---|
| `GET /notifications` без cookie | 401 `UNAUTHORIZED` |
| `GET /notifications?unread_only=true&page=1&page_size=2` при 3 непрочитанных и 1 прочитанном | 200, `items` — 2 шт., `total=3`, `unread_total=3` |
| `GET /notifications` (те же данные) | `total=4`, `unread_total=3`, порядок `created_at DESC, id DESC` |
| `POST /notifications/{id}/read`, своё непрочитанное | 204; следующий `GET` — `unread_total` меньше на 1 |
| то же повторно | 204 |
| `POST /notifications/{id}/read`, id уведомления другого пользователя | 404 `NOT_FOUND`, у владельца `is_read` не изменился (интеграционный) |
| `POST /notifications/abc/read` | 400 `VALIDATION_ERROR`, `field: notificationId` |
| `POST /notifications/read-all` без `X-CSRF-Token` | 403 `CSRF_INVALID` |
| `NotifyAdmins` при 2 активных и 1 заблокированном админе | 2 строки |
| ошибка `CreateNotificationsForOrganization` внутри `InTx` | транзакция откатывается, событие не сохранено (интеграционный: статус сущности прежний) |

## Готово, когда
- [ ] `service.Notifier` пишет в `notifications` querier'ом транзакции события; интеграционный тест доказывает откат уведомления вместе с событием.
- [ ] Каждый тип `NotificationType` из `api/schemas/enums.yaml` порождается кодом ровно по таблице шага 5; `kyc_submitted`, `application_submitted`, `aml_flag_created` получают только активные админы.
- [ ] `GET /notifications` возвращает `total` и `unread_total`; `unread_total` не зависит от `unread_only`.
- [ ] Чужое уведомление в `markNotificationRead` → 404.
- [ ] И общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: чистка уведомлений закрыта в [DECISIONS.md](DECISIONS.md), п. 39.
