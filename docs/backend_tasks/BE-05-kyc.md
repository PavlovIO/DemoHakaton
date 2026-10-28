# BE-05 · Профиль компании, бенефициары, KYC-документы

Issue: #21 · Milestone: M2 · Зависит от: BE-04 (`Principal.OrgID`, `x-roles`), PR #67 / BE-03 (`Principal`, `ClientMeta`); BE-13 и BE-06 — через интерфейсы, не блокируют · Блокирует: BE-12 (таблица переходов KYC, документы в карточке), BE-15 (IDOR-тест документа), BE-16

## Цель
Заёмщик и кредитор видят и правят профиль компании и бенефициаров, загружают, удаляют и скачивают KYC-документы (PDF/PNG/JPEG ≤ 10 МБ) и отправляют компанию на проверку: `draft|rejected → under_review`, с проверкой обязательных документов, AML-скринингом, записью в `status_history` и уведомлением админам. Админ скачивает документы любой компании.

## Что уже есть
- Контракт: `api/paths/organizations.yaml` — 7 операций задачи. Go-имена методов: `GetMyOrganization`, `UpdateMyOrganization`, `ListMyDocuments`, `UploadMyDocument`, `DeleteMyDocument`, `DownloadDocument`, `SubmitKYC` (не `SubmitKyc`). Заглушки 501 — `backend/internal/http/handler/organizations.go`.
- Схемы: `api/schemas/organizations.yaml` (`Organization`, `OrganizationUpdate`, `Beneficiary`, `KycDocument`), enum `KycDocumentType` — `api/schemas/enums.yaml`. У `api.KYCDocumentType` есть метод `Valid()` (`internal/http/api/models.gen.go`).
- SQL:
  - `backend/queries/organizations.sql`: `GetOrganizationByID`, `UpdateOrganization`, `SubmitKycForReview`.
  - `backend/queries/kyc.sql`: `CreateKycDocument`, `ListKycDocumentsByOrg`, `DeleteKycDocument`, `CreateRegistryCheck`.
  - `backend/queries/applications.sql`: `CreateStatusHistory`.
  - `backend/queries/notifications.sql`: `CreateNotification`.
- БД: CHECK в `kyc_documents` (`backend/migrations/00001_init.sql`) — `mime_type` из трёх значений, `0 < size_bytes ≤ 10485760`, `length(sha256) = 32`, `length(file_name) ≤ 255`. Это вторая линия защиты, первая — сервис.
- Сгенерированный strict-хендлер загрузки (`internal/http/api/organizationsapi/server.gen.go`, `UploadMyDocument`) вызывает `r.MultipartReader()` и кладёт `*multipart.Reader` в `UploadMyDocumentRequestObject.Body`. Это потоковое чтение: `ParseMultipartForm` не вызывается, файл целиком в память не попадает.
- `handler/validate.go`: `skipValidation` пропускает `POST /organizations/me/documents` (`isUpload`). `middleware.MaxBody(bodyLimit)` ограничивает тело загрузки до `maxBodyUpload = 10 МБ + 64 КБ`. Там же оставлен `TODO(uploadMyDocument)`: отвечать 413 из контракта, а не 400.
- PR #67: `service.Principal{UserID, OrgID *uuid.UUID, Role string}`, `middleware.PrincipalFromContext`, `middleware.ClientMetaFromContext`, `middleware.SecurityHeaders()` (глобально ставит `X-Content-Type-Options: nosniff` и `Cache-Control: no-store`), образец сервиса с маленьким интерфейсом стора и транзакцией — `AuthStore` + `AuthTx.InTx(ctx, func(q repository.Querier) error)` в `service/auth.go`.
- `.gitignore` содержит `/storage/` с комментарием «Uploaded KYC files in local dev».

