# BE-13 · AML: санкционный скрининг и правила

Issue: #29 · Milestone: M3 · Зависит от: BE-05 (бенефициары, место вызова в `submitKyc`); DB-SEC — желательно (роль загрузчика), не блокирует · Блокирует: BE-05 (настоящий скрининг вместо `NoopScreener`), BE-07 (`submitApplication`), BE-12 (флаги в карточке, запрет approve при открытом `high`), BE-16 (мок-санкционный список)

## Цель
При `submitKyc` компания и её бенефициары проверяются по санкционным спискам (pg_trgm → Jaro-Winkler) и по FATF-юрисдикциям. Сверяется и ответ реестра. При `submitApplication` проверяются страна назначения и сумма. Каждое срабатывание — строка в `aml_flags` с понятным `details`. Админ видит флаги в `GET /admin/aml/flags` и закрывает их через `POST /admin/aml/flags/{id}/resolve`. `make load-sanctions` загружает мок-список и UK Sanctions List.

## Что уже есть
- `backend/internal/aml/doc.go` — только doc-комментарий пакета.
- Контракт, `api/paths/admin.yaml`:
  - `adminListAmlFlags` — фильтры `status`, `severity`, `organization_id`, `application_id`; пагинация;
  - `adminResolveAmlFlag` — тело `{resolution: false_positive|confirmed, comment: 3..2000}`; ответы 404 и 409.
  - Go-имена: `AdminListAMLFlags`, `AdminResolveAMLFlag`, `api.AMLFlag`, `api.AdminListAMLFlagsParams`, `api.AdminResolveAMLFlagJSONBodyResolution`. Заглушки 501 — `handler/admin.go`.
  - ⚠️ Параметр `application_id` добавлен коммитом `0acece7` в ветке `feat/api-list-filters`; в `origin/main` его пока нет.
- Схема ответа `AmlFlag` (`api/schemas/aml.yaml`) требует `organization: OrganizationShort{id, name, country_code}`, то есть JOIN с `organizations`.
- SQL в `backend/queries/admin.sql`:
  - `CreateAmlFlag` — есть;
  - `ListAmlFlags` — без JOIN, без `application_id`, без `Count`;
  - `ResolveAmlFlag` — `WHERE id = $1 AND status = 'open'`;
  - `SearchSanctions` — `SELECT *`, `WHERE name_normalized % $1 ORDER BY similarity(...) DESC LIMIT $2`, score не возвращает.
- Таблицы — `docs/db/tables/aml_flags.md`, `docs/db/tables/sanctions_entries.md`; раздел «Санкционный скрининг» — `docs/db/schema.md`.
  - `sanctions_entries`: GIN `gin_trgm_ops` по `name_normalized`; `UNIQUE (source, external_id, name_normalized)` — алиасы одной записи.
  - `aml_flags_resolution_chk`: `open` ⇔ `resolved_at IS NULL`.
- `countries.fatf_list` (`black`/`grey`/NULL) и `fatf_as_of` — в `00001_init.sql`. Сид `00002_seed_dictionaries.sql` содержит **только чёрный список** (IR, KP, MM, на 2026-06-19); **серого нет**. Серый список (22 юрисдикции, июнь 2026) добавляет BE-02 миграцией `0000N_dictionaries_full.sql` (DECISIONS п. 22); до её мержа правило `grey → medium` проверяется только юнит-тестом.
- Enum: `aml_rule`, `aml_severity`, `aml_flag_status`; `notification_type` содержит `aml_flag_created`; `entity_type` — `aml_flag`.
- `golang.org/x/text` уже в `backend/go.mod` — для снятия диакритики.
- `docs/backend_tasks/DB-SEC-db-roles.md`:
  - у `lp_app` на `sanctions_entries` только `SELECT`;
  - загрузчик работает под `lp_migrator` через `MIGRATE_DATABASE_URL`;
  - `ALTER ROLE lp_app SET pg_trgm.similarity_threshold = 0.4`.

⚠️ Ловушки:
- **Порог pg_trgm.** Оператор `%` берёт порог из GUC `pg_trgm.similarity_threshold`.
  - До DB-SEC порог по умолчанию — 0.3, а `SET` на соединении из пула не доходит до нужного запроса и утекает в чужие (`docs/db/tables/sanctions_entries.md`).
  - После DB-SEC порог 0.4 задан на роли.
  - Запрос должен давать одинаковый результат в обоих случаях — см. «Решения».
