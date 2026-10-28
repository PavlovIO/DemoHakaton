# BE-16 · Сид демо-данных

Issue: #32 · Milestone: M4 · Зависит от: BE-03 (вход по паролю), BE-05 (где лежат файлы документов), BE-06 (мок-фикстуры реестра), BE-13 (нормализация имён, мок-список) · Блокирует: US-14 (сквозной демо-сценарий), QA-01

## Цель
`make db-reset` (или `make seed` на чистой БД) даёт рабочее демо: под каждым демо-аккаунтом входят одним общим паролем из README. Вариант **B** (по умолчанию) сразу показывает принятие оффера и сделку «Альфа ↔ Beta Corp» (500 000 EUR, 8 %, комиссия 1 % = 5 000.00 EUR), конкурирующий оффер Epsilon и историю за ~3 месяца для аналитики. Вариант **A** (`make db-reset SEED=a`) показывает KYC с нуля. В обоих: 8 демо-компаний, на витрине заявки для фильтров, у Gamma открыт `high`-флаг совпадения с мок-санкциями, у Delta — `medium`-флаг юрисдикции FATF.

## Что уже есть
- `backend/migrations/testdata/seed_demo.sql` (идемпотентный: явные UUID + `ON CONFLICT DO NOTHING` / `WHERE NOT EXISTS`) — по сути **вариант B** на двух компаниях:
  - «Альфа» (RU, `7700000001`) и «Beta Corp» (CY, `CY-123456`) — `verified`, по бенефициару у каждой;
  - пользователи `aaaaaaaa-0000-0000-0000-00000000000{1,2,3}` — admin, `alpha@liquidity.local`, `beta@liquidity.local`;
  - 6 KYC-документов;
  - заявка `3333…` `published` 500 000 EUR → DE, 6 мес., swift;
  - оффер `4444…` `sent` 8 % + 1 %, `bullet`;
  - `status_history` заявки и оффера, 4 уведомления, 3 сообщения чата.
- `Makefile`: `seed-demo` (`docker cp` + `psql -f` — так кириллица не ломается в PowerShell) входит в `make db: up migrate-up test-invariants seed-demo`. Цели `seed`, которую называет issue, нет.
- Пароли хэширует `service.HashPassword` (`backend/internal/service/password.go`): PHC-строка `$argon2id$v=19$m=65536,t=3,p=4$<salt RawStd base64>$<key RawStd base64>`, пароль нормализуется в NFC. Длина пароля — 10–128 символов (`ValidatePassword`, `RegisterRequest.password`).
- Ограничения схемы, которые сид обязан соблюдать (`00001_init.sql`): заёмщик — только `RU`, кредитор — только не `RU` (`organizations_type_country_chk`); `verified` ⇔ `verified_at` (`organizations_verified_chk`); один пользователь на компанию (`users_one_per_org_uq`); `agreed` ⇔ `accepted_offer_id` (`applications_agreed_chk`); `rejected` ⇔ `reject_reason` и `sent` ⇔ `decided_at IS NULL` у офферов; у сделки `application_id` и `offer_id` уникальны, стороны разные, `fee_percent` и `fee_amount` либо оба, либо ни одного.
- Каталог `/storage/` уже в `.gitignore` (локальные файлы KYC).
- ⚠️ **Пароли в сиде — заглушки** `$argon2id$v=19$PLACEHOLDER_…`. `parsePasswordHash` на них возвращает не-доменную ошибку, поэтому `POST /auth/login` под демо-аккаунтом отвечает **500 `INTERNAL`**, а не 401.
- ⚠️ Документы в сиде ссылаются на `storage_key` (`alpha-charter.pdf` …), которых нет на диске, и у всех один фиктивный `sha256`. По плану BE-05 `downloadDocument` на такой документ отвечает 404. Кроме того, такие ключи не подходят под формат BE-05 (32 hex).
- ⚠️ Заголовки уведомлений зашиты как `APP-000001`. Это верно, только пока заявка вставлена первой на чистой БД (`applications.number` — identity).
- ⚠️ ИНН «Альфы» `7700000001` не проходит контрольную сумму ИНН-10. Новый — в таблице шага 1.
- ⚠️ У верифицированных компаний нет записей `status_history` о KYC (`draft → under_review → verified`), а у Beta нет `registry_checks` — карточка админа (BE-12) будет неполной.
- ⚠️ CI (`.github/workflows/backend.yml`, job `test`) делает только `make migrate-up`: сид в CI не применяется, поломку SQL сида никто не заметит.
- ⚠️ Хранилище документов взято из плана BE-05 (`DOCUMENT_STORAGE_DIR`, ключ — 32 hex). Если при реализации BE-05 формат изменится — сверить шаг 7 до мержа.
- Сценарий US-14 (`scripts/issues.yaml`, ключ `US-14`):
  1. регистрация и верификация «Альфы» и «Beta Corp»;
  2. заявка 500 000 EUR / 6 мес. / DE;
  3. фильтр «Германия»;
  4. оффер 8 % + 1 %;
  5. чат и принятие;
  6. «Согласовано» + комиссия 5 000 EUR.

