# BE-12 · Админ: верификация, модерация, пользователи

Issue: #28 · Milestone: M3 · Зависит от: BE-04 (`Principal`, `x-roles: [admin]`), BE-05 (переходы KYC, запрос бенефициаров), BE-07 (переходы заявки, `status_history.go`), BE-11 (`Notifier`), BE-13 (AML-флаги), BE-14 (шаг 1 — хелпер аудита), BE-06 (`mock` в enum `RegistryLookupResult.source`) · Блокирует: BE-16, US-14 (верификация на демо)

## Цель
Админ видит очередь компаний и полную карточку компании: документы, бенефициары, AML-флаги, пользователи, последняя сверка с реестром. Он верифицирует или отклоняет KYC — при открытом `high`-флаге approve → 409. Он публикует или возвращает заявки, блокирует и разблокирует пользователей. Каждое решение пишет `status_history`, `audit_log` и уведомление.

## Что уже есть
- Контракт: `api/paths/admin.yaml` — 9 операций задачи (плюс `adminListAmlFlags`/`adminResolveAmlFlag` — BE-13, `adminAnalyticsSummary`/`adminListAuditLog` — BE-14). Тела: `AdminComment` (необязательный `comment`) и `AdminReason` (`reason`, `minLength: 3`) — `api/schemas/errors.yaml`. Ответ карточки — `OrganizationDetails` (`api/schemas/organizations.yaml`: `Organization` + `documents`, `aml_flags`, `users`, `registry_check`).
- Заглушки 501: `backend/internal/http/handler/admin.go`.
- Запросы:
  - `backend/queries/organizations.sql`: `GetOrganizationByID`, `ListOrganizations` / `CountOrganizations` (`narg` kyc_status, type, country_code, q), `ApproveKyc`, `RejectKyc`;
  - `backend/queries/applications.sql`: `GetApplicationByID` (BE-04 заменяет на `GetApplicationVisible`), `ApproveApplication`, `RejectApplication` (причина → `moderation_comment`), `CreateStatusHistory`;
  - `backend/queries/users.sql`: `GetUserByID`, `ListUsers` / `CountUsers`, `BlockUser`, `UnblockUser`;
  - `backend/queries/sessions.sql`: `DeleteSessionsByUserID`;
  - `backend/queries/kyc.sql`: `ListKycDocumentsByOrg`, `GetLatestRegistryCheck`;
  - `backend/queries/admin.sql`: `ListAmlFlags` (с `LIMIT/OFFSET`), `CreateAuditLog`.
- `GetSessionByTokenHash` уже требует `u.status = 'active'`, поэтому заблокированный пользователь теряет доступ сразу, ещё до удаления сессий. Удаление сессий (требование issue) — чистка, а не единственная защита.
- ⚠️ **`ListOrganizations` принимает один `kyc_status`, а контракт — массив** (`style: form, explode: true`). Нужен `= ANY(…)`.
- ⚠️ **`ApproveKyc`, `RejectKyc`, `ApproveApplication`, `RejectApplication` обновляют строку только по `id`**, без проверки текущего статуса (`docs/db/schema.md`, «Расхождения»). Без блокировки два параллельных решения проскочат проверку сервиса.
- ⚠️ `ApproveKyc`/`RejectKyc` возвращают урезанный набор колонок (нет `country_code`, `registration_number`, `legal_address`, `created_at`), а ответ `Organization` требует их и `beneficiaries`. После UPDATE организацию нужно перечитать.
- ⚠️ Запросов нет: пользователи организации, AML-флаги организации без пагинации, число открытых `high`-флагов, бенефициары (BE-05), блокировка строки организации или заявки по `id`. Существующий `GetApplicationForUpdate` ищет заявку **по id оффера** — для админа не подходит.
- ⚠️ `AmlFlag.organization` — обязательный `OrganizationShort`, а `aml_flags` его не хранит.
- ⚠️ `RegistryLookupResult.source` в контракте — `enum [egrul_dadata, gleif]`, а в БД `registry_source` есть ещё `mock`. Сверка из мок-реестра (BE-06, демо) отдала бы значение вне enum. Решено (DECISIONS п. 18): BE-06 добавляет `mock` в enum, карточка отдаёт `source` как есть, без маппинга. Если BE-06 ещё не влит — добавь `mock` в enum здесь (`api/schemas/organizations.yaml`, `make gen-api`, `make diff-api`) и отметь в PR.
- ⚠️ В контракте `adminBlockUser` нет ответов `400` и `409`, хотя тело обязательно: валидатор всё равно вернёт 400.
- ⚠️ `ListUsers` и `ListOrganizations` подставляют `q` в `ILIKE '%' || q || '%'` как есть: `%` и `_` из ввода работают как шаблоны. Это не уязвимость, но поиск по `a_b` найдёт `axb`. Решено экранировать (DECISIONS п. 40) — шаг 1.

