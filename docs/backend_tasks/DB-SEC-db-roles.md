# DB-SEC · Роли и права в Postgres

Issue: нет (раздел 2 плана владельца, относится к критерию «защита данных» из BE-15) · Milestone: M4 · Зависит от: — · Блокирует: —

## Цель

Приложение и интеграционные тесты подключаются к Postgres под ролью `lp_app` с минимальными DML-правами, goose — под `lp_migrator`, владельцем схемы. Под `lp_app` нельзя выполнить DDL, переписать `audit_log`, изменить сделку или выполнить `COPY … FROM PROGRAM`; тяжёлый запрос обрывается через 5 секунд.

## Что уже есть

- Приложение, goose, тесты и CI ходят под `lp` — это `POSTGRES_USER` контейнера: суперпользователь и владелец всех таблиц. В репозитории нет ни одного `GRANT`, `REVOKE` или `CREATE ROLE`.
- `Makefile`: `DATABASE_URL` с пользователем `lp`; `migrate-*` берут `$(DATABASE_URL)`; `PSQL` — `docker exec … psql -U $(POSTGRES_USER)`; `db: up migrate-up test-invariants seed-demo`; образец копирования SQL в контейнер — цель `seed-demo`.
- `.env.example`, `.github/workflows/backend.yml` (job `test`, сервис `postgres:16-alpine`, шаг `migrate`) — везде `lp:lp_dev_password`.
- `backend/cmd/server/main.go: newPool` — парсит `DATABASE_URL`, делает `Ping`.
- `backend/internal/service/dictionaries_integration_test.go: newPool(t)` — пул для интеграционных тестов, пропуск без `DATABASE_URL` или в `-short`.
- Миграции: `00001_init.sql`, `00002_seed_dictionaries.sql`. В `public` 19 таблиц, у каждой есть описание в `docs/db/tables/`.
- ⚠️ Другие задачи тоже добавляют миграции (BE-02 — `dictionaries_full`, полные справочники; BE-13 — возможно). Номер миграции бери следующий свободный на момент PR, а не `00003` вслепую.

## Решения

- Три роли (источник — `mds/db-security-plan.md`):

  | Роль | Атрибуты | Для чего |
  |---|---|---|
  | `lp` (как есть) | superuser | только bootstrap ролей, `make psql`, сид и invariants-тест в dev |
  | `lp_migrator` | LOGIN, без SUPERUSER/CREATEDB/CREATEROLE/BYPASSRLS; **владелец БД**, поэтому в PG16 владелец схемы `public` (`pg_database_owner`) | goose; загрузчик санкций `make load-sanctions` из BE-13. Расширения pgcrypto/citext/pg_trgm trusted — суперпользователь не нужен |
  | `lp_app` | LOGIN, без атрибутов, `CONNECTION LIMIT 50` | приложение и интеграционные тесты |

- Настройки ролей через `ALTER ROLE … SET`, чтобы их не мог забыть код:
  - `lp_app`: `statement_timeout=5s`, `lock_timeout=2s`, `idle_in_transaction_session_timeout=15s`, `search_path=public`, `pg_trgm.similarity_threshold=0.4` (порог больше не нужно ставить на соединении — снимает проблему из `docs/db/tables/sanctions_entries.md`);
  - `lp_migrator`: `lock_timeout=10s`, чтобы миграция не висела за чужой блокировкой.
- Права `lp_app` выведены из `backend/queries/*.sql` (кто что пишет), минимум:

  | Права | Таблицы |
  |---|---|
  | `SELECT` | countries, currencies, transfer_methods, sanctions_entries |
  | `SELECT, INSERT` | **audit_log**, status_history, **deals** (неизменяемый снапшот теперь гарантирует БД), messages, registry_checks |
  | `SELECT, INSERT, UPDATE` | organizations, users, applications (`FOR UPDATE` требует UPDATE), offers, notifications, aml_flags, message_reads (upsert) |
  | `SELECT, INSERT, UPDATE, DELETE` | sessions, kyc_documents, beneficiaries (замена списка — delete+insert в BE-05) |

