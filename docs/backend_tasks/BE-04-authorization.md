# BE-04 · Авторизация: роли, владение сущностями (IDOR)

Issue: #20 · Milestone: M1 · Зависит от: BE-03 (PR #67 — шаги 4–5) · Блокирует: BE-05, BE-07, BE-11, BE-15 (и через BE-07 — BE-08, BE-09, BE-10)

## Цель

Каждый запрос проходит одну и ту же цепочку проверок:

| # | Ситуация | Ответ | Где проверяется |
|---|---|---|---|
| 1 | путь или метод не из спеки | `404 NOT_FOUND` | валидатор (`handler/validate.go`) |
| 2 | операция публичная (`security: []`) | пропускается с сессией и без неё | `middleware.Authorize` |
| 3 | нет cookie `session`, сессия истекла или пользователь заблокирован | `401 UNAUTHORIZED` | `middleware.Authorize` (principal не положил `Authenticate`) |
| 4 | роли пользователя нет в `x-roles` операции | `403 FORBIDDEN` | `middleware.Authorize` |
| 5 | у операции `x-kyc-required: true`, а у компании `kyc_status != verified` | `403 KYC_NOT_VERIFIED` | `middleware.Authorize` |
| 6 | мутирующий запрос без `X-CSRF-Token` или с чужим | `403 CSRF_INVALID` | `middleware.CSRF` |
| 7 | запрос не соответствует схеме | `400 VALIDATION_ERROR` | валидатор |
| 8 | сущность чужая **или** не существует | `404 NOT_FOUND` — одинаково | SQL-запрос `*Visible` → `pgx.ErrNoRows` → `pgerr.Map` |

Роли и требование KYC записаны в спеке у каждой операции. Если у защищённой операции их забыли указать, падает тест, а не открывается ручка. Чтение заявки, оффера или сделки и списки `/applications`, `/offers`, `/deals` идут только через запросы с обязательными `viewer_role` и `viewer_org`: без организации запрос возвращает пусто, а не всё.

## Что уже есть

### Из PR #67 (BE-03), поверх него строится эта задача

