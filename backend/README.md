# Backend: REST API платформы ликвидности

Go-сервер (net/http, PostgreSQL), который реализует контракт [`api/openapi.yaml`](../api/openapi.yaml). Контракт — источник правды: по нему генерируются Go-сервер (oapi-codegen) и TS-клиент фронта (openapi-typescript), а каждый входящий запрос валидируется по нему в рантайме. В этом файле — что за API, как им пользоваться и как устроен код. Поля и схемы целиком — в самой спеке.

- Запуск локально, команды `make` — [корневой README](../README.md#запуск-бэкенда).
- Решения и их причины — [ADR-0001](../docs/adr/0001-architecture-and-api-contract.md) (архитектура), [ADR-0002](../docs/adr/0002-spec-layout-and-request-validation.md) (раскладка контракта, валидация), [ADR-0003](../docs/adr/0003-server-package-per-tag.md) (сервер по пакету на тег), [ADR-0004](../docs/adr/0004-authorization.md) (авторизация).
- Схема БД — [`docs/db/schema.md`](../docs/db/schema.md).
- Пишете код бэкенда — начните с [«Как устроен код»](#как-устроен-код): где пишем руками, а что генерируется.

## Быстрый старт

```sh
make db     # Postgres + миграции + демо-данные (нужен Docker)
make run    # API на http://localhost:8080/api/v1
curl localhost:8080/api/v1/healthz
# {"status":"ok"}
```

Без бэкенда фронт работает против мок-сервера, который отвечает примерами из спеки:

```sh
npx @stoplight/prism-cli mock api/openapi.yaml   # http://127.0.0.1:4010
```

## Статус реализации

Контракт описывает **51 операцию**. Каждая уже маршрутизируется и валидируется по спеке, а ещё не реализованные отвечают `501 NOT_IMPLEMENTED` в общем формате ошибок. Поэтому фронт видит итоговую форму API с первого дня.

| Работает | Операции |
|---|---|
| ✅ | `GET /healthz`, `GET /dictionaries/countries`, `GET /dictionaries/currencies`, `GET /dictionaries/transfer-methods` |
| ✅ | `POST /auth/register`, `POST /auth/login`, `POST /auth/logout`, `GET /auth/me` — вместе с сессией, CSRF и rate limit (ниже) |
| ✅ | `GET`/`PATCH /organizations/me`, `GET`/`POST /organizations/me/documents`, `DELETE /organizations/me/documents/{documentId}`, `GET /documents/{documentId}/file`, `POST /organizations/me/kyc/submit` — профиль, бенефициары, KYC-документы и отправка на проверку с AML-скринингом и сверкой с реестром |
| ✅ | `GET /registry/lookup` — поиск компании в мок-реестре для автозаполнения |
| ✅ | `GET`/`POST /applications`, `GET`/`PATCH /applications/{applicationId}`, `POST …/submit`, `POST …/cancel`, `POST …/reject-offers`, `GET …/history` — заявки: витрина с фильтрами, статусы, история, уведомления |
| ⏳ 501 | остальные 27 — реализуются по образцу справочников (см. «Как добавить операцию») |

## Общие правила

### Базовый URL и формат

- Все пути — относительно **`/api/v1`** (`http://localhost:8080/api/v1` локально).
- Тела запросов и ответов — `application/json; charset=utf-8`. Исключение — загрузка KYC-документа (`multipart/form-data`) и скачивание файла.
- Поля в JSON — `snake_case`. Идентификаторы — UUID (v7, сортируются по времени создания). Даты — RFC 3339 (`2026-10-22T12:00:00Z`).

### Аутентификация: сессия в cookie + CSRF

JWT не используется (ADR-0001, п. 3): сессию в `HttpOnly`-cookie нельзя украсть через XSS, а блокировка пользователя админом просто удаляет его сессии.

1. `POST /auth/login` ставит две cookie (`POST /auth/register` — тоже: после регистрации пользователь сразу залогинен):
   - `session` — `HttpOnly; Secure; SameSite=Lax`, непрозрачный токен; в БД хранится только его хэш;
   - `csrf_token` — **не** `HttpOnly`: фронт читает её из JavaScript.
2. Каждый запрос, кроме `GET`/`HEAD`/`OPTIONS`, обязан передать заголовок `X-CSRF-Token` со значением cookie `csrf_token` (double-submit). Иначе — `403 CSRF_INVALID`. Исключение: операции с `security: []` (`register`, `login`) CSRF не проверяют — до входа cookie `csrf_token` нет; от login-CSRF защищает обязательный `Content-Type: application/json`.
3. Фронт ходит с `credentials: "include"`.
4. Rate limit проверяется до хэширования пароля: `login` — 10 попыток на один email и 50 с одного IP за 15 минут (успешный вход сбрасывает счётчик email), `register` — 10 с одного IP за час. Превышение — `429 RATE_LIMITED` с заголовком `Retry-After`. Счётчики в памяти процесса.

Без сессии доступны операции с `security: []` в спеке: `POST /auth/register`, `POST /auth/login`, справочники `GET /dictionaries/*`, `GET /registry/lookup` и `GET /healthz`. Всё остальное без сессии — `401 UNAUTHORIZED`.

### Роли и доступ

- **Роль** пользователя — `borrower` (заёмщик, РФ), `lender` (кредитор, не РФ) или `admin` — определяет, какие операции ему доступны. Роли операции записаны в спеке расширением `x-roles`, требование KYC — `x-kyc-required: true`; их проверяет `middleware.Authorize`, а сервис повторяет проверку роли ([ADR-0004](../docs/adr/0004-authorization.md)).
- **Владение** проверяется на каждой сущности в SQL: заявки, офферы и сделки читаются только запросами `*Visible` с обязательными `viewer_role`/`viewer_org`. Чужая сущность отвечает **`404`**, а не `403`: так нельзя перебором узнать, что объект существует (защита от IDOR).
- **Списки** `GET /applications`, `GET /offers`, `GET /deals` зависят от роли: один и тот же эндпоинт отдаёт заёмщику его заявки, кредитору — витрину опубликованных и т.д.
- Пока компания не прошла KYC (`kyc_status != verified`), она не может создавать заявки и офферы — `403 KYC_NOT_VERIFIED`.

Порядок проверок на каждый запрос:

| # | Ситуация | Ответ | Где |
|---|---|---|---|
| 1 | путь или метод не из спеки | `404 NOT_FOUND` | валидатор |
| 2 | операция публичная (`security: []`) | пропускается с сессией и без | `middleware.Authorize` |
| 3 | нет сессии, она истекла или пользователь заблокирован | `401 UNAUTHORIZED` | `middleware.Authorize` |
| 4 | роли нет в `x-roles` операции | `403 FORBIDDEN` | `middleware.Authorize` |
| 5 | `x-kyc-required`, а компания не `verified` | `403 KYC_NOT_VERIFIED` | `middleware.Authorize` |
| 6 | мутирующий запрос без `X-CSRF-Token` или с чужим | `403 CSRF_INVALID` | `middleware.CSRF` |
| 7 | запрос не соответствует схеме | `400 VALIDATION_ERROR` | валидатор |
| 8 | сущность чужая **или** не существует | `404 NOT_FOUND` | SQL `*Visible` |

### Деньги, проценты, коды

- **Суммы и проценты — строки** в десятичном формате: `"500000.00"`, `"8.00"` (= 8 %). Float не используется нигде: в JSON он теряет точность, в БД — `NUMERIC`. Формат суммы — до 15 цифр и до 2 знаков после точки, процента — до 3 цифр и до 4 знаков.
- **Валюта — ISO 4217** (`RUB`, `USD`, `EUR`), **страна — ISO 3166-1 alpha-2** (`RU`, `DE`). Допустимые значения — справочники `GET /dictionaries/currencies` и `/countries`. У страны есть флаг `fatf_high_risk` (страна в чёрном или сером списке FATF).

### Ошибки

Любая ошибка — в одном формате, с машиночитаемым кодом и сообщением для пользователя:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Запрос не соответствует контракту",
    "details": [
      { "field": "amount", "message": "неверный формат" },
      { "field": "term_months", "message": "значение вне допустимого диапазона" }
    ]
  }
}
```

`details` есть только у ошибок валидации: по записи на поле. `field` — имя параметра (`page`, `applicationId`) или путь в теле (`beneficiaries.0.full_name`). Ввод пользователя в ответ не попадает.

| Код | HTTP | Когда |
|---|---|---|
| `VALIDATION_ERROR` | 400 | запрос не соответствует спеке: обязательное поле, формат, `enum`, `minimum`, битый JSON, тело больше лимита |
| `UNAUTHORIZED` | 401 | нет сессии или она истекла |
| `FORBIDDEN` | 403 | роль не позволяет операцию |
| `CSRF_INVALID` | 403 | нет `X-CSRF-Token` или он не совпадает с cookie |
| `KYC_NOT_VERIFIED` | 403 | компания не верифицирована (заявки, офферы) |
| `NOT_FOUND` | 404 | нет такой сущности **или она чужая**; неизвестный путь |
| `CONFLICT` | 409 | конфликт данных (например, email уже занят; реквизиты компании изменились во время `kyc/submit`) |
| `INVALID_STATUS_TRANSITION` | 409 | переход статуса не разрешён (см. «Статусы») |
| `FILE_TOO_LARGE` | 413 | загружаемый документ больше 10 МБ |
| `RATE_LIMITED` | 429 | слишком много попыток (`login`, `register`, `registry/lookup`); заголовок `Retry-After` — через сколько секунд повторить |
| `INTERNAL` | 500 | непредвиденная ошибка; подробности только в логе сервера |
| `NOT_IMPLEMENTED` | 501 | операция есть в контракте, но ещё не реализована |
| `UNAVAILABLE` | 503 | нет соединения с БД (`/healthz`) |
| `UPSTREAM_UNAVAILABLE` | 502 | внешний реестр не ответил (`registry/lookup`) — реквизиты заполняются вручную |

### Пагинация и polling

- Списки: `?page=1&page_size=20` (`page` с 1; `page_size` по умолчанию 20, максимум 100). Ответ — `{ "items": [...], "total": 137, "page": 1, "page_size": 20 }`.
- Чат и уведомления — **polling** вместо WebSocket (ADR-0001, п. 5): фронт раз в 3–5 с запрашивает `GET /offers/{offerId}/messages?after_id=<последний id>` и получает только новые сообщения.

### Лимиты

- Тело запроса — до **1 МБ** (JSON), больше — `400 VALIDATION_ERROR` с `field: "body"`. Файл документа — до **10 МБ**, больше — `413 FILE_TOO_LARGE`.
- Таймауты сервера: заголовки — 5 с, чтение — 15 с, запись — 30 с.

## Эндпоинты

Кто может вызвать операцию: **все** — без сессии; **авториз.** — любой пользователь с сессией (данные по его роли и владению); **B** — borrower, **L** — lender, **A** — admin.

### auth — регистрация и вход

| Метод | Путь | Что делает | Кто |
|---|---|---|---|
| POST | `/auth/register` | Регистрация компании и её первого пользователя (`role`: `borrower` / `lender`) → `201` | все |
| POST | `/auth/login` | Вход по email и паролю, ставит cookie `session` и `csrf_token` | все |
| POST | `/auth/logout` | Выход: инвалидирует сессию, чистит cookie → `204` | авториз. |
| GET | `/auth/me` | Текущий пользователь и его компания (`organization: null` у admin) | авториз. |

### organizations — профиль компании и KYC

| Метод | Путь | Что делает | Кто |
|---|---|---|---|
| GET | `/organizations/me` | Профиль своей компании | B, L |
| PATCH | `/organizations/me` | Редактировать профиль (только в статусах `draft` / `rejected`) | B, L |
| GET | `/organizations/me/documents` | Загруженные KYC-документы | B, L |
| POST | `/organizations/me/documents` | Загрузить документ: `multipart/form-data`, поля `doc_type`, `file` → `201` | B, L |
| DELETE | `/organizations/me/documents/{documentId}` | Удалить свой документ (только `draft` / `rejected`) → `204` | B, L |
| POST | `/organizations/me/kyc/submit` | Отправить компанию на верификацию (`draft` / `rejected` → `under_review`) | B, L |
| GET | `/documents/{documentId}/file` | Скачать файл документа | владелец, A |
| GET | `/registry/lookup` | Автозаполнение по реестру: ИНН/ОГРН для RU, LEI для остальных. **В MVP — мок** (ADR-0001, п. 6) | все |

- **Загрузка документа:** в `multipart/form-data` поле `doc_type` должно идти **до** `file` — сервер читает файл потоком и без типа не знает, куда его отнести; иначе `400 VALIDATION_ERROR`. Файл больше 10 МБ — `413 FILE_TOO_LARGE`.
- **Скачивание** `GET /documents/{documentId}/file`: только владелец документа или admin (чужой — `404`); ответ с `Content-Disposition: attachment`, чтобы браузер не исполнял файл в контексте сайта.
- **Отправка на проверку:** нужны обязательные документы и хотя бы один бенефициар (`400` с `details` по каждому недостающему). Сверка с реестром не блокирует отправку: при сбое реестра компания всё равно уходит в `under_review`, админ сверяет вручную. Если реквизиты изменились параллельно со сверкой — `409 CONFLICT`, повторить.
- **Поиск в реестре** `GET /registry/lookup` (без логина): реестр — мок на фикстурах демо-компаний, `source: "mock"`. Номер: для RU — ИНН (10 цифр) или ОГРН (13), для остальных стран — LEI (20 символов, регистр не важен); другой формат — `400` с `field: number`. Не нашли — `200` с `found: false`. Лимит — 10 запросов в минуту с IP и 60 в минуту всего, сверх — `429` с `Retry-After`. Реестр не ответил — `502 UPSTREAM_UNAVAILABLE` (в моке — номер `RU`/`0000000000`). Lookup в БД не пишет: снапшот в `registry_checks` сохраняет только `kyc/submit`.

### dictionaries — справочники

| Метод | Путь | Что делает | Кто |
|---|---|---|---|
| GET | `/dictionaries/countries` | Страны (ISO 3166-1) с флагом `fatf_high_risk` | все |
| GET | `/dictionaries/currencies` | Валюты (ISO 4217) | все |
| GET | `/dictionaries/transfer-methods` | Способы передачи средств (`swift`, `sepa`, `other`) | все |

### applications — заявки заёмщиков

| Метод | Путь | Что делает | Кто |
|---|---|---|---|
| GET | `/applications` | Список (по роли): заёмщику — свои в любом статусе, кредитору — витрина `published` и заявки с его офферами (архив), admin — все. Фильтры: `status`, `target_country` и `transfer_method` (несколько значений — ИЛИ), `currency`, `amount_min`/`amount_max` (только вместе с `currency`), `term_min`/`term_max`, `created_from`/`created_to`; кредитору — `has_my_offer` (архив откликов), admin — `borrower_organization_id`. Сортировка `sort` (по дате создания или публикации, сумме, сроку), пагинация | авториз. |
| POST | `/applications` | Создать заявку в статусе `draft` (KYC verified) → `201` | B |
| GET | `/applications/{applicationId}` | Карточка заявки | авториз. |
| PATCH | `/applications/{applicationId}` | Редактировать (владелец, `draft` / `rejected`) | B |
| POST | `/applications/{applicationId}/submit` | На модерацию (`draft` / `rejected` → `moderation`) | B |
| POST | `/applications/{applicationId}/cancel` | Отменить (любой статус, кроме `agreed` / `cancelled`); все `sent`-офферы → `rejected` с причиной `application_cancelled`, их кредиторам — `offer_rejected` | B |
| POST | `/applications/{applicationId}/reject-offers` | Отклонить все `sent`-офферы (причина `rejected_by_borrower`) → `{"rejected_count": n}`, `0` — тоже `200`; заявка остаётся `published` | B |
| GET | `/applications/{applicationId}/history` | История смены статусов заявки и её офферов; кредитор видит только свои офферы и без комментариев модерации | авториз. |

Ошибки заявок: `amount_min`/`amount_max` без `currency` → `400 VALIDATION_ERROR` (поле в `details`); чужая или невидимая заявка → `404 NOT_FOUND`; `PATCH` вне `draft`/`rejected`, `submit`/`cancel` из недопустимого статуса, `reject-offers` вне `published` → `409 INVALID_STATUS_TRANSITION`. `offers_count` — все офферы заявки в любом статусе (только владельцу и admin); `transfer_method_comment: ""` в `PATCH` очищает комментарий.

### offers — предложения кредиторов

| Метод | Путь | Что делает | Кто |
|---|---|---|---|
| GET | `/offers` | Список (по роли): кредитору — его предложения, заёмщику — входящие; архив | авториз. |
| GET | `/applications/{applicationId}/offers` | Офферы по заявке | авториз. |
| POST | `/applications/{applicationId}/offers` | Сделать оффер (KYC verified, заявка `published`, сумма ≤ суммы заявки) → `201` | L |
| GET | `/offers/{offerId}` | Карточка оффера (автор-кредитор, заёмщик-владелец заявки, admin) | авториз. |
| POST | `/offers/{offerId}/accept` | **Принять оффер → создаётся сделка** (`200`, тело — `Deal`); остальные офферы заявки отклоняются; повторный вызов → `409` | B |
| POST | `/offers/{offerId}/reject` | Отклонить оффер | B |
| POST | `/offers/{offerId}/withdraw` | Отозвать свой оффер (только из `sent`) | L |

### deals — сделки

| Метод | Путь | Что делает | Кто |
|---|---|---|---|
| GET | `/deals` | Сделки: стороны сделки или admin. Период `created_from`/`created_to` — по `agreed_at`; admin — `borrower_organization_id`, `lender_organization_id`; пагинация | авториз. |
| GET | `/deals/{dealId}` | Карточка сделки | стороны, A |

### chat — переписка по офферу

| Метод | Путь | Что делает | Кто |
|---|---|---|---|
| GET | `/offers/{offerId}/messages` | Сообщения (`after_id`, `limit` ≤ 200); чтение помечает прочитанными | стороны оффера; A — только чтение |
| POST | `/offers/{offerId}/messages` | Написать (оффер в статусе `sent` или `accepted`) → `201` | стороны оффера |

### notifications — уведомления

| Метод | Путь | Что делает | Кто |
|---|---|---|---|
| GET | `/notifications` | Уведомления текущего пользователя (polling) | авториз. |
| POST | `/notifications/{notificationId}/read` | Отметить прочитанным → `204` | авториз. |
| POST | `/notifications/read-all` | Отметить все прочитанными → `204` | авториз. |

### admin — модерация, KYC, AML, аналитика

| Метод | Путь | Что делает | Кто |
|---|---|---|---|
| GET | `/admin/organizations` | Компании; очередь верификации — `kyc_status=under_review` | A |
| GET | `/admin/organizations/{organizationId}` | Полная карточка: документы, бенефициары, AML-флаги, пользователи | A |
| POST | `/admin/organizations/{organizationId}/kyc/approve` | Верифицировать (`under_review` → `verified`) | A |
| POST | `/admin/organizations/{organizationId}/kyc/reject` | Отклонить с причиной (`under_review` → `rejected`) | A |
| POST | `/admin/applications/{applicationId}/approve` | Опубликовать заявку (`moderation` → `published`) | A |
| POST | `/admin/applications/{applicationId}/reject` | Вернуть на доработку с причиной (`moderation` → `rejected`) | A |
| GET | `/admin/aml/flags` | AML-флаги rule-based скрининга (санкции, FATF, порог суммы). Фильтры `status`, `severity`, `organization_id`, `application_id`; пагинация | A |
| POST | `/admin/aml/flags/{flagId}/resolve` | Закрыть AML-флаг | A |
| GET | `/admin/users` | Пользователи | A |
| POST | `/admin/users/{userId}/block` | Заблокировать (удаляет все его сессии) | A |
| POST | `/admin/users/{userId}/unblock` | Разблокировать | A |
| GET | `/admin/audit-log` | Журнал действий пользователей | A |
| GET | `/admin/analytics/summary` | Аналитика: число заявок, средний размер, топ-страны | A |

### system

| Метод | Путь | Что делает | Кто |
|---|---|---|---|
| GET | `/healthz` | Liveness/readiness: `200 {"status":"ok"}` или `503 UNAVAILABLE`, если БД не отвечает | все |

## Статусы

Переходы реализуются в `internal/service` таблицей разрешённых переходов; недопустимый переход — `409 INVALID_STATUS_TRANSITION`.

```
Организация (KYC):  draft ─submit→ under_review ─approve→ verified
                                      └─reject→ rejected ─submit→ under_review

