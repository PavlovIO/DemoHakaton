# BE-02 · Схема БД и миграции

Issue: #18 · Milestone: M1 · Зависит от: — (шаг 3 трогает `config.go` и `main.go`, их меняет PR #67, поэтому его делать после мержа #67) · Блокирует: BE-13 (правила по `fatf_list`), BE-16 (сид поверх справочников: страна Delta `KW`, способы `spfs`/`cips`)

## Цель

Справочники доведены до полноты новой миграцией: все страны ISO 3166-1 с серым и чёрным списками FATF (июнь 2026) и перечнем 430-р, 22 валюты, способы перевода с СПФС и CIPS. Сервер сам периодически удаляет просроченные сессии. Решения ревью схемы и владельцы оставшихся пробелов записаны в `docs/db/schema.md`.

## Что уже есть

- Схема, сид справочников и smoke-тест инвариантов: `backend/migrations/00001_init.sql`, `00002_seed_dictionaries.sql`, `testdata/invariants_test.sql` (19 проверок) — PR #50. `backend/sqlc.yaml` (`schema: migrations`), 11 файлов `backend/queries/*.sql`, сгенерированный `internal/repository/sqlc/` — PR #52–#55. **Второй чекбокс issue («sqlc.yaml, queries, make gen») уже выполнен**, его осталось только отметить.
- `00002_seed_dictionaries.sql`:
  - 32 страны. Чёрный список FATF — `IR`, `KP`, `MM` с `fatf_as_of = 2026-06-19`. Серых нет ни одной, хотя `VN` в сиде есть и в сером списке июня 2026 состоит. `is_unfriendly = true` у `CY DE NL FR IT AT LU IE CH GB US SG` — все они есть в перечне 430-р. В шапке файла прямо сказано «сверить с fatf-gafi.org» и «сверить с актуальной редакцией 430-р»;
  - 13 валют (`RUB EUR USD CNY AED KZT TRY HKD CHF GBP INR AMD BYN`), у всех `minor_units = 2`;
  - 3 способа перевода: `swift`, `sepa`, `other`;
  - Down удаляет все строки трёх таблиц.
- Потребители этих полей: `fatf_list` используется только в `ListCountries` (`backend/queries/dictionaries.sql`, `fatf_high_risk = fatf_list IS NOT NULL`). `is_unfriendly` сейчас не читает ни один запрос и ни одна схема API.
- ⚠️ В контракте `TransferMethod` — enum `[swift, sepa, other]` (`api/schemas/enums.yaml`), а `CurrencyCode` и `CountryCode` — только `pattern`. Новые способы перевода без правки enum валидатор отобьёт с `400`; новые валюты и страны контракт пропускает сам.
- ⚠️ Деньги хранятся как `numeric(17,2)`, а у `JPY` и `VND` по ISO 4217 `minor_units = 0`. Сумма `100.50 JPY` ляжет в БД без ошибки. Проверять дробную часть по `minor_units` — дело сервиса заявок и офферов (BE-07, BE-08); в этой задаче только правильно заполнить `minor_units`.
- `backend/queries/sessions.sql: DeleteExpiredSessions` (`:exec`) и индекс `sessions_expires_idx` есть, но запрос никто не вызывает. Просроченная сессия и так не пускает: `GetSessionByTokenHash` фильтрует `expires_at > now()`. Поэтому очистка нужна для гигиены хранилища, а не для безопасности.
- ⚠️ `make migrate-create` вызывает `goose create` без `-s` и создаёт файл с версией-таймстемпом (`20261023…_x.sql`), а существующие миграции нумеруются последовательно (`00001`, `00002`). Переименуй файл в следующий свободный номер. DB-SEC тоже добавляет миграцию (`app_role_grants`), поэтому номер бери тот, что свободен на момент PR. При двух файлах с одинаковым номером goose падает с ошибкой дубля версии — после rebase проверь `make migrate-status`.
- ⚠️ В `AGENTS.md` сказано: «менять уже существующие миграции — сначала спроси». Поэтому `00002` не правим, исправления идут новой миграцией.
- UPDATE-запросы переходов фильтруют только по `id`. Условие по текущему статусу добавляют задачи-владельцы сущностей, вместе со своими операциями: заявки — BE-07 (параметр `from_status`), `UpdateOrganization` и `SubmitKycForReview` — BE-05, `ApproveKyc` и `RejectKyc` — BE-12. В эту задачу оно не входит.
- В `docs/db/schema.md` есть раздел «Чего не хватает и где запросы расходятся с этим документом» — список пробелов без владельцев.

## Решения

