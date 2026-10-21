# ADR-0002: Раскладка контракта, бандл и валидация запросов по спеке

**Status:** Accepted
**Date:** 2026-10-21
**Deciders:** бэкенд
**Supersedes:** ADR-0001, п. 1 — в части «как собирается `api/openapi.yaml`» и «`spec.gen.go`»

## Context

ADR-0001 закрепил контракт-first: источник правды — `api/openapi.yaml`, Go-сервер генерируется oapi-codegen. К моменту BE-01 выяснилось, как это устроено на практике:

1. **Спека «пилилась» текстовой конкатенацией, а не `$ref`.** `api/paths/*.yaml` и `api/schemas/*.yaml` были телами ключей `paths:` и `components.schemas:` с несущими отступами, `api/root.yaml` заканчивался висящим `paths:`. По отдельности файлы невалидны, IDE не подсказывает, дубль имени схемы никто не заметит.
2. **Контракт не проверялся в CI.** `gen-check` следил только за тем, что `*.gen.go` соответствует спеке — не за тем, что спека валидна и не ломает клиентов. Притом файловое правило Make по таймстампам после `git checkout` считало бандл свежим и **не пересобирало его**: шаг «spec bundle is fresh» проходил, ничего не проверив.
3. **Генератор работал на дефолтах:** `Id`/`Kyc` вместо `ID`/`KYC`, голые enum-константы `High`/`Low`/`Medium` на весь пакет, `*[]T`, `Money` как `string` при том, что sqlc отдаёт `decimal.Decimal`.
4. **Запросы не валидировались по спеке:** strict server это не делает by design. `required`, `enum`, `pattern`, `minimum` не проверял никто.
5. **`spec.gen.go` (embedded-spec) никто не импортировал**, а его deflate-вывод зависел от версии Go — из-за него пришлось пиннить `toolchain`.

Реализовано было 4 операции из 51 — момент для правок, меняющих сгенерированные имена, самый дешёвый.

## Decision

### 1. Настоящий `$ref`-сплит, бандл генерируется и коммитится

```
api/
  openapi.root.yaml   # header + оглавление paths: из $ref на path item'ы      [источник]
  openapi.yaml        # бандл, только локальные $ref                     [ГЕНЕРИРУЕТСЯ, коммитится]
  components.yaml     # parameters + responses                            [источник]
  paths/<тег>.yaml    # paths: { /x: {...} }                               [источник]
  schemas/<домен>.yaml  # components: { schemas: {...} }                   [источник]
```

- Каждый файл — валидный частичный OpenAPI-документ. Ссылки явные и относительные: `'../schemas/errors.yaml#/components/schemas/Error'`; path item'ы из корня — `'./paths/system.yaml#/paths/~1healthz'` (`~1` — экранированный `/`, RFC 6901).
- Гранулярность — по тегам (10 файлов путей, 12 файлов схем), не файл на операцию: 51 операция на 10 файлов, конфликтов на проекте такого размера это не создаёт.
- `securitySchemes` лежит в корне: глобальный `security: [cookieAuth]` резолвится только из корневого `components`, а `InternalizeRefs` оставил бы для `$ref`-схемы безопасности самоссылку.
- **Бандлер — `backend/cmd/specbundle` на kin-openapi `InternalizeRefs`** со своим резолвером имён (последний сегмент фрагмента). Дефолтный резолвер склеивает путь файла и имя (`applications_Application`) и переименовал бы все Go-типы. Коллизия одного имени из двух файлов — ошибка сборки, а не молчаливое схлопывание.
- Бандл восстанавливает порядок верхних ключей и путей из источника: oapi-codegen регистрирует маршруты в порядке строк файла, и алфавитный порядок менял бы `server.gen.go`.
- Генератор, vacuum, oasdiff, фронт (`openapi-typescript`, Prism) читают **только бандл**. Кормить oapi-codegen сплитом напрямую нельзя: он берёт схемы только из `components.schemas` корня, а внешние компоненты kin-openapi в корень не кладёт — понадобилось бы 12 конфигов и 12 `*.gen.go`.

Приёмка переразбивки: `models.gen.go` и `server.gen.go` не изменились побайтово; `oasdiff` между старым и новым бандлом — только `info.description`.

### 2. Генератор: `output-options` и `x-go-type`

