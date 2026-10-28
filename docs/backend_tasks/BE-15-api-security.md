# BE-15 · Безопасность API

Issue: #31 · Milestone: M4 · Зависит от: BE-03 (PR #67), BE-04; IDOR-тесты — BE-05, BE-07, BE-08, BE-09, BE-10, BE-11 · Блокирует: — (критерий оценки «Безопасность: XSS, CSRF, защита данных»)

## Цель
Каждый чекбокс issue закрыт кодом и тестом. API-ответы несут защитные заголовки (HSTS — при HTTPS), CORS открыт только origin'у фронта, тело и время запроса ограничены. Чужой id любого ресурса → 404, POST без CSRF-заголовка → 403. Пароли и токены не появляются в логах даже на уровне `debug`.

## Что уже есть
Сверка чекбоксов issue с PR #67 (`git diff origin/main...origin/be-03-auth`) и `origin/main`:

| Чекбокс issue | Где сделано | Состояние |
|---|---|---|
| CSP без `unsafe-inline` | PR #67, `middleware/headers.go: SecurityHeaders` — `default-src 'none'; frame-ancestors 'none'` | ✅ для ответов API; SPA — см. «Открытые вопросы» |
| `X-Content-Type-Options: nosniff` | там же | ✅ |
| `Referrer-Policy` | там же, `no-referrer` | ✅ |
| `frame-ancestors 'none'` | там же + `X-Frame-Options: DENY` | ✅ |
| HSTS при HTTPS | — | ❌ шаг 2 |
| CORS только для origin фронта, с `credentials` | — | ❌ шаг 3 |
| Лимит размера тела | `origin/main`: `middleware.MaxBody(bodyLimit)` (`middleware/middleware.go`, `handler/validate.go`: 1 МБ JSON, 10 МБ + 64 КБ upload), подключён в `handler/router.go` | ✅; ⚠️ `LimitBody` из PR #67 — дубль, не подключён |
| Таймауты сервера | `backend/cmd/server/main.go`: `ReadHeaderTimeout 5s`, `ReadTimeout 15s`, `WriteTimeout 30s`, `IdleTimeout 60s` | ✅ (совпадает с README «Лимиты») |
| IDOR: чужой id → 404 по всем ресурсам | — | ❌ шаг 5, после BE-05/07–11 |
| CSRF: POST без заголовка → 403 | PR #67: `middleware/csrf.go` (double-submit, `subtle.ConstantTimeCompare`), `csrf_test.go: TestCSRFMiddleware`, `handler/auth_test.go: TestRouterAuthAndCSRFOrder` (logout с сессией без заголовка → 403) | 🟡 тест не проверяет `code`; шаг 4 |
| Секреты только из env | `internal/config/config.go` читает только env, `.env` в `.gitignore` | ✅ |
| Пароли и токены не в логах | — | ❌ утечка, шаг 6 |

Прочее:
- PR #67 собран на старом `main` с одним пакетом `api` (`api.NewStrictHandlerWithOptions`). В `origin/main` сервер уже разбит по тегам (ADR-0003). После rebase в `handler/router.go` из `main` нужно вставить:
  ```go
  SecurityHeaders(), WithRequestMeta(), Authenticate(...), CSRF()
  ```
  Тесты ниже проверяют итоговую цепочку, а не PR как есть.