- **Справочники расширяются одной новой миграцией** `0000N_dictionaries_full.sql`; `00002` не трогаем — она уже применена в dev-базах и в CI. Up — `INSERT … ON CONFLICT (code) DO UPDATE`, Down возвращает ровно состояние после `00002`.
- **Страны — полный ISO 3166-1 alpha-2** (249 кодов, `name_ru`, `name_en`) (DECISIONS п. 21). Кредитор может прийти из любой юрисдикции, а неполный справочник ломает регистрацию и фильтры. Русские названия — краткие из ОКСМ, английские — short name ISO 3166-1. У 32 стран из `00002` названия не меняются, обновляются только поля FATF и 430-р.
- **FATF — по итогам пленара июня 2026**, `fatf_as_of = 2026-06-19` (DECISIONS п. 22). До защиты список не обновляем, даже если итоги пленара 28–30.10.2026 выйдут раньше: `fatf_as_of` честно показывает дату сверки. Обновление — отдельной миграцией после 31.10 (DECISIONS п. 34).
- **`fatf_as_of` — дата публикации итогов пленара FATF**, из которого взят список; у стран вне списков — `NULL`. Ссылки на источники пишутся комментарием в миграции, чтобы следующая сверка начиналась с них.
- **`is_unfriendly` — перечень 430-р в редакции № 3216-р**: 49 государств, британские коронные владения и заморские территории, регионы ЕС с отдельным кодом ISO. Владения других стран перечня — `false`: в акте они не названы (DECISIONS п. 35). Поле только справочное, в AML-правилах не участвует (DECISIONS п. 19).
- **Валюты — 22** (DECISIONS п. 23): к 13 из `00002` добавляются `UZS KGS AZN GEL SGD THB VND EGP JPY`. `minor_units` — по ISO 4217 (`JPY`, `VND` — 0, остальные — 2).
- **Способы перевода — +`spfs` (СПФС Банка России) и `cips` (CIPS)** (DECISIONS п. 24): основные каналы платежей из РФ в обход SWIFT. Фильтр витрины по способу — BE-07.
- **Очистка сессий — горутина в процессе сервера с тикером.** pg_cron в образе `postgres:16` нет. Чистка при логине добавила бы DELETE в горячий путь. Интервал задаёт `SESSION_CLEANUP_INTERVAL`, по умолчанию `1h`.
- **`DeleteExpiredSessions` → `:execrows`**: число удалённых строк пишется в лог.

## Шаги

### 1. Ревью схемы и отметка сделанного

1. В #18 отметь чекбокс про `sqlc.yaml` и дай ссылки на PR #52–#55.
2. Оставь в #18 комментарий с тремя вопросами ревью, у каждого — предлагаемое решение:
   - **Один аккаунт на компанию** (`users_one_per_org_uq`). Для MVP оставляем. Чтобы снять ограничение, достаточно отдельной миграцией удалить индекс; код от индекса не зависит.
   - **Полиморфная `status_history`** (`entity_type` + `entity_id` без FK). Оставляем: журнал только дополняется, читается по паре (`entity_type`, `entity_id`). Целостность обеспечивается тем, что запись пишется в той же транзакции, что и переход (`internal/service/status_history.go`, BE-07).
   - **enum или справочники.** Статусы, роли, причины — это enum: их значения меняются только вместе с кодом. Страны, валюты и способы перевода — это таблицы: это данные, и API отдаёт их как справочники.
3. Когда решения подтвердит хотя бы ещё один бэкендер, перенеси их в `docs/db/schema.md`, в новый подраздел «Решения ревью схемы» перед «Что сознательно не делаем в MVP».

**Готово:** чекбоксы «sqlc» и «ревью» в #18 отмечены; в `docs/db/schema.md` есть подраздел «Решения ревью схемы» с тремя пунктами.

### 2. Полные справочники с FATF и 430-р

Данные уже сверены с первоисточниками — бери их из [DECISIONS.md](DECISIONS.md), раздел «Факты из первоисточников»:
- FATF, июнь 2026: чёрный — `IR KP MM`; серый (22) — `AO BO BA BG CM CI CD HT IQ KE KW LA LB MC NP PG SS SY VE VN VG YE`; URL трёх страниц FATF;
- 430-р, ред. № 3216-р: 49 государств; британские территории `JE GG IM AI VG GI BM KY FK MS PN SH TC IO GS`; регионы ЕС `AX GF GP MQ RE YT MF` (по п. 35 — тоже `true`).