## Решения
- **Методы — в сервисе домена-результата, а не в общем `service/admin.go`** (`mds/plan.md`, «Решение по архитектуре»):
  - approve/reject KYC и карточка компании — в сервис организаций BE-05 (файл, где `SubmitKyc`);
  - approve/reject заявки — в `service/applications.go` (BE-07);
  - блокировка — новый `service/users.go`.

  Хендлеры — в `handler/admin.go` (пакет на тег, ADR-0003).
- **Переход статуса = `SELECT … FOR UPDATE` → функция переходов сущности → UPDATE**, в одной транзакции с `status_history`, `audit_log` и уведомлением. Функции и хелперы создают владельцы (ADR-0001: одна функция переходов на сущность):
  - KYC (BE-05, `docs/backend_tasks/BE-05-kyc.md`): `nextKycStatus(from, KycApprove|KycReject)`, запрос `GetOrganizationForUpdate`, `ListBeneficiariesByOrg`;
  - заявка (BE-07, `BE-07-applications.md`): `NextApplicationStatus(from, approve|reject)`, `ApproveApplication`/`RejectApplication` с параметром `from_status`;
  - история (BE-07): `recordStatusChange(ctx, q, StatusChange{…})` в `service/status_history.go`.

  Если BE-12 пришла раньше владельца, сделать минимальную версию под этими именами в файле владельца и отметить это в PR.
- **Открытые `high`-флаги при approve KYC → 409 `CONFLICT`** с сообщением «У компании есть открытые AML-флаги высокой серьёзности». По правилу 409 (DECISIONS п. 6) это не недопустимый переход статуса, а данные, которые мешают решению; фронт различает случаи по `code`. В описание `adminApproveKyc` в `api/paths/admin.yaml` дописать код `CONFLICT`.
- **Публикацию заявки открытые `high`-флаги не блокируют** (DECISIONS п. 13): флаги видны на экране модерации (`adminListAmlFlags?application_id=`), решение за админом. В отличие от KYC, `AdminApprove` заявки флаги не считает.
- **Нельзя заблокировать администратора, в том числе себя → 403 `FORBIDDEN`.** Иначе можно остаться без админа, а 403 в контракте уже есть.
- **Блокировка идемпотентна:** повторный block → 200 без новой записи в `status_history`. Для блокировки нет `NotificationType` — уведомление не шлём, заблокированный всё равно не войдёт.
- **Причина отказа видна компании:**
  - KYC — `organizations.kyc_reject_reason` (поле `Organization.kyc_reject_reason` в `getMyOrganization` и `/auth/me`);
  - заявка — `applications.moderation_comment`.

  Необязательный `comment` при approve пишется только в `status_history.comment`.
- **`actor_role` в `status_history` = `admin`, `actor_user_id` = `p.UserID`.**

## Шаги

