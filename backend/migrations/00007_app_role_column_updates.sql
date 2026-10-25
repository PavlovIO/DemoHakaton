-- +goose Up
-- UPDATE роли lp_app — только по столбцам, которые меняют UPDATE-запросы backend/queries/*.sql.
-- Ошибка в коде или SQL-инъекция под lp_app не поднимет роль пользователя (users.role), не сменит
-- пароль, тип или страну компании, условия оффера и владельца заявки. Новый UPDATE по другому
-- столбцу — добавь его в GRANT здесь же (новой миграцией): без права запрос упадёт с 42501 в
-- интеграционных тестах сервиса. SELECT … FOR UPDATE хватает UPDATE на любой столбец таблицы.

REVOKE UPDATE ON organizations, users, applications, offers, notifications, aml_flags, message_reads FROM lp_app;

GRANT UPDATE (name, registration_number, legal_address, lei,
              kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, updated_at)
    ON organizations TO lp_app;
GRANT UPDATE (status, blocked_reason, last_login_at, updated_at) ON users TO lp_app;
GRANT UPDATE (amount, currency, target_country_code, purpose, term_months, transfer_method,
              transfer_method_comment, status, moderation_comment, accepted_offer_id,
              submitted_at, published_at, updated_at)
    ON applications TO lp_app;
GRANT UPDATE (status, reject_reason, reject_comment, decided_at) ON offers TO lp_app;
GRANT UPDATE (is_read) ON notifications TO lp_app;
GRANT UPDATE (status, resolved_by_user_id, resolution_comment, resolved_at) ON aml_flags TO lp_app;
GRANT UPDATE (last_read_message_id) ON message_reads TO lp_app;

-- +goose Down
REVOKE UPDATE (name, registration_number, legal_address, lei,
               kyc_status, kyc_reject_reason, kyc_submitted_at, verified_at, updated_at)
    ON organizations FROM lp_app;
REVOKE UPDATE (status, blocked_reason, last_login_at, updated_at) ON users FROM lp_app;
REVOKE UPDATE (amount, currency, target_country_code, purpose, term_months, transfer_method,
               transfer_method_comment, status, moderation_comment, accepted_offer_id,
               submitted_at, published_at, updated_at)
    ON applications FROM lp_app;
REVOKE UPDATE (status, reject_reason, reject_comment, decided_at) ON offers FROM lp_app;
REVOKE UPDATE (is_read) ON notifications FROM lp_app;
REVOKE UPDATE (status, resolved_by_user_id, resolution_comment, resolved_at) ON aml_flags FROM lp_app;
REVOKE UPDATE (last_read_message_id) ON message_reads FROM lp_app;

GRANT UPDATE ON organizations, users, applications, offers, notifications, aml_flags, message_reads TO lp_app;