- `ALTER DEFAULT PRIVILEGES` не используем: новая таблица получает права только явным `GRANT` в своей же миграции, а забытый `GRANT` ловит тест полноты (шаг 5).
- Права на identity-последовательности не выдаём: для `GENERATED ALWAYS AS IDENTITY` они не нужны. Если интеграционный тест упадёт с `42501` на sequence — добавить `GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO lp_app`.
- Bootstrap ролей — идемпотентный psql-скрипт вне `migrations/`, а не init-скрипт `docker-compose`: он работает и на свежем, и на существующем контейнере, и его не видят goose и sqlc.
- Сервер на суперпользователе не падает, а пишет warning: так ловится устаревший `.env`, не ломая запуск.
- **RLS по `organization_id` в M4 не делаем** (DECISIONS п. 32): IDOR закрывают фильтры `*Visible` из BE-04 и HTTP-тесты BE-15; RLS — возможная вторая линия защиты после M4.
- **Пароли ролей на стенде — переменные `LP_APP_PASSWORD` / `LP_MIGRATOR_PASSWORD`** (DECISIONS п. 32) из секретов CI/хостинга. Dev-дефолты — только в `Makefile` и `.env.example`.

## Шаги

Ветка `feat/db-roles` от `origin/main`. Коммиты по шагам: роли + миграция + Makefile/env; CI; `main.go`; тест; документация.

### 1. Bootstrap-скрипт ролей

Создать `backend/db/roles.sql`, запускается суперпользователем через `psql -v ON_ERROR_STOP=1`:
- создание ролей идемпотентно: `SELECT 'CREATE ROLE lp_app LOGIN' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'lp_app') \gexec`, то же для `lp_migrator`;
- `ALTER ROLE … PASSWORD :'app_password'` / `:'migrator_password'`, атрибуты и `SET`-настройки из «Решений»;
- `ALTER DATABASE :"DBNAME" OWNER TO lp_migrator`;
- `REVOKE CONNECT, TEMPORARY ON DATABASE :"DBNAME" FROM PUBLIC`, `GRANT CONNECT` обеим ролям; `REVOKE CREATE ON SCHEMA public FROM PUBLIC` (в PG16 уже так, пишем явно).

**Готово:** скрипт дважды подряд выполняется на одном контейнере без ошибок; `\du` показывает обе роли с нужными атрибутами.

### 2. Миграция с правами `lp_app`

`make migrate-create name=app_role_grants`, затем переименовать файл из таймстемпа (`goose create` без `-s`) в `0000N_app_role_grants.sql`, N — следующий свободный номер:
- Up: `GRANT USAGE ON SCHEMA public TO lp_app` и табличные `GRANT` строго по таблице из «Решений»;
- Down: `REVOKE ALL ON ALL TABLES IN SCHEMA public FROM lp_app`, `REVOKE USAGE ON SCHEMA public FROM lp_app`;
- комментарий в шапке: «добавляешь таблицу — добавь `GRANT` для `lp_app` в той же миграции».

Если роли `lp_app` нет, миграция падает — так и задумано: забытый bootstrap даёт громкую ошибку.

**Готово:** `make migrate-reset && make migrate-up` под `lp_migrator` проходит; `make gen-sqlc` не даёт диффа (GRANT не ломает парсер sqlc).

### 3. Makefile и `.env.example`

- `Makefile`: `DATABASE_URL ?= postgres://lp_app:lp_app_dev_password@…`; новый `MIGRATE_DATABASE_URL ?= postgres://lp_migrator:lp_migrator_dev_password@…`; `LP_APP_PASSWORD`, `LP_MIGRATOR_PASSWORD` с dev-дефолтами; все `migrate-*` → `$(MIGRATE_DATABASE_URL)`.
- Цель `db-roles` по образцу `seed-demo`: `docker cp backend/db/roles.sql` в контейнер + `$(PSQL) -v app_password=$(LP_APP_PASSWORD) -v migrator_password=$(LP_MIGRATOR_PASSWORD) -f /tmp/roles.sql`; добавить в `.PHONY`.
- `db: up db-roles migrate-up test-invariants seed-demo`; BE-13 вставляет `load-sanctions` после `migrate-up` — итог `up db-roles migrate-up load-sanctions test-invariants seed-demo`. `seed-demo` и `test-invariants` остаются под суперпользователем через `docker exec` — это dev.
- `.env.example`: `DATABASE_URL` под `lp_app`, `MIGRATE_DATABASE_URL`, два пароля; комментарий, что `POSTGRES_USER` теперь только для администрирования. `docker-compose.yml` не меняется.

**Готово:** на удалённом volume `make db-reset` проходит целиком: роли, миграции под `lp_migrator`, invariants, сид.

### 4. CI

`.github/workflows/backend.yml`, job `test`, перед шагом `migrate`: шаг `db roles` — `psql "postgres://lp:lp_dev_password@localhost:5432/liquidity_platform" -v ON_ERROR_STOP=1 -v app_password=… -v migrator_password=… -f backend/db/roles.sql` (psql есть на ubuntu-раннере). В `env` job: `DATABASE_URL` под `lp_app`, `MIGRATE_DATABASE_URL` под `lp_migrator`.

