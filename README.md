# Liquidity_platform
Требуется разработать MVP цифровой платформы ликвидности для трансграничного финансирования между юридическими лицами — площадки, соединяющей российские компании, ищущие зарубежное финансирование, и зарубежных кредиторов со свободными средствами.  

## Документация

| Что | Где |
|---|---|
| Контракт API (источник правды для фронта и бэка) | [`api/openapi.yaml`](api/openapi.yaml) |
| Архитектура и ключевые решения | [`docs/adr/0001-architecture-and-api-contract.md`](docs/adr/0001-architecture-and-api-contract.md) |
| Схема БД, ER-диаграмма, ключевые запросы | [`docs/db/schema.md`](docs/db/schema.md) |
| Миграции (goose) | [`backend/migrations`](backend/migrations) |
| Бэклог (источник для issues) | [`scripts/issues.yaml`](scripts/issues.yaml) |

Задачи разбиты на вехи M1–M4. Пользовательские сценарии помечены `user-story` и ссылаются на задачи фронта и бэка.