## Решения
- **Три файла вместо одного:** `seed_demo_common.sql` (общее), `seed_demo_a.sql`, `seed_demo_b.sql`. Make-переменная `SEED ?= b` выбирает вариант. **B — по умолчанию** (DECISIONS п. 25): `make db` нужен разработчикам каждый день, им удобнее готовые данные. Варианты несовместимы на одной БД (разные статусы тех же организаций), поэтому смена варианта — только через `make db-reset SEED=a`.
- **Восемь вымышленных компаний** из DECISIONS п. 27 — таблица шага 1. Это единственный источник демо-идентификаторов: фикстуры мок-реестра BE-06 и проверка «далеко от мок-санкций» BE-13 берут значения отсюда.
- **ИНН и LEI — с корректными контрольными суммами** (DECISIONS п. 29): ИНН-10 — алгоритм ФНС (веса `2 4 10 3 5 9 4 6 8`, `сумма mod 11 mod 10`); LEI — ISO 17442 (буквы → `10…35`, всё число `mod 97 = 1`). Префикс LEI `DEMO` не выдан ни одному LOU, поэтому с реальными LEI они не совпадут.
- **Один общий пароль на все демо-аккаунты** (DECISIONS п. 30) — `Demo-Pass-2026`, записан в README. Хэш генерируется один раз и коммитится в SQL; соответствие паролю проверяет unit-тест. Генерация при каждом сиде потребовала бы Go-рантайма в `make seed` и давала бы новый diff на каждый запуск.
- **На демо вживую регистрируется другая компания**, «Альфа» берётся из сида (DECISIONS п. 26). Повторная регистрация «Альфы» дала бы 409 (`organizations_regnum_uq`, уникальный email). Шаг 1 US-14 показывается на любой новой компании — это записывается в чек-лист README.
- **AML-совпадение — у «Gamma GmbH», а не у «Альфы» или Beta.** Открытый `high`-флаг блокирует approve KYC (BE-12, 409 `CONFLICT`). Флаг на компании основного сценария сломал бы US-14.
- **Флаги Gamma и Delta вставляются сидом**, как будто скрининг уже прошёл при `submitKyc`: сид обходит сервис и сам скрининг не запускает (DECISIONS п. 45). Запуск скрининга потребовал бы Go-команды `cmd/seed` и сделал бы сид зависимым от BE-13; живой скрининг на демо показывает повторная подача KYC Gamma. Запись `GAMA GmbH` живёт в мок-списке BE-13 (`internal/aml/sanctionslist/mock.json`), а не в сиде (DECISIONS п. 31): `make load-sanctions` удаляет и заново заливает все записи `source='mock'`, так что вставленная сидом строка пропала бы. Поэтому в `make db` `load-sanctions` идёт раньше `seed-demo`. Повторный live-скрининг даёт те же флаги.
- **Фоновые заявки для фильтров принадлежат «Вектору»** (RU, `verified`). В варианте A «Альфа» не верифицирована и не может легально иметь опубликованные заявки, а `users_one_per_org_uq` разрешает одного пользователя на компанию.
- **История для аналитики — только в B** (DECISIONS п. 28), генерируется `generate_series` детерминированно: UUID — `md5('demo-hist-<сущность>-' || i)::uuid`, суммы, валюты, страны и статусы — функции от `i`, даты — `now() - <интервал от i>`. `random()` не используется: повторный сид не добавляет строк, а графики одинаковы на каждом стенде.
- **Номера в `title` уведомлений вычисляются из `applications.number`** (`'APP-' || lpad(number::text, 6, '0')` в `INSERT … SELECT`), без жёстко заданного `APP-000001`.
- **Файлы документов — настоящие маленькие PDF в репозитории**, копируются в хранилище BE-05 той же make-целью. `sha256` и `size_bytes` в SQL — реальные значения этих файлов.
- Сид запускается суперпользователем `lp` через `docker exec` и после DB-SEC ([DB-SEC-db-roles.md](DB-SEC-db-roles.md)): ему нужны права на все таблицы, это dev-инструмент.

