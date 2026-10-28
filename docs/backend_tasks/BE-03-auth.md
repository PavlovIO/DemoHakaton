# BE-03 · Аутентификация: регистрация, вход, сессии, CSRF

Issue: #19 · Milestone: M1 · Зависит от: — · Блокирует: BE-04, BE-14 (аудит `auth.*`), BE-15

Реализация — PR #67 (ветка `origin/be-03-auth`, автор sergey-cherepanov1). Этот план описывает, что в PR уже сделано, что осталось и что нужно, чтобы его смержить. Смотреть дифф: `git diff origin/main...origin/be-03-auth`.

## Цель

Работают `POST /auth/register` (201 + cookie), `POST /auth/login` (200 + cookie), `POST /auth/logout` (204, cookie стёрты) и `GET /auth/me`. Запрос с валидной cookie `session` получает `service.Principal` в контексте. Мутирующий запрос без `X-CSRF-Token` → `403 CSRF_INVALID`. Подбор пароля упирается в `429 RATE_LIMITED` с `Retry-After`. PR #67 смержен в `main`.

## Что уже есть

PR #67 основан на `cdee052` (be-02). `main` с тех пор получил `refactor/api-split-by-tag` (#64: сервер по пакету на тег, ADR-0003) и docs-коммиты.

| Файл в PR | Что делает |
|---|---|
| `internal/service/auth.go` | `Auth`: `Register`, `Login`, `Logout`, `Me`, `Authenticate`; зависимости — интерфейсы `AuthStore` и `AuthTx` (образец для остальных сервисов, см. README) |
| `internal/service/principal.go` | `service.Principal{UserID uuid.UUID, OrgID *uuid.UUID, Role string}` |
| `internal/service/ratelimit.go` | `RateLimiter` (`Check`/`Hit`/`Reset`) и `MemoryRateLimiter` (скользящее окно в памяти) |
| `internal/http/middleware/auth.go` | `Authenticate(a Authenticator)`, `PrincipalFromContext`, `SessionTokenFromContext`, захардкоженная карта `publicRoutes`, `writeMiddlewareError` |
| `internal/http/middleware/csrf.go` | `CSRF()`: double-submit, `subtle.ConstantTimeCompare` |
| `internal/http/middleware/headers.go` | `SecurityHeaders()` (nosniff, `X-Frame-Options`, `Referrer-Policy`, CSP, `Cache-Control: no-store`) и неиспользуемый `LimitBody` |
| `internal/http/middleware/request_meta.go` | `WithRequestMeta()` → `service.ClientMeta{IP, UserAgent}` в контексте |
| `internal/http/handler/auth.go` | хендлеры и свои `Visit*Response` со `Set-Cookie` |
| `internal/apperr/apperr.go`, `handler/errors.go` | `apperr.RateLimitedAfter(d)` и заголовок `Retry-After` в `WriteError` |
| `internal/config/config.go`, `.env.example` | `SESSION_TTL` (по умолчанию `24h`), `COOKIE_SECURE` (по умолчанию `true`, в `.env.example` — `false`) |

### Чекбоксы issue → состояние PR

