-- Роли БД: lp_migrator (владелец схемы, goose) и lp_app (приложение, минимальные DML-права).
-- Права lp_app на таблицы выдают миграции (00006_app_role_grants.sql и следующие), здесь — только роли и БД.
--
-- Запускает суперпользователь (POSTGRES_USER контейнера), скрипт идемпотентен:
--   psql -v ON_ERROR_STOP=1 -v app_password=... -v migrator_password=... -f backend/db/roles.sql
-- Локально — make db-roles, в CI — шаг "db roles". Лежит вне migrations/: его не видят goose и sqlc.

-- CREATE ROLE не умеет IF NOT EXISTS — создаём через \gexec только отсутствующие.
SELECT 'CREATE ROLE lp_migrator LOGIN'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'lp_migrator') \gexec
SELECT 'CREATE ROLE lp_app LOGIN'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'lp_app') \gexec

-- Атрибуты задаём явно при каждом запуске: роль, созданная или изменённая руками, приводится к нужному виду.
-- Расширения из 00001 (pgcrypto, citext, pg_trgm) trusted — владельцу БД суперпользователь не нужен.
ALTER ROLE lp_migrator WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS
    PASSWORD :'migrator_password';
-- Миграция не должна висеть за чужой блокировкой: лучше упасть и перезапустить.
ALTER ROLE lp_migrator SET lock_timeout = '10s';

ALTER ROLE lp_app WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS
    CONNECTION LIMIT 50 PASSWORD :'app_password';
-- Таймауты на роли, а не в коде: их нельзя забыть в новом соединении или пуле.
ALTER ROLE lp_app SET statement_timeout = '5s';
ALTER ROLE lp_app SET lock_timeout = '2s';
ALTER ROLE lp_app SET idle_in_transaction_session_timeout = '15s';
ALTER ROLE lp_app SET search_path = public;
-- Порог оператора % из pg_trgm для SearchSanctions: на роли, а не SET в коде — SET утекает в чужие
-- запросы пула. Инвариант: не выше aml.candidateMinSimilarity (0.4), иначе % отсечёт кандидатов раньше явного порога.
ALTER ROLE lp_app SET pg_trgm.similarity_threshold = 0.4;

-- Владелец БД в PG16 владеет и схемой public (pg_database_owner), поэтому goose под lp_migrator
-- создаёт таблицы, а lp_app — нет.
ALTER DATABASE :"DBNAME" OWNER TO lp_migrator;

-- Подключаться к БД могут только наши роли; временные таблицы lp_app не нужны.
REVOKE CONNECT, TEMPORARY ON DATABASE :"DBNAME" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"DBNAME" TO lp_migrator, lp_app;
-- В PG16 это уже так по умолчанию, пишем явно: CREATE в public — только у владельца.
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