- `service.Principal{UserID uuid.UUID, OrgID *uuid.UUID, Role string}` — `internal/service/principal.go`. **Поля `KycStatus` нет.**
- `middleware.Authenticate(a Authenticator)` — `internal/http/middleware/auth.go`. Отвечает `401` сам, если маршрут не входит в захардкоженную карту `publicRoutes` (7 путей, только GET и POST). `PrincipalFromContext(ctx) (service.Principal, bool)` и `SessionTokenFromContext` — там же.
- `middleware.CSRF()` — `internal/http/middleware/csrf.go`, пропускает те же `publicRoutes` через `isPublic(r)`.
- `service.Auth.Authenticate` — `internal/service/auth.go`: `sha256(cookie)` → `GetSessionByTokenHash` → `TouchSession` → `Principal`.
- Цепочка в `handler/router.go` PR: `… → WithRequestMeta → Authenticate → CSRF → validateRequests`. Ответы на вопросы стыка (формат токена, что хэшируется, кто вызывает `TouchSession`) — в [BE-03](BE-03-auth.md#стык-с-be-04-ответы-на-вопросы-из-п-11-mdsplanmd).

### Контракт и валидатор

- Глобальный `security: [cookieAuth]` — `api/openapi.root.yaml`. Публичные операции (`security: []`, 7 штук, в бандле `api/openapi.yaml` сохраняются): `register`, `login` (`paths/auth.yaml`), `listCountries`, `listCurrencies`, `listTransferMethods` (`paths/dictionaries.yaml`), `registryLookup` (`paths/organizations.yaml`), `healthz` (`paths/system.yaml`).
- Роли сейчас записаны только текстом в `summary` и `description` (`(lender, KYC verified, …)`); расширений `x-roles` и `x-kyc-required` нет ни у одной операции.
- `403` объявлен только у админских операций, `createApplication`, `createOffer`, `login`, `getMyOrganization`. У остальных операций, которые могут его вернуть (роль, KYC, CSRF), его нет.
- `handler/validate.go`: валидатор `nethttp-middleware` с `NoopAuthenticationFunc`, `Prefix: /api/v1` и `Skipper`, который **пропускает multipart-загрузку и `HEAD`**. Внутри он ищет операцию роутером `kin-openapi/routers/gorillamux`.
- `internal/openapi`: `MustDoc()` — разобранная встроенная спека, `Servers = nil`.
- kin-openapi v0.142 (`backend/go.mod`): `Operation.Security *SecurityRequirements` — `nil` значит «наследует глобальный», не-`nil` пустой — публичная операция; `Operation.Extensions map[string]any` — `x-roles: [lender]` приходит как `[]any{"lender"}`, `x-kyc-required: true` как `bool`. `gorillamux.FindRoute` возвращает `routers.ErrPathNotFound` / `ErrMethodNotAllowed` и не находит `HEAD` (см. комментарий к `skipValidation`).

### Запросы — ловушки из plan.md (проверено, все актуальны)

- ⚠️ `backend/queries/sessions.sql: GetSessionByTokenHash` джойнит только `users`: **`kyc_status` не возвращает**.
- ⚠️ `applications.sql: GetApplicationByID`, `offers.sql: GetOfferByID`, `deals.sql: GetDealByID` — `WHERE id = $1`, без владельца.
- ⚠️ `applications.sql: ListApplications` / `CountApplications`: `borrower_org_id` и `lender_org_id` — `sqlc.narg`, условие `narg IS NULL OR …`. Если оба `NULL`, возвращаются **все заявки, включая чужие черновики**.
- ⚠️ `offers.sql: ListOffers` / `CountOffers`: `lender_org_id` — `narg` (`NULL` → все офферы). **Фильтра для заёмщика («офферы на мои заявки») нет вообще.**
- ⚠️ `deals.sql: ListDeals` / `CountDeals`: оба фильтра `narg` → без них все сделки.
- Вызывающих у этих запросов нет: `grep -rn "GetApplicationByID\|GetOfferByID\|GetDealByID" backend --include=*.go` находит только `repository/sqlc` и пример в doc-комментарии `internal/repository/pgerr/pgerr.go`. Заменять их безопасно.
- `docs/db/tables/applications.md`, `offers.md`, `deals.md` утверждают «IDOR проверяет сервис» — после задачи это неверно.

## Решения

Источники: `mds/plan.md` п. 1 и блок «Решения по BE-04» в Context `mds/db-security-plan.md`; если они расходятся, верен второй — он новее.

1. **Роли и KYC проверяет своё middleware `middleware.Authorize` перед валидатором.** Операцию оно находит роутером kin-openapi по встроенной спеке и читает `security`, `x-roles`, `x-kyc-required`. Strict-middleware не подходит: у него свой тип в каждом из 10 пакетов `<tag>api`, `operationID` приходит Go-именем, а валидатор пропускает multipart-загрузку — её роль не проверил бы никто.
2. **Публичность операции определяет спека (`security: []`).** Карта `publicRoutes` удаляется. Несуществующий путь отдаёт `404`, а не `401` (ревью PR #67, №4), `HEAD` к публичной операции проходит (№3).
3. **`Authenticate` становится «мягким»:** валидная сессия → principal в контекст; нет cookie или сессия невалидна → запрос идёт дальше без principal; ошибка БД → `500`. Решение «`401` или пропустить» принимает `Authorize`, потому что только он знает, публичная ли операция.
4. **CSRF стоит после `Authorize`** и проверяется у найденной в спеке непубличной операции с методом не GET/HEAD/OPTIONS. У защищённой операции к этому моменту сессия гарантирована (п. 1.1 plan.md). Публичные `register` и `login` CSRF не проверяют, как и в PR #67 (DECISIONS п. 1; обоснование и где это записано — [BE-03, «Решения»](BE-03-auth.md#решения)).
5. **`Principal.KycStatus string`** (`""` у admin). Он читается тем же запросом сессии на каждом запросе и не кэшируется, поэтому одобрение KYC админом действует со следующего запроса.
6. **`GetSessionByTokenHash` получает `LEFT JOIN organizations`.** Именно LEFT: у admin `organization_id IS NULL` (`users_admin_org_chk`), с INNER JOIN админ не смог бы войти.
7. **`GetApplicationByID`, `GetOfferByID`, `GetDealByID` заменяются на `*Visible`, старые удаляются.** `viewer_role` и `viewer_org` — обязательные `sqlc.arg`, не `narg`. Правило видимости стоит в `WHERE` как `OR` веток по ролям. Неизвестная роль не попадает ни в одну ветку и получает пустой результат. У admin `viewer_org = uuid.Nil`: ветка admin его не читает, а если роль перепутана, `uuid.Nil` не совпадёт ни с одной организацией.
8. **Списки — на том же правиле видимости.** Бизнес-фильтры (`statuses`, `currency`, …) остаются `narg`: они только сужают выборку и не могут её открыть.
9. **Чужое и несуществующее неразличимы:** оба дают `pgx.ErrNoRows`, а `pgerr.Map(err, "Заявка не найдена")` превращает это в `404`. Отдельного «403 за чужое» нет нигде.
10. **Роль проверяет middleware, и сервис повторяет проверку** (DECISIONS п. 36). Middleware даёт ранний отказ до валидатора, в том числе для multipart. Проверка в сервисе — защита в глубину: одна строка, тестируется без роутера и страхует от ошибки в `x-roles`. Ещё одна линия — SQL: запрос с `viewer_role`/`viewer_org` не вернёт чужих данных. ADR-0001 п. 4: «роль проверяется в middleware, владение — в каждом SQL-запросе» — дополняется «сервис повторяет проверку роли».
11. **KYC проверяется в middleware и повторно в сервисе по `Principal.KycStatus`, а не в транзакции создания** (DECISIONS п. 36). У `verified` нет исходящих переходов (ADR-0001), поэтому между проверкой и INSERT статус не может испортиться. Строку «Создавать заявки и офферы можно только при `kyc_status = verified` — Сервис» в `docs/db/schema.md` поправить на «HTTP-middleware по `x-kyc-required` и сервис».
12. **`'403': Forbidden` объявлен у всех 44 защищённых операций** (DECISIONS п. 2): любая из них может вернуть `403` — роль, KYC или CSRF. Добавляется одним коммитом в шаге 1, а тест не даёт новой операции появиться без него. Так BE-03, BE-08, BE-09, BE-10 контракт под `403` не трогают.
13. **Видимость BE-04 проверяет интеграционным тестом запросов** (`TestVisibilityIntegration`); HTTP-тесты IDOR по всем ресурсам — в BE-15 (DECISIONS п. 3). Операции сделок принадлежат BE-09, в BE-04 их не реализуем ради HTTP-теста.
14. **Один запрос с `OR`-ветками по роли, пока нет измерений.** pgx готовит запросы; после нескольких вызовов Postgres может выбрать generic plan, который не отбрасывает ненужные ветки, и индекс по организации не используется. На объёмах демо это незаметно. Если `EXPLAIN ANALYZE` на списках покажет проблему, запрос делится на три (`…ForBorrower/Lender/Admin`) с выбором в Go.
15. **`AccessRules` живёт в `internal/http/middleware/`** (README индекса); спеку получает параметром `doc`, а не импортом. Если `NewAccessRules` разрастётся (например, `x-rate-limit`), его можно вынести в `internal/http/access`.

## Шаги

Шаги 1–3 от PR #67 не зависят: их можно делать в ветке от `origin/main` сразу. Шаги 4–5 — после мержа #67 или rebase на `origin/be-03-auth`.

### 1. Контракт: `x-roles`, `x-kyc-required`, `403`

Пропиши расширения у каждой из 44 защищённых операций в `api/paths/*.yaml` (на уровне операции, рядом с `operationId`):

| `x-roles` | `x-kyc-required` | Операции |
|---|---|---|
| — (публичные, `security: []`) | — | `register`, `login`, `listCountries`, `listCurrencies`, `listTransferMethods`, `registryLookup`, `healthz` |
| `[borrower, lender, admin]` | — | `logout`, `getMe`, `listApplications`, `getApplication`, `getApplicationHistory`, `listApplicationOffers`, `listOffers`, `getOffer`, `listDeals`, `getDeal`, `listMessages`, `listNotifications`, `markNotificationRead`, `markAllNotificationsRead`, `downloadDocument` |
| `[borrower, lender]` | — | `getMyOrganization`, `updateMyOrganization`, `listMyDocuments`, `uploadMyDocument`, `deleteMyDocument`, `submitKyc`, `sendMessage` |
| `[borrower]` | `true` | `createApplication` |
| `[borrower]` | — | `updateApplication`, `submitApplication`, `cancelApplication`, `rejectAllOffers`, `acceptOffer`, `rejectOffer` |
| `[lender]` | `true` | `createOffer` |
| `[lender]` | — | `withdrawOffer` |
| `[admin]` | — | все 13 операций `paths/admin.yaml` |

Таблица выведена из колонки «Кто» в `backend/README.md` («Эндпоинты») и описаний операций. Итого 7 + 15 + 7 + 1 + 6 + 1 + 1 + 13 = 51.

Одним коммитом добавь `'403': { $ref: '../components.yaml#/components/responses/Forbidden' }` **каждой** операции без `security: []` — всем 44, там, где его ещё нет (решение 12). В `info.description` (`api/openapi.root.yaml`, раздел «Авторизация») допиши: роли операции — `x-roles`, требование KYC — `x-kyc-required`, порядок проверок — таблица из «Цели». Затем `make gen-api`, `make lint-api`, `make diff-api`.

**Готово:** `grep -c "x-roles" api/openapi.yaml` = 44; `TestProtectedOperationsDeclare403` (шаг 2, рядом с `TestAccessRulesFromEmbeddedSpec`) зелёный; `make gen-check` без диффа; `make diff-api` не показывает breaking-изменений (добавление ответа `403` — не breaking; если oasdiff считает иначе, обсудить с владельцем до мержа).

### 2. Правила доступа из спеки: `middleware.NewAccessRules`

Новый файл `backend/internal/http/middleware/authorize.go`:

```go
// AccessRule — кто может вызвать операцию: из security, x-roles и x-kyc-required спеки.
type AccessRule struct {
    Public      bool     // security: []
    Roles       []string // x-roles
    KYCRequired bool     // x-kyc-required
}

// AccessRules — правила по operationId и роутер kin-openapi, который находит операцию по запросу.
type AccessRules struct { /* router routers.Router; byOperation map[string]AccessRule */ }

// NewAccessRules разбирает спеку один раз при старте. Ошибка = баг в спеке, сервер не стартует.
func NewAccessRules(doc *openapi3.T) (*AccessRules, error)

// Rule — правило операции (для тестов и отладки).
func (a *AccessRules) Rule(operationID string) (AccessRule, bool)
```

`NewAccessRules` возвращает ошибку, если:
- у защищённой операции (`Security == nil` или непустой) нет `x-roles` или список пуст;
- в `x-roles` есть значение не из `borrower`/`lender`/`admin` (сравнивать с `repository.UserRoleBorrower` и т. д.);
- `x-roles` есть у публичной операции;
- `x-kyc-required` не `bool`, или стоит у операции, в чьих `x-roles` есть `admin` (у админа нет компании и KYC).

Роутер — `gorillamux.NewRouter(doc)`, тот же, что внутри валидатора.

**Готово:** `TestAccessRulesFromEmbeddedSpec` и `TestNewAccessRulesRejectsBadSpec` зелёные (см. «Тесты»).

### 3. SQL: `kyc_status` в сессии, `*Visible`-запросы, списки

1. `backend/queries/sessions.sql: GetSessionByTokenHash` — добавь `o.kyc_status` и `LEFT JOIN organizations o ON o.id = u.organization_id`. sqlc сгенерирует `KycStatus repository.NullKycStatus`.
2. `backend/queries/applications.sql` — удали `GetApplicationByID`, добавь:
   ```sql
   -- name: GetApplicationVisible :one
   -- Заявка, если пользователь (viewer_role, viewer_org) её видит. Чужая и несуществующая
   -- одинаково дают 0 строк → 404 (IDOR). Правило совпадает с ListApplicationsVisible.
   SELECT * FROM applications
   WHERE id = sqlc.arg('id')
     AND (
          sqlc.arg('viewer_role')::user_role = 'admin'
       OR (sqlc.arg('viewer_role')::user_role = 'borrower'
           AND borrower_organization_id = sqlc.arg('viewer_org')::uuid)
       OR (sqlc.arg('viewer_role')::user_role = 'lender'
           AND (status = 'published'
                OR EXISTS (SELECT 1 FROM offers o
                            WHERE o.application_id = applications.id
                              AND o.lender_organization_id = sqlc.arg('viewer_org')::uuid)))
     );
   ```
   `ListApplications` / `CountApplications` переименуй в `ListApplicationsVisible` / `CountApplicationsVisible`: строки с `narg('borrower_org_id')` и `narg('lender_org_id')` замени тем же блоком видимости, остальные фильтры и сортировку не трогай.
3. `backend/queries/offers.sql` — удали `GetOfferByID`, добавь `GetOfferVisible`. Ветки:
   - `admin` — всё;
   - `lender` — `lender_organization_id = viewer_org`;
   - `borrower` — `EXISTS (SELECT 1 FROM applications a WHERE a.id = offers.application_id AND a.borrower_organization_id = viewer_org)`.

   Через `EXISTS`, а не JOIN, чтобы результат остался моделью `repository.Offer`. `ListOffers` / `CountOffers` → `ListOffersVisible` / `CountOffersVisible`: `narg('lender_org_id')` заменить блоком видимости; `application_id` и `statuses` оставить `narg`.
4. `backend/queries/deals.sql` — удали `GetDealByID`, добавь `GetDealVisible` (`admin` — всё; `borrower` — `borrower_organization_id = viewer_org`; `lender` — `lender_organization_id = viewer_org`). `ListDeals` / `CountDeals` → `ListDealsVisible` / `CountDealsVisible` с тем же блоком вместо двух `narg`.
5. ⚠️ В `List*Visible` и `Count*Visible` блок видимости должен совпадать символ в символ: sqlc не умеет общие фрагменты. Это ловит тест `Count == len(List)` (см. «Тесты»).
6. Пример в doc-комментарии `internal/repository/pgerr/pgerr.go` замени на `q.GetApplicationVisible(ctx, …)`.
7. `make gen-sqlc`.

Фильтры только для admin из контракта (`borrower_organization_id`, `lender_organization_id`) и новые фильтры списков (ветка `feat/api-list-filters`: `target_country[]`, `has_my_offer`, `created_from`/`created_to`, `published_at_*`) добавляют BE-07/08/09 в эти же `*Visible`-запросы как `narg`-условия, которые только сужают выборку.

**Готово:** `grep -rn "GetApplicationByID\|GetOfferByID\|GetDealByID\|narg('borrower_org_id')\|narg('lender_org_id')" backend/queries` пусто; `make gen-check` без диффа; `TestVisibilityIntegration` зелёный в `make test-all`.

### 4. `Principal.KycStatus`, мягкий `Authenticate`, `Authorize`, CSRF по спеке

После мержа PR #67 (или в ветке поверх `origin/be-03-auth`).

1. `internal/service/principal.go`:
   ```go
   type Principal struct {
       UserID    uuid.UUID
       OrgID     *uuid.UUID
       Role      string
       KycStatus string // kyc_status организации; "" у admin
   }
   // ViewerRole — параметр viewer_role для запросов *Visible.
   func (p Principal) ViewerRole() repository.UserRole
   // ViewerOrg — параметр viewer_org; у admin uuid.Nil (см. «Решения», п. 7).
   func (p Principal) ViewerOrg() uuid.UUID
   ```
   `service.Auth.Authenticate` заполняет `KycStatus` из `row.KycStatus` (`Valid == false` → `""`).
2. `middleware/auth.go`: `Authenticate` больше не отвечает `401`. Удали `publicRoutes` и `isPublic`. Ошибка `Authenticator` с кодом `UNAUTHORIZED` → `next` без principal; любая другая → `writeErr` (`500`).
3. `middleware/authorize.go`: `Authorize(rules *AccessRules, prefix string, writeErr func(http.ResponseWriter, *http.Request, error)) func(http.Handler) http.Handler`. Порядок на запрос:
   1. Копия запроса для поиска: путь без `prefix`; `HEAD` ищется как `GET`. Ошибка `FindRoute` → `next`: путь не из спеки, `404` отдаст валидатор.
   2. Правило операции кладётся в контекст (`accessRuleFromContext` — для CSRF).
   3. `Public` → `next`.
   4. Нет principal → `apperr.Unauthorized("Требуется вход")`.
   5. `Role` не в `Roles` → `apperr.Forbidden("Недостаточно прав для операции")`.
   6. `KYCRequired && KycStatus != string(repository.KycStatusVerified)` → `apperr.KYCNotVerified()`.

   Над функцией — комментарий с таблицей порядка проверок: это ключевой блок проверки доступа (`AGENTS.md`, «Комментарии»).
4. `middleware/csrf.go`: вместо `isPublic(r)` — `rule, ok := accessRuleFromContext(ctx)`. Проверка пропускается, если `!ok` (путь не из спеки), `rule.Public` или метод безопасный.
5. `handler/router.go`, `NewRouter`: `rules, err := middleware.NewAccessRules(openapi.MustDoc())`, при ошибке — `panic`. Цепочка:
   `RequestLog → Recover → SecurityHeaders → MaxBody → WithRequestMeta → Authenticate → Authorize → CSRF → validateRequests`.
6. `handler/principal.go`: `currentPrincipal(ctx) (service.Principal, error)` — `middleware.PrincipalFromContext` или `apperr.Unauthorized`. `GetMe` использует его вместо своей проверки. Последующие задачи берут principal только через этот хелпер.
7. Тесты PR #67, которые опирались на `publicRoutes`, перепиши:
   - `middleware/auth_test.go` — ожидания `401` переезжают в `TestAuthorize`;
   - `middleware/csrf_test.go` — правило в контекст кладёт сам тест;
   - в `handler/validate_test.go` удали `needsTestAuthentication`: сессию и CSRF-заголовок ставь на каждый запрос, публичные операции их игнорируют. Тестовый `Authenticator` выдаёт principal по значению cookie (`borrower-verified`, `borrower-draft`, `lender-verified`, `admin`, `expired` → `UNAUTHORIZED`).

**Готово:** `grep -rn "publicRoutes\|isPublic" backend/internal` пусто; `TestAuthorize`, `TestCSRFUsesAccessRule`, `TestAccessOrder`, `TestAuthAuthenticateKycStatus` зелёные; `curl -i localhost:8080/api/v1/nope` → `404 NOT_FOUND`; `curl -I localhost:8080/api/v1/healthz` → `200`; `curl -i localhost:8080/api/v1/applications` без cookie → `401 UNAUTHORIZED`.

### 5. Документация и ADR

1. `docs/adr/0004-authorization.md` (`AGENTS.md`: схема авторизации — ADR): роли из спеки (`x-roles`, `x-kyc-required`) + своё middleware перед валидатором; видимость в SQL (`*Visible` с обязательными `viewer_role`/`viewer_org`); отвергнутые варианты — strict-middleware, карта operationId → роли в Go, проверка владельца в сервисе по возвращённой строке.
2. `backend/README.md`:
   - «Аутентификация»: «без сессии доступны операции с `security: []`» вместо перечня;
   - «Роли и доступ»: таблица порядка проверок из «Цели» и абзац «роли операции — `x-roles` в спеке»;
   - «Как устроен код»: список `middleware/`;
   - «Как добавить операцию»: шаг «укажи `x-roles` (и `x-kyc-required`) у операции; сущность читай через `*Visible` с `p.ViewerRole()`, `p.ViewerOrg()`».
3. `internal/http/handler/handler.go` (doc-комментарий «Как добавить операцию») — тот же шаг.
4. `AGENTS.md`, «Безопасность → Доступ (IDOR)»: правило «Роль … проверяй в сервисе» оставить и дополнить: «роль и KYC объявляй у операции в спеке (`x-roles`/`x-kyc-required`, проверяет `middleware.Authorize`) и повторяй в сервисе; владение — запросы `*Visible` с обязательными `viewer_role`/`viewer_org`». Это совпадает с ADR-0001 п. 4, но правка касается правил безопасности — согласуй её с владельцем репо в PR.
5. `docs/db/schema.md`:
   - «Где проверяется каждый инвариант»: строка про KYC → «HTTP-middleware по `x-kyc-required` и сервис»; строка про IDOR → «Репозиторий: запросы `*Visible`»;
   - «Покрытие sqlc»: новые имена запросов.

   В `docs/db/tables/applications.md`, `offers.md`, `deals.md`, `sessions.md` обнови таблицы запросов и убери фразы «защита от IDOR — в сервисе».

**Готово:** `grep -rn "GetApplicationByID\|GetOfferByID\|GetDealByID\|IDOR — в сервисе" docs backend/README.md AGENTS.md` пусто; ADR-0004 есть и упомянут в шапке `backend/README.md` рядом с ADR-0001…0003.

### Запросы по id, которые переводят на видимость другие задачи

Шаблон — шаг 3. Сначала проверить видимость родительской сущности (`GetApplicationVisible` / `GetOfferVisible` → `404`), потом выполнять действие с фильтром владельца в `WHERE`.

| Запрос | Что добавить | Задача |
|---|---|---|
| `UpdateApplication`, `SubmitApplication`, `CancelApplication` | `AND borrower_organization_id = @viewer_org` | BE-07 |
| `RejectSentOffersByApplication` (заменяет `RejectAllOffersForApplication`) | владелец проверен `LockApplicationForBorrower` в той же транзакции | BE-07 |
| `getApplicationHistory` (запроса ещё нет) | сначала `GetApplicationVisible` | BE-07 |
| `ListOffersByApplication` | borrower-владелец и admin — все, lender — только свой (`description` операции) | BE-08 |
| `WithdrawOffer` | `AND lender_organization_id = @viewer_org` | BE-08 |
| `RejectOffer`, `AcceptOffer` | `EXISTS` заявки с `borrower_organization_id = @viewer_org` | BE-08, BE-09 |
| `GetApplicationForUpdate` | владелец и `status = 'published'` (`docs/db/schema.md`, «Принятие оффера») | BE-09 |
| `ListMessages`, `CreateMessage`, `CountUnreadMessages` | сначала `GetOfferVisible` | BE-10 |
| получение KYC-документа по id (запроса ещё нет) | владелец-организация или admin | BE-05 |

## Тесты

### Правила из спеки (`middleware`, без БД)

- `TestAccessRulesFromEmbeddedSpec`: `NewAccessRules(openapi.MustDoc())` без ошибки; публичных операций 7, защищённых 44; проверка выборочных правил:
  - `createApplication` → `{Roles: [borrower], KYCRequired: true}`;
  - `createOffer` → `{[lender], true}`;
  - `adminBlockUser` → `{[admin], false}`;
  - `getMe` → все три роли;
  - `healthz` → `Public`.
- `TestProtectedOperationsDeclare403`: обходит все операции `openapi.MustDoc()`; у каждой, где `Security == nil` или непустой, есть `Responses.Value("403")`. Сообщение об ошибке называет `operationId` без `403`.
- `TestNewAccessRulesRejectsBadSpec` (табличный, маленькие спеки через `openapi3.NewLoader().LoadFromData`): защищённая операция без `x-roles` → ошибка; `x-roles: [superuser]` → ошибка; `x-roles` у `security: []` → ошибка; `x-kyc-required: "yes"` → ошибка; `x-kyc-required: true` при `x-roles: [admin]` → ошибка.

### `TestAuthorize` (`middleware`, `httptest`, правила из встроенной спеки, principal кладёт тест)

| Запрос | Principal | Ожидание |
|---|---|---|
| `GET /api/v1/healthz` | нет | next |
| `HEAD /api/v1/healthz` | нет | next |
| `POST /api/v1/auth/login` | нет | next |
| `GET /api/v1/registry/lookup` | нет | next |
| `GET /api/v1/nope` | нет | next (404 даст валидатор) |
| `GET /api/v1/applications` | нет | `401 UNAUTHORIZED` |
| `POST /api/v1/applications` | lender, verified | `403 FORBIDDEN` |
| `POST /api/v1/applications` | admin | `403 FORBIDDEN` |
| `POST /api/v1/applications` | borrower, draft | `403 KYC_NOT_VERIFIED` |
| `POST /api/v1/applications` | borrower, verified | next |
| `POST /api/v1/applications/{id}/offers` | lender, under_review | `403 KYC_NOT_VERIFIED` |
| `POST /api/v1/offers/{id}/accept` | lender, verified | `403 FORBIDDEN` |
| `POST /api/v1/organizations/me/documents` (multipart) | admin | `403 FORBIDDEN` — валидатор этот запрос пропускает, роль проверяется всё равно |
| `GET /api/v1/admin/users` | borrower, verified | `403 FORBIDDEN` |
| `GET /api/v1/admin/users` | admin | next |
| `GET /api/v1/deals/{id}` | lender, draft | next (KYC для чтения не нужен) |

### `TestCSRFUsesAccessRule` (`middleware`)

- `POST /auth/login` без заголовка → next;
- `POST /applications` с principal, без заголовка → `403 CSRF_INVALID`;
- то же с совпадающими cookie и заголовком → next;
- `POST /api/v1/nope` без заголовка → next.

### `TestAccessOrder` (`handler`, настоящий `NewRouter`, фейковый `AuthService`)

- без cookie `GET /applications` → `401`; cookie `expired` → `401`;
- cookie `expired`, `POST /auth/login` → `200`: протухшая cookie не мешает войти;
- `borrower-draft`, `POST /applications` без CSRF → `403 KYC_NOT_VERIFIED` (KYC раньше CSRF);
- `borrower-verified`, `POST /applications` с CSRF и телом `{}` → `400 VALIDATION_ERROR` (валидатор после доступа);
- `borrower-verified`, `POST /applications` с CSRF и валидным телом → `501 NOT_IMPLEMENTED`;
- `GET /api/v1/nope` с cookie и без → `404 NOT_FOUND`;
- `HEAD /api/v1/healthz` → `200`.

### `TestAuthAuthenticateKycStatus` (`service`, фейковый store)

- строка сессии с `KycStatus{verified, Valid: true}` → `Principal.KycStatus == "verified"`;
- admin (`Valid: false`) → `""`, `OrgID == nil`, `ViewerOrg() == uuid.Nil`.

### `TestVisibilityIntegration` (`internal/service/visibility_integration_test.go`)

Переиспользует `newPool` из `dictionaries_integration_test.go`. Всё внутри одной транзакции с `Rollback`, запросы через `repository.New(tx)`.

Фикстура:
- организации: заёмщики `B1`, `B2`, кредиторы `L1`, `L2`, у каждой по пользователю;
- заявки: `A1` (`B1`, `published`), `A2` (`B1`, `draft`), `A3` (`B2`, `published`), `A4` (`B2`, `agreed`);
- офферы: `O1` (`L1` → `A1`, `sent`), `O2` (`L2` → `A1`, `sent`), `O3` (`L1` → `A4`, `accepted`);
- сделка `D1` (`A4`/`O3`, `B2`–`L1`).

⚠️ Порядок вставки задают CHECK-ограничения: `A4` создаётся как `published`, затем `O3` как `sent`. После этого `O3` переводится в `accepted` с `decided_at`, а `A4` — в `agreed` с `accepted_offer_id`. Иначе сработают `applications_agreed_chk` и CHECK офферов на `decided_at`.

| Запрос | Видит | Не видит |
|---|---|---|
| `GetApplicationVisible` | B1: A1, A2 · L1: A1, A3, A4 · L2: A1, A3 · admin: A2 | B1: A3 · B2: A2 · L1: A2 · L2: A4 |
| `GetOfferVisible` | L1: O1, O3 · B1: O1, O2 · B2: O3 · admin: O2 | L1: O2 · B1: O3 · L2: O1 |
| `GetDealVisible` | B2, L1, admin: D1 | B1, L2: D1 |

- Каждое «не видит» → `pgx.ErrNoRows`, а `pgerr.Map` превращает его в `apperr` с кодом `NOT_FOUND`. Тот же код у случайного UUID: чужое неотличимо от несуществующего.
- `List*Visible` (`limit` 1000) для каждого зрителя содержит ровно «видит» из фикстуры и не содержит «не видит». Демо-сид в БД тоже есть, поэтому проверяй вхождение id фикстуры, а не длину.
- `Count*Visible` == `len(List*Visible)` для каждого зрителя и каждой сущности: так ловится расхождение блоков видимости.
- Закрыто по умолчанию: `viewer_role = 'borrower'`, `viewer_org = uuid.Nil` → все три `List*Visible` пустые.

## Готово, когда

- [ ] `service.Principal` содержит `UserID`, `OrgID`, `Role`, `KycStatus`; `middleware.Authenticate` кладёт его в контекст из одного запроса `GetSessionByTokenHash` (с `LEFT JOIN organizations`); `TestAuthAuthenticateKycStatus` зелёный.
- [ ] Роли и KYC у всех 44 защищённых операций заданы в спеке (`x-roles`, `x-kyc-required`); `middleware.Authorize` их проверяет; спека без `x-roles` у защищённой операции не проходит `TestAccessRulesFromEmbeddedSpec`; `TestAuthorize` и `TestAccessOrder` зелёные.
- [ ] У всех 44 защищённых операций в контракте объявлен `403` (`TestProtectedOperationsDeclare403`).
- [ ] Компания не в статусе `verified`: `POST /applications` и `POST /applications/{id}/offers` → `403 KYC_NOT_VERIFIED`.
- [ ] `GetApplicationByID`, `GetOfferByID`, `GetDealByID` удалены; `Get*Visible` с обязательными `viewer_role`/`viewer_org` отдают `ErrNoRows` на чужое (→ `404`); `TestVisibilityIntegration` зелёный.
- [ ] `/applications`, `/offers`, `/deals` читают `List*Visible` / `Count*Visible`: правило видимости по роли — в SQL; без организации результат пустой.
- [ ] `publicRoutes` удалён: публичность берётся из `security: []`; несуществующий путь → `404`, `HEAD /healthz` → `200`.
- [ ] ADR-0004, `backend/README.md` («Аутентификация», «Роли и доступ», «Как устроен код», «Как добавить операцию»), `AGENTS.md`, `docs/db/schema.md` и `docs/db/tables/*.md` описывают новую схему.
- [ ] И общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: HTTP-тест IDOR — BE-15 (DECISIONS п. 3, решение 13), CSRF для `register`/`login` — DECISIONS п. 1; производительность `OR`-веток и место `AccessRules` — решения 14 и 15.