⚠️ Ловушки:
- **Валидатор не проверяет загрузку.** Из-за `skipValidation` `doc_type` (enum), обязательность `doc_type` и `file` проверяет хендлер. Это единственное место, где ручная валидация в хендлере нужна; правило AGENTS.md «не дублируй валидацию» здесь не нарушается.
- **В `SubmitKycForReview` и `UpdateOrganization` нет условия по статусу в `WHERE`** (`docs/db/tables/organizations.md`). Параллельные PATCH и submit проскочат проверку сервиса. Решение — блокировка строки (шаг 2).
- **`SubmitKycForReview` возвращает не все поля** (`RETURNING id, type, name, kyc_status, …`). Для ответа `Organization` нужны `country_code`, `registration_number`, `legal_address`, `lei`, `created_at` и бенефициары, поэтому после перехода организацию надо перечитать.
- **`UpdateOrganization` не может обнулить `lei`**: стоит `COALESCE(sqlc.narg('lei'), lei)`, а сгенерированное поле `OrganizationUpdate.LEI` имеет тип `*string`. Отсутствующее поле и `"lei": null` неразличимы. Решение — пустая строка (см. «Решения»); но `pattern: '^[A-Z0-9]{20}$'` у `OrganizationUpdate.lei` пустую строку не пропустит, его надо ослабить (шаг 1).
- **`OrganizationUpdate.Beneficiaries` — `[]Beneficiary` без указателя** (`prefer-skip-optional-pointer-on-container-types`). Отличать «поле не передано» от «очистить список» можно только по `nil`: `encoding/json` для `[]` даёт пустой не-nil срез, для отсутствующего поля и `null` срез остаётся `nil`.
- **`DeleteKycDocument` — `:exec` без `RETURNING storage_key`**: файл на диске нечем найти, а «не найдено» неотличимо от успеха.
- **У `ListKycDocumentsByOrg` нет `ORDER BY`**, порядок в ответе случайный.
- **Сгенерированный ответ скачивания** `DownloadDocument200ApplicationOctetStreamResponse{Body, ContentLength}` ставит только `Content-Type` и `Content-Length`. `Content-Disposition` в нём нет.
- **`make run` запускает сервер из `backend/`** (`go -C backend run`). Относительный путь `storage/...` превратится в `backend/storage/...`, а под `/storage/` в `.gitignore` он не попадает.
- **`ReadTimeout: 15s`** в `cmd/server/main.go` действует на всё тело. 10 МБ за 15 с — это ≥ 5,6 Мбит/с; на медленном канале загрузка оборвётся.
- **Сид** (`backend/migrations/testdata/seed_demo.sql`) вставляет `kyc_documents` с `storage_key` вида `alpha-charter.pdf`, а файлов на диске нет.
- **PR #67 собран на старой раскладке** (один пакет `api`). На `main` сервер разнесён по пакетам `<tag>api` (ADR-0003); код пиши под `organizationsapi`.

## Решения
- **Хранилище — локальный каталог из env `DOCUMENT_STORAGE_DIR`.** Ключ файла — 16 байт `crypto/rand` в hex, без расширения и без связи с `file_name`. Права: каталог `0700`, файлы `0600`. Сервер не раздаёт этот каталог статикой: файл отдаётся только через `downloadDocument`, и это выполняет требование «вне веб-корня». Почему: S3 для MVP избыточен, а интерфейс `DocumentFiles` позволит заменить хранилище, не трогая сервис.
- **Пакет `internal/storage`** (новый) с `storage.Local`. Сервис зависит от интерфейса `DocumentFiles`, объявленного рядом с собой (правило README: интерфейсы на стороне потребителя).
- **Тип файла определяем по своим сигнатурам** (`%PDF-`, `\x89PNG\r\n\x1a\n`, `\xFF\xD8\xFF`), а не через `http.DetectContentType`. Почему: сервис не импортирует `net/http` (AGENTS.md, слои), а белый список из трёх сигнатур точнее WHATWG-сниффера. Расширение файла и `Content-Type` части multipart игнорируются.
- **Потоковая запись:** первые 512 байт читаются в буфер для сигнатуры, затем `io.MultiReader(head, rest)` → `io.LimitReader(max+1)` → `io.TeeReader` в `sha256` → файл. В памяти держится только буфер `io.Copy`.
- **413 = новый код `FILE_TOO_LARGE`** в `apperr`, `statusByCode`, `openapi.root.yaml` («Ошибки») и `backend/README.md`. Почему: контракт обещает 413, а `statusByCode` — единственный источник «код → статус» (`handler/errors.go`). `*http.MaxBytesError` переводит в этот код хендлер, обёрткой над читателем части, чтобы сервис не знал про `net/http`.
- **`doc_type` должен идти в форме до `file`.** Иначе пришлось бы сначала писать файл во временное хранилище, а тип проверять потом. Порядок частей фиксируем в `description` операции (изменение только описания — не ломающее); браузерный `FormData` сохраняет порядок `append`.
- **Заголовки скачивания объявляем в контракте**: `headers` у ответа 200 — `Content-Disposition` и `X-Content-Type-Options`. Тогда oapi-codegen сам генерирует их установку, а фронт видит их в спеке. Имя файла кодирует `mime.FormatMediaType("attachment", {"filename": name})`: он делает RFC 2231 для кириллицы и экранирует кавычки, так что инъекция в заголовок невозможна. `nosniff` ставим явно, не полагаясь на глобальный `SecurityHeaders`.
- **Порядок «БД ↔ файл».**
  - Загрузка: сначала файл, потом строка в БД; при ошибке транзакции файл удаляется.
  - Удаление: сначала строка в БД (коммит), потом файл; ошибку удаления файла только логируем.
  - Почему: осиротевший файл безвреден, а строка без файла — это 404 на скачивании.
