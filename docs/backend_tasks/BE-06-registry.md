# BE-06 · Реестры: автозаполнение по ИНН / LEI (DaData, GLEIF, mock)

Issue: #22 · Milestone: M2 · Зависит от: PR #67 / BE-03 (`MemoryRateLimiter`, `ClientMeta`, публичный маршрут `/registry/lookup`) · Блокирует: BE-05 (снапшот реестра при `submitKyc`), BE-13 (вход правила `registry_mismatch`), BE-12 (`registry_check` в карточке компании), BE-16 (фикстуры мока ↔ сид)

## Цель
`GET /api/v1/registry/lookup?country_code=RU&number=7700000009` без логина возвращает данные компании из реестра: ЕГРЮЛ через DaData для RU, GLEIF по LEI для остальных стран, либо мок — выбор по env. Ответ ограничен по частоте (429) и деградирует в 502, если реестр недоступен. При `submitKyc` тот же поиск сохраняет снапшот в `registry_checks` и передаёт результат в AML.

## Что уже есть
- Контракт: `api/paths/organizations.yaml`, операция `registryLookup`:
  - `security: []`, параметры `country_code` (pattern из `CountryCode`) и `number` (8–20 символов);
  - ответы 200 / 400 / 429 / 502.
- Схема ответа: `RegistryLookupResult` в `api/schemas/organizations.yaml`. Поле `source` — enum **`[egrul_dadata, gleif]`, значения `mock` нет** — его добавляет шаг 1.
- Go-имена: метод `RegistryLookup`, параметры `api.RegistryLookupParams{CountryCode, Number}`, модель `api.RegistryLookupResult`, enum `api.RegistryLookupResultSource`. Заглушка 501 — `handler/organizations.go`.
- Пакет `backend/internal/registry/` — только `doc.go`.
- БД:
  - enum `registry_source` = `egrul_dadata | gleif | mock` (`backend/migrations/00001_init.sql`);
  - таблица `registry_checks` (`docs/db/tables/registry_checks.md`), запросы `CreateRegistryCheck` и `GetLatestRegistryCheck` в `backend/queries/kyc.sql`.
- PR #67 (`origin/be-03-auth`):
  - `backend/internal/service/ratelimit.go` — `RateLimiter{Check, Hit, Reset}` и `MemoryRateLimiter` со скользящим окном; BE-03 (шаг 3) добавляет атомарный `Allow(key, limit, window) (retryAfter, ok)` — им и пользоваться;
  - `apperr.RateLimitedAfter(d)` и `WriteError`, который ставит `Retry-After`;
  - `middleware.ClientMetaFromContext(ctx)` → `service.ClientMeta{IP *netip.Addr, UserAgent}`;
  - `publicRoutes` в `middleware/auth.go` уже содержит `GET /api/v1/registry/lookup`.
- `.env.example`: ключей DaData нет. Есть только `POSTGRES_*`, `DATABASE_URL`, `HTTP_ADDR`, `LOG_*`, `SHUTDOWN_TIMEOUT`, `DB_MAX_CONNS`; PR #67 добавляет `COOKIE_SECURE` и `SESSION_TTL`.
- Конфиг: `backend/internal/config/config.go`. Все ошибки собираются через `errors.Join`.