## Шаги

### 1. Демо-идентификаторы — одна таблица на все задачи
Зафиксировать в этом плане и в `README.md` («Демо-аккаунты», без колонки `id`). BE-06 (фикстуры мок-реестра) и BE-13 (мок-список) используют **эти** значения и ссылаются на эту таблицу, а не копируют её.

| # | Компания | Организация `id` / пользователь `id` | Тип / страна | `registration_number` | `lei` | Email | Вариант A | Вариант B |
|---|---|---|---|---|---|---|---|---|
| — | admin | — / `aaaaaaaa-0000-0000-0000-000000000001` | admin | — | — | `admin@liquidity.local` | active | active |
| 1 | ООО «Альфа» | `11111111-1111-1111-1111-111111111111` / `…0002` | borrower / RU | `7700000009` | — | `alpha@liquidity.local` | `draft`, профиль заполнен, документов нет | `verified` |
| 2 | Beta Corp Ltd | `22222222-2222-2222-2222-222222222222` / `…0003` | lender / CY | `CY-123456` | `DEMO00BETACORP000162` | `beta@liquidity.local` | `under_review`, документы загружены | `verified` |
| 3 | Gamma GmbH | `55555555-5555-5555-5555-555555555555` / `…0004` | lender / DE | `HRB-555555` | `DEMO00GAMMAGMBH00148` | `gamma@liquidity.local` | `under_review` + открытый `high`-флаг | то же |
| 4 | АО «Вектор» | `66666666-6666-6666-6666-666666666666` / `…0005` | borrower / RU | `7700000016` | — | `vector@liquidity.local` | `verified`, владелец фоновых заявок | то же |
| 5 | Delta Trading | `77777777-7777-7777-7777-777777777777` / `…0006` | lender / KW | `KW-777777` | `DEMO00DELTATRADE0174` | `delta@liquidity.local` | `under_review` + открытый флаг `high_risk_jurisdiction/medium` | то же |
| 6 | Epsilon Capital Pte | `88888888-8888-8888-8888-888888888888` / `…0007` | lender / SG | `201988888E` | `DEMO00EPSILONCAP0155` | `epsilon@liquidity.local` | `verified` | `verified`; оффер к главной заявке |
| 7 | ООО «Омега» | `99999999-9999-9999-9999-999999999999` / `…0008` | borrower / RU | `7700000023` | — | `omega@liquidity.local` | `rejected` с причиной | то же |
| 8 | ООО «Сигма» | `eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee` / `…0009` | borrower / RU | `7700000030` | — | `sigma@liquidity.local` | `verified`, пользователь `blocked` | то же |

Пользователи — `aaaaaaaa-0000-0000-0000-00000000000N`, N из колонки. Пароль у всех — `Demo-Pass-2026`.

Состояние в мок-реестре BE-06: «Омега» — `liquidated` (даёт флаг `registry_mismatch`), остальные — `active`. Бенефициары: у каждой компании один-два вымышленных человека с гражданством вне списков FATF; их имена тоже проверяет тест шага 5.

**Готово:** таблица в `README.md` совпадает с этой; тест шага 2 пересчитывает контрольные суммы всех ИНН и LEI; BE-06 и BE-13 ссылаются на эту таблицу.

### 2. Хэш пароля и тест на него
1. `backend/cmd/hashpw/main.go`: читает пароль из stdin (не из аргумента — он виден в истории shell и `ps`), печатает `service.HashPassword`. Doc-комментарий: «dev-утилита для сида».
2. Сгенерировать один хэш `Demo-Pass-2026` и вставить его всем 9 пользователям в `seed_demo_common.sql`.
3. `backend/internal/service/seed_demo_test.go` (unit, без БД), `TestSeedDemoPasswords`:
   - читает `../../migrations/testdata/seed_demo_common.sql`;
   - регэкспом достаёт пары `email → password_hash`; их ровно 9, email — из таблицы шага 1;
   - `ValidatePassword("Demo-Pass-2026") == nil`, `VerifyPassword(ctx, "Demo-Pass-2026", hash) == true` для каждого хэша;
   - ни один хэш не содержит `PLACEHOLDER`.