### 1. SQL
`backend/queries/organizations.sql`:
- `ListOrganizations` / `CountOrganizations`: заменить условие на `(sqlc.narg('kyc_statuses')::text[] IS NULL OR kyc_status::text = ANY(sqlc.narg('kyc_statuses')::text[]))`. Массив `text[]`, а не `kyc_status[]`: pgx v5 не кодирует массив незарегистрированного enum.
- `GetOrganizationForUpdate` добавляет BE-05. Если его ещё нет — добавить: те же колонки, что у `GetOrganizationByID`, плюс `FOR UPDATE`.
- `ApproveKyc`, `RejectKyc`: добавить в `WHERE` `AND kyc_status = 'under_review'` — страховка к блокировке из шага 3 (так же BE-05 охраняет `SubmitKycForReview`, BE-07 — переходы заявок). 0 строк → `apperr.InvalidTransition`, а не `pgerr.Map` → 404.

`backend/queries/applications.sql`: добавить `LockApplicationByID :one` — `SELECT * FROM applications WHERE id = $1 FOR UPDATE`. Это админский аналог `LockApplicationForBorrower` из BE-07: у админа нет организации, фильтра по владельцу не будет.

Поиск `q` в `ListUsers`/`CountUsers` и `ListOrganizations`/`CountOrganizations`: `ILIKE '%' || sqlc.narg('q') || '%' ESCAPE '\'`. Сервис перед вызовом экранирует ввод `escapeLike(q string) string` (`internal/service/like.go`: `strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)`), DECISIONS п. 40.

`backend/queries/users.sql`: добавить `ListUsersByOrganization :many` — те же колонки, что у `ListUsers`, `WHERE organization_id = $1 ORDER BY created_at`.

`backend/queries/admin.sql`:
- `ListAmlFlagsByOrganization :many` — `SELECT f.*, o.name AS organization_name, o.country_code AS organization_country_code FROM aml_flags f JOIN organizations o ON o.id = f.organization_id WHERE f.organization_id = $1 ORDER BY (f.status = 'open') DESC, f.created_at DESC`;
- `CountOpenHighAmlFlags :one` — `SELECT count(*) FROM aml_flags WHERE organization_id = $1 AND status = 'open' AND severity = 'high'`.

Бенефициары — `ListBeneficiariesByOrg` из BE-05. Если его нет — добавить в `backend/queries/organizations.sql` и отметить в PR.

**Готово:** `make gen-check` без диффа; в `querier.go` есть все новые методы, `ListOrganizationsParams.KycStatuses` — `[]string`.

### 2. Списки и карточка
- Сервис организаций: `AdminList(ctx, p Principal, f OrganizationFilter, limit, offset)` → `ListOrganizations` + `CountOrganizations`.
- `AdminGet(ctx, p Principal, id uuid.UUID) (OrganizationDetails, error)`:
  1. `GetOrganizationByID` (`ErrNoRows` → `pgerr.Map` → 404);
  2. бенефициары, `ListKycDocumentsByOrg`, `ListAmlFlagsByOrganization`, `ListUsersByOrganization`;
  3. `GetLatestRegistryCheck` (`ErrNoRows` → `registry_check: null`, не 404).
- `service/users.go`: `AdminList(ctx, p, f UserFilter, limit, offset)` → `ListUsers` + `CountUsers`.
- Хендлеры `AdminListOrganizations`, `AdminGetOrganization`, `AdminListUsers` в `handler/admin.go`. `password_hash` в ответ не попадает: `ListUsers` его не выбирает, `GetUserByID` — выбирает, поэтому маппить только поля `User` из контракта.

**Готово:**
- `GET /api/v1/admin/organizations?kyc_status=under_review&kyc_status=rejected` под admin → 200, в `items` только эти статусы;
- `GET /api/v1/admin/organizations/{id}` → 200, все 4 массива присутствуют (пустые — `[]`, не `null`), `registry_check: null` без сверки;
- тест хендлера с фейком зелёный.

