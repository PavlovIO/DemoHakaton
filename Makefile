# Локальные настройки берём из .env (если есть), иначе — дефолты ниже.
-include .env
export

POSTGRES_USER ?= lp
POSTGRES_DB   ?= liquidity_platform
DATABASE_URL  ?= postgres://lp:lp_dev_password@localhost:5432/liquidity_platform?sslmode=disable
PG_CONTAINER  ?= lp_postgres
MIGRATIONS    := backend/migrations
TESTDATA      := backend/migrations/testdata

# Инструменты с зафиксированными версиями — ставить вручную не нужно.
# goose: go run (первый запуск ~1 мин на компиляцию, дальше из кэша Go).
# sqlc: Docker-образ (go run на Windows требует cgo/gcc).
GOOSE_VERSION := v3.29.0
SQLC_VERSION  := 1.31.1
GOOSE         := go run github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
PSQL          := docker exec -i $(PG_CONTAINER) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB) -v ON_ERROR_STOP=1

.PHONY: db db-reset up down logs ps migrate-up migrate-down migrate-reset migrate-status migrate-create seed-demo test-invariants psql gen

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

gen:
	docker run --rm -v "$(CURDIR):/src" -w /src sqlc/sqlc:$(SQLC_VERSION) generate -f backend/sqlc.yaml