- **Гонки статуса закрывает блокировка `SELECT … FOR UPDATE` строки организации** в каждой мутирующей операции (PATCH, upload, delete, submit). Дополнительно в `WHERE` ставим охрану статуса `kyc_status IN ('draft','rejected')`.
- **Таблица переходов KYC — одна функция на всю сущность** (ADR-0001, «Статусные машины»). Её создаёт BE-05, BE-12 использует для approve/reject.

  | Статус \ действие | `edit` (PATCH, upload, delete) | `submit` | `approve` (BE-12) | `reject` (BE-12) |
  |---|---|---|---|---|
  | `draft` | разрешено | → `under_review` | 409 | 409 |
  | `under_review` | 409 | 409 | → `verified` | → `rejected` |
  | `verified` | 409 | 409 | 409 | 409 |
  | `rejected` | разрешено | → `under_review` | 409 | 409 |

- **Обязательные документы — 400 `VALIDATION_ERROR`, а не 409.** В контракте `submitKyc` есть 400. В `details` — по записи на каждый недостающий тип: `field: "documents"`, `message: "нет документа: charter"`.
  - Всем: `charter`, `registration_certificate`, `address_proof`, `beneficiaries_register`.
  - Кредитору дополнительно: `source_of_funds` (описание `submitKyc` в контракте).
- **AML и реестр — через интерфейсы, BE-05 их не ждёт.**
  - `aml.Screener` (README, «Общие куски кода», владелец BE-13). Если BE-13 ещё не смержен, создай минимальный `internal/aml/screener.go` дословно по шагу 1 плана BE-13 (типы + `NoopScreener`) и сохранение флагов `saveAmlFindings` по его шагу 6.
  - Сверка с реестром — необязательная зависимость `CompanyVerifier`. Пока она `nil` (BE-06 не подключён), `OrganizationSubject.Registry = nil`, и правило реестра не применяется.
  - Скрининг падает → submit падает целиком (500, откат). AML — защитный контроль, без него в `under_review` не переводим.
- **Сетевой вызов реестра — до транзакции.** Внутри транзакции держится блокировка строки, а у `lp_app` `idle_in_transaction_session_timeout=15s` (DB-SEC).
- **Профиль:** `country_code` и `type` не меняются (их нет в `OrganizationUpdate`). Сумма `ownership_percent` ≤ 100 проверяется в сервисе (`docs/db/tables/beneficiaries.md`), иначе 400 `field: "beneficiaries"`. Список бенефициаров заменяется целиком (delete + insert в одной транзакции, `docs/db/schema.md`).
- **`lei` очищается пустой строкой: `"lei": ""` → `NULL`** (DECISIONS п. 7). Правило одно для всех PATCH (`lei` здесь, `transfer_method_comment` в BE-07): отсутствующее поле или `null` — не трогать, `""` — очистить, значение — записать. В SQL это `NULLIF(…, '')`. `nullable-type: true` в `oapi-codegen` не включаем: он меняет типы всех nullable-полей во всех пакетах (`gencfg_test.go` требует одинакового конфига).
- **`submitKyc` требует хотя бы одного бенефициара** (DECISIONS п. 8): без бенефициаров AML нечего проверять, кроме компании. Пустой список → 400 `VALIDATION_ERROR`, `field: "beneficiaries"`.
- **Скачивание документа админом пишется в `audit_log`** как `document.download` (DECISIONS п. 16): админ читает чужие документы, журнал нужен для разбора инцидентов. Скачивание владельцем не пишется.
- **Файлы документов из сида** кладёт `make seed-demo` (BE-16, шаг 7); до этого скачивание документа из сида отвечает 404 с предупреждением в логе.

## Шаги