| Опция | Решение |
|---|---|
| `name-normalizer: ToCamelCaseWithInitialisms` + `additional-initialisms` (KYC, AML, FATF, LEI, SWIFT, SEPA, IBAN, VAT, OFAC) | да: `KYCStatus`, `ApplicationID`, `FATFHighRisk`; wire-формат не меняется |
| `compatibility.always-prefix-enum-values` | да: `AMLSeverityHigh` вместо `High` на весь пакет |
| `prefer-skip-optional-pointer-on-container-types` | да: `[]T` вместо `*[]T` |
| `x-go-type: decimal.Decimal` на `Money`/`Percent` | да: `type Money = decimal.Decimal` — алиас, поля sqlc присваиваются без конвертаций |
| `nullable-type`, `generate-types-for-anonymous-schemas`, `resolve-type-name-collisions`, `prefer-skip-optional-pointer`, `skip-enum-validate`, `client` | нет (см. «Отклонённые альтернативы») |

Два конфига (`models.yaml`, `server.yaml`) пишут в один пакет, поэтому блок `compatibility`/`output-options` обязан совпадать дословно — это проверяет `internal/http/api/gencfg_test.go`.

Фильтры `amount_min`/`amount_max` описаны инлайновой строкой с тем же паттерном, а не `$ref` на `Money`: для query-параметра со struct-типом runtime oapi-codegen ищет в query поля структуры и молча возвращает `nil` — фильтр исчезал бы без ошибки.

### 3. Спека в рантайме — `//go:embed`, не `embedded-spec`

`spec.gen.go` и `oapi-codegen/spec.yaml` удалены. `specbundle` пишет бандл в два места: `api/openapi.yaml` и `backend/internal/openapi/openapi.yaml` (`//go:embed` не умеет `../`, модуль — `backend/`). Пакет `internal/openapi` отдаёт `Spec []byte` и `Doc()`/`MustDoc()` (разбор один раз на процесс), обнуляет `Servers` (префикс `/api/v1` снимает middleware) и регистрирует формат `uuid` без проверки версии (UUIDv7).

Цена — 77 КБ дубля в git. Выигрыш: артефакт побайтово равен файлу, никакого deflate и зависимости от тулчейна; расхождение копии ловит существующий `gen-check` (она под `backend/internal`).

### 4. Валидация запросов по спеке в рантайме

`oapi-codegen/nethttp-middleware` перед роутингом, внутри `Recover`. Четыре обязательные настройки — без любой из них ломается всё сразу: `AuthenticationFunc: Noop` (иначе 401 на каждый запрос из-за глобального `security`), `Prefix: /api/v1` (иначе 404 на всё), `Skipper` для multipart-загрузки (kin-openapi читает файл целиком в память), `MultiError` (запись на поле в `error.details`).

`middleware.MaxBody` перед валидатором: 1 МБ по умолчанию, 10 МБ для загрузки документов. Валидатор читает тело целиком в память — без лимита любой POST был бы неограниченным буфером.

Маппинг ошибок (`handler/validate.go`): нет маршрута → 404 `NOT_FOUND` как раньше; параметр → его имя; тело → JSON-pointer (`beneficiaries.0.full_name`); короткий русский текст по нарушенному правилу схемы; `SchemaError.Reason` по контракту kin-openapi не содержит исходное значение, ввод пользователя в ответ не попадает. `ErrorHandlerOpts.StatusCode` игнорируется: статус определяет `statusByCode` по коду `apperr` — единственный источник правды.

### 5. CI

Один workflow `backend.yml` (второй файл с `paths:` удвоил бы ловушку «required check висит в Expected»). Джобы: `lint` (gofmt, vet, `tidy -diff`, golangci-lint, actionlint), `gen-check` (оба дерева: `api` и `backend/internal`), `api-lint` (vacuum с `api/vacuum-ruleset.yaml`, любое новое предупреждение — ошибка), `api-breaking` (oasdiff против base PR), `test`, `build` (ждёт `gen-check`). Версии инструментов — `versions.env`, единый источник для Makefile и CI. `bundle-api` — нефайловая цель, бандл пересобирается всегда и перезаписывается только при изменении.

## Consequences

