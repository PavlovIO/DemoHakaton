# План этапа 3 — страница администратора (пункт 10)

> Актуальная база: main `231978e` (этапы 1–2D + 2C влиты). Контракт менять НЕ нужно —
> все админ-ручки уже есть в `api/openapi.yaml` и типизированы в `frontend/src/lib/api-schema.ts`.
> Числовые лимиты, справочники, поллинг — готовы из этапов 1–2.

## Что уже есть
| Ресурс | Контракт (operationId) | Фронт |
|---|---|---|
| Очередь KYC | adminListOrganizations / approveKyc / rejectKyc | ✅ useKyc.ts + вкладка в AdminPanel |
| Модерация заявок | adminApproveApplication / adminRejectApplication | ✅ useApplications.ts + вкладка в AdminPanel |
| AML-флаги | adminListAmlFlags, POST /admin/aml/flags/{id}/resolve | ❌ нет хуков/UI |
| Пользователи | adminListUsers, block/unblock | ❌ нет хуков/UI |
| Аналитика | adminAnalyticsSummary (top_n) | ❌ нет хуков/UI |
| Аудит-лог | adminListAuditLog | ⚠️ опционально (в плане не обязателен) |
| Маршруты | — | ❌ catch-all `{ path: '*', element: <AdminPanel /> }` в router.tsx |

Текущий `AdminPanel.tsx` — 141 строка, две вкладки (`kyc` | `apps`), реальные данные.

---

## Шаг 3A — Хуки админ-API (`useAdmin.ts`)
**Файлы:** создать `frontend/src/hooks/useAdmin.ts`; тест `frontend/src/hooks/useAdmin.integration.test.tsx`.

1. Типы брать из `components['schemas']`: `Organization`, `User`, `AmlFlag`,
   `AnalyticsSummary`, статусные enum'ы (`AmlFlagStatus`, `UserStatus`). Не объявлять свои дубликаты.
2. Хуки по образцу существующих (queryKey-константы, `LIST_POLL_INTERVAL_MS` из `lib/polling.ts`):
   - `useAdminOrganizations(params)` → GET `/admin/organizations` (фильтры: kyc_status, type, country_code, q, пагинация page/page_size);
   - `useAdminUsers(params)` → GET `/admin/users` (role, status, q);
   - `useBlockUser() / useUnblockUser()` → POST `/admin/users/{id}/block|unblock`, инвалидация списка пользователей;
   - `useAmlFlags(params)` → GET `/admin/aml/flags` (status, severity, organization_id, application_id);
   - `useResolveAmlFlag()` → POST `/admin/aml/flags/{flagId}/resolve` (комментарий в теле — сверить с контрактом при реализации);
   - `useAnalyticsSummary(topN?)` → GET `/admin/analytics/summary`, staleTime 60 сек (не поллить чаще).
3. Все mutation — через `api.post`, ошибки — `handleApiError`, toast'ы на месте вызова (как в AdminPanel сейчас).

**Коммит:** `feat(admin): хуки AML, пользователей и аналитики`

## Шаг 3B — Разделение вкладок: шапка + боковое меню, маршруты вместо catch-all
**Файлы:** `router.tsx`, `AdminPanel.tsx` (рефакторинг в layout), создать `frontend/src/pages/admin/KycQueueTab.tsx`, `ApplicationsModerationTab.tsx`.

1. В `router.tsx` заменить `{ path: '*' }` на дочерние маршруты:
   - `/admin` (index) → redirect на `/admin/kyc` (Navigate);
   - `/admin/kyc` — «Очередь KYC»;
   - `/admin/applications` — «Модерация заявок»;
   - `/admin/aml`, `/admin/users`, `/admin/analytics` — заготовки под шаги 3C–3E (или сразу заглушка «В разработке»).
2. `AdminPanel.tsx` превратить в `AdminLayout`: шапка с двумя основными вкладками (KYC / Модерация заявок — как требует пункт 10 «присутствующие вкладки разделить»), ниже — Outlet; боковое/вторичное меню для AML/Пользователи/Аналитика.
3. Состояние `activeTab` убрать — активная вкладка выводится из `useLocation()` (подсветка по префиксу пути).
4. Содержимое существующих вкладок перенести 1:1 в таб-компоненты без изменения логики (query/mutations/handleApiError).
5. Пункты меню админа в дропдауне профиля должны вести на конкретные маршруты (проверить `ProfileDropdown.tsx`).