### 3. Решения по KYC
Сервис организаций, одна `InTx` на каждый метод:
- `ApproveKyc(ctx, p, orgID, comment *string, meta ClientMeta) (Organization, error)`:
  1. `GetOrganizationForUpdate` (404);
  2. `nextKycStatus(status, KycApprove)` (не `under_review` → `apperr.InvalidTransition` → 409);
  3. `CountOpenHighAmlFlags > 0` → `apperr.Conflict(...)`;
  4. `ApproveKyc`;
  5. `recordStatusChange` (`organization`, `under_review → verified`, actor admin, comment);
  6. `Notifier.NotifyOrganization(kyc_approved)`;
  7. аудит `kyc.approve` (хелпер BE-14);
  8. перечитать организацию с бенефициарами для ответа.
- `RejectKyc(ctx, p, orgID, reason string, meta)` — то же с `KycReject`, проверки флагов нет, `RejectKyc(id, reason)`, `status_history.comment = reason`, уведомление `kyc_rejected`, аудит `kyc.reject` с `meta.reason`.

Порядок «проверка перехода → проверка флагов» важен: approve уже верифицированной компании отвечает `INVALID_STATUS_TRANSITION`, а не `CONFLICT`.

**Готово:** табличный тест `TestApproveKyc` на фейковом querier: `under_review` без флагов → OK и вызваны `ApproveKyc`, `CreateStatusHistory`, `CreateNotificationsForOrganization`, `CreateAuditLog`; `under_review` + 1 high → `CodeConflict`, `ApproveKyc` не вызван; `verified` → `CodeInvalidTransition`; нет строки → `CodeNotFound`. `TestRejectKyc` — аналогично.

### 4. Модерация заявок
`service/applications.go`:
- `AdminApprove(ctx, p, appID, comment *string, meta)`:
  1. `LockApplicationByID` (`ErrNoRows` → 404);
  2. `NextApplicationStatus(cur, approve)` (не `moderation` → 409);
  3. `ApproveApplication(id, from_status=cur)`;
  4. `recordStatusChange` (`application`, `moderation → published`, comment);
  5. уведомление `application_published` организации-заёмщику (`borrower_organization_id`);
  6. аудит `application.approve`.
- `AdminReject(ctx, p, appID, reason, meta)` — `NextApplicationStatus(cur, reject)`, `RejectApplication(id, reason, from_status)`, уведомление `application_rejected`, аудит `application.reject`.

Ответ — `Application` контракта. Маппинг строки в `Application` делает BE-07; переиспользовать его.

**Готово:** `TestAdminApproveApplication`: `moderation` → OK; `draft`/`published` → `CodeInvalidTransition`; нет строки → `CodeNotFound`. Интеграционно: после approve `GET /applications?status=published` под lender содержит заявку.

### 5. Блокировка пользователей
`service/users.go`:
- `Block(ctx, p, userID, reason string, meta) (User, error)`, одна `InTx`:
  1. `GetUserByID` (404);
  2. `role = admin` → `apperr.Forbidden("Нельзя заблокировать администратора")`;
  3. уже `blocked` → вернуть пользователя без записей;
  4. `BlockUser(id, reason)`;
  5. `DeleteSessionsByUserID`;
  6. `recordStatusChange` (`user`, `active → blocked`, comment = reason);
  7. аудит `user.block`.
- `Unblock(ctx, p, userID, meta)` — симметрично (`UnblockUser`, история `blocked → active`, аудит `user.unblock`). Сессии не восстанавливаются: пользователь входит заново.

**Готово:** интеграционный тест `TestBlockUserDeletesSessions`:
- у пользователя 2 сессии, `Block` → `SELECT count(*) FROM sessions WHERE user_id = $1` = 0;
- запрос со старой cookie → 401;
- `Unblock` → `POST /auth/login` → 200.