**Готово:** `make lint-actions` зелёный; на PR job `test` проходит с новым шагом.

### 5. Проверка при старте сервера

`backend/cmd/server/main.go: newPool`:
- `poolCfg.ConnConfig.RuntimeParams["application_name"] = "lp-backend"` — соединения видны в `pg_stat_activity`;
- после `Ping` — `SELECT rolsuper FROM pg_roles WHERE rolname = current_user`; `slog.Warn`, если суперпользователь;
- `slog.Warn`, если хост не `localhost`/`127.0.0.1`, а `poolCfg.ConnConfig.TLSConfig == nil` (`sslmode=disable`). `sslmode=require`/`verify-full` на стенде задаётся в `DATABASE_URL`, кода не требует.

**Готово:** `make run` с `DATABASE_URL` под `lp` пишет warning про суперпользователя; под `lp_app` — не пишет; `SELECT usename, application_name FROM pg_stat_activity` показывает `lp_app` / `lp-backend`.

### 6. Интеграционный тест прав

`backend/internal/service/db_privileges_integration_test.go`, переиспользует `newPool(t)`. Каждый случай — в своей транзакции с `Rollback`, чтобы под суперпользователем тест ничего не сломал:
- первым — `rolsuper` у `current_user` равен false, иначе `t.Fatal("DATABASE_URL подключается суперпользователем — обнови .env из .env.example")`;
- табличный набор, каждый ожидает SQLSTATE `42501`: `DROP TABLE audit_log`, `CREATE TABLE x(id int)`, `UPDATE audit_log …`, `DELETE FROM audit_log`, `UPDATE deals …`, `INSERT INTO countries …`, `TRUNCATE sessions`, `COPY … FROM PROGRAM 'id'`;
- `SHOW statement_timeout` = `5s`;
- полнота: для каждой таблицы `public`, кроме `goose_db_version`, `has_table_privilege('lp_app', t, 'SELECT')` истинно.

Положенные права проверятся сами: `TestTxManagerIntegrationRollback` (INSERT+SELECT `audit_log`) и `TestDictionariesIntegration` идут под `lp_app`.

**Готово:** `make test-all` зелёный под `lp_app`; с `DATABASE_URL` под `lp` новый тест падает с сообщением из первого пункта.

### 7. Документация

- `README.md`, «Запуск бэкенда»: `make db` теперь создаёт роли; на существующем volume — `make db-reset` (таблицы принадлежат `lp`) и обновить `.env` из `.env.example`.
- `docs/db/schema.md`: раздел «Роли и права» (две таблицы из «Решений»); в «Как запустить» goose — через `$MIGRATE_DATABASE_URL`; в «Где проверяется каждый инвариант» строки «audit_log только INSERT» и «сделка неизменяема» → «БД (права `lp_app`)».
- `docs/db/tables/sanctions_entries.md`: порог `pg_trgm` задан на роли `lp_app`.
- `mds/plan.md`: отметить раздел 2.

**Готово:** каждый пункт выше отражён в диффе PR.

## Тесты

| Случай | Ожидание |
|---|---|
| `current_user` под `DATABASE_URL` | `rolsuper = false` |
| `DROP TABLE audit_log` | `42501` |
| `CREATE TABLE x(id int)` | `42501` |
| `UPDATE audit_log SET action = 'x'` / `DELETE FROM audit_log` | `42501` |
| `UPDATE deals SET fee_amount = 0` | `42501` |
| `INSERT INTO countries …` | `42501` |
| `TRUNCATE sessions` | `42501` |
| `COPY (SELECT 1) TO PROGRAM 'id'` | `42501` |
| `SHOW statement_timeout` | `5s` |
| каждая таблица `public` без `goose_db_version` | `has_table_privilege('lp_app', t, 'SELECT')` |

## Готово, когда

- [ ] `make db-reset` с нуля и повторный `make db-roles` проходят без ошибок.
- [ ] `make migrate-reset && make migrate-up` под `lp_migrator` работают в обе стороны.
- [ ] `make test-all` зелёный под `lp_app`, включая новый тест прав.
- [ ] `make run` + `curl localhost:8080/api/v1/healthz` и `/api/v1/dictionaries/countries` отвечают 200 под `lp_app`.
- [ ] CI на PR зелёный с шагом `db roles`.
- [ ] и общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: пароли ролей и RLS закрыты в [DECISIONS.md](DECISIONS.md), п. 32.