### 1. Контракт и коды ошибок
1. В `api/paths/organizations.yaml`:
   - `uploadMyDocument.description`: «поле `doc_type` передаётся до `file`»;
   - ответ 200 `downloadDocument`: `headers` с `Content-Disposition` (string) и `X-Content-Type-Options` (string, `enum: [nosniff]`).
2. В `api/schemas/organizations.yaml` у `OrganizationUpdate.lei`: `pattern: '^([A-Z0-9]{20})?$'` (пустая строка допустима) и описание «`""` очищает LEI; отсутствие поля или `null` — оставить как есть». У `Organization.lei` (ответ) pattern не меняется.
3. В `api/paths/organizations.yaml` в `description` `submitKyc` дописать: «нужен хотя бы один бенефициар, иначе 400 `VALIDATION_ERROR` с `field: beneficiaries`».
4. Проверить `x-roles` из BE-04:
   - `[borrower, lender]` — у `getMyOrganization`, `updateMyOrganization`, `listMyDocuments`, `uploadMyDocument`, `deleteMyDocument`, `submitKyc`;
   - `[borrower, lender, admin]` — у `downloadDocument`.

   Если BE-04 ещё не добавил их — добавить.
5. Код `FILE_TOO_LARGE` (413):
   - `backend/internal/apperr/apperr.go`: `CodeFileTooLarge` и конструктор `FileTooLarge()`;
   - `handler/errors.go`: строка в `statusByCode`;
   - `api/openapi.root.yaml`: список кодов в разделе «Ошибки»;
   - `backend/README.md`: таблица «Ошибки»; в «Лимиты» заменить «больше — 400» на «файл больше 10 МБ — 413 `FILE_TOO_LARGE`».
6. Запустить `make gen-api`, `make lint-api`, `make diff-api` (ослабление `pattern` в запросе — не breaking).

**Готово:** `make gen-check` чистый; в `organizationsapi/server.gen.go` у ответа 200 `DownloadDocument` есть структура заголовков, и `Visit…` их ставит. Если oapi-codegen заголовки для `application/octet-stream` не сгенерировал — в `handler/organizations.go` объявить свой тип с методом `VisitDownloadDocumentResponse(w http.ResponseWriter) error`: он ставит три заголовка и копирует тело. `CodeFileTooLarge` добавлен в список `TestEveryCodeHasStatus` и в `TestErrorMapping` (`handler/errors_test.go`) со статусом 413.

### 2. SQL
Изменить в `backend/queries/organizations.sql`:
- `GetOrganizationForUpdate :one` — новый: те же колонки, что у `GetOrganizationByID`, плюс `FOR UPDATE`.
- `UpdateOrganization` — добавить `AND kyc_status IN ('draft','rejected')` в `WHERE`; `lei` — `CASE WHEN sqlc.narg('lei')::text IS NULL THEN lei ELSE NULLIF(sqlc.narg('lei')::text, '') END` вместо `COALESCE(sqlc.narg('lei'), lei)` (`null` — не трогать, `""` — очистить).
- `SubmitKycForReview` — добавить `AND kyc_status IN ('draft','rejected')`; `RETURNING` — все колонки, как у `GetOrganizationByID`.
- `ListBeneficiariesByOrg :many` — `WHERE organization_id = $1 ORDER BY created_at, id`.
- `DeleteBeneficiariesByOrg :exec`.
- `CreateBeneficiary :one` — вставка в цикле: максимум 50 строк (`maxItems: 50`), `:copyfrom` не нужен.

Изменить в `backend/queries/kyc.sql`:
- `ListKycDocumentsByOrg` — `ORDER BY uploaded_at, id`.
- `GetKycDocumentVisible :one` — новый, по шаблону BE-04 `*Visible`: `WHERE id = @id AND (@viewer_role::user_role = 'admin' OR organization_id = @viewer_org::uuid)`. Админу `viewer_org` передаётся `uuid.Nil`.
- `DeleteKycDocument` → `:one … RETURNING storage_key`. Нет строки → 404.

Затем `make gen-sqlc`. Вызывающих у изменённых запросов нет (`grep -rn "SubmitKycForReview\|UpdateOrganization\|DeleteKycDocument" backend/internal --include=*.go` находит только `repository/sqlc`).

**Готово:** `make gen-sqlc` без ошибок; `go -C backend build ./...` зелёный; в `docs/db/tables/kyc_documents.md`, `beneficiaries.md` и `organizations.md` раздел «sqlc-запросы» обновлён, а закрытые замечания удалены.

