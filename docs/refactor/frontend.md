# Рефакторинг фронтенда: что осталось

**Дата:** 2026-10-25
**Основа:** ветка PR #66 (`feat/frontend-applications-kyc`), коммит `38baa06`.

Список собран по ревью PR #66 (оси Standards и Spec) и по работе над его замечаниями. Баги из ревью уже исправлены в `38baa06`. Здесь осталось то, что исправлено не было. Пункты независимы, если не сказано иное. Выполненный пункт удаляйте из списка или помечайте ссылкой на PR.

## 1. Типы API отстают от контракта

`frontend/src/lib/api-schema.ts` сгенерирован по старой версии `api/openapi.yaml`. Свежая генерация (`npm run gen:api`) даёт расхождение в 93 строки:

- в `GET /applications` нет фильтров `has_my_offer`, `transfer_method`, `lender_organization_id` и значений `sort` `published_at_desc` / `published_at_asc`;
- нет схемы `TransferMethod`, есть и другие мелкие расхождения.

Из-за этого витрина сортирует по `created_at_desc`, хотя контракт велит `status=published&sort=published_at_desc` (`api/openapi.yaml`, описание `listApplications`). Архив откликов кредитора (`has_my_offer=true`, issue #43) тоже нельзя запросить типобезопасно.

**Что сделать:** перегенерировать типы, затем перевести `useMarketplace` (`hooks/useApplications.ts`) на `sort: 'published_at_desc'` и прогнать `tsc -b`. Новые поля могут сломать компиляцию в местах, где типы собраны руками. Стоит добавить в CI проверку, что `api-schema.ts` совпадает с генерацией, по аналогии с `make gen-check` на бэкенде.

## 2. Магические числа

| Где | Что | Предложение |
|---|---|---|
| `hooks/useMessages.ts` | `refetchInterval: 5_000` | именованная константа рядом с `BATCH` |
| `hooks/useNotifications.ts` | `refetchInterval: 30_000` | именованная константа |
| `pages/Marketplace.tsx` | дефолты оффера `'8.00'` и `'1.00'` записаны дважды: в `useState` и в `openOfferModal` | один объект `OFFER_DEFAULTS` |
| `pages/KycStub.tsx` | текст «PDF, PNG или JPEG, до 10 МБ» | константы рядом с хуком загрузки. Лимит и типы взяты из `uploadMyDocument` в контракте: 10 MB, иначе 413 `FILE_TOO_LARGE`. Те же константы стоит проверять на клиенте до отправки |

## 3. Дублирование

- **Бейджи статусов.** `getStatusBadge` со switch или цепочки `status === … && <Badge>` собраны отдельно в `Applications.tsx`, `Offers.tsx`, `KycStub.tsx` и частично в `AdminPanel.tsx`. Нужен общий компонент или словарь `статус → { variant, label }` для каждой сущности.
- **Форматирование времени.** `new Date(x).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })` записано в `components/ChatModal.tsx` и `components/NotificationsDropdown.tsx`. Перенести в `lib/` рядом с `lib/money.ts`.
- **Одобрить / отклонить в AdminPanel.** Обработчики KYC и модерации заявок устроены одинаково: `prompt` для причины, `mutate`, `toast`, `handleApiError`. Можно вынести общий хелпер.

## 4. UX-заглушки

- `prompt()` для причины отказа (`AdminPanel.tsx`, два места) и `confirm()` для отзыва оффера (`Offers.tsx`) лучше заменить модалками, как в `ChatModal`.
- Маршрут `applications/:id` в `router.tsx` рендерит `<div>Детали заявки</div>`.
- `KycStub` и `ProfileStub` уже работают с API, но названы заглушками. Переименовать, например, в `KycPage` и `ProfilePage`.

## 5. Тестируемость

- `router.tsx` создаёт роутер на уровне модуля (`createBrowserRouter`), поэтому маршруты нельзя отрендерить в тесте. Сейчас тесты страниц (`pages/pages.integration.test.tsx`) повторяют обёртку роутера руками: `renderPage` оборачивает страницу в `PrivateRoute`. Если вынести массив `routes` и экспортировать его, тесты смогут работать через `createMemoryRouter`, а `PrivateRoute` не придётся копировать.
- Лимитер бэкенда (10 регистраций с IP в час, лимит логинов) держит состояние в памяти. Полный прогон `npm run test:integration` подряд упирается в 429, тогда помогает `docker restart lp_backend`. Варианты решения: тестовый режим лимитера в конфиге бэкенда или меньше логинов в `global-setup`.
- `vite.config.ts` использует `__dirname`, и Vite предупреждает, что это не поддерживается в `configLoader: 'native'`. Заменить на `import.meta.dirname`.

## 6. PR #66 и issues

- В описании PR девять `Closes` (#37–#43, #45, #46), но ни один issue не выполнен целиком. #43 (архив) и #46 (дашборд аналитики) не начаты. #36 и #44 ветка тоже затрагивает. Нужно предложить автору заменить `Closes` на `Refs`. Описание PR чужое, правит его автор.
- Перед закрытием каждого issue сверить код с его критериями приёмки. После фиксов ревью эта сверка ещё не проводилась.
- Страницы не проверены вручную в браузере после перехода на API (KYC, витрина, чат, админка).

## Как устроены тесты (чего не видно в коде)

- Интеграционные тесты идут против живого бэкенда (`make db && make backend-up`). Харнесс лежит в `src/test/integration/`.
- Тест хука проверяет поведение через публичный хук. Тест страницы рендерит её в `PrivateRoute` и шпионит за `fetch`, чтобы проверить, что чужая роль не делает запросов.
- После `act(() => refetch())` `result.current` обновляется не сразу: notifyManager в TanStack Query уведомляет подписчиков асинхронно. Финальные проверки оборачивайте в `waitFor`.
- Node fetch в jsdom не понимает `FormData` и `File`, поэтому `browser-fetch.ts` собирает multipart вручную (`encodeForm`).
- `/auth/me` отдаёт организацию без бенефициаров. Полный профиль есть только в `GET /organizations/me`.
