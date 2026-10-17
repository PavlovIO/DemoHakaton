DATABASE_URL ?= postgres://lp:lp_dev_password@localhost:5432/liquidity_platform?sslmode=disable
PG_CONTAINER ?= lp_postgres
MIGRATIONS   := backend/migrations
TESTDATA     := backend/migrations/testdata

.PHONY: up down logs ps migrate-up migrate-down migrate-reset migrate-status migrate-create seed-demo test-invariants psql gen schema

up:
	docker compose up -d postgres

down:
	docker compose down

logs:
	docker compose logs -f postgres

ps:
	docker compose ps

migrate-up:
	goose -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" down

migrate-reset:
	goose -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" reset

migrate-status:
	goose -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" status

migrate-create:
	@echo "usage: make migrate-create name=<name>"
	goose -dir $(MIGRATIONS) create $(name) sql

seed-demo:
	docker cp $(TESTDATA)/seed_demo.sql $(PG_CONTAINER):/tmp/seed_demo.sql
	docker exec -i $(PG_CONTAINER) psql -U lp -d liquidity_platform -v ON_ERROR_STOP=1 -f /tmp/seed_demo.sql

test-invariants:
	docker cp $(TESTDATA)/invariants_test.sql $(PG_CONTAINER):/tmp/invariants_test.sql
	docker exec -i $(PG_CONTAINER) psql -U lp -d liquidity_platform -v ON_ERROR_STOP=1 -f /tmp/invariants_test.sql

psql:
	docker exec -it $(PG_CONTAINER) psql -U lp -d liquidity_platform

gen:
	sqlc generate -f backend/sqlc.yaml