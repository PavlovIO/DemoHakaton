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
make db                # Postgres + миграции + мок-санкционный список + демо-данные
make run               # API на http://localhost:8080/api/v1
curl localhost:8080/api/v1/healthz
```

Переменные окружения — в `.env.example`. Для входа важны две: `SESSION_TTL` — срок жизни сессии (по умолчанию `24h`, не продлевается) и `COOKIE_SECURE` — флаг `Secure` у cookie (по умолчанию `true`; в `.env.example` — `false`, иначе браузер не сохранит cookie на `http://localhost`).

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

Загруженные KYC-документы сервер хранит в каталоге `DOCUMENT_STORAGE_DIR` (по умолчанию `storage/documents` в корне репо, в `.gitignore`; права `0700`, файлы `0600` под случайными именами). Каталог должен переживать перезапуски: когда появится контейнер бэкенда — named volume на этот путь. Сервер не стартует, если в каталог нельзя писать.

Как добавить реализацию операции API — см. комментарий в [`backend/internal/http/handler/handler.go`](backend/internal/http/handler/handler.go); образец — справочники (`dictionaries.go` + `dictionaries_test.go`). Сервер сгенерирован по пакету на тег (`backend/internal/http/api/<tag>api`), модели — общий пакет `api`, реализации — `backend/internal/http/handler/<tag>.go` ([ADR-0003](docs/adr/0003-server-package-per-tag.md)). Бизнес-логика — ядро `backend/internal/service` и пакеты доменов `auth`, `kyc`, `lending`, `notifications`, `dictionaries` ([ADR-0005](docs/adr/0005-service-packages-by-domain.md)).

## Для разработчиков

Один раз после клонирования:

```sh
make hooks   # ставит golangci-lint, lefthook, actionlint и git-хуки из lefthook.yml
```

- **pre-commit:** gofmt и golangci-lint по изменённым `.go`, actionlint по workflow, vacuum по `api/**`.
- **pre-push:** `go vet`, `go mod tidy -diff` и `go test -short`.
- Пропустить разово: `LEFTHOOK=0 git commit ...`. Хуки — быстрая обратная связь, окончательная проверка в CI (`.github/workflows/backend.yml`).
