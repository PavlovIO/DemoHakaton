# План этапа 2: справочники из БД, предложения в вкладке заёмщика, редактирование заявки

Этап разбит на 4 независимых шага (2A–2D), каждый — отдельным коммитом по правилам
Conventional Commits из `AGENTS.md`. Контракт API (`api/`) **не меняется**: все нужные
эндпоинты уже описаны и реализованы на бэкенде.

Проверка каждого шага: `npm run build` + тесты фронтенда; `make verify` не требуется
(бэкенд не трогаем).

---

## Шаг 2A. Хуки справочников валют/стран/способов перевода (пункт 6)

Бэкенд уже отдаёт (проверено): `GET /dictionaries/countries`, `/dictionaries/currencies`,
`/dictionaries/transfer-methods` — все с `security: []`, данные сидятся миграциями
(`00002_seed_dictionaries.sql`, `00005_fatf_grey_transfer_methods.sql`: ~13 валют, страны
с флагами FATF/is_unfriendly). Сейчас фронт хардкодит 3 валюты и 3 страны.

**Новые файлы:**
- `src/hooks/useDictionaries.ts` — три хука на `useQuery` + типизация из `api-schema.ts`
  (никаких ручных DTO):
  - `useCountries()` → key `['dictionaries','countries']`
  - `useCurrencies()` → key `['dictionaries','currencies']`
  - `useTransferMethods()` → key `['dictionaries','transfer-methods']`
  - Общие опции: `staleTime: 24h`, `gcTime` по умолчанию, `retry: 1` — справочники почти
    статичны; запросы без авторизации, можно кэшировать агрессивно.
- Интеграционный тест `src/hooks/useDictionaries.integration.test.tsx` (образец —
  существующие `*.integration.test.tsx`): проверяет, что хук резолвит массив из мока/prism.

**Замена хардкода на данные из БД:**
- `src/pages/Applications.tsx` (модалка создания):
  - `<select>` валют (сейчас EUR/USD/RUB) → `useCurrencies()`, `<option value={c.code}>{c.code} — {c.name}</option>`;
  - «Целевая юрисдикция» (сейчас DE/CN/TR) → `useCountries()`, подпись `name_ru (CODE)`,
    для стран с `fatf_list`/`is_unfriendly` — пометка (например, суффикс «⚠ FATF»), если
    поле пришло из схемы;
  - «Способ перевода» (сейчас swift/sepa/other) → `useTransferMethods()`.
- `src/pages/Marketplace.tsx` — фильтры страны (строка ~100) и валюты (~109) те же хуки.
- Fallback на время загрузки: показывать disabled-`select` со словом «Загрузка…»,
  чтобы форма не отправлялась с пустым значением; при ошибке запроса — оставить текущий
  короткий хардкод как аварийный набор (EUR/USD/RUB, DE/CN/TR, swift/sepa/other).

**Контракт типов:** значения `currency`/`transfer_method` в state остаются типами из
сгенерированной схемы (`Application['currency']` и т.п.) — каст через `as` сохраняется,
enum-ограничений в контракте на них нет (`CurrencyCode` = pattern `^[A-Z]{3}$`).

---

## Шаг 2B. Предложения по заявке в вкладке «Мои заявки» (первая часть пункта 7)

Хук `useApplicationOffers(applicationId)` уже есть (`src/hooks/useOffers.ts:23`), но
вкладка заёмщика (`Applications.tsx`) его не использует — сейчас офферы видны только
после клика «Показать предложения» в модалке(?), проверить фактическое поведение перед
стартом и вынести карточки офферов прямо в список заявок.

**Изменения в `src/pages/Applications.tsx`:**
- В карточке каждой заявки статуса `published`/`agreed` — раскрывающийся блок
  («Предложения (N)»): `useApplicationOffers(app.id)`, query enabled по `app.id`.
  Для `draft`/`moderation`/`rejected` — не запрашивать (офферов быть не может).
- Ренд строки оффера: сумма+валюта (`formatMoney`), ставка, срок, график
  (**через человекочитаемую метку графика из `src/lib/labels.ts`, добавленную на этапе 1**),
  статус (`sent/accepted/rejected/withdrawn` → Badge), кнопка «Принять» →
  `useAcceptOffer` (инвалидации OFFERS/APPLICATIONS/DEALS уже настроены), кнопка
  отзыва для своих офферов заёмщика не нужна (заимоffеры — у кредитора).
- Toast на принятие: «Сделка … зафиксирована» — обработчик уже есть в `handleCreate`?
  свериться со строкой ~86, переиспользовать логику.