⚠️ Ловушки:
- **`MemoryRateLimiter` в PR #67 нигде не подключён.** Его подключает BE-03 (шаг 3) для `login`/`register` — один экземпляр в `main.go`; BE-06 берёт тот же экземпляр, ключи различаются префиксом. Если BE-06 пришла раньше — создай экземпляр сам и отметь в PR.
- **`Check` и `Hit` не атомарны** — поэтому BE-03 вводит `Allow`; `Check`/`Hit` в новом коде не используй.
- **`ClientMeta.IP` берётся из `r.RemoteAddr`.** За обратным прокси у всех клиентов будет один IP. Для MVP прокси нет; `X-Forwarded-For` не читаем — его подделывает клиент.
- **Контрольную сумму ИНН и LEI в lookup не проверяем:** неверный номер реестр и так вернёт как `Found=false`. Старый ИНН «Альфы» `7700000001` в `seed_demo.sql` сумму не проходит; BE-16 заменяет его на `7700000009`, и все демо-номера суммы проходят, так что проверку можно добавить позже без поломки сида.
- **ADR-0001 расходится сам с собой.**
  - В рабочем дереве п. 6 описывает настоящие DaData/GLEIF.
  - Незамёрженная локальная ветка `docs/adr-0001-kyc-aml-mocks` переписывает п. 6 на «в MVP — моки, сетевых вызовов и API-ключей нет».
  - `backend/README.md` в таблице organizations пишет «В MVP — мок».
  - Решено (DECISIONS п. 17): ветку смержить с формулировкой «по умолчанию мок; live-адаптеры DaData/GLEIF включаются `REGISTRY_PROVIDER=live`» — шаг 8.
- **PR #67 собран на старой раскладке** (один пакет `api`). Код пиши под `organizationsapi` (ADR-0003).
- У `registry_checks` при ролях DB-SEC у `lp_app` только `SELECT, INSERT`. Запрос «привязать проверку к организации» (`UPDATE`) не сработает, и он не нужен — см. «Решения».

## Решения
- **Три адаптера за одним интерфейсом, по умолчанию — мок** (DECISIONS п. 17). `REGISTRY_PROVIDER=mock|live`, дефолт `mock`. `live` = DaData для `RU`, GLEIF для остальных стран. Почему: issue требует все три адаптера, а ADR-ветка и README — чтобы демо работало без сети и ключей. Мок по умолчанию закрывает оба требования.
- **Интерфейс и типы** — пакет `internal/registry`. Он не зависит от `repository`; маппинг в типы БД делает сервис.
  ```go
  type Source string // "egrul_dadata" | "gleif" | "mock" — значения enum registry_source
  type Result struct {
      Source             Source
      Query              string          // что искали: ИНН/ОГРН или LEI
      Found              bool
      Name               string          // краткое: DaData name.short_with_opf, GLEIF legalName.name
      FullName           string          // DaData name.full_with_opf; у GLEIF совпадает с Name
      RegistrationNumber string          // DaData data.inn, GLEIF entity.registeredAs
      LegalAddress       string
      Status             string          // нормализованный: active, liquidating, liquidated, bankrupt, reorganizing, inactive
      Raw                json.RawMessage // сырой ответ (≤ 1 МБ) для registry_checks.raw_response
      CheckedAt          time.Time
  }
  type CompanyVerifier interface {
      Lookup(ctx context.Context, countryCode, number string) (Result, error)
  }
  var ErrUnavailable = errors.New("registry unavailable") // таймаут, 5xx, 429, 401/403 от реестра
  ```
- **Нормализация статуса — в `registry`**, а правило `registry_mismatch` — в `aml` (BE-13). Почему: все AML-правила лежат в одном пакете, а знание форматов DaData/GLEIF — в адаптерах.
  - DaData `state.status`: `ACTIVE`, `LIQUIDATING`, `LIQUIDATED`, `BANKRUPT`, `REORGANIZING` → нижний регистр.
  - GLEIF `entity.status`: `ACTIVE`, `INACTIVE` → нижний регистр.
  - Неизвестное значение → как есть в нижнем регистре.
- **Публичный lookup в БД не пишет. Снапшот сохраняется при `submitKyc`** — уже с `organization_id`. Почему:
  - запись в БД с неаутентифицированной ручки — вектор забивания таблицы;
  - снапшот без `organization_id` ни к чему не привязать (`AttachRegistryCheck` нет, `UPDATE` у `lp_app` нет);
  - доказательная база нужна на момент submit (`OrganizationDetails.registry_check`: «Результат сверки с реестром на момент submit KYC»).

  Пункт issue «Снапшот ответа в registry_checks» закрывается в `submitKyc`.