**Тесты:** в `pages.integration.test.tsx` — переход на `/admin/kyc` и `/admin/applications` рендерит соответствующие заголовки; неизвестный путь `/admin/xxx` → 404-заглушка (не панель).

**Коммит:** `refactor(admin): вкладки KYC и модерации как отдельные маршруты`

## Шаг 3C — Вкладка AML-флагов
**Файлы:** создать `frontend/src/pages/admin/AmlFlagsTab.tsx`; роут `/admin/aml`.

1. Таблица флагов: `rule`, `severity` (бейдж: low/medium/high — цвета как у статусов), `organization.name`,
   `application_id` (ссылка/модалка заявки, переиспользовать модалку просмотра из 2C если применимо), `created_at` (Intl.DateTimeFormat), `status`.
2. Фильтры: статус (open/resolved — значения строго из enum контракта), severity. Пагинация через существующий `LoadMore`.
3. Действие «Снять флаг»: кнопка только для `status=open`; модальное подтверждение с обязательным комментарием
   (заменить `prompt()` на нормальную модалку — заодно можно унифицировать reject-KYC/reject-app позже, но вне объёма этого шага).
4. После resolve — инвалидация списка флагов; toast.

**Тесты:** интеграционный — список open-флагов рендерится, resolve убирает флаг из фильтра open (если сид-данные содержат флаг; иначе — mock-transport юнит-тест хука).

**Коммит:** `feat(admin): вкладка AML-флагов с снятием флага`

## Шаг 3D — Вкладка пользователей
**Файлы:** создать `frontend/src/pages/admin/UsersTab.tsx`; роут `/admin/users`.

1. Поиск `q` (debounce 300 мс — паттерн уже есть в Marketplace), фильтры role/status (enum'ы из контракта).
2. Таблица: email, роль, статус (бейдж), страна, дата регистрации.
3. Кнопки «Заблокировать»/«Разблокировать» с подтверждением и причиной (по контракту block принимает reason — сверить); блокировка админа своей собственной учётной записи — скрыть кнопку для текущего user id (есть `useAuth`).
4. Инвалидации + toast; ошибки 403/404 через `handleApiError`.

**Коммит:** `feat(admin): вкладка пользователей с блокировкой`

## Шаг 3E — Вкладка аналитики
**Файлы:** создать `frontend/src/pages/admin/AnalyticsTab.tsx`; роут `/admin/analytics`.

1. Карточки-счётчики: applications_total, offers_total, deals_total.
2. `applications_by_status` — горизонтальные бары на чистом CSS/Tailwind (без новых зависимостей; recharts не добавлять без необходимости). Человекочитаемые подписи статусов — через `labels.ts`.
3. `amounts_by_currency` — таблица сумм по валютам (формат `formatMoney`, лимит отображения — ничего не ограничиваем, данные уже валидны).
4. `top_demand_countries` / `top_supply_countries` — два списка топов (названия стран обогащать из `useCountries()` — шаг 2A уже даёт хук).
5. `top_n` — селектор 5/10/20, параметр уходит в query.

**Коммит:** `feat(admin): вкладка аналитики (summary)`

## Шаг 3F — Финальная проверка
1. `npx tsc --noEmit`, eslint, `npm run test` (unit), сборка `npm run build`.
2. Интеграционные тесты: `make db` + `npm run test:integration` (локально, требует Docker).
3. Прогон сценария вручную под admin: очереди пустые/наполненные, resolve флага, block/unblock, аналитика обновляется после действий в очередях (инвалидации).
4. Обновить `docs/plan-stage2-frontend.md` → чекбоксы этапа 3 или новый README-раздел.

---

## Порядок и оценка
| Шаг | Зависимости | Объём |
|---|---|---|
| 3A | — | S (чистые хуки) |
| 3B | — | M (рефакторинг роутинга) |
| 3C | 3A, 3B | M |
| 3D | 3A, 3B | M |
| 3E | 3A, 3B | S–M |
| 3F | все | S |

Вне объёма этапа 3 (отложены): SSE вместо поллинга, audit-log UI, замена `prompt()` глобально,
модалка просмотра заявки из пункта 8 (следующий кандидат — этап 4, вместе с пунктом 9 про комиссию,
если текст о комиссии ещё не покрыт на витрине офферов — проверить при старте этапа 4).
