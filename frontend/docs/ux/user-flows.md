
# Черновик: `docs/ux/user-flows.md`

## 1. Карта экранов и маршрутов (Screen Map)

### Публичные маршруты (без авторизации)
| Экран | Маршрут | Роль | Ключевые `operationId` |
| :--- | :--- | :--- | :--- |
| Вход | `/login` | Все | `login` |
| Регистрация | `/register` | Все | `register`, `registryLookup` (автозаполнение) |

### Заёмщик (Borrower)
| Экран | Маршрут | Ключевые `operationId` | Примечание |
| :--- | :--- | :--- | :--- |
| Сводка (Dashboard) | `/dashboard` | `getMe`, `listApplications` (мои) | Быстрый доступ к активным заявкам |
| Верификация (KYC) | `/kyc` | `getMyOrganization`, `listMyDocuments`, `uploadMyDocument`, `submitKyc` | Блокирует создание заявок, если не `verified` |
| Мои заявки | `/applications` | `listApplications` | Список со статусами и фильтрами |
| Создание заявки | `/applications/new` | `createApplication` | Форма с валидацией сумм и сроков |
| Карточка заявки | `/applications/:id` | `getApplication`, `listApplicationOffers`, `updateApplication`, `submitApplication`, `cancelApplication`, `rejectAllOffers`, `getApplicationHistory` | Видит все офферы, может принять/отклонить |
| Карточка оффера | `/offers/:id` | `getOffer`, `acceptOffer`, `rejectOffer` | Возможность отозвать (`sent` → `withdrawn`) |
| Чат по офферу | `/offers/:id/messages` | `listMessages`, `sendMessage` | Polling каждые 3-5 сек |
| Мои сделки | `/deals` | `listDeals` | Архив согласованных сделок (`Deal`) |
| Карточка сделки | `/deals/:id` | `getDeal` | Возможность посмотреть проведенную сделку |
| Уведомления | `/notifications` | `listNotifications`, `markNotificationRead`, `markAllNotificationsRead` | Колокольчик в хедере |

### Кредитор (Lender)
| Экран | Маршрут | Ключевые `operationId` | Примечание |
| :--- | :--- | :--- | :--- |
| Сводка (Dashboard) | `/dashboard` | `getMe`, `listApplications` (витрина) | Только `published` заявки |
| Верификация (KYC) | `/kyc` | (те же, что у заёмщика) | Обязателен документ `source_of_funds` |
| Витрина заявок | `/applications` | `listApplications` | Фильтры: страна, сумма, срок, валюта |
| Карточка заявки | `/applications/:id` | `getApplication` | Просмотр деталей перед оффером |
| Создание оффера | `/applications/:id/offer`| `createOffer` | Валидация: `amount` ≤ суммы заявки |
| Мои предложения | `/offers` | `listOffers` | Список отправленных офферов |
| Карточка оффера | `/offers/:id` | `getOffer`, `withdrawOffer` | Возможность отозвать (`sent` → `withdrawn`) |
| Чат по офферу | `/offers/:id/messages` | `listMessages`, `sendMessage` | Обсуждение деталей с заёмщиком |
| Мои сделки | `/deals` | `listDeals` | Архив принятых офферов |
| Карточка сделки | `/deals/:id` | `getDeal` | Возможность посмотреть проведенную сделку |
| Уведомления | `/notifications` | `listNotifications`, `markNotificationRead`, `markAllNotificationsRead` | Колокольчик в хедере |

### Администратор (Admin)
| Экран | Маршрут | Ключевые `operationId` | Примечание |
| :--- | :--- | :--- | :--- |
| Аналитика | `/admin/analytics` | `adminAnalyticsSummary` | Графики: топ стран, объёмы по валютам |
| Очередь KYC | `/admin/organizations` | `adminListOrganizations` | Просмотр череди на подтверждение|
|Заявка KYC | `/admin/organizations/:id` |  `adminGetOrganization`, `adminApproveKyc`, `adminRejectKyc` | Просмотр документов, AML-флагов | 
| Модерация заявок| `/admin/applications` | `listApplications`, `adminApproveApplication`, `adminRejectApplication` | Проверка AML-правил заявки |
| AML-мониторинг | `/admin/aml` | `adminListAmlFlags`, `adminResolveAmlFlag` | Работа с флагами (`false_positive` / `confirmed`) |
| Пользователи | `/admin/users` | `adminListUsers`, `adminBlockUser`, `adminUnblockUser` | Список пользователей |
| Пользователь | `/admin/users/:id` | `adminGetUser`, `adminBlockUser`, `adminUnblockUser` | Блокировка нарушителей |
| Аудит | `/admin/audit-log` | `adminListAuditLog` | Журнал действий |

---

## 2. Словарь статусов (RU / EN)

Чтобы интерфейсы были консистентными, используем единый маппинг из OpenAPI-enum:

**KYC Организации (`KycStatus`)**
- `draft` → «Черновик»
- `under_review` → «На проверке»
- `verified` → «Верифицирован»
- `rejected` → «Отклонен»

**Заявка (`ApplicationStatus`)**
- `draft` → «Черновик»
- `moderation` → «На модерации»
- `rejected` → «Отклонена» (с комментарием модератора)
- `published` → «Опубликована» (видна кредиторам)
- `agreed` → «Согласована» (оффер принят, создана сделка)
- `cancelled` → «Отменена»

**Оффер (`OfferStatus`)**
- `sent` → «Отправлено»
- `accepted` → «Принято»
- `rejected` → «Отклонено»
- `withdrawn` → «Отозвано»

**Причины отклонения оффера (`OfferRejectReason`)**
- `rejected_by_borrower` → «Отклонено заёмщиком»
- `another_offer_accepted` → «Принят другой оффер» (системная)
- `application_cancelled` → «Заявка отменена» (системная)

---

## 3. Граничные случаи и Empty States (UX-правила)

1. **Отказ KYC (`kyc_status = rejected`)**:
   - *UI:* Показываем баннер с `kyc_reject_reason` из API.
   - *Действие:* Кнопка «Исправить и подать снова» разблокирует редактирование профиля и загрузку документов.
2. **Заявка без предложений (`offers_count = 0`)**:
   - *UI:* В карточке заявки блок «Предложения» показывает Empty State: «Пока нет предложений. Ожидайте откликов кредиторов.».
3. **Кредитор отозвал предложение (`status = withdrawn`)**:
   - *UI:* Оффер становится неактивным (серый цвет), кнопка «Принять» скрыта. Чат остаётся доступным для чтения истории.
4. **Попытка принять оффер при параллельном запросе (Race Condition)**:
   - *UI:* При получении `409 INVALID_STATUS_TRANSITION` показываем тост: «Это предложение уже было принято или отозвано. Обновите страницу».
5. **Заблокированный пользователь (`403 FORBIDDEN`)**:
   - *UI:* Глобальный перехватчик (axios interceptor) при коде `USER_BLOCKED` редиректит на страницу `/blocked` с текстом: «Ваш аккаунт заблокирован администратором. Причина: {reason}. Обратитесь в поддержку».
6. **Потеря CSRF-токена**:
   - *UI:* При `403 CSRF_INVALID` автоматически делаем `GET /auth/me` (или специальный endpoint refresh), чтобы обновить cookie, и повторяем запрос. Если не вышло — разлогиниваем с сообщением «Сессия истекла».