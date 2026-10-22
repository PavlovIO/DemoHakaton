# Liquidity_platform

[![backend](https://github.com/MaxPastukhov15/Liquidity_platform/actions/workflows/backend.yml/badge.svg)](https://github.com/MaxPastukhov15/Liquidity_platform/actions/workflows/backend.yml)
Требуется разработать MVP цифровой платформы ликвидности для трансграничного финансирования между юридическими лицами — площадки, соединяющей российские компании, ищущие зарубежное финансирование, и зарубежных кредиторов со свободными средствами.  

## Документация

| Что | Где |
|---|---|
| Контракт API (источник правды для фронта и бэка) | [`api/openapi.yaml`](api/openapi.yaml) — бандл, собирается из [`api/openapi.root.yaml`](api/openapi.root.yaml), [`api/paths/`](api/paths), [`api/components.yaml`](api/components.yaml) и [`api/schemas/`](api/schemas) |
| Архитектура и ключевые решения | [`docs/adr/0001-architecture-and-api-contract.md`](docs/adr/0001-architecture-and-api-contract.md), [`docs/adr/0002-spec-layout-and-request-validation.md`](docs/adr/0002-spec-layout-and-request-validation.md) (раскладка контракта, валидация запросов), [`docs/adr/0003-server-package-per-tag.md`](docs/adr/0003-server-package-per-tag.md) (сервер по пакету на тег) |
| REST API: аутентификация, ошибки, все эндпоинты, сценарий от регистрации до сделки, устройство кода | [`backend/README.md`](backend/README.md) |
| Схема БД, ER-диаграмма, ключевые запросы | [`docs/db/schema.md`](docs/db/schema.md) |
| Миграции (goose) | [`backend/migrations`](backend/migrations) |
| Бэклог (источник для issues) | [`scripts/issues.yaml`](scripts/issues.yaml) |

Задачи разбиты на вехи M1–M4. Пользовательские сценарии помечены `user-story` и ссылаются на задачи фронта и бэка.

## Запуск бэкенда

Нужны Go 1.26+ и Docker. Точная версия Go для сборки и кодгена зафиксирована в `backend/go.mod` (`toolchain`), Go скачает её сам.

```sh
cp .env.example .env   # один раз
make db                # Postgres + роли БД + миграции + мок-санкционный список + демо-данные (см. «Демо-данные»)
make run               # API на http://localhost:8080/api/v1
curl localhost:8080/api/v1/healthz
```

Приложение и интеграционные тесты подключаются к Postgres под ролью `lp_app` (`DATABASE_URL`, только DML), миграции — под `lp_migrator` (`MIGRATE_DATABASE_URL`, владелец схемы); роли создаёт `make db-roles` (входит в `make db`). Подробнее — [docs/db/schema.md](docs/db/schema.md#роли-и-права).

**Volume создан до появления ролей** (таблицы принадлежат `lp`, сервер пишет warning про суперпользователя): обнови `.env` из `.env.example` и пересоздай БД — `make db-reset` (удаляет данные).

Переменные окружения — в `.env.example`. Для входа важны две: `SESSION_TTL` — срок жизни сессии (по умолчанию `24h`, не продлевается) и `COOKIE_SECURE` — флаг `Secure` у cookie (по умолчанию `true`; в `.env.example` — `false`, иначе браузер не сохранит cookie на `http://localhost`).
`CORS_ALLOWED_ORIGINS` — origin фронта на другом хосте через запятую; по умолчанию пусто, CORS выключен: фронт ходит в API через тот же origin (Vite proxy).

| Команда | Что делает |
|---|---|
| `make gen` | Пересобрать бандл `api/openapi.yaml` (и его копию для `go:embed`) из источников и перегенерировать код (oapi-codegen) и `backend/queries` (sqlc). Правятся `api/openapi.root.yaml`, `api/paths/*.yaml`, `api/components.yaml`, `api/schemas/*.yaml` — не бандл; CI (`gen-check`) проверяет, что всё пересобрано |
| `make lint-api` | Линтер контракта (vacuum, правила в `api/vacuum-ruleset.yaml`) |
| `make diff-api` | Ломающие изменения контракта относительно `origin/main` (oasdiff); `BASE_REF=<ref>` — другая база |
| `make verify` | Всё, что гоняет CI, кроме интеграционных тестов и `gen-check`: lint, lint-api, tidy-check, lint-actions, test |
| `make test` | Юнит-тесты, БД не нужна |
| `make test-all` | Все тесты, включая интеграционные (нужен `make db`) |
| `make lint` | golangci-lint |
| `make build` | Бинарник в `backend/bin/` |
| `make load-sanctions` | Залить санкционный список для AML-скрининга (по умолчанию — вымышленный мок-список, он же входит в `make db`). Список заменяется целиком |

Официальный UK Sanctions List: скачайте CSV со страницы [The UK Sanctions List](https://www.gov.uk/government/publications/the-uk-sanctions-list) (ссылка «CSV», без регистрации) и выполните `make load-sanctions SANCTIONS_SOURCE=uk_fcdo SANCTIONS_FILE=path/to/UK-Sanctions-List.csv`. Файл в репозиторий не коммитится. Загрузчик подключается через `MIGRATE_DATABASE_URL`, а без неё — через `DATABASE_URL`.

Загруженные KYC-документы сервер хранит в каталоге `DOCUMENT_STORAGE_DIR` (по умолчанию `storage/documents` в корне репо, в `.gitignore`; права `0700`, файлы `0600` под случайными именами). Каталог должен переживать перезапуски: в контейнере это named volume (см. ниже). Сервер не стартует, если в каталог нельзя писать.

### Бэкенд в Docker

Вместо `make run` бэкенд можно поднять контейнером (`Dockerfile.backend` в корне, сервис `backend` в `docker-compose.yml`). Схему, санкции и SQL-сид по-прежнему готовит `make db` с хоста:

```sh
make db        # Postgres + миграции + санкции + демо-данные
make backend-up    # demo-PDF в volume документов, сборка образа, API на http://localhost:8080/api/v1
make backend-logs  # логи бэкенда (JSON)
```

| Что | Как |
|---|---|
| Образ | Многоэтапная сборка: Go из `toolchain` в `go.mod`, статический бинарник на `alpine`, пользователь `app` (uid 10001), не root |
| БД | `postgres:5432` внутри сети compose из `POSTGRES_*`; другая — `BACKEND_DATABASE_URL` |
| Документы | volume `lp_documents` → `/var/lib/liquidity-platform/documents`; `make backend-seed-files` (входит в `backend-up`) кладёт туда PDF сида. `make db-reset` удаляет и его |
| Порт | `BACKEND_PORT` (по умолчанию `8080`); одновременно с `make run` — задайте другой |
| Настройки | `LOG_LEVEL`, `SESSION_TTL`, `COOKIE_SECURE` (по умолчанию `false` — локальный http), `CORS_ALLOWED_ORIGINS`, `DB_MAX_CONNS` — из `.env`; `LOG_FORMAT=json` |

`make down` останавливает и бэкенд, и Postgres. CI (`backend.yml`, джоба `docker`) собирает оба образа и поднимает compose до healthy.

### Демо-данные

`make db` заливает демо-сид (`backend/migrations/testdata/seed_demo_*.sql`) и кладёт PDF документов в `DOCUMENT_STORAGE_DIR`. Вариантов два, на одной БД они несовместимы — переключение только с чистой базы:

| Команда | Вариант |
|---|---|
| `make db-reset` | **B** (по умолчанию): «Альфа» и Beta Corp верифицированы, у заявки «Альфы» 500 000 EUR → DE офферы Beta (8 % + комиссия 1 %) и Epsilon (9.5 %), есть чат; история за ~3 месяца для аналитики |
| `make db-reset SEED=a` | **A**: KYC с нуля — «Альфа» в `draft` без документов, Beta ждёт проверки |

`make seed` (`make seed-demo`) повторно применяет сид к текущей БД; сид идемпотентен.

#### Демо-аккаунты

⚠️ Только для локального демо. Пароль у всех — **`Demo-Pass-2026`**. Компании вымышленные; ИНН и LEI — с корректными контрольными суммами, префикс LEI `DEMO` не выдан ни одному LOU.

| Компания | Тип / страна | Рег. номер | LEI | Email | Вариант A | Вариант B |
|---|---|---|---|---|---|---|
| admin | admin | — | — | `admin@liquidity.local` | active | active |
| ООО «Альфа» | заёмщик / RU | `7700000009` | — | `alpha@liquidity.local` | `draft`, профиль заполнен, документов нет | `verified` |
| Beta Corp Ltd | кредитор / CY | `CY-123456` | `DEMO00BETACORP000162` | `beta@liquidity.local` | `under_review`, документы загружены | `verified` |
| Gamma GmbH | кредитор / DE | `HRB-555555` | `DEMO00GAMMAGMBH00148` | `gamma@liquidity.local` | `under_review` + открытый AML-флаг `high` (санкции) | то же |
| АО «Вектор» | заёмщик / RU | `7700000016` | — | `vector@liquidity.local` | `verified`, владелец заявок на витрине | то же |
| Delta Trading | кредитор / KW | `KW-777777` | `DEMO00DELTATRADE0174` | `delta@liquidity.local` | `under_review` + открытый флаг FATF `medium` | то же |
| Epsilon Capital Pte | кредитор / SG | `201988888E` | `DEMO00EPSILONCAP0155` | `epsilon@liquidity.local` | `verified` | `verified`, оффер к заявке «Альфы» |
| ООО «Омега» | заёмщик / RU | `7700000023` | — | `omega@liquidity.local` | `rejected` (ликвидирована по реестру) | то же |
| ООО «Сигма» | заёмщик / RU | `7700000030` | — | `sigma@liquidity.local` | `verified`, пользователь заблокирован | то же |

#### Сценарий US-14 по шагам

Вариант **B** (`make db-reset`):
1. Регистрация — **новой** компанией (не «Альфой»: она уже в сиде, повторная регистрация даст 409).
2. Вход `alpha@liquidity.local` → заявка 500 000 EUR / 6 мес. / DE уже опубликована, у неё офферы Beta и Epsilon.
3. Вход `beta@liquidity.local` → витрина, фильтр «Германия»: две заявки, нужная — 500 000 EUR на 6 мес.
4. Чат по офферу Beta, затем «Альфа» принимает оффер Beta → заявка «Согласовано», сделка с комиссией 5 000.00 EUR; оффер Epsilon автоматически отклонён.
5. Admin: аналитика за ~3 месяца; журнал аудита — блокировка «Сигмы»; вход `sigma@` → «Аккаунт заблокирован».

Вариант **A** (`make db-reset SEED=a`):
1. Регистрация новой компании.
2. Вход `alpha@liquidity.local` → загрузка документов, отправка KYC.
3. Admin одобряет KYC «Альфы» и Beta; одобрение Gamma → 409 (открытый `high`-флаг), у Delta виден флаг FATF.
4. «Альфа» создаёт заявку 500 000 EUR / 6 мес. / DE → admin публикует → Beta находит её фильтром «Германия» → оффер 8 % + 1 % → чат → «Альфа» принимает.

Как добавить реализацию операции API — см. комментарий в [`backend/internal/http/handler/handler.go`](backend/internal/http/handler/handler.go); образец — справочники (`dictionaries.go` + `dictionaries_test.go`). Сервер сгенерирован по пакету на тег (`backend/internal/http/api/<tag>api`), модели — общий пакет `api`, реализации — `backend/internal/http/handler/<tag>.go` ([ADR-0003](docs/adr/0003-server-package-per-tag.md)). Бизнес-логика — ядро `backend/internal/service` и пакеты доменов `auth`, `kyc`, `lending`, `notifications`, `dictionaries` ([ADR-0005](docs/adr/0005-service-packages-by-domain.md)).

## Запуск фронтенда

Нужны Node.js 20+ и npm. Фронтенд настроен на проксирование запросов `/api` на локальный бэкенд (`http://localhost:8080`), поэтому для полноценной работы **бэкенд должен быть запущен**.

```sh
cd frontend
npm install          # установка зависимостей (один раз)
npm run gen:api      # генерация строгих TS-типов из api/openapi.yaml (обязательно после изменений в бэке)
npm run dev          # запуск Vite-сервера на http://localhost:5173
```
| Команда | Что делает |
|---|---|
| `npm run dev` | Запуск dev-сервера 
| `npm run gen:api` | Генерация TypeScript-интерфейсов из OpenAPI-контракта через openapi-typescript
| `npm run build` | Продакшен-сборка (TypeScript check + Vite build) в папку frontend/dist
| `npm run preview` | Локальный просмотр собранного продакшен-билда
| `npm run lint` | Проверка кода через ESLint (TypeScript + React правила)

На странице логина введите специальные email-адреса, чтобы получить нужную роль: \
`borrower@test.com` - войдёте как Заёмщик (с верифицированной компанией).\
`lender@test.com `- войдёте как Кредитор.\
`admin@test.com` - войдёте как Администратор.

## Для разработчиков

Один раз после клонирования:

```sh
make hooks   # ставит golangci-lint, lefthook, actionlint и git-хуки из lefthook.yml
```

- **pre-commit:** gofmt и golangci-lint по изменённым `.go`, actionlint по workflow, vacuum по `api/**`.
- **pre-push:** `go vet`, `go mod tidy -diff` и `go test -short`.
- Пропустить разово: `LEFTHOOK=0 git commit ...`. Хуки — быстрая обратная связь, окончательная проверка в CI (`.github/workflows/backend.yml`).