### 3. Таблица переходов KYC
`backend/internal/service/kyc_status.go`:
- тип `KycAction` (`KycEdit`, `KycSubmit`, `KycApprove`, `KycReject`);
- функция `nextKycStatus(from repository.KycStatus, action KycAction) (repository.KycStatus, error)` по таблице из «Решений». Для `edit` возвращает тот же статус.
- Ошибки:
  - для `edit` — `apperr.New(apperr.CodeInvalidTransition, "Профиль и документы можно менять только в статусах draft и rejected")`;
  - для остальных — `apperr.InvalidTransition(from, to)`.

**Готово:** табличный `TestNextKycStatus` покрывает все 16 пар «статус × действие» и зелёный.

### 4. Хранилище файлов и конфиг
1. `backend/internal/storage/local.go`:
   - `type Local struct{ dir string }` и `NewLocal(dir string) (*Local, error)`: `os.MkdirAll(dir, 0o700)` и проба записи — создать и удалить файл. Ошибка → сервер не стартует.
   - `Create() (key string, w io.WriteCloser, err error)` — ключ из `crypto/rand`, `os.OpenFile(…, O_CREATE|O_EXCL|O_WRONLY, 0o600)`.
   - `Open(key) (io.ReadCloser, int64, error)` — размер из `Stat`.
   - `Remove(key) error`.
   - Перед `filepath.Join` каждый ключ проверяется `filepath.IsLocal(key)` и запретом разделителей. Недопустимый ключ → ошибка (защита от traversal через подменённый `storage_key`).
2. `internal/config/config.go`: `DocumentStorageDir` из `DOCUMENT_STORAGE_DIR`, по умолчанию `storage/documents`.
3. Makefile: `DOCUMENT_STORAGE_DIR ?= $(CURDIR)/storage/documents` — абсолютный путь, потому что `make run` работает из `backend/`.
4. `.gitignore`: `/storage/` → `storage/`.
5. `.env.example` и `README.md` («Запуск бэкенда»): переменная и что каталог должен переживать перезапуски. Когда появится контейнер бэкенда — named volume на этот путь.
6. `cmd/server/main.go`: создать `storage.NewLocal(cfg.DocumentStorageDir)`.

**Готово:** `TestLocalStorage` (на `t.TempDir()`) проверяет четыре случая — create → open → содержимое совпадает → remove → `Open` даёт `fs.ErrNotExist`; `Open("../x")` → ошибка. `config_test` проверяет дефолт и чтение `DOCUMENT_STORAGE_DIR`.

### 5. Сервис KYC: каркас и профиль
`backend/internal/service/kyc.go`:
- `type Kyc struct` с зависимостями:
  - `KycStore` — чтения вне транзакции: `GetOrganizationByID`, `ListBeneficiariesByOrg`, `ListKycDocumentsByOrg`, `GetKycDocumentVisible`;
  - `KycTx` — `InTx` по образцу `AuthTx`;
  - `DocumentFiles` — `Create`, `Open`, `Remove`;
  - `OrgScreener` — `ScreenOrganization(ctx, aml.OrganizationSubject) ([]aml.Finding, error)`;
  - `CompanyVerifier` — может быть `nil`, сигнатура из плана BE-06;
  - `Notifier` — из BE-11.
- Конструктор `NewKyc(...)`.

Методы профиля:
- `MyOrganization(ctx, p Principal) (OrganizationProfile, error)`:
  - `p.OrgID == nil` → `apperr.Forbidden`;
  - `OrganizationProfile{Org, Beneficiaries}` — свой тип сервиса (`mds/plan.md`, п. 7: у сущностей со статусом сервис отдаёт свои типы).
- `UpdateMyOrganization(ctx, p, in OrganizationUpdateInput)`:
  - `in.Beneficiaries` — `*[]BeneficiaryInput`: `nil` → не трогать, пустой срез → очистить;
  - сумма долей > 100 → 400 `field: "beneficiaries"`;
  - транзакция: `GetOrganizationForUpdate` → `nextKycStatus(status, KycEdit)` → `UpdateOrganization` → при наличии списка `DeleteBeneficiariesByOrg` + `CreateBeneficiary` в цикле;
  - ошибки уникальности (`registration_number` в стране, `lei`) переводит `pgerr.Map` → 409 `CONFLICT`.