Заявка:             draft ─submit→ moderation ─approve→ published ─accept offer→ agreed
                                      └─reject→ rejected ─submit→ moderation
                    draft | moderation | rejected | published ─cancel→ cancelled

Оффер:              sent ─accept→ accepted
                         ─reject→ rejected   (borrower | another_offer_accepted | application_cancelled)
                         ─withdraw→ withdrawn
```

Принятие оффера — одна транзакция БД с блокировкой строки заявки: оффер → `accepted`, остальные `sent`-офферы заявки → `rejected` (`another_offer_accepted`), заявка → `agreed`, создаётся сделка со снапшотом условий. Повторный или параллельный `accept` по той же заявке отвечает `409`, вторая сделка не появится.

## Сквозной сценарий: от регистрации до сделки

Так выглядит демо по контракту (`curl` с файлом cookie; `$CSRF` — значение cookie `csrf_token` после входа).

```sh
API=http://localhost:8080/api/v1

# 1. Заёмщик (РФ) регистрируется — компания в статусе draft
curl -c b.txt -X POST $API/auth/register -H 'Content-Type: application/json' -d '{
  "email": "cfo@alfa.ru", "password": "correct-horse-1", "role": "borrower",
  "organization": { "name": "ООО Альфа", "country_code": "RU",
                    "registration_number": "7707083893", "legal_address": "Москва, ул. Тверская, 1" }
}'