- **Интеграционные тесты под `lp_app` не могут вставить строки в `sanctions_entries`** (после DB-SEC только `SELECT`). Тесты опираются на мок-список, загруженный `make load-sanctions`; значит, загрузка должна быть в `make db` и в CI.
- **`ResolveAmlFlag` не отличает «нет флага» от «уже закрыт».** Оба случая дают `pgx.ErrNoRows`, а контракт требует 404 и 409 соответственно.
- **`ListAmlFlags` фильтрует через `narg` («нет значения → всё»).** Для админского списка это верно; правило BE-04 об обязательных параметрах касается списков по ролям.
- **Порог суммы нигде не задан**: ни в контракте, ни в ADR, ни в схеме. Значения решены — см. «Решения», правило «Сумма».
- **Имена в `sanctions_entries.name_normalized` и в запросе должны нормализоваться одной функцией.** Поменял нормализацию — перезагрузи списки (`make load-sanctions`), иначе совпадения пропадут молча.
- **`docs/db/tables/aml_flags.md` указывает путь `/admin/aml-flags`**, а в контракте — `/admin/aml/flags`.

## Решения
- **`aml.Screener` только находит, а не пишет.** Он читает санкции и FATF и возвращает `[]Finding`.
  - Флаги сохраняет вызывающий сервис хелпером `saveAmlFindings` внутри своей транзакции, вместе со сменой статуса. Так флаги атомарны с переходом (ADR-0001: уведомления и события — в той же транзакции).
  - Сам скрининг тестируется без БД.
  - Уведомление `aml_flag_created` вызывающий отправляет одно на весь скрининг (прогон), а не на флаг, если создан хотя бы один флаг (DECISIONS п. 14).
- **Канонический API пакета** (шаг 1). BE-05 и BE-07 используют его как есть; если BE-05 пришёл раньше, он копирует шаг 1 дословно.
- **Кандидаты — `%` плюс явный порог в запросе.**
  ```sql
  WHERE name_normalized % @query AND similarity(name_normalized, @query) >= @min_similarity
  ```
  - `%` оставлен ради GIN-индекса; второе условие делает результат независимым от GUC, пока `GUC ≤ min_similarity`.
  - До DB-SEC: GUC 0.3, `%` отдаёт ≥ 0.3, условие режет до 0.4.
  - После DB-SEC: GUC 0.4, условие ничего не меняет.
  - Если `ALTER ROLE … SET` по какой-то причине не применился — это первый случай, результат тот же.
  - `SET`/`SET LOCAL` в коде не используем.
  - `const candidateMinSimilarity = 0.4` — равен значению на роли; инвариант «GUC роли ≤ `candidateMinSimilarity`» записать в комментарии запроса и в `docs/db/schema.md`.
- **Финальный скоринг — свой Jaro-Winkler** в `internal/aml` (~40 строк, p = 0.1, общий префикс до 4). Почему: зависимость ради одной функции не нужна, а табличные тесты по эталонным значениям фиксируют реализацию.
  - Score = `max(JW(a, b), JW(sortTokens(a), sortTokens(b)))`: JW чувствителен к порядку слов, а в списках встречаются и «Petrov Ivan», и «Ivan Petrov».