- ⚠️ **Утечка пароля в лог.** `handler/validate.go: specError` кладёт исходную ошибку kin-openapi в `apperr.Error.Err` («полный текст — только в лог»), а `handler.WriteError` логирует её полем `err` (уровень `Debug` для 4xx). У kin-openapi v0.142.0 `SchemaError.Error()` при `openapi3.SchemaErrorDetailsDisabled == false` (по умолчанию) печатает `Value:` — значение поля. `POST /auth/register` с паролем короче 10 символов (`RegisterRequest.password.minLength: 10`) при `LOG_LEVEL=debug` пишет пароль в лог.
- `middleware.CSRF` в PR #67 пропускает публичные `POST /auth/register` и `/auth/login`: cookie `csrf_token` до входа ещё нет. Так и решено (DECISIONS п. 1); формулировку исключения в AGENTS.md, `backend/README.md` и `info.description` контракта вносит BE-03, шаг 5. Login CSRF закрывает другое: валидатор требует `Content-Type: application/json` (кросс-сайтовая форма шлёт `text/plain` или `form-urlencoded` → 400), а JSON с другого origin требует CORS-preflight.
- ⚠️ `service.MemoryRateLimiter` (PR #67, `service/ratelimit.go`) и `Retry-After` в `handler/errors.go` есть, но лимитер **нигде не вызывается**: `Login`/`Register` его не используют. Это чекбокс BE-03, не этой задачи: его подключает BE-03, шаг 3 — убедиться, что он закрыт до мержа PR #67.
- Cookie (PR #67, `handler/auth.go: sessionCookies`): `session` — `HttpOnly; SameSite=Lax; Path=/; Secure` по `COOKIE_SECURE` (по умолчанию `true`); `csrf_token` — без `HttpOnly`.
- Защита на уровне БД (роли `lp_app`/`lp_migrator`, `audit_log` только `INSERT`, `statement_timeout`) — отдельный план [DB-SEC-db-roles.md](DB-SEC-db-roles.md).

## Решения
- **Основной путь — фронт и API на одном origin** (Vite `server.proxy` в dev: `/api` → `localhost:8080`, `frontend/vite.config.ts` в ветках `frontend-setup` и `feat/frontend-applications-kyc`; reverse-proxy в проде). CORS в dev не нужен (DECISIONS п. 5). Тогда CORS не нужен, а double-submit работает: SPA читает `csrf_token` из `document.cookie` только если cookie выставлена для того же хоста. CORS-middleware — запасной путь, выключенный по умолчанию (`CORS_ALLOWED_ORIGINS` пуст → заголовков нет).
- **CORS — точное совпадение `Origin` со списком**, `Access-Control-Allow-Credentials: true`, `Vary: Origin`. `*` запрещён: с `credentials` браузер его не примет, а отражение любого `Origin` открывает API любому сайту.
- **HSTS — только если запрос пришёл по HTTPS** (`r.TLS != nil` или `X-Forwarded-Proto: https`). `max-age=31536000; includeSubDomains`, без `preload`. По RFC 6797 §8.1 браузер игнорирует HSTS по HTTP, так что подделанный `X-Forwarded-Proto` вреда не делает.
- **`LimitBody` из PR #67 удалить** при rebase: `MaxBody` из `main` уже делает то же с разными лимитами для upload.
- **Утечку значений в ошибках валидатора закрыть глобально:** `openapi3.SchemaErrorDetailsDisabled = true` в `init()` пакета `handler`. Причина ошибки (путь и правило) остаётся, значение — нет. `details` в ответе от этого не меняются: их строит `fieldErrors`.
- **CSP для SPA ставит INF-01** (DECISIONS п. 43): заголовки API не защищают HTML фронта, а критерий XSS оценивают по SPA. Frontend-контейнер (nginx) отдаёт prod-сборку с заголовком `default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'`. Production-сборка Vite обходится без inline-скриптов, dev-сервер — нет, поэтому dev без CSP. Эту строку вписать в issue INF-01 (шаг 7).
- **`MaxHeaderBytes` — дефолт Go (1 МБ)** (DECISIONS п. 44): заголовки ограничены по времени `ReadHeaderTimeout`, выигрыш от 64 КБ мал.
- **IDOR-тесты — интеграционные**, через настоящий роутер, настоящие сервисы и Postgres: фейковый сервис не проверит `WHERE` в SQL. HTTP-тесты IDOR по всем ресурсам — здесь; BE-04 проверяет видимость только тестом запросов (DECISIONS п. 3).
- **IP клиента — `r.RemoteAddr`** (DECISIONS п. 4). `X-Forwarded-For` не читаем: без списка доверенных прокси его подделывает клиент. `TRUSTED_PROXIES` — отдельной задачей, когда появится прод-развёртывание.

## Шаги

### 1. Сверка после rebase PR #67
Убедиться, что в `handler/router.go` цепочка такая (BE-14 добавит `RequestID` первым):
```
RequestLog → Recover → SecurityHeaders → CORS (шаг 3) → MaxBody → WithRequestMeta → Authenticate → Authorize → CSRF → validateRequests
```
`LimitBody` и его тест удалены из `middleware/headers.go`.

**Готово:** `grep -rn "LimitBody" backend` пусто; тест `TestSecurityHeadersOnEveryResponse` (`handler`, через `NewRouter`) — для `GET /api/v1/healthz` (200), `GET /api/v1/nope` (404) и `POST /api/v1/auth/logout` без сессии (401) в ответе есть `Content-Security-Policy`, `X-Content-Type-Options: nosniff`, `Referrer-Policy`, `X-Frame-Options`.

### 2. HSTS
`middleware/headers.go`: в `SecurityHeaders` — `if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" { w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains") }`. Константа с именем, не магическое число.

**Готово:** `TestSecurityHeadersHSTS`: обычный `httptest.NewRequest` → заголовка нет; `X-Forwarded-Proto: https` → есть; `req.TLS = &tls.ConnectionState{}` → есть.

### 3. CORS (выключен по умолчанию)
1. `internal/config/config.go`: `CORSAllowedOrigins []string` из `CORS_ALLOWED_ORIGINS` (через запятую, пусто — выключено). Каждое значение — `scheme://host[:port]` без пути, иначе ошибка конфига. Добавить в `.env.example` с комментарием и в `README.md` («Запуск бэкенда»).
2. `middleware/cors.go: CORS(allowed []string)`:
   - нет `Origin` или он не в списке → запрос дальше без CORS-заголовков;
   - разрешён → `Access-Control-Allow-Origin: <origin>`, `Access-Control-Allow-Credentials: true`, `Access-Control-Expose-Headers: Retry-After, X-Request-ID`, `Vary: Origin`;
   - preflight (`OPTIONS` + `Access-Control-Request-Method`) от разрешённого origin → 204 с `Access-Control-Allow-Methods: GET, POST, PATCH, DELETE`, `Access-Control-Allow-Headers: Content-Type, X-CSRF-Token, X-Request-ID`, `Access-Control-Max-Age: 600`, **без** вызова `next`. Иначе `Authenticate` ответит 401, а валидатор — 404: `OPTIONS` в спеке нет.
3. Подключить в `router.go` после `SecurityHeaders`; `cfg.CORSAllowedOrigins` передать через `handler.Deps`.

**Готово:** `TestCORS` (табличный, через `NewRouter`):
- `Origin: http://localhost:5173` при списке с ним → `Access-Control-Allow-Origin` = он и `Allow-Credentials: true`;
- `Origin: https://evil.example` → заголовков нет;
- preflight от разрешённого → 204 и `Allow-Headers` содержит `X-CSRF-Token`;
- пустой список → заголовков нет ни у кого.

### 4. Тест CSRF на бизнес-ручке
`handler/csrf_router_test.go`, через `NewRouter` с фейковым `Authenticator` (сессия валидна):
- `POST /api/v1/notifications/read-all` без `X-CSRF-Token` → 403, `error.code = CSRF_INVALID`, фейковый сервис не вызван;
- с заголовком, не равным cookie → 403;
- с совпадающим → не 403;
- `GET /api/v1/notifications` без заголовка → не 403.

Отдельно — `POST /api/v1/auth/login` с `Content-Type: text/plain` и валидным JSON в теле → 400 `VALIDATION_ERROR`, сервис не вызван. Это проверка того, что публичные POST без CSRF не принимают кросс-сайтовую форму.

**Готово:** тесты зелёные. В `backend/README.md` («Аутентификация», п. 2) добавлено: «кроме `POST /auth/register` и `/auth/login` — у них ещё нет cookie; их защищает обязательный `Content-Type: application/json`». То же — в AGENTS.md («Безопасность → CSRF»).

### 5. IDOR по всем ресурсам (после BE-05, BE-07…BE-11)
1. Вынести сборку зависимостей из `run()` в `backend/cmd/server/main.go` в `func newRouter(pool *pgxpool.Pool, cfg config.Config) http.Handler`, чтобы тест собирал тот же граф, что и прод.
2. `backend/cmd/server/idor_integration_test.go` (`package main`, пропуск по образцу `newPool` из `service/dictionaries_integration_test.go`). Фикстуры:
   - через `POST /auth/register` — две компании-заёмщика (A, B) и два кредитора (L1, L2) со случайными email и рег. номерами;
   - `UPDATE organizations SET kyc_status='verified', verified_at=now()` через пул;
   - A создаёт заявку → её публикует SQL; L1 делает оффер; A пишет в чат, принимает → сделка; A загружает документ.
3. Табличный тест: «владелец» и «чужой» — пользователь той же роли из другой организации. Ожидание — 404 `NOT_FOUND`, а для мутаций ещё и неизменное состояние в БД:

| Ресурс | Запрос чужого | Чужой |
|---|---|---|
| заявка | `GET /applications/{id}`, `PATCH /applications/{id}`, `POST …/submit`, `…/cancel`, `…/reject-offers`, `GET …/history` | B |
| офферы заявки | `GET /applications/{id}/offers` | B |
| оффер | `GET /offers/{id}`, `POST …/accept`, `…/reject` | B |
| оффер | `POST /offers/{id}/withdraw` | L2 |
| сделка | `GET /deals/{id}` | B и L2 |
| чат | `GET /offers/{id}/messages`, `POST /offers/{id}/messages` | B и L2 |
| документ | `GET /documents/{id}/file`, `DELETE /organizations/me/documents/{id}` | B |
| уведомление | `POST /notifications/{id}/read` | B |

   Плюс несуществующий UUID для каждой строки → тоже 404, тот же `code` и `message`, что для чужого: ответы неразличимы.

**Готово:** `TestIDOR` зелёный в `make test-all`. Новая ручка с `{…Id}` в пути без строки в таблице ловится проверкой в тесте: список путей с параметром-id из `openapi.MustDoc()` сравнивается с покрытыми.

### 6. Пароли и токены не попадают в логи
1. `handler/validate.go` (или `handler.go`): `func init() { openapi3.SchemaErrorDetailsDisabled = true }` с комментарием, почему: `SchemaError.Error()` печатает значение поля, а `WriteError` логирует `err`.
2. Логи `Auth.audit` и `Authenticate` в PR #67 пишут только `action`/`err` — проверено чтением; в `meta` аудита паролей нет.
3. Тест `backend/internal/http/handler/log_secrets_test.go`:
   - логгер `slog.NewTextHandler(&buf, Level: Debug)` через `slog.SetDefault`, восстановление в `t.Cleanup`; роутер с фейковым `AuthService`, который возвращает фиксированные `token`/`csrfToken`;
   - сценарии: `register` с паролем `"short-pw1"` (400), `register` с валидным паролем (201), `login` с неверным паролем (401), `login` успешный, `logout` с cookie `session=<token>`;
   - для каждого — `buf` не содержит ни пароля, ни `token`, ни `csrfToken`.
   - Без шага 6.1 первый сценарий падает — проверить это один раз вручную, откатив строку.

**Готово:** `TestLogsDoNotContainSecrets` зелёный.

### 7. Документация
- `backend/README.md`: раздел «Безопасность» — заголовки ответа, HSTS при HTTPS, CORS (`CORS_ALLOWED_ORIGINS`, по умолчанию выключен), исключение CSRF для `register`/`login`.
- Ссылка на `docs/backend_tasks/DB-SEC-db-roles.md` — для прав в БД.
- Комментарий в issue INF-01 (`scripts/issues.yaml`, ключ `INF-01`, и сам issue на GitHub): чекбокс «nginx frontend-контейнера отдаёт CSP» со строкой заголовка из «Решений».

**Готово:** раздел есть, переменная — в `.env.example` и `README.md`.

## Тесты
| Случай | Ожидание |
|---|---|
| любой ответ API (200, 401, 404) | `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` |
| запрос с `X-Forwarded-Proto: https` | `Strict-Transport-Security: max-age=31536000; includeSubDomains` |
| запрос по HTTP | нет `Strict-Transport-Security` |
| `Origin` из `CORS_ALLOWED_ORIGINS` | `Access-Control-Allow-Origin` = origin, `Allow-Credentials: true`, `Vary: Origin` |
| чужой `Origin` | нет `Access-Control-Allow-Origin` |
| `OPTIONS` preflight от разрешённого origin | 204 без cookie, не 401/404 |
| JSON-тело 1 МБ + 1 байт | 400 `VALIDATION_ERROR`, `field: body` |
| медленный клиент шлёт заголовки дольше 5 с | соединение закрыто сервером (`ReadHeaderTimeout`) — проверяется чтением `main.go`, без теста |
| `POST /notifications/read-all` с сессией без `X-CSRF-Token` | 403 `CSRF_INVALID` |
| `POST /auth/login`, `Content-Type: text/plain` | 400 `VALIDATION_ERROR` |
| чужой id по любой строке таблицы шага 5 | 404 `NOT_FOUND`, ответ идентичен несуществующему id |
| `register` с паролем 9 символов при `LOG_LEVEL=debug` | в логе нет пароля |

## Готово, когда
- [ ] На всех ответах API есть CSP без `unsafe-inline`, `nosniff`, `Referrer-Policy`, `frame-ancestors 'none'`; HSTS — только при HTTPS (тесты шагов 1–2).
- [ ] CORS отвечает только origin'ам из `CORS_ALLOWED_ORIGINS` с `credentials`; по умолчанию выключен (`TestCORS`).
- [ ] Тело > 1 МБ → 400, загрузка документа > 10 МБ → 413 `FILE_TOO_LARGE` (BE-05); таймауты сервера заданы в `main.go` и совпадают с README «Лимиты».
- [ ] `TestIDOR`: чужой id → 404 для заявки, оффера, сделки, чата, документа (и уведомления).
- [ ] POST без `X-CSRF-Token` → 403 `CSRF_INVALID` на бизнес-ручке.
- [ ] Секреты только из env; `TestLogsDoNotContainSecrets` доказывает, что пароль и токены не попадают в лог даже на `debug`.
- [ ] И общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: закрыты в [DECISIONS.md](DECISIONS.md) — исключение CSRF (п. 1), IDOR-тесты (п. 3), IP за прокси (п. 4), CORS в dev (п. 5), CSP для SPA (п. 43), `MaxHeaderBytes` (п. 44).