1. Список стран собери одноразовым скриптом (в репозиторий не коммитить): эталон кодов — 249 официально присвоенных кодов ISO 3166-1 alpha-2 и `name_en` (short name, например пакет `iso-codes`); `name_ru` — краткое наименование из ОКСМ по тому же коду. Коды ОКСМ вне ISO (`AB`, `OS`) не берём. На выходе — SQL `VALUES`, отсортированный по `code`. Источники и дату выгрузки — в комментарий миграции.
2. `make migrate-create name=dictionaries_full`, переименуй файл в `backend/migrations/0000N_dictionaries_full.sql` (N — следующий свободный номер). Шапка — комментарий с URL FATF, реквизитами 430-р (№ 430-р от 05.03.2022 в ред. № 3216-р от 29.10.2022, publication.pravo.gov.ru `0001202203070001`), ОКСМ и датой сверки 23.10.2026.
   - Up, страны: `INSERT INTO countries (code, name_ru, name_en) VALUES … ON CONFLICT (code) DO NOTHING` — 32 существующие строки не меняются. Затем три `UPDATE`:
     - `SET fatf_list = NULL, fatf_as_of = NULL` для всех, потом `SET fatf_list = 'black', fatf_as_of = '2026-06-19' WHERE code IN (…)` и то же для `'grey'`;
     - `SET is_unfriendly = (code IN (…))` одним выражением по полному списку из 49 + 15 + 7 кодов.
   - Up, валюты: `INSERT … ON CONFLICT DO NOTHING` — 9 новых кодов с `minor_units` по ISO 4217.
   - Up, способы перевода: `('spfs', 'СПФС Банка России')`, `('cips', 'CIPS (Китай)')`.
   - Down: удалить добавленные коды из трёх таблиц и вернуть 32 странам значения `fatf_list`, `fatf_as_of`, `is_unfriendly` из `00002`. Если на добавленные коды уже ссылаются строки (`organizations`, `applications`), Down падает по FK — для dev это приемлемо, в комментарии Down так и написать.
3. В описание PR — таблица изменений по 32 исходным странам «код — было — стало» (меняются как минимум `VN` → `grey`; `is_unfriendly` у исходных совпадает с перечнем) и итоговые количества.
4. `testdata/invariants_test.sql` — три новые проверки: `count(*) FROM countries` = 249; `count(*) WHERE fatf_list = 'grey'` = 22, `'black'` = 3; `count(*) WHERE is_unfriendly` = 71.
5. Контракт: в `api/schemas/enums.yaml` `TransferMethod.enum` → `[swift, sepa, spfs, cips, other]`; `make gen-api`, `make lint-api`, `make diff-api` (новое значение enum в ответах oasdiff может пометить как ломающее — фронта ещё нет, отметить в PR). Проверь `grep -rn "TransferMethodSwift\|TransferMethodOther" backend/internal` — если где-то есть `switch` по enum, добавь ветки.
6. Обнови `docs/db/schema.md` (абзац под таблицей «Справочники»: убрать «нужно сверить…» и «не полный ISO», записать даты и источники) и `docs/db/tables/countries.md`, `currencies.md`, `transfer_methods.md`, если они есть.

**Готово:**
- `make db-reset` проходит (миграции, инварианты с новыми проверками, демо-сид);
- `make migrate-down && make migrate-up` на БД без демо-сида проходят без ошибок;
- `curl localhost:8080/api/v1/dictionaries/countries` отдаёт `"fatf_high_risk": true` для `VN` и `KW`, `false` для `AE`;
- `api/openapi.yaml` содержит `spfs` и `cips` в `TransferMethod`;
- `GET /api/v1/dictionaries/currencies` — 22 валюты (в API `minor_units` нет; в БД `SELECT code FROM currencies WHERE minor_units = 0` → `JPY`, `VND`); `GET /api/v1/dictionaries/transfer-methods` содержит `spfs` и `cips`;
- `make gen-check` без диффа.

### 3. Периодическая очистка просроченных сессий

Делать после мержа PR #67: он меняет `internal/config/config.go`, `cmd/server/main.go` и `.env.example`.

1. `backend/queries/sessions.sql`: `-- name: DeleteExpiredSessions :execrows`, затем `make gen-sqlc`.
2. Новый файл `backend/internal/service/sessions_cleanup.go`:
   ```go
   // SessionCleanupStore — то, что чистке нужно от репозитория (*repository.Queries подходит).
   type SessionCleanupStore interface {
       DeleteExpiredSessions(ctx context.Context) (int64, error)
   }

   // RunSessionCleanup удаляет просроченные сессии сразу и затем раз в interval, пока ctx не отменён.
   func RunSessionCleanup(ctx context.Context, store SessionCleanupStore, interval time.Duration)
   ```
   Ошибка запроса пишется в `slog.Warn`, цикл продолжается. Если удалено больше нуля строк — `slog.Info("expired sessions deleted", "count", n)`.