Хендлеры в `handler/organizations.go`:
- зависимость `KycService` — в `Deps` и `Handler` (`handler/handler.go`);
- `Principal` берётся через `currentPrincipal(ctx)` (BE-04, `handler/principal.go`);
- в `UpdateMyOrganization` `req.Body.Beneficiaries != nil` превращается в указатель на срез.

**Готово:** `TestKycUpdateMyOrganization` (фейки стора и транзакции) покрывает пять случаев — `draft` → 200 с новыми полями; `under_review` → 409; сумма долей 60 + 50 → 400; `beneficiaries` не передан → список не тронут; `beneficiaries: []` → список очищен. Очистка `lei` проверяется в `TestKycIntegration` (SQL). `TestGetMyOrganizationHandler` через `httptest`: 200 с `beneficiaries: []` у компании без бенефициаров.

### 6. Документы: список, загрузка, удаление, скачивание
Сервис (`service/kyc_documents.go`):

**`receiveFile(dst io.Writer, src io.Reader, max int64) (mime string, size int64, sum []byte, err error)`** — чистая функция:
- читает до 512 байт (`io.ReadFull`, `io.ErrUnexpectedEOF` — не ошибка) и сравнивает сигнатуры;
- 0 байт → 400 `field: "file"`, «пустой файл»;
- неизвестная сигнатура → 400 `field: "file"`, «допустимы только PDF, PNG, JPEG»;
- больше `max` → `apperr.FileTooLarge()`;
- `const maxDocumentSize = 10 << 20` — совпадает с CHECK в БД.

**`UploadDocument(ctx, p, docType repository.KycDocumentType, fileName string, content io.Reader) (repository.KycDocument, error)`**, порядок:
1. `GetOrganizationByID` и `nextKycStatus(…, KycEdit)` — ранний 409 до приёма 10 МБ.
2. `files.Create()` → `receiveFile`. Ошибка → `files.Remove(key)`.
3. Транзакция: `GetOrganizationForUpdate` и повторный `nextKycStatus` (submit мог пройти, пока шёл файл) → `CreateKycDocument` → аудит `document.upload` (`meta`: `doc_type`, `organization_id`) тем же `q`. Ошибка → `files.Remove(key)`.
4. Имя файла:
   - `fileName` нужен только для показа;
   - убрать управляющие символы и обрезать до 255 рун;
   - пустое → `document`.

**`DeleteDocument(ctx, p, id)`**: транзакция `GetOrganizationForUpdate` → `nextKycStatus(…, KycEdit)` → `DeleteKycDocument` (нет строки → 404). После коммита `files.Remove`, ошибку логировать на `Warn`.

**`OpenDocument(ctx, p, id) (DocumentFile, error)`**:
- `GetKycDocumentVisible(id, p.Role, orgOrNil)`: чужой или несуществующий → 404 `NOT_FOUND`;
- `files.Open`: `fs.ErrNotExist` → 404 и `slog.Warn` с `document_id` (случай документов из сида);
- если `p.Role == admin` — `audit_log` `document.download` (`entity_type = document`, `meta`: `organization_id`) через хелпер BE-14 (`service/audit.go`); если его ещё нет — минимальная версия по README индекса и отметка в PR. Транзакции у скачивания нет, поэтому запись — до отдачи файла; ошибка записи → 500, файл не отдаётся (BE-14: «действие без следа в журнале хуже отказа»).

Хендлер `UploadMyDocument`:
- цикл `req.Body.NextPart()`;
- часть `doc_type`: `io.LimitReader(part, 64)`, `api.KYCDocumentType(v).Valid()`, иначе 400 `field: "doc_type"`;
- часть `file`:
  - если `doc_type` ещё не было → 400 `field: "doc_type"`, «обязательное поле, передаётся до file»;
  - иначе вызвать сервис с `part.FileName()` и читателем-обёрткой;
- обёртка переводит `*http.MaxBytesError` в `apperr.FileTooLarge()`; та же проверка — для ошибки `NextPart()`;
- конец формы без `file` → 400 `field: "file"`.

Хендлер `DownloadDocument`: ответ 200 с `Body`, `ContentLength` и заголовками из шага 1.

Read deadline загрузки: в `NewRouter` (или middleware рядом с `MaxBody`) для `isUpload(r)` вызвать `http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadReadTimeout))`, `const uploadReadTimeout = 60 * time.Second`. Удалить `TODO(uploadMyDocument)` в `validate.go`.