4. В том же файле `TestSeedDemoIdentifiers`: регэкспом достаёт `registration_number` RU-компаний и все `lei` из сида и проверяет контрольные суммы (функции `innChecksumValid` и `leiChecksumValid` в тесте, если в коде BE-06 их ещё нет). Затем читает `../registry/mockdata/companies.json` (BE-06) и проверяет, что у каждой демо-компании есть фикстура по ключу `(country_code, ИНН)` для RU и `(country_code, LEI)` для остальных.

**Готово:** `go -C backend test -short -run 'TestSeedDemo' ./internal/service/` зелёный; `grep -r PLACEHOLDER backend/migrations` пусто.

### 3. Разбить сид на common / A / B
`backend/migrations/testdata/`:
- **`seed_demo_common.sql`:**
  - все 8 организаций и их бенефициары. Статусы: «Альфа» и Beta — `draft`; Gamma и Delta — `under_review`; «Вектор», Epsilon и «Сигма» — `verified`; «Омега» — `rejected` с `kyc_reject_reason` («Компания ликвидирована по данным реестра»);
  - пользователи (все 9, с одним хэшем); «Сигма» — `status = 'blocked'`, `blocked_reason` заполнен;
  - KYC-документы Gamma, «Вектора», Delta, Epsilon, «Омеги», «Сигмы»;
  - `registry_checks` (`source = 'mock'`, имя и номер — из таблицы шага 1): «Омега» — `found = true`, статус `liquidated`; остальные не-draft компании — `active`;
  - AML-флаги: Gamma `sanctions_match/high/open` (шаг 6); Delta `high_risk_jurisdiction/medium/open`, `details` — `Страна регистрации KW — серый список FATF (на 2026-06-19)`; «Омега» `registry_mismatch/high` (`status = 'confirmed'`, флаг уже разобран при отказе; `high` — как правило BE-13 для `liquidated`);
  - KYC-история (`status_history`, `entity_type = organization`) всех не-draft компаний, у «Омеги» — `under_review → rejected` с причиной в `comment`;
  - блокировка «Сигмы» — как её пишет BE-12: `status_history` (`user`, `active → blocked`, `comment` = причина) и `audit_log` `user.block` от admin;
  - фоновые заявки «Вектора» (шаг 4) с `status_history`.

  Common самодостаточен (FK `users.organization_id` выполняются внутри него), варианты — короткие `UPDATE` статусов плюс свои данные. Переход `draft → verified` в варианте B идёт одним `UPDATE` с `verified_at`: `organizations_verified_chk` требует их вместе.
- **`seed_demo_a.sql`:**
  - Beta → `under_review` (`kyc_submitted_at = now() - 1 day`) + документы `charter`, `registration_certificate`, `address_proof`, `beneficiaries_register`, `source_of_funds` + `status_history draft→under_review` + уведомление админу `kyc_submitted`;
  - «Альфа» остаётся `draft` без документов.
- **`seed_demo_b.sql`:**
  - «Альфа» и Beta → `verified` + документы + `status_history` (`draft→under_review→verified`) + `registry_checks` (`active`);
  - главная заявка, оффер Beta, история, уведомления (заголовки через `number`) и чат — перенести из нынешнего `seed_demo.sql`;
  - оффер Epsilon к главной заявке: `sent`, 9.5 %, без комиссии, `bullet` + уведомление «Альфе» `offer_received`. После принятия оффера Beta на демо он становится `rejected` с `another_offer_accepted` (BE-09) и уходит в архив откликов Epsilon;
  - история для аналитики (шаг 5).

Удалить `seed_demo.sql`. Все вставки идемпотентны (`ON CONFLICT DO NOTHING` / `WHERE NOT EXISTS`), обёрнуты в `BEGIN … COMMIT`. Даты — относительно `now()`.

**Готово:**
- `make db-reset` (B) и `make db-reset SEED=a` проходят без ошибок;
- повторный `make seed-demo` на той же БД — без ошибок и без новых строк (`SELECT count(*) FROM notifications` и `FROM applications` не меняются);
- `invariants_test.sql` проходит до сида (как сейчас в `make db`);
- `SELECT kyc_status, count(*) FROM organizations GROUP BY 1` совпадает с таблицей шага 1 для выбранного варианта.

