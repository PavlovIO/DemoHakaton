-- +goose Up
-- Права роли приложения lp_app (создаёт backend/db/roles.sql; без неё миграция падает — так задумано).
-- Минимум, выведенный из backend/queries/*.sql. Добавляешь таблицу — добавь GRANT для lp_app
-- в той же миграции: ALTER DEFAULT PRIVILEGES не используем, забытый GRANT ловит
-- TestDBPrivilegesIntegration (только отсутствие SELECT: недостающие INSERT/UPDATE ловят интеграционные
-- тесты сервисов). Последовательности не нужны: id и номера — uuid или GENERATED ALWAYS AS
-- IDENTITY, а identity не требует USAGE на свою последовательность.

GRANT USAGE ON SCHEMA public TO lp_app;

-- Справочники и санкционные списки приложение только читает.
GRANT SELECT ON countries, currencies, transfer_methods, sanctions_entries TO lp_app;

-- Журналы и снапшоты только дописываются: audit_log нельзя переписать, а сделку изменить.
GRANT SELECT, INSERT ON audit_log, status_history, deals, messages, registry_checks TO lp_app;

-- applications: SELECT ... FOR UPDATE требует права UPDATE; message_reads — upsert (ON CONFLICT DO UPDATE).
GRANT SELECT, INSERT, UPDATE ON organizations, users, applications, offers, notifications, aml_flags, message_reads TO lp_app;

-- beneficiaries: список заменяется целиком (delete + insert).
GRANT SELECT, INSERT, UPDATE, DELETE ON sessions, kyc_documents, beneficiaries TO lp_app;

-- +goose Down
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM lp_app;
REVOKE USAGE ON SCHEMA public FROM lp_app;