**Готово:**
- табличный `TestReceiveFile` зелёный (случаи — в «Тестах»);
- `TestUploadMyDocumentHandler` через настоящий роутер даёт 201, 400, 413 и 409 по таблице из «Тестов»;
- ручная проверка:
  ```sh
  curl -b cookies -H "X-CSRF-Token: $T" -F doc_type=charter -F file=@charter.pdf localhost:8080/api/v1/organizations/me/documents
  ```
  → 201, файл лежит в `$DOCUMENT_STORAGE_DIR` под 32-символьным hex-именем;
- `curl -i …/documents/<id>/file` → заголовки `Content-Disposition: attachment; filename*=utf-8''…` и `X-Content-Type-Options: nosniff`.

### 7. `submitKyc`
`Kyc.SubmitKyc(ctx, p Principal) (OrganizationProfile, error)`.

**До транзакции:**
1. `GetOrganizationByID` и `nextKycStatus(…, KycSubmit)` — ранний 409.
2. Сверка с реестром, если `verifier != nil`:
   - RU → `Lookup(ctx, "RU", registration_number)`;
   - не-RU с `lei` → `Lookup(ctx, country, lei)`;
   - не-RU без `lei` → `Registry{Outcome: RegistryNoIdentifier}`;
   - ошибка `registry.ErrUnavailable` → `Registry{Outcome: RegistryUnavailable}`, submit продолжается: это «мягкая деградация» из issue #22.

**Транзакция** `InTx`, по порядку:
1. `GetOrganizationForUpdate` → `nextKycStatus(status, KycSubmit)`.
2. `ListKycDocumentsByOrg` → множество типов → недостающие обязательные → 400 с `details`.
3. `ListBeneficiariesByOrg` → пусто → 400 `field: "beneficiaries"` → иначе `aml.OrganizationSubject{Name, CountryCode, Beneficiaries, Registry}` → `screener.ScreenOrganization`.
4. `SubmitKycForReview` (нет строки → 409).
5. Запись в `status_history`: `entity_type = organization`, `from`/`to`, `actor_user_id = p.UserID`, `actor_role = p.Role`. Хелпер `service/status_history.go` (BE-07); если его ещё нет — минимальная версия по README и отметка в PR.
6. Если сверка была — `CreateRegistryCheck` с `organization_id` (маппинг из плана BE-06).
7. `saveAmlFindings(ctx, q, orgID, nil, findings)` (BE-13, шаг 6).
8. Аудит `kyc.submit` тем же `q` (BE-14: аудит пишется в транзакции действия; если хелпера ещё нет — минимальная версия по README индекса).
9. Notifier → всем активным админам:
   - `kyc_submitted`, entity `organization`;
   - если создан хотя бы один флаг — одно `aml_flag_created` на весь скрининг.

**После коммита:**
1. Перечитать профиль и вернуть его.

Подключить `aml.NoopScreener{}` (или настоящий из BE-13) и `verifier` (`nil` или реализацию из BE-06) в `cmd/server/main.go`.

**Готово:**
- `TestSubmitKyc` (фейки) зелёный по таблице из «Тестов»;
- интеграционный `TestSubmitKycIntegration`: компания в `draft` с четырьмя документами → `under_review`, одна строка в `status_history` и по `notifications` на каждого админа; повторный вызов → 409.

### 8. Документация
`backend/README.md`:
- «Статус реализации» — 7 операций;
- раздел organizations — правило «`doc_type` до `file`», 413, «скачивание: владелец или admin, `Content-Disposition: attachment`».

`docs/db/schema.md` → «Покрытие sqlc»: обновить списки запросов `organizations.sql`/`kyc.sql` и убрать закрытые пробелы (бенефициары, получение документа).

**Готово:** `git diff --stat` показывает изменения в `backend/README.md`, `README.md` и `docs/db/`; `make verify` зелёный.

## Тесты
- `TestNextKycStatus` — 16 строк таблицы: `draft+submit → under_review`, `rejected+submit → under_review`, `under_review+submit → 409`, `verified+edit → 409`, `rejected+edit → ok`, …
- `TestReceiveFile` (табличный):

  | Вход | Ожидание |
  |---|---|
  | `%PDF-1.4…` | `application/pdf` |
  | 8 байт PNG + данные | `image/png` |
  | `FF D8 FF E0…` | `image/jpeg` |
  | PDF-содержимое | `application/pdf` — имя `x.png` на тип не влияет |
  | `GIF89a`, `PK\x03\x04`, `MZ`, `<html>`, `%PDF` без дефиса | 400 `field=file` |
  | 0 байт | 400 |
  | ровно `10<<20` байт | ok |
  | `10<<20 + 1` | `FILE_TOO_LARGE` |
  | файл короче 512 байт с верной сигнатурой | ok |
  | любой успешный вход | `sha256` совпадает с `sha256.Sum256(content)` |