# 2. Загружает документы и отправляет компанию на KYC
curl -b b.txt -H "X-CSRF-Token: $CSRF" -F doc_type=charter -F file=@charter.pdf $API/organizations/me/documents
curl -b b.txt -H "X-CSRF-Token: $CSRF" -X POST $API/organizations/me/kyc/submit

# 3. Админ проверяет карточку и AML-флаги, верифицирует
curl -b a.txt -H "X-CSRF-Token: $CSRF_A" -X POST $API/admin/organizations/$ORG_ID/kyc/approve

# 4. Заёмщик создаёт заявку и отправляет на модерацию; админ публикует
curl -b b.txt -H "X-CSRF-Token: $CSRF" -X POST $API/applications -H 'Content-Type: application/json' -d '{
  "amount": "500000.00", "currency": "EUR", "target_country_code": "DE",
  "purpose": "Пополнение оборотного капитала", "term_months": 12, "transfer_method": "swift"
}'
curl -b b.txt -H "X-CSRF-Token: $CSRF" -X POST $API/applications/$APP_ID/submit
curl -b a.txt -H "X-CSRF-Token: $CSRF_A" -X POST $API/admin/applications/$APP_ID/approve

# 5. Кредитор (не РФ, KYC verified) видит заявку на витрине и делает оффер
curl -b l.txt "$API/applications?page=1&page_size=20"
curl -b l.txt -H "X-CSRF-Token: $CSRF_L" -X POST $API/applications/$APP_ID/offers -H 'Content-Type: application/json' -d '{
  "amount": "500000.00", "interest_rate_annual": "8.00", "repayment_schedule": "annuity"
}'