**Положительные**
- Файлы контракта валидны по отдельности, IDE резолвит `$ref`, дубли имён — ошибка сборки.
- Контракт проверяется в CI: структура (vacuum), совместимость (oasdiff), актуальность бандла и сгенерированного кода (gen-check, теперь без дыры с mtime).
- Контракт исполняется до написания хендлеров: для 47 нереализованных операций мусор получает 400 с полем, а не 501; когда хендлер появится, его вход уже гарантирован спекой.
- `Money` — `decimal.Decimal` в API-типах и в sqlc, ручных конвертаций нет.
- `toolchain` в `go.mod` больше не держит `gen-check`; оставлен закреплённым ради стабильности gofmt.

**Отрицательные / принятые**
- **Комментарии и flow-стиль в `api/openapi.yaml` не сохраняются**: бандл — артефакт, комментарии живут в источниках. Точность ограничена моделью kin-openapi — это и ловит `oasdiff`, пустой дифф `*.gen.go` этого не поймает.
- **Поведение всех 51 операций изменилось одним коммитом:** мусорные запросы — 400 вместо 501, у тела появился лимит. Любой маршрут вне спеки (`/docs`, `/openapi.yaml`) валидатор отдаст как 404 — когда появится Swagger UI, он пойдёт мимо валидируемой цепочки через внешний `mux`.
- `openapi3filter` при `SkipSettingDefaults: false` дописывает `page=1&page_size=20` в `r.URL.RawQuery`; `r.RequestURI` больше нельзя логировать как «что просил клиент».
- Сборка бандла может падать (битая ссылка, опечатка в пути, коллизия имён) — `cat` падать не мог.
- 77 КБ дубля спеки под `backend/internal/openapi`.
- Dependabot не видит `versions.env` и `go run tool@version`: vacuum/oasdiff/goose/sqlc/golangci-lint поднимаются руками. Обновление oapi-codegen от Dependabot закономерно роняет `gen-check` — это сигнал прогнать `make gen` в ветке.
- `gorillamux.NewRouter(spec)` строится на каждый `NewRouter()`; при сотнях тестов — завернуть в `sync.OnceValue`.

## Отклонённые альтернативы

- **Кормить oapi-codegen сплитом напрямую** (13 конфигов с `import-mapping`). Схемы берутся только из `components.schemas` корня; `import-mapping` требует литеральных путей до `#` без нормализации (три разные глубины на один файл); в графе схем межфайловые циклы (`organizations` ↔ `auth`, `organizations` ↔ `aml`), так что отдельные пакеты невозможны — вся цена без выигрыша.
- **`vacuum bundle` / `redocly bundle` как бандлер.** Режим по умолчанию встраивает внешние объекты инлайном — 62 именованных типа растворились бы в анонимные структуры.
- **Файл на операцию в `paths/`.** 51 файл и 51 относительный путь без выигрыша по конфликтам.
- **Симлинк для `go:embed`** (права администратора на Windows), перенос `api/openapi.yaml` внутрь `backend/` (контракт перестаёт быть общим с фронтом), генерация `.go` со спекой в строке (в описаниях есть обратные кавычки).
- **`nullable-type: true` глобально.** 42 поля → `nullable.Nullable[T]` плюс зависимость, а разница с `*T` есть ровно в одном PATCH-поле (`OrganizationUpdate.lei`) — поставить точечно `x-go-type` при реализации `updateMyOrganization`.
- **`generate-types-for-anonymous-schemas`.** Инлайновых схем 5, из них 4 — тела запросов, для которых генератор и так делает `XxxJSONBody`; нужны имена — назвать схемы в спеке.
- **`resolve-type-name-collisions`.** Молча переименовывает; пусть генератор падает громко.
- **`prefer-skip-optional-pointer[-with-omitzero]`.** Убивает «отсутствует vs ноль» в PATCH-телах: сумма `0` неотличима от «не передано».
- **oapi-codegen как прямая зависимость в `go.mod`.** Модуль подключён директивой `tool`, `go mod tidy` возвращает его в `// indirect`; «починка» уронила бы проверку `tidy -diff`.
- **Типизированный клиент (`client: true`) сейчас.** Эталонный тест проверяет точный JSON на проводе; генерированный клиент десериализует теми же типами и спрятал бы именно те ошибки, которые тест ловит. Добавить вместе с e2e-smoke.
- **`--hard-mode` у vacuum** (OWASP): потребует rate-limit-заголовков, которых в MVP нет. **Пин экшенов по SHA** — отдельный коммит.
- **Отдельный `api.yml` в CI.** Workflow с `paths:` на PR без изменений в `api/**` не запускается, и обязательная джоба висит «Expected — Waiting for status».