| Чекбокс #19 | Состояние | Где / что не так |
|---|---|---|
| Регистрация в одной транзакции; страна по роли; `admin` недоступна | ✅ | `Auth.Register`: `CreateOrganization` + `CreateUser` + `CreateSession` внутри `tx.InTx`. Страна проверяется в сервисе (`borrower` → `RU`, `lender` → не `RU`). `admin` отбивает уже валидатор (`enum: [borrower, lender]` в `api/schemas/auth.yaml`) и ещё раз сервис. ⚠️ Текст конфликта теряется, см. ревью №6 |
| Пароли argon2id, минимум 10 | ✅ | сделано в BE-02 (PR #57, `service/password.go`) |
| Сессия: 32 байта, cookie `HttpOnly; Secure; SameSite=Lax; Path=/`, в БД `sha256`; TTL + `last_seen_at` | ✅ | `randomToken`, `hashToken`, `sessionCookies` в `handler/auth.go`. `Secure` берётся из `COOKIE_SECURE`. TTL абсолютный (`expires_at` при создании не продлевается); `TouchSession` на каждый запрос |
| CSRF: cookie `csrf_token` (не HttpOnly) + `X-CSRF-Token` на не-GET → 403 | ✅ с оговоркой | `middleware/csrf.go`. Пропускает `publicRoutes` (`register`, `login`) — так и решено (DECISIONS п. 1), но `AGENTS.md` и контракт пока требуют «без исключений»: правка документов — шаг 5 |
| Rate limit `login`/`register` (IP и email) → 429 + `Retry-After` | ❌ | лимитер и `Retry-After` написаны, но **нигде не вызываются**: в `Auth` нет поля лимитера (ревью №1) |
| Заблокированный → 403 при входе; одинаковое сообщение для «нет email» и «неверный пароль» | ✅ | `Login`: блокировка проверяется после верного пароля → `403 «Аккаунт заблокирован»`; для неизвестного email сверяется фиктивный хэш (`fakePassword`), чтобы время ответа было тем же, сообщение одно — «Неверный email или пароль» |
| `auth.login`, `auth.login_failed` в `audit_log` | ✅ | `Auth.audit`, `meta.reason` = `unknown_email` / `bad_password` / `blocked`. Запись best-effort: ошибка уходит в лог и не ломает вход |
| Тесты: хэш пароля, CSRF-middleware, истечение сессии | ⚠️ | `password_test.go` (BE-02) и `middleware/csrf_test.go` есть. **Теста истечения нет**: истечение проверяет SQL (`expires_at > now()` в `GetSessionByTokenHash`), а `TestAuthAuthenticateAndLogout` работает на фейковом store |

### Стык с BE-04: ответы на вопросы из п. 1.1 `mds/plan.md`

Это факты из кода PR, на них опирается BE-04:

| Вопрос | Ответ в PR #67 |
|---|---|
| Формат токена | 32 байта из `crypto/rand` → `base64.RawURLEncoding` (43 символа без `=`). CSRF-токен генерируется так же, отдельно, в БД не хранится и с сессией не связан |
| Что хэшируется в `token_hash` | `sha256` **строки из cookie** (`[]byte(token)` от base64url-текста), а не сырых 32 байт (`service.hashToken`). Длина хэша 32 байта и проходит `CHECK (length(token_hash) = 32)` |
| Кто продлевает сессию | `service.Auth.Authenticate` после `GetSessionByTokenHash` вызывает `TouchSession` (`last_seen_at = now()`) на каждый запрос с сессией; ошибка Touch только логируется. `expires_at` не продлевается никем |
| Где живёт текущий пользователь | `service.Principal` в `internal/service/principal.go` (не `internal/auth`, как предлагал plan.md); в контекст кладёт `middleware.Authenticate`, читает `middleware.PrincipalFromContext(ctx) (service.Principal, bool)`. Для `logout` есть ещё `middleware.SessionTokenFromContext`. Зависимость middleware — интерфейс `middleware.Authenticator` рядом с ним |
| Порядок CSRF | Цепочка в `handler/router.go` PR: `Recover → RequestLog → SecurityHeaders → MaxBody → WithRequestMeta → Authenticate → CSRF → validateRequests`. CSRF стоит после `Authenticate`; публичные маршруты (`publicRoutes`) и GET/HEAD/OPTIONS он пропускает. Так как `Authenticate` уже отбил защищённые запросы без сессии, CSRF фактически проверяется только у запросов с валидной сессией |

### Ревью PR (комментарий владельца репо в #67) — что из этого блокирует мерж

| № | Замечание | Блокирует мерж | Кто |
|---|---|---|---|
| 1 | Rate limiter не вызывается | да (чекбокс issue, контракт 429) | BE-03 |
| 2 | `Recover` вынесен наружу `RequestLog`: 500 после паники не попадает в access log | да | BE-03 |
| 3 | `HEAD` к публичным маршрутам → 401 (`isPublic` знает только GET/POST) | да | BE-03 |
| 4 | `publicRoutes` захардкожен; несуществующий путь → 401 вместо 404 | нет | BE-04 заменяет на `security: []` из спеки |
| 5 | `Authenticator` получается type assertion'ом от `h.auth` | да | BE-03 |
| 6 | `mapConflict` передаёт текст не в тот параметр `pgerr.Map` → всегда «Запись уже существует» | да | BE-03 |
| 7 | `Login` не транзакционный: сессия и `last_login_at` остаются при ошибке после них | да | BE-03 |
| 8 | Лимитер: `Hit` перебирает все ключи под мьютексом при > 10 000 ключей; `Check` и `Hit` не атомарны | частично: атомарность — да, перебор — нет | BE-03 |
| 9 | `writeMiddlewareError` дублирует `statusByCode` и не ставит `Retry-After` | да | BE-03 |
| 10 | Каждый запрос = SELECT + UPDATE сессии, `GetMe` ещё раз читает пользователя | нет | после мержа, по желанию |

⚠️ Тесты middleware в PR ходят по путям, которых нет в спеке (`/api/v1/users/me`, `/api/v1/transfers`). Пока middleware не смотрит в спеку, это работает, но BE-04 эти тесты перепишет.

## Решения

- **План строится поверх PR #67, а не заново.** Автор PR доводит его по таблице ревью; спековый список публичных операций, `KycStatus` в `Principal` и «мягкий» `Authenticate` — это BE-04, в этом PR их не делать.
- **Захардкоженный `publicRoutes` до BE-04 остаётся**, исправляется только `HEAD` (ревью №3): так PR меньше расходится с BE-04, который этот код всё равно удалит.
- **Rate limit проверяется до argon2id.** Иначе поток запросов занимает 4 слота хэширования (`service/password.go`) и ставит в очередь логины настоящих пользователей (ревью №1).
- **Ошибки middleware пишутся через `handler.WriteError`, переданный параметром**, как `onPanic` у `Recover`: middleware не может импортировать `handler` (цикл импортов), а второй маппинг код → статус разойдётся с `errors.go` (ревью №9).
- **Отсутствующий `Authenticator` — паника на старте, а не тихий 401 на все ручки** (ревью №5).
- **CSRF не проверяется у операций с `security: []`** (`register`, `login`) (DECISIONS п. 1): до входа у фронта ещё нет cookie `csrf_token`. Login-CSRF закрывает обязательный `Content-Type: application/json`: HTML-форма с другого сайта такое тело не отправит, а валидатор отбивает другой `Content-Type` с `400`. Отдельную операцию `GET /auth/csrf` не вводим.
- **`'403'` у `logout`** (его отдаёт CSRF) в контракт добавляет BE-04 — одним коммитом для всех защищённых операций (DECISIONS п. 2). Здесь контракт не трогаем.
- **IP клиента — из `r.RemoteAddr`** (DECISIONS п. 4): прокси в MVP нет, а `X-Forwarded-For` подделывает клиент. Доверенный `X-Forwarded-For` с `TRUSTED_PROXIES` — когда появится прод.

## Шаги

### 1. Rebase на `origin/main`

`git merge-tree --write-tree origin/main origin/be-03-auth` показывает 4 конфликта:

| Файл | Конфликт | Как разрешить |
|---|---|---|
| `handler/auth.go` | add/add: в `main` заглушки с типами `authapi.*`, в PR — реализация с `api.*` | Взять реализацию PR и заменить `api.RegisterRequestObject` → `authapi.RegisterRequestObject`, `api.GetMe200JSONResponse` → `authapi.GetMe200JSONResponse` и т. д. для всех `*RequestObject` / `*ResponseObject`. Модели (`api.Me`, `api.User`, `api.Organization`) остаются в пакете `api`. Свои `authRegisterResponse` / `authLoginResponse` / `authLogoutResponse` реализуют интерфейсы из `authapi` (`VisitRegisterResponse` и т. д.) |
| `handler/router.go` | в PR `NewRouter` собран на `api.NewStrictHandlerWithOptions`, в `main` — `routes(h)` с `<tag>api.Register` | Оставить `routes(h)` из `main`, из PR взять только цепочку middleware (с исправлениями из шага 2) |
| `handler/errors.go` | `main` переписал `paramName` (`43e05c8`), PR добавил `Retry-After` | Оставить код `main`, добавить блок `Retry-After` и `retryAfterSeconds` из PR |
| `handler/unimplemented.go` | удалён в `main` (заглушки разъехались по `<tag>.go`), изменён в PR | `git rm`: auth-заглушки уже заменены реализацией в `handler/auth.go` |

`handler/handler.go`, `dictionaries_test.go`, `errors_test.go`, `validate_test.go` сливаются без конфликтов, но могут не собраться: в `handler.go` должен остаться блок `var _ <tag>api.StrictServerInterface` из `main`, без `var _ api.StrictServerInterface` из PR.

**Готово:** ветка перебазирована на `origin/main`; `go -C backend build ./... && go -C backend vet ./...` проходят; `make gen-check` без диффа; `TestEveryOperationIsRouted` зелёный.

### 2. Исправления из ревью

1. **№2:** в `NewRouter` верни порядок из `main`: `RequestLog` — самый внешний, `Recover` — внутри него. Итог: `RequestLog → Recover → SecurityHeaders → MaxBody → WithRequestMeta → Authenticate → CSRF → validateRequests`.
2. **№3:** `isPublic` для `HEAD` ищет путь в `publicRoutes[http.MethodGet]`.
3. **№5:** добавь `Authenticate` в интерфейс `handler.AuthService` и передавай `h.auth` в `middleware.Authenticate` напрямую. Удали `unavailableAuthenticator`. Если `Deps.Auth == nil`, `NewRouter` паникует с понятным текстом.
4. **№6:** перепиши `mapConflict`: при `23505` (`errors.As(err, &pgErr) && pgErr.Code == "23505"`) верни `apperr.Conflict(msg)` с текстом по ограничению (`pgErr.ConstraintName`): email → «Пользователь с таким email уже существует», рег. номер или LEI → «Организация с таким номером уже зарегистрирована». Остальное — через `pgerr.Map`.
5. **№7:** в `Login` выполни `UpdateLastLogin`, `CreateSession` и запись `auth.login` в одном `a.tx.InTx`: аудит бизнес-действия пишется в его транзакции, ошибка записи откатывает вход (правило BE-14). Best-effort вне транзакции — только `auth.login_failed`.
6. **№9:** `Authenticate(a, writeErr)` и `CSRF(writeErr)` принимают `writeErr func(http.ResponseWriter, *http.Request, error)`; `NewRouter` передаёт `WriteError`. Удали `writeMiddlewareError` и `middlewareError*`.
7. Удали неиспользуемый `middleware.LimitBody`: его роль уже выполняет `MaxBody`.

**Готово:** тесты из раздела «Тесты» для №2, №3, №6, №7 зелёные; `grep -rn "writeMiddlewareError\|LimitBody\|unavailableAuthenticator" backend/internal` пусто.

### 3. Rate limit на `login` и `register`

1. В `RateLimiter` добавь атомарный метод `Allow(key string, limit int, window time.Duration) (retryAfter time.Duration, ok bool)`: проверка и запись под одной блокировкой (ревью №8). Перебор всех ключей в `Hit` замени чисткой по таймеру или раз в N вызовов.
2. `service.NewAuth` принимает `RateLimiter` и лимиты (структура `AuthLimits` с константами по умолчанию):
   - `login`: 10 неудачных попыток на email за 15 минут и 50 попыток на IP за 15 минут;
   - `register`: 10 попыток на IP за час.

   Ключи: `login:email:<нормализованный email>`, `login:ip:<ip>`, `register:ip:<ip>`; если `meta.IP == nil` — `<ip>` = `unknown`.
3. Проверка лимита — **первым делом** в `Login` и `Register`, до `HashPassword` / `VerifyPassword`. Превышение → `apperr.RateLimitedAfter(retryAfter)`. Неудачный вход записывает событие в оба login-ключа; успешный вход сбрасывает email-ключ (`Reset`).
4. `cmd/server/main.go`: `service.NewAuth(queries, txManager, cfg.SessionTTL, service.NewMemoryRateLimiter(), service.DefaultAuthLimits)`.

**Готово:** `TestAuthLoginRateLimit` и `TestAuthRegisterRateLimit` зелёные; при `make run` 11-я подряд неудачная попытка входа на один email отвечает `429` с заголовком `Retry-After: <секунды>`.

### 4. Тест истечения сессии и блокировки

Интеграционный тест `TestAuthIntegrationSessionLifecycle` в `internal/service/auth_integration_test.go`. Переиспользуй `newPool` из `dictionaries_integration_test.go`. Пользователь — со случайным суффиксом email и рег. номера; не удаляется: у `lp_app` после DB-SEC нет `DELETE` на `users` (так же устроены фикстуры BE-07).

**Готово:** тест зелёный в `make test-all` и пропускается в `make test`.

### 5. Документация и мерж

1. `backend/README.md`: в «Статус реализации» строка ✅ с `register`, `login`, `logout`, `getMe`; удали фразу «Middleware для них — в работе вместе с операциями `auth`»; в «Как устроен код» в описании `middleware/` перечисли новые middleware.
2. Корневой `README.md` («Запуск бэкенда»): `SESSION_TTL`, `COOKIE_SECURE` (`false` нужен для `http://localhost`, иначе браузер не сохранит cookie).
3. Исключение CSRF для `security: []` (DECISIONS п. 1) — одной и той же формулировкой в трёх местах: `api/openapi.root.yaml` (`info.description`, раздел про CSRF), `backend/README.md` («Аутентификация»), `AGENTS.md` («Безопасность → CSRF»: «все запросы, кроме GET/HEAD/OPTIONS и публичных операций с `security: []` (`register`, `login`), обязаны слать `X-CSRF-Token`…»). Формулировка: «операции с `security: []` CSRF не проверяют — до входа cookie `csrf_token` нет; от login-CSRF защищает обязательный `Content-Type: application/json`».
4. В PR отметь, что `publicRoutes` и `Principal` без `KycStatus` — временные, их переделывает BE-04.

**Готово:** `grep -n "security: \[\]" AGENTS.md backend/README.md api/openapi.root.yaml` находит исключение во всех трёх файлах; чекбоксы #19 отмечены; PR #67 смержен; `git diff origin/main...origin/be-03-auth` пуст.

## Тесты

Сверх тех, что уже есть в PR (`TestAuthRegister`, `TestAuthLogin`, `TestAuthLoginAudit`, `TestAuthAuthenticateAndLogout`, `TestAuthMe`, `TestRegisterAndLoginSetSessionCookies`, `TestAuthErrorsAreMapped`, `TestLogoutClearsCookies`, `TestGetMeRequiresPrincipalAndMapsAdmin`, `TestRouterAuthAndCSRFOrder`, `TestCSRFMiddleware`, `TestAuthenticateMiddleware`, `TestMemoryRateLimiter*`):

- **№2:** хендлер паникует → в логе есть строка `http request … status=500` (тест с `slog`-обработчиком в буфер).
- **№3:** `HEAD /api/v1/healthz` без cookie → `200`, а не `401`.
- **№6:** `CreateUser` возвращает `23505` по email → `409`, `message` = «Пользователь с таким email уже существует»; `23505` по рег. номеру → «Организация с таким номером уже зарегистрирована».
- **№7:** фейковый `CreateSession` падает → `Login` возвращает ошибку, `UpdateLastLogin` откатан (фейковая транзакция не применила изменения), в аудите нет `auth.login`.
- **Rate limit:**
  - 10 неудачных входов на один email → 11-й (даже с верным паролем) `429 RATE_LIMITED`, `RetryAfter > 0`, `VerifyPassword` не вызывался;
  - успешный вход сбрасывает email-счётчик;
  - 11-я регистрация с одного IP за час → `429`;
  - через `NewMemoryRateLimiterWithClock` время сдвигается за окно → снова пускает;
  - 100 горутин `Allow` с `limit=10` → ровно 10 `ok`.
- **Жизненный цикл сессии (интеграционный):**
  - `Register` → `Authenticate(token)` возвращает `Principal`, `last_seen_at` вырос;
  - `UPDATE sessions SET expires_at = now() - interval '1 second'` → `Authenticate` → `UNAUTHORIZED`;
  - новая сессия, `UPDATE users SET status = 'blocked'` → `Authenticate` → `UNAUTHORIZED`;
  - `Logout` → строки в `sessions` нет.
- **Роутер:** `POST /api/v1/auth/logout` с сессией, но без `X-CSRF-Token` → `403 CSRF_INVALID` (уже есть в `TestRouterAuthAndCSRFOrder`); с совпадающими cookie и заголовком → `204`, обе cookie с `Max-Age=0`.
- **Login-CSRF:** `POST /api/v1/auth/login` без cookie и без `X-CSRF-Token`, `Content-Type: application/json` → `200`; то же тело с `Content-Type: application/x-www-form-urlencoded` или `text/plain` → `400`.

## Готово, когда

- [ ] Регистрация создаёт организацию, пользователя и сессию в одной транзакции; `borrower` не из `RU` и `lender` из `RU` → `400`; `role: admin` → `400`; дубль email → `409` с текстом про email.
- [ ] Пароль хэшируется argon2id (`service/password.go`), короче 10 символов → `400`.
- [ ] `login` ставит `session` (`HttpOnly; Secure` при `COOKIE_SECURE=true`; `SameSite=Lax; Path=/`) и `csrf_token` (не HttpOnly); в `sessions.token_hash` лежит `sha256` токена; истёкшая сессия → `401` (`TestAuthIntegrationSessionLifecycle`).
- [ ] Мутирующий запрос с сессией без `X-CSRF-Token` или с несовпадающим → `403 CSRF_INVALID`.
- [ ] 11-я неудачная попытка входа на email за 15 минут → `429` с `Retry-After`; лимит проверяется до argon2id.
- [ ] Заблокированный пользователь с верным паролем → `403`; неизвестный email и неверный пароль → одинаковые `401 «Неверный email или пароль»`.
- [ ] Успешный и неудачный вход пишут `auth.login` / `auth.login_failed` в `audit_log`.
- [ ] Замечания ревью №1–3, 5–7, 9 закрыты; PR #67 перебазирован на `main` и смержен.
- [ ] И общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: закрыты в [DECISIONS.md](DECISIONS.md) — CSRF для `register`/`login` (п. 1), `403` у `logout` (п. 2), IP за прокси (п. 4).