### 4. Фоновые заявки для фильтров
В `seed_demo_common.sql` — 5 заявок «Вектора» `published` (`submitted_at`/`published_at` в прошлом, `status_history` `draft→moderation→published`):

| Сумма | Валюта | Страна | Срок, мес. | Способ |
|---|---|---|---|---|
| 250 000.00 | EUR | DE | 12 | sepa |
| 1 200 000.00 | USD | AE | 6 | swift |
| 3 000 000.00 | CNY | CN | 3 | cips |
| 150 000.00 | EUR | NL | 24 | sepa |
| 800 000.00 | USD | TR | 9 | spfs |

Способы `cips` и `spfs` появляются в справочнике в BE-02 (DECISIONS п. 24); если BE-02 не влит — `swift` и `other`, и поправить после. Ещё одна заявка «Вектора» `moderation` (для очереди модерации админа) и одна `draft`. В варианте B главная заявка «Альфы» (500 000 EUR → DE, 6 мес.) — шестая `published`. Фильтр «Германия» показывает две заявки, и нужная находится по сумме и сроку.

**Готово:** после `make db-reset` под Beta (после BE-07) `GET /api/v1/applications?target_country=DE` → 2 заявки в B, 1 в A; без фильтра — 6 / 5 (заявки истории шага 5 не `published`, витрину не засоряют).

### 5. История для аналитики (только вариант B)
В `seed_demo_b.sql` — блок `INSERT … SELECT … FROM generate_series(1, N) AS i` по решению «История для аналитики». Участники: заёмщики «Альфа», «Вектор», «Сигма»; кредиторы Beta и Epsilon (только `verified`-компании).
- ~40 заявок за последние ~90 дней (`created_at = now() - (90 - 2*i) * interval '1 day'`), валюты EUR/USD/CNY/RUB, страны назначения DE/AE/CN/TR/NL/KZ, статусы `agreed`, `cancelled`, `rejected` (не `published` — витрину шага 4 не трогаем);
- ~60 офферов к ним: у каждой `agreed`-заявки один `accepted` и до одного `rejected` с `another_offer_accepted`; у остальных — `rejected`/`withdrawn`;
- ~15 сделок — по одной на `agreed`-заявку; `fee_amount = round(amount * fee_percent / 100, 2)`, как считает BE-09.

Порядок вставки из-за `applications_agreed_chk`: заявки → офферы → `UPDATE applications SET status='agreed', accepted_offer_id=…` → сделки. `status_history` для истории не пишется: аналитика (BE-14) читает только `applications`, `offers`, `deals`.

**Готово:** после `make db-reset` под admin `GET /api/v1/admin/analytics/summary` → `applications_total` ≥ 40, в `amounts_by_currency` ≥ 3 валюты, `top_demand_countries` и `top_supply_countries` не пустые; повторный сид не меняет `count(*)` в `applications`, `offers`, `deals`.

### 6. AML-совпадение для Gamma
- В мок-список BE-13 (`backend/internal/aml/sanctionslist/mock.json`) — запись `external_id='demo-gama-001'`, `entity_kind='organization'`, `name='GAMA GmbH'`, `country_code='DE'`. Если BE-13 ещё не влит — передать запись его исполнителю, в сид её не вставлять.
- В `seed_demo_common.sql` — `aml_flags`: `organization_id = Gamma`, `rule='sanctions_match'`, `severity='high'`, `status='open'`, `details` — ровно строка, которую сформировал бы скрининг BE-13 для этой находки (формат — BE-13, «`details`»). Иначе дедупликация BE-13 (точное совпадение `details`) не узнает флаг, и повторный скрининг создаст дубль.
- Тест `TestSeedFlagDetails` (пакет `aml`, после BE-13): строки `details` флагов Gamma и Delta из `seed_demo_common.sql` равны выводу функций форматирования `aml` для тех же находок.

