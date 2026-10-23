# Локальные настройки берём из .env (если есть), иначе — дефолты ниже.
-include .env
# Версии инструментов (обязательный include: без него цели с go run не соберутся).
include versions.env
export

POSTGRES_USER ?= lp
POSTGRES_DB   ?= liquidity_platform
DATABASE_URL  ?= postgres://lp:lp_dev_password@localhost:5432/liquidity_platform?sslmode=disable
PG_CONTAINER  ?= lp_postgres
# Абсолютный путь: make run запускает сервер из backend/ (go -C backend), и относительный
# storage/documents оказался бы в backend/storage — вне .gitignore.
DOCUMENT_STORAGE_DIR ?= $(CURDIR)/storage/documents
MIGRATIONS    := backend/migrations
TESTDATA      := backend/migrations/testdata

# Инструменты с зафиксированными версиями (versions.env) — ставить вручную не нужно.
# goose/vacuum/oasdiff: go run (первый запуск ~1 мин на компиляцию, дальше из кэша Go).
# sqlc: Docker-образ (go run на Windows требует cgo/gcc).
GOOSE         := go run github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
VACUUM        := go run github.com/daveshanley/vacuum@$(VACUUM_VERSION)
OASDIFF       := go run github.com/oasdiff/oasdiff@$(OASDIFF_VERSION)
# vacuum иначе ходит в сеть за проверкой новой версии при каждом запуске.
VACUUM_NO_UPDATE_CHECK := true
# Git Bash на Windows иначе превращает "-w /src" в "C:/Program Files/Git/src" (экспортируется ниже через export).
MSYS_NO_PATHCONV := 1
PSQL          := docker exec -i $(PG_CONTAINER) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB) -v ON_ERROR_STOP=1

.PHONY: db db-reset up down logs ps migrate-up migrate-down migrate-reset migrate-status migrate-create seed-demo test-invariants psql gen gen-api gen-sqlc bundle-api lint-api diff-api gen-check run build test test-all lint tidy-check lint-actions verify tools hooks

# Полный цикл: postgres + миграции + проверка инвариантов + демо-данные.
db: up migrate-up test-invariants seed-demo

# То же самое с чистой базы (удаляет volume).
db-reset:
	docker compose down -v
	$(MAKE) db

# --wait ждёт, пока healthcheck станет healthy.
up:
	docker compose up -d --wait postgres

down:
	docker compose down

logs:
	docker compose logs -f postgres

ps:
	docker compose ps

migrate-up:
	$(GOOSE) -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" up

migrate-down:
	$(GOOSE) -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" down

migrate-reset:
	$(GOOSE) -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" reset

migrate-status:
	$(GOOSE) -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" status

migrate-create:
	@echo "usage: make migrate-create name=<name>"
	$(GOOSE) -dir $(MIGRATIONS) create $(name) sql

# docker cp + psql -f, а не pipe: иначе в PowerShell кириллица превращается в mojibake.
seed-demo:
	docker cp $(TESTDATA)/seed_demo.sql $(PG_CONTAINER):/tmp/seed_demo.sql
	$(PSQL) -f /tmp/seed_demo.sql

test-invariants:
	docker cp $(TESTDATA)/invariants_test.sql $(PG_CONTAINER):/tmp/invariants_test.sql
	$(PSQL) -f /tmp/invariants_test.sql

psql:
	docker exec -it $(PG_CONTAINER) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

gen: gen-api gen-sqlc