- **Валидация `number` — в сервисе**; ошибка → 400 `field: "number"`:
  - `RU`: `^\d{10}$` (ИНН юрлица) или `^\d{13}$` (ОГРН) — контракт: «ИНН/ОГРН для RU»;
  - остальные страны: `strings.ToUpper`, затем `^[A-Z0-9]{20}$` (LEI).
- **Ошибки реестра:**
  - сетевая ошибка, таймаут, HTTP 5xx, 429, 401/403 → `ErrUnavailable` → в lookup — 502 `UPSTREAM_UNAVAILABLE` (новый код);
  - 401/403 логируются на `Error` — это неверный ключ, а не сбой реестра;
  - «не найдено» (DaData — пустой `suggestions`, GLEIF — 404) → `Found=false` и 200.
- **Rate limit — в сервисе**, на `RateLimiter` из PR #67. Почему: сервис уже получает `ClientMeta`, а middleware пришлось бы учить один путь.
  - `registry_lookup:ip:<ip>` — `lookupPerIPLimit = 10` за `lookupWindow = time.Minute`;
  - `registry_lookup:global` — `lookupGlobalLimit = 60` в минуту: бережёт дневную квоту DaData;
  - проверка и учёт — одним `Allow` на каждый ключ: сначала IP, потом глобальный; отказ по глобальному после учёта IP допустим (запрос всё равно не прошёл);
  - `Retry-After` = больший из двух `retryAfter`.
- **HTTP-клиент:**
  - один `*http.Client{Timeout: cfg.RegistryTimeout}` (дефолт 5s) плюс `ctx` запроса;
  - тело ответа читается через `io.LimitReader(…, 1<<20)`;
  - ключ DaData только в заголовке `Authorization: Token …` — в URL и в логи он не попадает;
  - базовые URL — поля адаптера, дефолты — константы; в тестах подменяются адресом `httptest.Server`.