# 6. Стороны договариваются в чате, заёмщик принимает оффер → сделка
curl -b b.txt -H "X-CSRF-Token: $CSRF" -X POST $API/offers/$OFFER_ID/messages -H 'Content-Type: application/json' -d '{"text":"Готовы подписать"}'
curl -b b.txt -H "X-CSRF-Token: $CSRF" -X POST $API/offers/$OFFER_ID/accept
curl -b b.txt $API/deals
```

Точные схемы тел и ответов — в спеке: `RegisterRequest`, `ApplicationCreate`, `OfferCreate` и др. в [`api/schemas/`](../api/schemas).

## Как устроен код

Три слоя (ADR-0001, п. 2), каждый знает только о соседнем снизу:

```
cmd/server/main.go            сборка зависимостей вручную, http.Server, graceful shutdown
internal/
  http/                       СЛОЙ 1 — транспорт
    middleware/               RequestLog, Recover, SecurityHeaders, MaxBody, WithRequestMeta,
                              Authenticate (сессия → principal), Authorize (x-roles, x-kyc-required),
                              CSRF
    handler/                  пишем руками: *Handler реализует контракт
      <tag>.go                операции тега: auth.go, offers.go, … (заглушки → 501)
      router.go               цепочка middleware + роутинг всех тегов
      errors.go               apperr.Code → HTTP-статус: одно место на весь API
      validate.go             валидация запроса по спеке → VALIDATION_ERROR с полями
    api/                      *.gen.go ГЕНЕРИРУЮТСЯ (make gen), не редактировать
      models.gen.go           модели контракта — один пакет на весь контракт
      <tag>api/server.gen.go  сервер одного тега: роутинг, разбор параметров, интерфейс
      <tag>api/gen.go         руками: doc пакета + go:generate тега
      <tag>api/register.go    руками: Register(mux, …) — вызывается из routes()
  service/                    СЛОЙ 2 — бизнес-логика, статусные машины, транзакции; модульные тесты
  repository/                 СЛОЙ 3 — данные
    sqlc/                     ГЕНЕРИРУЕТСЯ из queries/*.sql (sqlc)
    pgerr/                    ошибки Postgres → apperr
  apperr/                     доменные ошибки (код + сообщение), без зависимостей
  aml/, registry/             AML-скрининг и реестры — моки в MVP
  openapi/                    спека через go:embed — для валидатора
migrations/                   goose
queries/                      SQL для sqlc
```

При работе над фичей руками пишут в четырёх местах: контракт `api/paths/*.yaml` + `api/schemas/*.yaml`, `handler/<tag>.go`, `service/`, `queries/*.sql` (плюс `migrations/`, если меняется схема БД). Каркас — `middleware/`, `router.go`, `errors.go`, `validate.go`, а в `internal/http/api/` — `doc.go`, `gencfg_test.go` и `<tag>api/{gen,register}.go` — тоже рукописный, но меняется только при добавлении тега. Всё остальное, что занимает больше всего строк (`*.gen.go`, `repository/sqlc/`, бандлы `openapi.yaml`), — выход генераторов: его не читают целиком, как не читают скомпилированный бинарник. Из `<tag>api/server.gen.go` нужно три вещи:

- `StrictServerInterface` — список методов тега, которые реализует `*Handler` (поиск по `type StrictServerInterface`);
- `<Op>RequestObject` — что пришло: path- и query-параметры, тело (`Body`);
- `<Op>ResponseObject` и его реализации `<Op><статус>JSONResponse` — какие ответы разрешены спекой. Чужой статус не скомпилируется.

К ним ведёт F12 из `handler/<tag>.go`. Модели (`Offer`, `Error`, enum'ы) — в `internal/http/api/models.gen.go`.

Путь запроса `GET /api/v1/dictionaries/countries`: `RequestLog → Recover → SecurityHeaders → MaxBody → WithRequestMeta → Authenticate → Authorize → CSRF → валидатор по спеке → роутинг dictionariesapi → Handler.ListCountries → service.Dictionaries → sqlc → Postgres`. Ошибка с любого уровня возвращается как значение и превращается в ответ в `errors.go`.

Почему модели — один пакет, а сервер — пакет на тег: схемы ссылаются друг на друга по кругу (`organizations` ↔ `auth` ↔ `aml`), а Go запрещает циклы импортов. Операции же независимы, поэтому каждый тег — свой пакет со своим интерфейсом на 1–13 методов ([ADR-0003](../docs/adr/0003-server-package-per-tag.md)). sqlc устроен так же: запросы по доменам, `models.go` общий.

### Как добавить операцию

Пошаговая инструкция — в комментарии пакета [`internal/http/handler/handler.go`](internal/http/handler/handler.go). Образец — справочники: [`handler/dictionaries.go`](internal/http/handler/dictionaries.go) + [`dictionaries_test.go`](internal/http/handler/dictionaries_test.go) + [`service/dictionaries.go`](internal/service/dictionaries.go). Коротко:

1. Изменить контракт (если нужно) в `api/paths/<tag>.yaml` / `api/schemas/*.yaml` → `make gen`. У новой операции указать `x-roles` (и `x-kyc-required`, если нужен KYC) и ответ `403` — без них не пройдут тесты.
2. SQL — в `queries/<домен>.sql` → `make gen`.
3. Логика — в `internal/service`, с табличными тестами. Principal — через `currentPrincipal(ctx)`; роль и KYC повторить в сервисе (`p.RequireRole(...)`, `p.RequireKYCVerified()`); сущность читать через `*Visible` с `p.ViewerRole()`, `p.ViewerOrg()`.
4. В `handler/<tag>.go` заменить тело заглушки: вызвать сервис, перевести типы. Ошибки — `return nil, apperr.X(...)`, статус выберет `errors.go`.

### Тесты

```sh
make test       # юнит-тесты, БД не нужна
make test-all   # + интеграционные с Postgres (сначала make db)
```

Тесты хендлеров поднимают настоящий роутер через `httptest` (путь, статус, JSON на проводе, формат ошибок), а сервис подменяют фейком. Инварианты контракта проверяются отдельно: у каждой операции спеки ровно один тег и каждая зарегистрирована в роутере (`handler/routes_test.go`); сгенерированный код совпадает со спекой (CI `gen-check`); контракт без ломающих изменений (CI `api-breaking`, oasdiff).