- `TestUploadMyDocumentHandler` (настоящий роутер, фейковый сервис, где нужно):

  | Запрос | Ответ |
  |---|---|
  | без `file` | 400 `field=file` |
  | `file` раньше `doc_type` | 400 `field=doc_type` |
  | `doc_type=passport` | 400 `field=doc_type` |
  | тело 10 МБ + 100 КБ | 413 `FILE_TOO_LARGE` (срабатывает `MaxBody`) |
  | файл 10 МБ + 1 Б | 413 |
  | сервис вернул `InvalidTransition` | 409 |
  | `Content-Type: application/json` | 400 |

- `TestUploadDocument` (сервис): ошибка `CreateKycDocument` → `Remove` вызван с тем же ключом; статус сменился на `under_review` между ранней проверкой и транзакцией → 409 и `Remove` вызван.
- `TestDeleteDocument`: чужой id → 404; `under_review` → 409, строка и файл на месте; успех → `Remove` вызван после коммита.
- `TestDownloadDocument`: свой → 200 + заголовки, аудита нет; чужой (borrower B запрашивает документ A) → 404; admin → 200 и одна запись `document.download`; файла нет на диске → 404.
- `TestSubmitKyc`:

  | Состояние | Ответ |
  |---|---|
  | `draft`, 4 документа, borrower | 200 `under_review` |
  | `draft`, 4 документа, бенефициаров нет | 400 `field=beneficiaries`, скрининг не вызван |
  | lender без `source_of_funds` | 400, в `details` `source_of_funds` |
  | нет `charter` и `address_proof` | 400, две записи в `details` |
  | `under_review` | 409 |
  | `verified` | 409 |
  | `rejected` + документы | 200, `kyc_reject_reason` = null |
  | скрининг вернул ошибку | 500, `SubmitKycForReview` не вызван |
  | скрининг вернул 2 находки | `CreateAmlFlag` ×2 и одно `aml_flag_created` |
  | `verifier` вернул `ErrUnavailable` | 200, `Registry.Outcome = unavailable` передан в скрининг |

- Интеграционные (`-short` пропускает): `TestKycIntegration` — PATCH с бенефициарами → GET возвращает их; повторный PATCH с `[]` → пусто; PATCH `lei: "DEMO00BETACORP000162"` → записан, PATCH без `lei` → не изменился, PATCH `lei: ""` → `NULL`; загрузка PNG → строка в `kyc_documents` с `sha256` длиной 32; удаление → строки нет, файла нет.

## Готово, когда
- [ ] PATCH, upload и delete в `under_review`/`verified` → 409 `INVALID_STATUS_TRANSITION` (тесты `TestKycUpdateMyOrganization`, `TestDeleteDocument`, `TestUploadDocument`).
- [ ] Загрузка: multipart читается потоком; файл > 10 МБ → 413; тип по сигнатуре (PDF/PNG/JPEG), иначе 400; в БД `sha256` (32 байта) и случайный `storage_key`; файл лежит в `DOCUMENT_STORAGE_DIR` с правами `0600` (тесты `TestReceiveFile`, `TestUploadMyDocumentHandler`).
- [ ] Скачивание: своя организация или admin, чужой → 404; в ответе `Content-Disposition: attachment` и `X-Content-Type-Options: nosniff` (`TestDownloadDocument`).
- [ ] PATCH `lei: ""` очищает LEI; контракт описывает правило (`TestKycIntegration`).
- [ ] Скачивание документа админом пишет `document.download` в `audit_log` (`TestDownloadDocument`).
- [ ] `submitKyc`: без обязательных документов или без бенефициаров → 400; успех → `under_review`, строка `status_history`, AML-скрининг вызван, админы получили `kyc_submitted` (`TestSubmitKyc`, `TestSubmitKycIntegration`).
- [ ] Табличные тесты переходов KYC и валидации файлов зелёные.
- [ ] И общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: закрыты в [DECISIONS.md](DECISIONS.md) — очистка `lei` (п. 7), бенефициар для `submitKyc` (п. 8), аудит скачивания (п. 16); файлы сида — BE-16, шаг 7.