# ───────────────────────── контракт OpenAPI ─────────────────────────
# Источник — многофайловый: api/openapi.root.yaml ссылается ($ref) на api/paths/*.yaml,
# api/components.yaml и api/schemas/*.yaml. specbundle (kin-openapi) поднимает внешние
# компоненты в один api/openapi.yaml — его и читают oapi-codegen, vacuum, oasdiff и фронт.
API_ROOT  := api/openapi.root.yaml
API_SPEC  := api/openapi.yaml
# Копия бандла для //go:embed (go:embed не умеет ../, модуль Go — backend/). Коммитится.
API_SPEC_EMBED := backend/internal/openapi/openapi.yaml
API_PARTS := $(API_ROOT) $(wildcard api/paths/*.yaml) api/components.yaml $(wildcard api/schemas/*.yaml)

# Цель НЕ файловая: после git clone у всех файлов одинаковый mtime, и make счёл бы
# openapi.yaml свежим — CI проверял бы пустоту. specbundle перезаписывает файл только
# если содержимое изменилось, поэтому лишних правок mtime нет.
bundle-api:
	go -C backend run ./cmd/specbundle -o $(abspath $(API_SPEC)) -o $(abspath $(API_SPEC_EMBED)) $(abspath $(API_ROOT))

# HTTP-слой из api/openapi.yaml (oapi-codegen — go tool из backend/go.mod), ADR-0003:
#   models.yaml -> internal/http/api/models.gen.go (все схемы, общий пакет api);
#   server.yaml -> internal/http/api/<tag>api/server.gen.go, по пакету на тег; тег и пакет
#   задаёт //go:generate в <tag>api/gen.go, поэтому новый тег не требует правок здесь.
# Отдельные строки вместо $(foreach) и "&& true": так работает и в cmd.exe (Windows), и в sh (CI).
gen-api: bundle-api
	go -C backend tool oapi-codegen -config oapi-codegen/models.yaml ../api/openapi.yaml
	go -C backend generate ./internal/http/api/...

gen-sqlc:
	docker run --rm -v "$(CURDIR):/src" -w /src sqlc/sqlc:$(SQLC_VERSION) generate -f backend/sqlc.yaml

# Линтер контракта (vacuum, правила в api/vacuum-ruleset.yaml): любое предупреждение — ошибка.
lint-api:
	$(VACUUM) lint -q -b -n warn -r api/vacuum-ruleset.yaml $(API_SPEC)

# Ломающие изменения контракта. По умолчанию сравнивает с origin/main (нужен fetch);
# BASE_SPEC=<файл> — сравнить с готовым файлом (так делает CI).
BASE_REF  ?= origin/main
BASE_SPEC ?= $(BASE_REF):$(API_SPEC)
OASDIFF_FLAGS ?=
diff-api:
	$(OASDIFF) breaking "$(BASE_SPEC)" $(API_SPEC) --fail-on ERR -l ru $(OASDIFF_FLAGS)

# Сгенерированное совпадает с источниками (то, что проверяет CI gen-check).
gen-check: gen
	git add -N api backend/internal
	git diff --exit-code --stat -- api backend/internal

# ───────────────────────── backend ─────────────────────────
# Сервер на :8080, API под /api/v1. Нужна поднятая БД (make db).
run:
	go -C backend run ./cmd/server

build:
	go -C backend build -o bin/server$(shell go env GOEXE) ./cmd/server

# Юнит-тесты, БД не нужна (интеграционные пропускаются через -short).
test:
	go -C backend test -short ./...

# Все тесты, включая интеграционные с Postgres из DATABASE_URL (сначала make db).
test-all:
	DATABASE_URL="$(DATABASE_URL)" go -C backend test -count=1 ./...

lint:
	cd backend && golangci-lint run ./...

tidy-check:
	go -C backend mod tidy -diff

lint-actions:
	actionlint

# Всё, что гоняет CI, кроме интеграционных тестов и gen-check (ему нужен Docker для sqlc).
verify: lint lint-api tidy-check lint-actions test

# ───────────────────────── инструменты разработчика ─────────────────────────
# Линтеры и git-хуки ставятся в $(go env GOPATH)/bin (версии те же, что в CI — versions.env).
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install github.com/evilmartians/lefthook/v2@$(LEFTHOOK_VERSION)
	go install github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

# Один раз после клонирования: ставит инструменты и git-хуки из lefthook.yml.
hooks: tools
	lefthook install
