# Liquidity_platform

[![backend](https://github.com/MaxPastukhov15/Liquidity_platform/actions/workflows/backend.yml/badge.svg)](https://github.com/MaxPastukhov15/Liquidity_platform/actions/workflows/backend.yml)
Требуется разработать MVP цифровой платформы ликвидности для трансграничного финансирования между юридическими лицами — площадки, соединяющей российские компании, ищущие зарубежное финансирование, и зарубежных кредиторов со свободными средствами.  

## Документация

| Что | Где |
|---|---|
| Контракт API (источник правды для фронта и бэка) | [`api/openapi.yaml`](api/openapi.yaml) — бандл, собирается из [`api/openapi.root.yaml`](api/openapi.root.yaml), [`api/paths/`](api/paths), [`api/components.yaml`](api/components.yaml) и [`api/schemas/`](api/schemas) |
| Архитектура и ключевые решения | [`docs/adr/0001-architecture-and-api-contract.md`](docs/adr/0001-architecture-and-api-contract.md), [`docs/adr/0002-spec-layout-and-request-validation.md`](docs/adr/0002-spec-layout-and-request-validation.md) (раскладка контракта, валидация запросов) |
| Схема БД, ER-диаграмма, ключевые запросы | [`docs/db/schema.md`](docs/db/schema.md) |
| Миграции (goose) | [`backend/migrations`](backend/migrations) |
| Бэклог (источник для issues) | [`scripts/issues.yaml`](scripts/issues.yaml) |

Задачи разбиты на вехи M1–M4. Пользовательские сценарии помечены `user-story` и ссылаются на задачи фронта и бэка.

## Запуск бэкенда

Нужны Go 1.26+ и Docker. Точная версия Go для сборки и кодгена зафиксирована в `backend/go.mod` (`toolchain`), Go скачает её сам.

```sh
cp .env.example .env   # один раз
make db                # Postgres + миграции + демо-данные
make run               # API на http://localhost:8080/api/v1
curl localhost:8080/api/v1/healthz
```

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

Как добавить реализацию операции API — см. комментарий в [`backend/internal/http/handler/handler.go`](backend/internal/http/handler/handler.go); образец — справочники (`dictionaries.go` + `dictionaries_test.go`). Сервер сгенерирован по пакету на тег (`backend/internal/http/api/<tag>api`), модели — общий пакет `api`, реализации — `backend/internal/http/handler/<tag>.go` ([ADR-0003](docs/adr/0003-server-package-per-tag.md)).

## Для разработчиков

Один раз после клонирования:

```sh
make hooks   # ставит golangci-lint, lefthook, actionlint и git-хуки из lefthook.yml
```

- **pre-commit:** gofmt и golangci-lint по изменённым `.go`, actionlint по workflow, vacuum по `api/**`.
- **pre-push:** `go vet`, `go mod tidy -diff` и `go test -short`.
- Пропустить разово: `LEFTHOOK=0 git commit ...`. Хуки — быстрая обратная связь, окончательная проверка в CI (`.github/workflows/backend.yml`).