Почему `GAMA`, а не точная копия: на демо видно именно нечёткое совпадение.
- Jaro-Winkler(`gama`, `gamma`) ≈ 0.953 ≥ 0.90 → `high` (порог из issue #29).
- `similarity('gama', 'gamma')` по триграммам = 4/7 ≈ 0.57 ≥ 0.4 → кандидат проходит `SearchSanctions`.

Тест `TestSeedSanctionsMatch` (в пакете `aml`, после BE-13): в `mock.json` есть `demo-gama-001`, и скор `aml.Normalize("GAMA GmbH")` против `aml.Normalize("Gamma GmbH")` ≥ 0.90. Других записей мок-списка со скором ≥ 0.85 против имён остальных семи компаний таблицы шага 1 и всех бенефициаров сида быть не должно: флаг `high` на них заблокировал бы одобрение KYC в US-14.

**Готово:**
- тест зелёный;
- после `make db-reset` `GET /api/v1/admin/aml/flags?status=open` под admin содержит флаг Gamma (`high`) и флаг Delta (`medium`);
- `POST /api/v1/admin/organizations/55555555-…/kyc/approve` → 409 `CONFLICT` (BE-12).

### 7. Файлы документов
1. `backend/migrations/testdata/demo_files/`: 1–2 минимальных валидных PDF (несколько КБ) с текстом «DEMO» — magic bytes `%PDF` проходят проверку BE-05.
2. По `BE-05-kyc.md` хранилище — каталог `DOCUMENT_STORAGE_DIR` (Makefile: `$(CURDIR)/storage/documents`), ключ — 32 hex-символа без расширения, проверяется `filepath.IsLocal`. Каждой строке `kyc_documents` в сиде — свой фиксированный 32-hex `storage_key`, например `de0000000000000000000000000000a1`. `sha256` и `size_bytes` — реальные значения файла (`sha256sum`).
3. Make-цель `seed-files` копирует каждый файл из `demo_files/` в `$(DOCUMENT_STORAGE_DIR)/<storage_key>` и вызывается из `seed-demo`. Соответствие «ключ → файл» — `demo_files/manifest.txt` (строка `<key> <file>`). Makefile обязан работать и в `cmd.exe` (комментарий у `gen-api` в `Makefile`), поэтому вместо `cp`/`mkdir -p` — `go -C backend run ./cmd/seedfiles -manifest … -dst $(DOCUMENT_STORAGE_DIR)`. Утилита создаёт каталог `0700`, файлы — `0600`, как BE-05.

**Готово:** под admin `GET /api/v1/documents/{id}/file` для документа Beta → 200, `Content-Type: application/pdf`, sha256 тела совпадает с `kyc_documents.sha256`.

### 8. Makefile, CI, README
- `Makefile`:
  ```make
  SEED ?= b
  seed-demo: seed-files
  	docker cp $(TESTDATA)/seed_demo_common.sql $(PG_CONTAINER):/tmp/seed_demo_common.sql
  	docker cp $(TESTDATA)/seed_demo_$(SEED).sql $(PG_CONTAINER):/tmp/seed_demo_variant.sql
  	$(PSQL) -f /tmp/seed_demo_common.sql -f /tmp/seed_demo_variant.sql
  seed: seed-demo
  ```
  Добавить `seed` в `.PHONY`. Неизвестный `SEED` (нет файла) → `docker cp` падает с понятной ошибкой — этого достаточно. Порядок `db:` — `up db-roles migrate-up load-sanctions test-invariants seed-demo` (DB-SEC, BE-13).
- `.github/workflows/backend.yml`, job `test`, после `migrate` и `load-sanctions`: шаг `seed` под суперпользователем (как шаг `db roles` DB-SEC: `DATABASE_URL` в CI — это `lp_app` без прав на запись в часть таблиц) — `psql "postgres://lp:lp_dev_password@localhost:5432/liquidity_platform" -v ON_ERROR_STOP=1 -f backend/migrations/testdata/seed_demo_common.sql -f backend/migrations/testdata/seed_demo_b.sql`. Это ловит битый SQL сида, а интеграционные тесты заодно проверяются на заселённой БД, как локально после `make db`. Без копирования файлов: тесты к ним не обращаются.
- `README.md`:
  - раздел «Демо-аккаунты» — таблица шага 1 без колонки `id`, общий пароль `Demo-Pass-2026`, пометка «только для локального демо»;
  - «Запуск бэкенда» — `make db-reset` для показа сделки (по умолчанию), `make db-reset SEED=a` — для показа KYC;
  - короткий чек-лист US-14 по варианту; шаг 1 — «зарегистрировать новую компанию (не "Альфу": она уже в сиде)».

**Готово:** `make lint-actions` зелёный; CI на PR проходит шаг `seed`; `make seed` работает.

### 9. Прогон US-14 на обоих вариантах (после BE-05…BE-14)
Ручной прогон по шагам README с `curl` (или через фронт), результаты — в описании PR.
- **B:**
  1. login `alpha@` → `GET /applications/3333…/offers` показывает офферы Beta и Epsilon;
  2. сообщение в чат по офферу Beta;
  3. `POST /offers/4444…/accept` → 200, сделка `amount=500000.00`, `fee_amount=5000.00`, заявка `agreed`; оффер Epsilon → `rejected`, `another_offer_accepted`;
  4. admin → `GET /admin/analytics/summary` показывает историю; `GET /admin/audit-log?action=user.block` — блокировку «Сигмы»; login `sigma@` → `403` «Аккаунт заблокирован» (BE-03); admin unblock «Сигмы» → login 200.
- **A:**
  1. регистрация новой компании → 201;
  2. login `alpha@` → загрузка документов, `submitKyc`;
  3. admin approve «Альфа» и Beta → 200; approve Gamma → 409; у Delta виден флаг FATF;
  4. «Альфа» создаёт заявку → admin публикует → Beta фильтрует по DE → оффер → accept.

**Готово:** оба прогона проходят на `make db-reset` без ручных правок БД — критерий US-14 «`docker compose up` + сид → сценарий проходится по шагам из README».

## Тесты
| Случай | Ожидание |
|---|---|
| `TestSeedDemoPasswords` | 9 пользователей, у всех хэш `Demo-Pass-2026` проходит `VerifyPassword`, пароль проходит `ValidatePassword` |
| `TestSeedDemoIdentifiers` | все ИНН сида проходят контрольную сумму ИНН-10, все LEI — `mod 97 = 1`; у каждой компании есть фикстура мок-реестра BE-06 |
| `POST /auth/login` `{"email":"beta@liquidity.local","password":"Demo-Pass-2026"}` | 200, cookie `session` и `csrf_token` |
| то же с неверным паролем | 401 `UNAUTHORIZED` (не 500) |
| `make seed-demo` дважды подряд | второй запуск без ошибок, число строк в `applications`, `offers`, `deals`, `notifications`, `messages` не изменилось |
| `make db-reset SEED=a` → `GET /organizations/me` под `alpha@` | `kyc_status = draft` |
| `make db-reset` → то же | `kyc_status = verified` |
| `GET /notifications` под `alpha@` (B) | `title` содержит номер реальной главной заявки (`APP-` + `applications.number`) |
| `TestSeedSanctionsMatch` | `demo-gama-001` в `mock.json`, скор с «Gamma GmbH» ≥ 0.90; остальные демо-имена и бенефициары < 0.85 |
| `GET /admin/analytics/summary` (B) | ≥ 40 заявок, ≥ 3 валюты |
| скачивание документа из сида | 200, sha256 тела = `kyc_documents.sha256` |
| CI job `test` | шаг `seed` зелёный, интеграционные тесты зелёные на заселённой БД |

## Готово, когда
- [ ] В сиде admin и 8 компаний из таблицы шага 1; под каждым активным пользователем входят паролем `Demo-Pass-2026` (`TestSeedDemoPasswords` + ручной login → 200), «Сигма» заблокирована.
- [ ] ИНН и LEI проходят контрольные суммы (`TestSeedDemoIdentifiers`) и совпадают с фикстурами мок-реестра BE-06.
- [ ] `make db-reset SEED=a` — «Альфа» и Beta не верифицированы (KYC показывается на демо); `make db-reset` (B) — сделку можно принять сразу, комиссия 5 000.00 EUR, конкурирующий оффер Epsilon автоотклоняется.
- [ ] Минимум 5 опубликованных заявок разных стран, валют и сроков; фильтр по стране, валюте и сроку даёт разные результаты.
- [ ] Запись в мок-санкционном списке похожа на «Gamma GmbH» и даёт открытый `high`-флаг; approve KYC Gamma → 409. У Delta открыт флаг FATF `medium`.
- [ ] В варианте B аналитика показывает историю за ~3 месяца.
- [ ] `make seed` работает на чистой БД; сид применяется в CI.
- [ ] И общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: запуск скрининга сидом закрыт в [DECISIONS.md](DECISIONS.md), п. 45; формат хранилища — ⚠️ в «Что уже есть».