### 6. Хендлеры решений и контракт
- `handler/admin.go`: `AdminApproveKyc`, `AdminRejectKyc`, `AdminApproveApplication`, `AdminRejectApplication`, `AdminBlockUser`, `AdminUnblockUser`. `meta` — `middleware.ClientMetaFromContext` (PR #67), `Principal` — `currentPrincipal(ctx)` (BE-04, `handler/principal.go`).
- `api/paths/admin.yaml`:
  - в `description` у `adminApproveKyc` указать «409 `CONFLICT` — открытые high-флаги, 409 `INVALID_STATUS_TRANSITION` — не `under_review`»;
  - у `adminBlockUser` добавить ответ `'400': BadRequest`.
  - `make gen-api`, `make lint-api`, `make diff-api` (добавление ответа не ломающее).
- Роли (`x-roles`, BE-04): все операции тега `admin` — `[admin]`.

**Готово:** `httptest`-тесты: `POST /admin/organizations/{id}/kyc/reject` с `{"reason":"no"}` → 400 `VALIDATION_ERROR` (`field: reason`); при `CodeConflict` от фейка → 409 `CONFLICT`; под borrower (после BE-04) → 403 `FORBIDDEN`.

### 7. Документация
`backend/README.md`, раздел «admin»:
- 409 `CONFLICT` при high-флагах;
- блокировка админа → 403, блокировка удаляет сессии;
- где компания видит причину отказа.

Отметить 9 операций в «Статус реализации».

**Готово:** раздел совпадает с кодом ошибок в тестах шагов 3–6.

## Тесты
| Случай | Ожидание |
|---|---|
| `GET /admin/organizations` под lender | 403 `FORBIDDEN` |
| `GET /admin/organizations/{random uuid}` | 404 `NOT_FOUND` |
| `GET /admin/organizations?kyc_status=bogus` | 400 `VALIDATION_ERROR`, `field: kyc_status` |
| approve KYC, `under_review`, без флагов | 200, `kyc_status=verified`, `verified_at` заполнен; в `status_history` строка `under_review→verified`; у пользователей компании `kyc_approved`; в `audit_log` — `kyc.approve` с IP и User-Agent |
| `escapeLike`: `a_b%c\` | `a\_b\%c\\`; интеграционно `GET /admin/users?q=a_b` не находит `axb@…` |
| approve KYC, открытый `high`-флаг | 409 `CONFLICT`, статус остался `under_review`, записей в history/audit/notifications нет |
| approve KYC, флаг `high` закрыт как `false_positive` | 200 |
| approve KYC, `draft` | 409 `INVALID_STATUS_TRANSITION` |
| reject KYC без тела | 400 `VALIDATION_ERROR` |
| reject KYC с `reason` | 200, `kyc_reject_reason` = reason; `GET /organizations/me` под компанией показывает причину |
| два параллельных approve одной компании (интеграционный) | один 200, второй 409 `INVALID_STATUS_TRANSITION`; в `status_history` одна строка |
| approve заявки, `moderation` | 200, `status=published`, `published_at` заполнен, уведомление `application_published` |
| reject заявки | 200, `moderation_comment` = reason, уведомление `application_rejected` |
| block borrower | 200 `status=blocked`; сессий 0; старая cookie → 401; `POST /auth/login` → отказ |
| block admin (себя или другого) | 403 `FORBIDDEN` |
| block уже заблокированного | 200, новых строк в `status_history` нет |
| unblock | 200 `status=active`, login → 200 |

## Готово, когда
- [ ] `adminGetOrganization` отдаёт документы, бенефициары, AML-флаги (с `organization`), пользователей и последний `registry_check` (или `null`).
- [ ] Approve KYC при открытом `high`-флаге → 409 `CONFLICT`, состояние БД не меняется (интеграционный тест).
- [ ] Reject KYC и reject заявки без `reason` → 400; причина видна компании (`kyc_reject_reason`, `moderation_comment`).
- [ ] Блокировка удаляет все сессии пользователя (тест считает строки `sessions`), разблокировка позволяет войти снова.
- [ ] Каждое решение (approve/reject KYC, approve/reject заявки, block/unblock) пишет `status_history` и `audit_log`, а KYC и заявки — ещё и уведомление; это проверено тестами сервиса.
- [ ] И общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: закрыты в [DECISIONS.md](DECISIONS.md) — `mock` в `source` (п. 18), публикация при `high`-флагах (п. 13), экранирование `q` (п. 40).