- **Пороги** (issue #29, `docs/db/schema.md`): `score ≥ 0.90` → `high`; `0.85 ≤ score < 0.90` → `medium`; ниже — флага нет. Именованные константы `sanctionsHighScore`, `sanctionsMediumScore`.
- **Один флаг на субъект** (компанию или одного бенефициара) — по лучшему кандидату, а не по каждому совпадению. Это защита от лавины флагов на распространённых именах.
  - Название компании сравнивается только с записями `entity_kind ∈ {organization, other}`.
  - Бенефициар — только с `{person, other}`.
  - Нормализованный запрос короче 3 символов не ищется: у pg_trgm нет триграмм, а JW по такой строке — шум.
- **Нормализация `aml.Normalize`** — одна функция для загрузчика и скрининга, по шагам:
  1. NFKD + удаление диакритики (`golang.org/x/text`: `runes.Remove(runes.In(unicode.Mn))`) → нижний регистр.
  2. Транслитерация кириллицы по таблице:

     | Буквы | Латиница |
     |---|---|
     | а б в г д е ё ж з и й к л м н о п р с т у ф | a b v g d e e zh z i i k l m n o p r s t u f |
     | х ц ч ш щ ъ ы ь э ю я | kh ts ch sh shch ie y — e iu ia |

     Это ICAO (загранпаспорт), `ь` выбрасывается.
  3. Всё, что не буква и не цифра, заменяется пробелом.
  4. Удаление токенов организационно-правовых форм: `ooo oao zao pao ao nao ip llc ltd limited gmbh ag inc incorporated corp corporation co llp plc jsc pjsc ojsc cjsc sa sas sarl bv nv srl spa`.
  5. Схлопывание пробелов.

  Формы удаляются после транслитерации, поэтому список один для кириллицы и латиницы.
- **Две схемы транслитерации: ICAO и BGN/PCGN, берётся максимум score** (DECISIONS п. 41). ICAO даёт `iurii`, а в санкционных списках пишут `Yuri`: JW(`iurii`, `yuri`) ≈ 0.78 — ниже порога, совпадение пропало бы.
  - `Normalize` (ICAO) остаётся единственной функцией для загрузчика: `name_normalized` в БД — по ICAO, списки в основном латинские, на них схема не влияет.
  - Для скрининга `NormalizeVariants(name) []string` — `[ICAO]` для латиницы и `[ICAO, BGN/PCGN]` для кириллицы, без дублей. Каждый вариант ищется через `SearchSanctions`; по кандидату берётся лучший `nameScore` по всем вариантам.
  - BGN/PCGN отличается от ICAO буквами: `й → y`, `ю → yu`, `я → ya`, `ы → y`, `ъ → ""`, `е`/`ё` в начале слова и после гласной → `ye`. Пример: `Юрий` → `yuriy`; JW(`yuriy`, `yuri`) ≈ 0.96 → `high`.
- **Правила** (`internal/aml/rules.go`):

  | Правило | Субъект | Условие | `rule` | `severity` |
  |---|---|---|---|---|
  | Санкции | компания; каждый бенефициар | score ≥ 0.90 | `sanctions_match` | `high` |
  | | | 0.85 ≤ score < 0.90 | `sanctions_match` | `medium` |
  | FATF | страна компании; гражданство каждого бенефициара; `target_country_code` заявки | `fatf_list = black` | `high_risk_jurisdiction` | `high` |
  | | | `fatf_list = grey` | `high_risk_jurisdiction` | `medium` |
  | Сумма | заявка | `amount > amountThresholds[currency]` | `amount_threshold` | `medium` |
  | Реестр | компания, `Registry.Outcome = checked` | `!Found` | `registry_mismatch` | `medium` |
  | | | `Status = liquidated` | `registry_mismatch` | `high` |
  | | | `Status ∉ {active, ""}` (кроме `liquidated`) | `registry_mismatch` | `medium` |
  | | | `max JW(Normalize(Name), Normalize(registry name)) < 0.90` | `registry_mismatch` | `medium` |
  | Реестр | `Outcome = unavailable` или `no_identifier` | — | `other` | `low` |
  | Реестр | `Registry == nil` (сверка не подключена) | — | — | — |

  - Пороги — константы `amountThresholds` в `rules.go` (DECISIONS п. 20): `RUB 100 000 000`, `USD 1 000 000`, `EUR 1 000 000`, `CNY 7 000 000` — эквивалент ~100 млн ₽. Env-настройку не делаем: на демо порог не меняют.
  - Валюта без порога → правило суммы не применяется.
  - Демо-сделка «Альфа ↔ Beta» (500 000 EUR) ниже порога — флага нет.
- **`countries.is_unfriendly` (430-р) в правилах не участвует** (DECISIONS п. 19): поле справочное, issue его не требует.
- **Правило FATF на демо показывает Delta Trading (`KW`, серый список)** — открытый флаг `high_risk_jurisdiction/medium` из сида BE-16 (DECISIONS п. 33). Повторный скрининг Delta даёт тот же флаг.
- **`details` — по-русски, с источником и score.** Строка детерминирована, чтобы работала дедупликация. Примеры:
  - `Совпадение бенефициара «Иванов Иван» со списком mock (запись M-002 «Ivanov Ivan»), score 0.92`;
  - `Страна регистрации IR — чёрный список FATF (на 2026-06-19)`;
  - `Название по реестру «ООО «Омега»», статус liquidated`.

  Score выводится с 2 знаками (`%.2f`).
- **Дедупликация.** Находка пропускается, если у той же компании и заявки (`application_id IS NOT DISTINCT FROM`) уже есть флаг с тем же `rule` и `details` в статусе `open` или `false_positive`. Повторный `submitKyc` после `rejected` не плодит копии, а закрытое как ложное срабатывание не всплывает снова. `confirmed` создаётся заново.
- **Ошибка скрининга = ошибка операции.** `Screener` возвращает `error`, вызывающий откатывает переход: fail-closed. Скрининг читает только локальную БД.
- **Загрузчик** — отдельная команда `backend/cmd/loadsanctions`, не миграция и не код сервера.
  - Подключается через `MIGRATE_DATABASE_URL`, а без него — через `DATABASE_URL`, если DB-SEC ещё не сделан (db-security-plan: «загрузчик — под `lp_migrator`, не под `lp_app`»).
  - Источник заменяется атомарно: в одной транзакции `DELETE … WHERE source = $1` и `COPY` новых строк (`:copyfrom`). Upsert со «сборкой мусора» по `loaded_at` сложнее, а частичная загрузка невозможна.
- **Официальный список — UK Sanctions List (FCDO) в CSV.** Скачивается без токена, ссылка «CSV» на https://www.gov.uk/government/publications/the-uk-sanctions-list. EU FSF XML требует токен в URL.
  - Файл не коммитим: размер, лицензия, актуальность. В репо — 5–10 строк в `testdata` для теста парсера.
  - `source = 'uk_fcdo'` — значение из `docs/db/tables/sanctions_entries.md`.

## Шаги

### 1. API пакета `aml` и заглушка
`backend/internal/aml/screener.go`:
```go
// Person — бенефициар для скрининга.
type Person struct {
	FullName               string
	CitizenshipCountryCode string
}

// RegistryOutcome — чем закончилась сверка с реестром (BE-06).
type RegistryOutcome string

const (
	RegistryChecked      RegistryOutcome = "checked"       // реестр ответил; Found может быть false
	RegistryUnavailable  RegistryOutcome = "unavailable"   // реестр не ответил — нужна ручная сверка
	RegistryNoIdentifier RegistryOutcome = "no_identifier" // не-RU без LEI — искать не по чему
)

type RegistryResult struct {
	Outcome RegistryOutcome
	Found   bool
	Names   []string // краткое и полное название по реестру
	Status  string   // нормализованный статус (см. план BE-06)
}

type OrganizationSubject struct {
	Name          string
	CountryCode   string
	Beneficiaries []Person
	Registry      *RegistryResult // nil — сверка не подключена, правило реестра не применяется
}

type ApplicationSubject struct {
	TargetCountryCode string
	Amount            decimal.Decimal
	CurrencyCode      string
}

// Finding — одно срабатывание правила; в aml_flags его пишет вызывающий сервис.
type Finding struct {
	Rule     repository.AmlRule
	Severity repository.AmlSeverity
	Details  string
}

type Screener interface {
	ScreenOrganization(ctx context.Context, s OrganizationSubject) ([]Finding, error)
	ScreenApplication(ctx context.Context, s ApplicationSubject) ([]Finding, error)
}

// NoopScreener — заглушка, пока правила не подключены: находок нет.
type NoopScreener struct{}
```
Обновить doc-комментарий в `doc.go`.

**Готово:** `go -C backend build ./...` зелёный; `var _ Screener = NoopScreener{}` компилируется.

### 2. Нормализация
`internal/aml/normalize.go`: `Normalize(name string) string` (ICAO), `NormalizeVariants(name string) []string` (ICAO + BGN/PCGN для кириллицы) и `sortTokens(s string) string` по правилам из «Решений».

**Готово:** табличные `TestNormalize` и `TestNormalizeVariants` зелёные (случаи — в «Тестах»).

### 3. Jaro-Winkler
`internal/aml/jarowinkler.go`: `jaroWinkler(a, b string) float64` по рунам, `p = 0.1`, префикс ≤ 4; две пустые строки → 1, одна пустая → 0. Плюс `nameScore(a, b string) float64 = max(jaroWinkler(a, b), jaroWinkler(sortTokens(a), sortTokens(b)))`.

**Готово:** `TestJaroWinkler` с эталонами (точность 0.001):

| Пара | JW |
|---|---|
| `martha` / `marhta` | 0.961 |
| `dwayne` / `duane` | 0.840 |
| `dixon` / `dicksonx` | 0.813 |
| `abc` / `abc` | 1 |
| `abc` / `xyz` | 0 |

`TestNameScore`: `petrov ivan` / `ivan petrov` → 1.

### 4. SQL
`backend/queries/admin.sql`:

**`SearchSanctions`** — переписать:
```sql
SELECT id, source, external_id, entity_kind, name, name_normalized,
       similarity(name_normalized, @query::text)::real AS trgm_score
FROM sanctions_entries
WHERE name_normalized % @query::text
  AND similarity(name_normalized, @query::text) >= @min_similarity::real
ORDER BY trgm_score DESC, id
LIMIT @max_candidates;
```
Комментарий над запросом — инвариант про GUC. `const maxSanctionCandidates = 20`.

**`ListCountriesFatf :many`**:
```sql
SELECT code, fatf_list, fatf_as_of FROM countries
WHERE code = ANY(@codes::text[]) AND fatf_list IS NOT NULL
```

**`ListAmlFlags`** — переписать:
- `SELECT sqlc.embed(f), o.name AS organization_name, o.country_code AS organization_country_code FROM aml_flags f JOIN organizations o ON o.id = f.organization_id`;
- добавить фильтр `sqlc.narg('application_id')::uuid`;
- `ORDER BY f.created_at DESC, f.id DESC LIMIT … OFFSET …`.

**Новые запросы для флагов:**
- `CountAmlFlags :one` — те же фильтры;
- `GetAmlFlagByID :one` — тот же JOIN, `WHERE f.id = $1`;
- `HasSimilarAmlFlag :one` — `SELECT EXISTS (…)` по правилу дедупликации из «Решений».

Новый `backend/queries/sanctions.sql`:
- `DeleteSanctionsBySource :execrows`;
- `InsertSanctionsEntries :copyfrom` — колонки `source, external_id, entity_kind, name, name_normalized, country_code, raw`.

Затем `make gen-sqlc`. Вызывающих у изменённых запросов нет — `grep -rn "SearchSanctions\|ListAmlFlags" backend/internal --include=*.go` находит только `repository/sqlc`.

**Готово:** `make gen-sqlc` без ошибок, `go -C backend build ./...` зелёный. Интеграционный `TestSearchSanctionsThresholdIndependent` (нужен загруженный мок, шаг 7): в двух транзакциях с `SET LOCAL pg_trgm.similarity_threshold = 0.3` и `= 0.4` `SearchSanctions` по одному запросу возвращает одинаковые id.

### 5. `RuleScreener`
`internal/aml/rules.go`:
- зависимость `Store` (интерфейс рядом): `SearchSanctions(ctx, repository.SearchSanctionsParams) ([]repository.SearchSanctionsRow, error)` и `ListCountriesFatf(ctx, []string) ([]repository.ListCountriesFatfRow, error)`. `*repository.Queries` ему удовлетворяет;
- `NewRuleScreener(store Store) *RuleScreener`.

`ScreenOrganization`:
1. FATF: коды компании и бенефициаров одним запросом `ListCountriesFatf`.
2. Санкции: компания, затем бенефициары по порядку. `NormalizeVariants` → `SearchSanctions` по каждому варианту → фильтр по `entity_kind` → лучший `nameScore` по всем вариантам и кандидатам → пороги. Один флаг на субъект, как и раньше.
3. Реестр — по таблице правил.

`ScreenApplication`: FATF по `TargetCountryCode` + сумма.

Константы: `sanctionsHighScore`, `sanctionsMediumScore`, `candidateMinSimilarity`, `maxSanctionCandidates`, `minQueryLen = 3`, `registryNameMinScore = 0.90`, `amountThresholds map[string]decimal.Decimal`. Порядок находок детерминирован: FATF → санкции → реестр.

Над каждым правилом — комментарий: почему такой порог и какой источник (AGENTS.md: «AML-правила комментируй»).

**Готово:** `TestScreenOrganization` и `TestScreenApplication` (фейковый `Store`) зелёные по таблице из «Тестов».

### 6. Сохранение флагов и админские операции
`backend/internal/service/aml.go`.

**`saveAmlFindings`**:
```go
saveAmlFindings(ctx context.Context, q repository.Querier, orgID uuid.UUID, appID *uuid.UUID, findings []aml.Finding) ([]repository.AmlFlag, error)
```
По каждой находке: `HasSimilarAmlFlag` → если её ещё нет, `CreateAmlFlag`. Возвращает только созданные флаги; уведомление шлёт вызывающий.

**`AmlFlags.List(ctx, filter, page, pageSize)`** → `ListAmlFlags` + `CountAmlFlags`.

**`AmlFlags.Resolve(ctx, p Principal, id int64, resolution repository.AmlFlagStatus, comment string)`**:
1. `ResolveAmlFlag`.
2. `ErrNoRows` → `GetAmlFlagByID`: нет строки → 404 `NOT_FOUND`; есть → 409 `apperr.InvalidTransition(status, resolution)`.
3. Успех → `GetAmlFlagByID` для ответа с `organization`.
4. `audit_log` `aml_flag.resolve` (`entity_type = aml_flag`) хелпером BE-14; если его ещё нет — минимальная версия по README.

Хендлеры в `handler/admin.go`:
- `AdminListAMLFlags`: page по умолчанию 1, `page_size` 20 (README, «Пагинация»);
- `AdminResolveAMLFlag`;
- маппинг строки → `api.AMLFlag` с `Organization: api.OrganizationShort{…}`;
- `x-roles: [admin]` у обеих операций проверяет BE-04.

**Готово:**
- `TestAmlFlagsResolve` (фейки): open → 200 со статусом `false_positive`, `resolved_at` не null; уже `confirmed` → 409; нет id → 404;
- `TestSaveAmlFindings`: при существующем `open`-дубле `CreateAmlFlag` не вызывается;
- `TestAdminListAMLFlagsHandler`: `?severity=high&status=open` доходит до сервиса; ответ содержит `organization.name`.

### 7. Загрузчик `make load-sanctions`
`backend/internal/aml/sanctionslist/` — новый пакет:
- `type Entry struct{ ExternalID, EntityKind, Name string; CountryCode *string; Raw json.RawMessage }`.
- `mock.json` (`//go:embed`) и `ParseMock(r io.Reader) ([]Entry, error)`:
  - около 10 вымышленных записей: организации и персоны, кириллица и латиница, алиасы с одинаковым `external_id`;
  - демо-запись `demo-gama-001` «GAMA GmbH» (organization, DE) — совпадение для Gamma из сида BE-16 (шаг 6 его плана). Остальные записи — со скором < 0.85 против всех 8 демо-компаний ([BE-16, шаг 1](BE-16-demo-seed.md#1-демо-идентификаторы--одна-таблица-на-все-задачи)) и их бенефициаров; это проверяет `TestSeedSanctionsMatch` из BE-16.
- `ParseUK(r io.Reader) ([]Entry, error)`:
  - строку заголовка найти по наличию колонок `Unique ID` и `Name 6` (над ней в файле служебная строка с датой);
  - колонки брать по имени, а не по номеру;
  - нет нужных колонок → ошибка со списком недостающих;
  - `ExternalID` = `Unique ID`;
  - `Name` = непустые `Name 1`…`Name 5` + `Name 6` через пробел;
  - `EntityKind` по колонке типа: `Individual → person`, `Entity → organization`, остальное → `other`;
  - `CountryCode` = nil — в файле названия стран, а не ISO-коды.
  - Названия колонок выше — по памяти: сверить с реальным файлом и зафиксировать в `testdata/uk_sample.csv` (5–10 строк, включая алиас).

`backend/cmd/loadsanctions/main.go`:
- флаги `-source mock|uk_fcdo` и `-file` (обязателен для `uk_fcdo`);
- URL БД: `MIGRATE_DATABASE_URL`, иначе `DATABASE_URL`;
- `name_normalized = aml.Normalize(name)`, пустые пропускаются;
- дедупликация по `(external_id, name_normalized)`;
- одна транзакция: `DeleteSanctionsBySource` + `InsertSanctionsEntries`;
- вывод: `source=mock deleted=N inserted=M`.

`Makefile`:
```make
SANCTIONS_SOURCE ?= mock
SANCTIONS_FILE   ?=
load-sanctions:
	go -C backend run ./cmd/loadsanctions -source=$(SANCTIONS_SOURCE) $(if $(SANCTIONS_FILE),-file=$(abspath $(SANCTIONS_FILE)))
```
- `abspath` обязателен: `go -C backend` меняет рабочий каталог.
- Добавить цель в `.PHONY` и в `db:` после `migrate-up`. Если DB-SEC уже поменял `db:`, то порядок такой: `up db-roles migrate-up load-sanctions test-invariants seed-demo`.

`.github/workflows/backend.yml`, job `test`: шаг `make load-sanctions` после `migrate`. `README.md`, «Запуск бэкенда»: как загрузить UK-список.

**Готово:**
- `TestParseMock` зелёный; `TestParseUK` на `testdata/uk_sample.csv` зелёный, а на файле без колонки `Name 6` — ошибка с её именем;
- `make db-reset` выводит `source=mock … inserted=N`, N > 0;
- повторный `make load-sanctions` даёт то же N (атомарная замена, без дублей);
- `make load-sanctions SANCTIONS_SOURCE=uk_fcdo SANCTIONS_FILE=<скачанный CSV>` загружает больше 1000 строк;
- после DB-SEC загрузка под `DATABASE_URL` роли `lp_app` падает с `42501` — проверить руками.

### 8. Подключение
`cmd/server/main.go`: `screener := aml.NewRuleScreener(queries)` передаётся в `service.NewKyc` (BE-05) и в сервис заявок (BE-07) вместо `aml.NoopScreener{}`, плюс `service.AmlFlags` в `handler.Deps`. BE-07 вызывает `ScreenApplication` в `submitApplication` и `saveAmlFindings(…, &applicationID, …)`. Если BE-07 ещё нет — оставить в его плане ссылку на этот шаг.

**Готово:** интеграционный `TestSubmitKycAmlIntegration`:
- бенефициар с именем записи мок-списка и гражданством `IR` → после `submitKyc` у компании два флага: `sanctions_match/high` и `high_risk_jurisdiction/high`, плюс одно уведомление `aml_flag_created` на каждого админа;
- повторный submit после `rejected` новых флагов не создаёт.

### 9. Документация
- `docs/db/schema.md`, «Санкционный скрининг»: убрать `SET pg_trgm.similarity_threshold`, показать запрос с `@min_similarity` и инвариант; дописать правило «один флаг на субъект» и `sortTokens`.
- `docs/db/tables/sanctions_entries.md`: новые запросы и загрузчик; заменить замечание про `SET` на «результат не зависит от GUC при GUC ≤ 0.4».
- `docs/db/tables/aml_flags.md`: путь `/admin/aml/flags`; закрыть замечание про `Count`/`GetByID`.
- `backend/README.md`: строки admin AML в «Статус реализации»; таблица правил AML — коротко, со ссылкой на `internal/aml/rules.go`.

**Готово:** `grep -rn "SET pg_trgm" docs/db/` пусто; `make verify` зелёный.

## Тесты
- `TestNormalize`:

  | Вход | Выход |
  |---|---|
  | `ООО «Альфа»` | `alfa` |
  | `Альфа, ООО` | `alfa` |
  | `Beta Corp Ltd.` | `beta` |
  | `Müller GmbH` | `muller` |
  | `Щукин Юрий` | `shchukin iurii` |
  | `  Gamma   Holding  ` | `gamma holding` |
  | `АО` | `""` |
  | одинаковый результат для `Иванов Иван` и `IVANOV IVAN` | `ivanov ivan` |

- `TestNormalizeVariants`: `Юрий Щукин` → `[iurii shchukin, yuriy shchukin]`; `Yuri Shchukin` → `[yuri shchukin]`; `Иванов Иван` → `[ivanov ivan]` (варианты совпали — без дублей); `Елена` → `[elena, yelena]`.
- В `TestScreenOrganization`: бенефициар `Юрий Петров` против записи мок-списка `Yuri Petrov` → `sanctions_match/high` (через вариант BGN/PCGN).

- `TestJaroWinkler`, `TestNameScore` — шаг 3.
- `TestScreenOrganization` (табличный, фейковый `Store` возвращает кандидатов с заданными `name_normalized`):

  | Субъект и кандидат | Находка |
  |---|---|
  | бенефициар `Ivanov Ivan`, кандидат `ivanov ivan`, person | `sanctions_match/high`, в `details` есть `score 1.00` и `mock` |
  | score ровно 0.90 | `high` |
  | 0.8999 | `medium` |
  | 0.85 | `medium` |
  | 0.8499 | нет |
  | компания, кандидат с `entity_kind = person` | нет |
  | два кандидата 0.86 и 0.95 | одна находка `high` |
  | нормализованное имя `li` (длина < 3) | `SearchSanctions` не вызван |
  | страна компании `KP` (black) | `high_risk_jurisdiction/high` |
  | гражданство бенефициара в `grey` | `medium` |
  | `Registry{checked, Found=false}` | `registry_mismatch/medium` |
  | `Status = liquidated` | `registry_mismatch/high` |
  | `Names = ["ООО «Альфа»"]` при имени `Альфа` | нет |
  | `Names = ["ООО «Омега»"]` при имени `Альфа` | `registry_mismatch/medium` |
  | `Outcome = unavailable` | `other/low` |
  | `Registry = nil` | правило реестра не сработало |
  | `Store` вернул ошибку | ошибка, находок нет |

- `TestScreenApplication`: target `IR` → `high`; `RUB` сверх порога → `amount_threshold/medium`; ровно порог → нет (условие «выше»); валюта без порога → нет.
- `TestSaveAmlFindings`, `TestAmlFlagsResolve`, `TestAdminListAMLFlagsHandler` — шаг 6.
- `TestParseMock`, `TestParseUK` — шаг 7.
- Интеграционные: `TestSearchSanctionsThresholdIndependent` (шаг 4), `TestSubmitKycAmlIntegration` (шаг 8). Если мок-список не загружен, тест падает с текстом `запусти make load-sanctions` — это не `Skip`, чтобы CI не прошёл молча.

## Готово, когда
- [ ] `make load-sanctions` загружает мок-список (по умолчанию и в `make db`) и UK Sanctions List CSV (`SANCTIONS_SOURCE=uk_fcdo`); работает под ролью миграций (`TestParseMock`, `TestParseUK`, ручная проверка шага 7).
- [ ] `aml.Normalize`: нижний регистр, транслитерация кириллицы, удаление орг. форм (ООО, LLC, GmbH, Ltd), схлопывание пробелов (`TestNormalize`); скрининг ищет по ICAO и BGN/PCGN и берёт максимум (`TestNormalizeVariants`, `TestScreenOrganization`).
- [ ] Кандидаты — через pg_trgm с порогом в запросе (результат не зависит от GUC), финальный score — Jaro-Winkler в Go; ≥ 0.90 → `high`, 0.85–0.90 → `medium` (`TestJaroWinkler`, `TestScreenOrganization`, `TestSearchSanctionsThresholdIndependent`).
- [ ] Проверяются компания и каждый бенефициар (`TestScreenOrganization`, `TestSubmitKycAmlIntegration`).
- [ ] FATF black → `high`, grey → `medium`; сумма выше порога → `medium`; `registry_mismatch` по ответу реестра (`TestScreenOrganization`, `TestScreenApplication`).
- [ ] У каждого флага `details` с источником и score (`TestScreenOrganization`).
- [ ] `adminListAmlFlags` с фильтрами и пагинацией, `adminResolveAmlFlag` с 404 и 409 (`TestAmlFlagsResolve`, `TestAdminListAMLFlagsHandler`).
- [ ] Тесты нормализации и порогов зелёные.
- [ ] И общий критерий из [README](README.md#общий-критерий-готовности-любой-задачи).

## Открытые вопросы

Нет: закрыты в [DECISIONS.md](DECISIONS.md) — порог суммы (п. 20), серый список FATF (п. 22, делает BE-02), записи мок-списка под демо (п. 31), `is_unfriendly` (п. 19), транслитерация (п. 41).