- Empty state: «Предложений пока нет».

Тест: расширить `src/pages/pages.integration.test.tsx` — рендер вкладки с заявкой
`published` и мок-офферами, проверка кнопки «Принять».

---

## Шаг 2C. Редактирование заявки до отправки на модерацию (вторая часть пункта 7)

Бэкенд разрешает `PATCH /applications/{id}` (operationId `updateApplication`) только
владельцу и только в статусах **`draft` / `rejected`** (см. `api/paths/applications.yaml:127`);
`POST /applications/{id}/submit` (`submitApplication`) переводит `draft|rejected → moderation`.
Фронт ни то, ни другое пока не вызывает — в хуках есть только create/approve/reject/submit
(`submit` проверить; если `useSubmitApplication` уже дёргает submit — использовать его).

**Хуки (`src/hooks/useApplications.ts`):**
- Если нет — добавить `useUpdateApplication()` (mutation, путь `/applications/${id}`,
  onSuccess → invalidate `APPLICATIONS_KEY`) и убедиться, что `useSubmitApplication()`
  существует (он есть, строка 79) и инвалидирует список.
- Тип тела — `ApplicationUpdate` из `api-schema.ts` (не дублировать руками).

**UI (`src/pages/Applications.tsx`):**
- Переиспользовать существующую модалку создания: состояние `editingApp: Application | null`.
  Открытие модалки в режиме редактирования = те же поля, предзаполненные из заявки,
  заголовок «Редактирование заявки», кнопка «Сохранить» вместо «Создать».
- Кнопка **«Редактировать»** в карточке заявки — только для статусов `draft` и `rejected`
  (ровно те, что допускает бэк; для `moderation/published/agreed/cancelled` — скрыта).
- После сохранения — toast «Заявка обновлена», список обновится сам через invalidate.
- Отдельная кнопка **«Отправить на модерацию»** для `draft`/`rejected` → `useSubmitApplication`
  (если её ещё нет в UI); при успехе статус в списке станет `moderation` без перезагрузки.
- Ошибки 409 InvalidTransition показывать через существующий маппинг `src/lib/api-errors.ts`.

Тест: в `pages.integration.test.tsx` — клик «Редактировать» открывает модалку с
предзаполненными полями; PATCH уходит с корректным телом (перехват через integration actors).

---

## Шаг 2D. Фоновое обновление списков без перезагрузки страницы (остаток пункта 1)

На этапе 1 включён `refetchOnWindowFocus` — этого мало для «уведомления/страницы не
обновляются»: данные должны приходить, когда пользователь уже на странице.

**Изменения минимальные:**
- `src/lib/query-client.ts` — дефолтный `staleTime` оставить 30 с; добавить
  `refetchIntervalInBackground: false` (не долбить сервер во вкладке браузера в фоне).
- Списковые query'и (`useMyApplications`, `useMarketplace`, `useModerationQueue`,
  `useMyOffers`, `useDeals`): `refetchInterval: 30_000` — константа
  `LIST_POLL_INTERVAL_MS = 30_000` в `src/lib/query-client.ts` (без магических чисел
  в каждом хуке). Уведомления уже поллятся каждые 30 с (`useNotifications.ts:15`) —
  вынести их интервал на ту же константу.
- Проверить, что toast-уведомления о новых notifications появляются автоматически
  (если компонент подписан на тот же query — да; иначе — маленький effect в
  `NotificationsDropdown.tsx`, сравнивающий id последней прочитанной/пришедшей).

Не делаем (отложено): SSE/WebSocket — требует изменений контракта, вне рамок этапа 2.

---

## Порядок выполнения и объём

| Шаг | Файлы | Оценка |
|-----|-------|--------|
| 2A | useDictionaries.ts (+test), Applications.tsx, Marketplace.tsx | средний |
| 2B | Applications.tsx, labels/Badge, pages.integration.test.tsx | средний |
| 2C | useApplications.ts, Applications.tsx, pages.integration.test.tsx | средний |
| 2D | query-client.ts, 5 хуков, useNotifications.ts | небольшой |

Коммиты: `feat(frontend): справочники валют и стран из БД в формах и фильтрах`,
`feat(frontend): предложения по заявке во вкладке заёмщика`,
`feat(frontend): редактирование и отправка на модерацию заявок`,
`feat(frontend): поллинг списков каждые 30 секунд`.

Этап 3 (модалка просмотра заявок, страница админа с AML/пользователями/аналитикой) —
следующий план после завершения этого.