3. `internal/config/config.go`: поле `SessionCleanupInterval` из `SESSION_CLEANUP_INTERVAL`, по умолчанию `1h`, только положительная длительность (по образцу `SHUTDOWN_TIMEOUT`). Добавь случай в `config_test.go`, строку в `.env.example`, строку в корневой `README.md` («Запуск бэкенда»).
4. `cmd/server/main.go`, `run`: после создания `queries` запусти чистку в горутине под `sync.WaitGroup`. После `srv.Shutdown` вызови `wg.Wait()`. Иначе `defer pool.Close()` может закрыть пул посреди DELETE.

**Готово:** `TestRunSessionCleanup` (фейковый store, interval 10 мс) зелёный: первый вызов происходит сразу, ошибка store не останавливает цикл, после отмены ctx функция возвращается. `TestDeleteExpiredSessionsIntegration` зелёный в `make test-all`. При `make run` с `SESSION_CLEANUP_INTERVAL=10s` и вручную вставленной просроченной сессией в логе через ≤ 10 с есть `expired sessions deleted count=1`.

### 4. Пробелы из `docs/db/schema.md` — с владельцами

Перепиши раздел «Чего не хватает и где запросы расходятся с этим документом» в таблицу «Пробел | Задача | Статус». Владельцы берутся из индекса `docs/backend_tasks/README.md`:

| Пробел | Задача |
|---|---|
| `beneficiaries`: нет запросов замены списка | BE-05 |
| `kyc_documents`: нет получения по `id` для скачивания | BE-05 |
| `status_history`: нет чтения для `GET /applications/{id}/history` | BE-07 |
| Витрина: `ListApplications` без JOIN `organizations`, без `my_offer_id`, сортировка не по `published_at` | BE-07 |
| `aml_flags`: нет `Count` и `GetByID` | BE-12 (списки и закрытие флага), BE-13 (запись) |
| `sanctions_entries`: нет запросов загрузчика | BE-13 |
| `audit_log`: нет фильтров и `Count`; аналитика без периода и с лишним исключением `cancelled` | BE-14 |
| IDOR: `GetApplicationByID`, `GetOfferByID`, `GetDealByID`, списки на `narg` | BE-04 |
| IDOR: `ListOffersByApplication`, UPDATE офферов | BE-08 |
| IDOR: `ListMessages` | BE-10 |
| Принятие оффера: `GetApplicationForUpdate` без владельца и статуса, `RejectOtherOffers` без `RETURNING`, `fee_amount` считается в SQL | BE-09 |
| Переходы статусов без условия в `WHERE` | BE-07 (заявки), BE-05 и BE-12 (KYC) |

**Готово:** у каждой строки раздела есть задача из индекса README.

## Тесты

- `TestRunSessionCleanup` (`internal/service/sessions_cleanup_test.go`, без БД): фейк считает вызовы → ≥ 1 вызов до первого тика; фейк возвращает ошибку → цикл продолжается; `cancel()` → функция вернулась в пределах 100 мс.
- `TestDeleteExpiredSessionsIntegration` (`internal/service/`, переиспользует `newPool` из `dictionaries_integration_test.go`, всё внутри транзакции с `Rollback`): пользователь, сессия с `expires_at = now() - interval '1 minute'` и сессия с `now() + interval '1 hour'` → `DeleteExpiredSessions` возвращает `1`, живая сессия осталась.
- Справочники: новые проверки `invariants_test.sql` из шага 2 (249 стран, 22 серых, 3 чёрных, 71 `is_unfriendly`) — в `make db-reset`; `TestListCountriesIntegration` (если есть в `dictionaries_integration_test.go` — дополнить, иначе создать): `VN` и `KW` → `FatfHighRisk = true`, `AE` → `false`.

## Готово, когда

- [ ] В #18 отмечен чекбокс «`sqlc.yaml`, `backend/queries/`, `make gen`» со ссылками на PR #52–#55.
- [ ] Решения ревью схемы (один аккаунт на компанию, полиморфная `status_history`, enum или справочники) подтверждены в #18 и записаны в `docs/db/schema.md`.
- [ ] Новая миграция `0000N_dictionaries_full.sql`: 249 стран ISO 3166-1, FATF июня 2026 (`fatf_as_of = 2026-06-19`), `is_unfriendly` по 430-р в ред. № 3216-р с территориями, 22 валюты, `spfs` и `cips`; источники — в её комментарии; `make db-reset` и `migrate-down`/`migrate-up` проходят.
- [ ] Сервер удаляет просроченные сессии раз в `SESSION_CLEANUP_INTERVAL`; `TestRunSessionCleanup` и `TestDeleteExpiredSessionsIntegration` зелёные.
- [ ] Раздел пробелов в `docs/db/schema.md` — таблица с задачей-владельцем у каждой строки.
- [ ] И общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: все закрыты в [DECISIONS.md](DECISIONS.md) (п. 19, 21–24, 34, 35). Обновление FATF по пленару 28–30.10.2026 — отдельная задача после 31.10.