- **`source` в ответе API для мока — `mock`** (DECISIONS п. 18). Значение добавляется в enum `RegistryLookupResult.source` (шаг 1); так админ в `OrganizationDetails.registry_check` (BE-12) видит, что сверка шла по мок-реестру. В `registry_checks.source` пишется то же `mock` — enum БД его уже знает.
- **Кэш ответов реестра не делаем** (DECISIONS п. 38): по умолчанию мок, квоту live берегут лимиты ниже. Фронт вызывает lookup по blur или кнопке, а не на каждый символ — пометка для FE-04.
- **Мок — JSON-фикстуры, встроенные через `go:embed`** (`internal/registry/mockdata/companies.json`). Ключ — пара `(country_code, number)`: для `RU` — ИНН, для остальных стран — LEI.
- **Фикстуры строятся из таблицы демо-компаний [BE-16, шаг 1](BE-16-demo-seed.md#1-демо-идентификаторы--одна-таблица-на-все-задачи)** (DECISIONS п. 29): все 8 компаний, номера и названия — оттуда, здесь не дублируются. Статус у всех `active`, кроме ООО «Омега» — `liquidated` (даёт флаг `registry_mismatch`). Добавил компанию в сид — добавь фикстуру; соответствие проверяет `TestSeedDemoIdentifiers` (BE-16).
- Особая фикстура: `RU`, `0000000000` → мок возвращает `ErrUnavailable` (демо 502 и деградации `submitKyc`). Любой другой номер → `Found=false`.

## Шаги

### 1. Контракт и код 502
- `apperr`:
  - `CodeUpstreamUnavailable = "UPSTREAM_UNAVAILABLE"`;
  - конструктор `UpstreamUnavailable(msg string, err error)`.
- `handler/errors.go`: `statusByCode` → `http.StatusBadGateway`.
- `api/openapi.root.yaml`, раздел «Ошибки»: добавить код.
- `backend/README.md`:
  - таблица «Ошибки»: `UPSTREAM_UNAVAILABLE | 502 | внешний реестр не ответил (registry/lookup)`;
  - строка `RATE_LIMITED`: «… `login`, `register`, `registry/lookup`».
- `api/schemas/organizations.yaml`: `mock` в enum `RegistryLookupResult.source`, в `description` поля — «`mock` — сверка по встроенным фикстурам (`REGISTRY_PROVIDER=mock`)». Затем `make gen-api` и `make diff-api`: добавление значения в enum ответа oasdiff может пометить как ломающее — для клиентов это не так (фронт ещё не создан), в описании PR это отметить.

**Готово:** `CodeUpstreamUnavailable` добавлен в список `TestEveryCodeHasStatus` и в `TestErrorMapping` (`handler/errors_test.go`) со статусом 502; `api.RegistryLookupResultSourceMock` сгенерирован; `make lint-api` зелёный.

### 2. Конфиг
В `internal/config/config.go`:
- `RegistryProvider` (`REGISTRY_PROVIDER`, `mock|live`, дефолт `mock`; другое значение → ошибка);
- `DaDataAPIKey` (`DADATA_API_KEY`); при `live` и пустом ключе → ошибка `DADATA_API_KEY is required when REGISTRY_PROVIDER=live`;
- `RegistryTimeout` (`REGISTRY_TIMEOUT`, дефолт `5s`, > 0).

В `.env.example` добавить блок с комментарием «мок по умолчанию; live ходит в DaData и GLEIF»:
```
REGISTRY_PROVIDER=mock
DADATA_API_KEY=
REGISTRY_TIMEOUT=5s
```
Затем обновить `README.md` («Запуск бэкенда»).

**Готово:** `config_test` покрывает пять случаев — дефолты; `live` без ключа → ошибка; `REGISTRY_PROVIDER=foo` → ошибка; `REGISTRY_TIMEOUT=0` → ошибка; `live` + ключ → ok.

### 3. Пакет `registry`: типы, мок, выбор адаптера
- `registry/registry.go`: типы из «Решений»; doc-комментарий пакета обновить — убрать «клиенты DaData/GLEIF» как единственный вариант.
- `registry/mock.go` + `mockdata/companies.json`: `Mock.Lookup`, `Source = "mock"`, `Raw` — запись фикстуры.
- `registry/new.go`: `New(cfg Config) (CompanyVerifier, error)`:
  - `mock` → `Mock`;
  - `live` → `byCountry{ru: DaData, other: GLEIF}`, где `Lookup` выбирает адаптер по `countryCode == "RU"`.

**Готово:** табличный `TestMockLookup` проверяет все 8 демо-фикстур (у «Омеги» `Status = liquidated`), неизвестный номер → `Found=false`, `0000000000` → `errors.Is(err, ErrUnavailable)`; `TestNewSelectsAdapter`: `RU` → DaData, `DE` → GLEIF (тест проверяет тип внутреннего адаптера или вызов фейкового сервера).

### 4. Адаптер DaData
`registry/dadata.go`. Запрос:
- `POST {base}/suggestions/api/4_1/rs/findById/party`, base по умолчанию `https://suggestions.dadata.ru`;
- заголовки `Authorization: Token <key>`, `Content-Type: application/json`, `Accept: application/json`;
- тело `{"query": number, "type": "LEGAL"}`.

Разбор `suggestions[0].data`:
- `inn` → `RegistrationNumber`;
- `name.short_with_opf` → `Name`, `name.full_with_opf` → `FullName`;
- `address.unrestricted_value` (или `address.value`) → `LegalAddress`;
- `state.status` → `Status`.

Формат сверить с документацией https://dadata.ru/api/find-party/ и зафиксировать в `testdata/dadata_found.json` и `testdata/dadata_empty.json`.

**Готово:** `TestDaData` против `httptest.Server`:
- found → поля и `Status = "active"`;
- пустой `suggestions` → `Found=false`;
- сервер проверяет заголовок `Authorization: Token test-key`;
- 403 / 429 / 500 → `ErrUnavailable`;
- ответ дольше таймаута (клиент с `Timeout: 50ms`) → `ErrUnavailable`;
- тело 2 МБ → ошибка разбора, а не OOM.

### 5. Адаптер GLEIF
`registry/gleif.go`. Запрос: `GET {base}/api/v1/lei-records/{lei}`, base по умолчанию `https://api.gleif.org`, `Accept: application/vnd.api+json`, ключ не нужен.

Разбор `data.attributes.entity`:
- `legalName.name` → `Name` и `FullName`;
- `registeredAs` → `RegistrationNumber`;
- `legalAddress` (`addressLines` + `postalCode` + `city` + `country` через `", "`) → `LegalAddress`;
- `status` → `Status`.

404 → `Found=false`. Формат сверить с https://api.gleif.org/api/v1 (JSON:API) и зафиксировать в `testdata/gleif_record.json`.

**Готово:** `TestGLEIF` против `httptest.Server`: found → поля; 404 → `Found=false`; 500 и таймаут → `ErrUnavailable`; путь запроса содержит LEI в верхнем регистре.

### 6. Сервис и хендлер lookup
`service/registry.go`:
- `type Registry struct{ verifier registry.CompanyVerifier; limiter RateLimiter; now func() time.Time }`;
- `Lookup(ctx, meta ClientMeta, countryCode, number string) (registry.Result, error)`:
  1. Валидация `number` → 400.
  2. Rate limit → `apperr.RateLimitedAfter`.
  3. `verifier.Lookup`; `ErrUnavailable` → `apperr.UpstreamUnavailable("Реестр временно недоступен, заполните данные вручную", err)`.

Порядок «сначала валидация, потом лимит» — сознательный: битые запросы не тратят квоту честного клиента. Защиту от перебора даёт глобальный лимит.

Там же — два маппинга для `submitKyc` (BE-05):
- `registryCheckParams(orgID uuid.UUID, r registry.Result) repository.CreateRegistryCheckParams`;
- `amlRegistryResult(r registry.Result, err error) *aml.RegistryResult` — `Outcome` = `checked` или `unavailable`, `Names = [Name, FullName]`, `Status`.

`handler/organizations.go`, `RegistryLookup`:
- `meta := middleware.ClientMetaFromContext(ctx)`;
- маппинг `Result` → `api.RegistryLookupResult`: при `Found=false` строковые поля — `nil`; `CheckedAt` заполнен всегда.

`cmd/server/main.go`: `registry.New(cfg)`, общий `service.NewMemoryRateLimiter()`, зависимость `RegistryService` в `handler.Deps`.

**Готово:**
- `TestRegistryLookupService` (фейковый verifier, `NewMemoryRateLimiterWithClock`) зелёный по таблице из «Тестов»;
- `TestRegistryLookupHandler`: без cookie → 200 (маршрут публичный);
- `make run` с `REGISTRY_PROVIDER=mock`:
  - `curl "localhost:8080/api/v1/registry/lookup?country_code=RU&number=7700000009"` → 200 `found: true`, `name: "ООО «Альфа»"`, `source: "mock"`;
  - 11-й запрос за минуту → 429 с `Retry-After`;
  - `number=0000000000` → 502 `UPSTREAM_UNAVAILABLE`.

### 7. Встроить в `submitKyc`
Если BE-05 уже смержен:
- передать `verifier` в `service.NewKyc` в `main.go`;
- убедиться, что `SubmitKyc` вызывает `Lookup` до транзакции, а внутри транзакции — `CreateRegistryCheck(registryCheckParams(...))`, и передаёт `amlRegistryResult(...)` в `OrganizationSubject.Registry` (шаг 7 плана BE-05).

Если BE-05 ещё нет — этот шаг выполняет BE-05.

**Готово:** интеграционный тест BE-05 `TestSubmitKycIntegration` с мок-реестром → в `registry_checks` одна строка: `organization_id` компании, `source = 'mock'`, `found = true`, `raw_response` не NULL.

### 8. Документация
- `backend/README.md`, раздел organizations: «В MVP — мок» → «по умолчанию мок (`REGISTRY_PROVIDER=mock`), `live` — DaData/GLEIF; лимит 10 запросов в минуту с IP; 502, если реестр недоступен».
- `docs/db/tables/registry_checks.md`: снапшот пишется только при `submitKyc`, lookup до регистрации в БД не пишет; `AttachRegistryCheck` не нужен.
- ADR-0001, п. 6: смержить ветку `docs/adr-0001-kyc-aml-mocks`, заменив формулировку на «по умолчанию мок; live-адаптеры DaData/GLEIF включаются `REGISTRY_PROVIDER=live`» (DECISIONS п. 17).

**Готово:** изменения есть в `backend/README.md`, `docs/db/tables/registry_checks.md` и ADR-0001 в `main`; `make verify` зелёный.

## Тесты
- `TestRegistryLookupService` (табличный):

  | Вход | Ожидание |
  |---|---|
  | `RU`, `7700000009` | вызов verifier, 200 |
  | `RU`, `1027700132195` (ОГРН, 13 цифр) | ok |
  | `RU`, `77000000` (8 цифр) | 400 `field=number`, verifier не вызван |
  | `RU`, `77000000AB` | 400 |
  | `DE`, `demo00gammagmbh00148` | verifier получает `DEMO00GAMMAGMBH00148` |
  | `DE`, `DE123456789` (не LEI) | 400 |
  | 11-й запрос с одного IP за 60 с | 429, `RetryAfter > 0` |
  | тот же IP через 61 с (фейковые часы) | ok |
  | 61-й запрос с разных IP | 429 (глобальный лимит) |
  | verifier → `ErrUnavailable` | `UPSTREAM_UNAVAILABLE` |
  | verifier → `Found=false` | 200, `found=false` |

- `TestDaData`, `TestGLEIF`, `TestMockLookup`, `TestNewSelectsAdapter` — шаги 3–5.
- Маппинг статусов: `ACTIVE→active`, `LIQUIDATED→liquidated`, `INACTIVE→inactive`, `""→""`.
- `TestAmlRegistryResult`: `err = ErrUnavailable` → `Outcome = unavailable`; `Found=false` → `Outcome = checked`, `Found = false`.
- В логах нет ключа: адаптер с ключом `secret-key` возвращает ошибку, вывод `slog` в буфер не содержит `secret-key`.

## Готово, когда
- [ ] `registry.CompanyVerifier` с тремя адаптерами: DaData `findById/party` (RU), GLEIF `lei-records/{lei}`, `mock` с фикстурами 8 демо-компаний BE-16, «Омега» — ликвидирована (`TestDaData`, `TestGLEIF`, `TestMockLookup`).
- [ ] `RegistryLookupResult.source` в контракте содержит `mock`; мок отвечает `source: mock`.
- [ ] Адаптер выбирается по `REGISTRY_PROVIDER`; реестр недоступен → `registryLookup` отвечает 502 `UPSTREAM_UNAVAILABLE`, а `submitKyc` проходит с флагом ручной проверки (BE-13) (`TestRegistryLookupService`, `TestSubmitKyc`).
- [ ] При `submitKyc` снапшот сохраняется в `registry_checks` (сырые данные + нормализованные поля, `organization_id` заполнен).
- [ ] Нормализованные `Name`, `FullName` и `Status` передаются в `aml.RegistryResult` для правила `registry_mismatch` (`TestAmlRegistryResult`).
- [ ] Rate limit на `registryLookup` (10 запросов в минуту с IP, 60 — глобально) → 429 с `Retry-After`; таймаут HTTP-клиента задаётся `REGISTRY_TIMEOUT`.
- [ ] И общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: закрыты в [DECISIONS.md](DECISIONS.md) — `mock` в `source` (п. 18), ADR-0001 (п. 17), LEI в сиде (п. 29), кэш (п. 38).
