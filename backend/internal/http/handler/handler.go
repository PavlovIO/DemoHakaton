// Package handler реализует StrictServerInterface всех пакетов internal/http/api/<tag>api
// (сгенерированы из api/openapi.yaml, по пакету на тег — ADR-0003).
//
// Раскладка: файл на тег (auth.go, offers.go, ...) — операции этого тега; общие части —
// handler.go (Deps), router.go, errors.go, validate.go.
//
// Как добавить операцию (образец — dictionaries.go + dictionaries_test.go):
//  1. Найти заглушку метода в файле своего тега (offers.go для тега offers) и заменить её тело.
//     Все методы принадлежат одному типу *Handler, Go разрешает раскладывать их по файлам.
//     Вход и выход — типы пакета тега (offersapi.AcceptOfferRequestObject), модели — пакет api.
//  2. Новый тег в спеке = новый пакет <tag>api (скопировать gen.go и register.go соседнего),
//     строки в generatedTags (routes_test.go), в NewRouter и в проверках ниже.
//  3. Бизнес-логику держать в internal/service; хендлер только переводит api-типы ⇄ доменные.
//  4. Ошибки НЕ собирать через типизированные XxxNNNJSONResponse: вернуть nil, apperr.X(...).
//     Маппинг код → HTTP-статус централизован в errors.go (WriteError).
//     Типизированные ответы используются только для успешных ответов (и Healthz503).
//  5. Зависимость нового сервиса — интерфейс в Deps, объявленный здесь (на стороне потребителя),
//     чтобы в тестах подставлять фейк.
//  6. Имена в пакете api и в sqlc нормализуются по-разному: api.KYCStatus / api.Country.FATFHighRisk,
//     но repository.KycStatus / ListCountriesRow.FatfHighRisk (ID совпадает). Опечатка здесь —
//     ошибка компиляции, а не рантайма, но при переносе полей вручную это легко перепутать.
package handler

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/adminapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/applicationsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/authapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/chatapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/dealsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/dictionariesapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/notificationsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/offersapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/organizationsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/systemapi"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Pinger — проверка соединения с БД для /healthz (*pgxpool.Pool подходит).
type Pinger interface {
	Ping(ctx context.Context) error
}

// DictionaryService — то, что хендлерам нужно от service.Dictionaries.
type DictionaryService interface {
	Countries(ctx context.Context) ([]repository.ListCountriesRow, error)
	Currencies(ctx context.Context) ([]repository.ListCurrenciesRow, error)
	TransferMethods(ctx context.Context) ([]repository.TransferMethod, error)
}

// Deps — всё, от чего зависят хендлеры. Новые сервисы добавляются сюда.
type Deps struct {
	DB    Pinger
	Dicts DictionaryService
}

// Handler логирует через slog.Default() — его настраивает cmd/server.
type Handler struct {
	db    Pinger
	dicts DictionaryService
}

// Компилятор проверяет, что Handler реализует все операции спеки: по интерфейсу на тег.
var (
	_ authapi.StrictServerInterface          = (*Handler)(nil)
	_ organizationsapi.StrictServerInterface = (*Handler)(nil)
	_ dictionariesapi.StrictServerInterface  = (*Handler)(nil)
	_ applicationsapi.StrictServerInterface  = (*Handler)(nil)
	_ offersapi.StrictServerInterface        = (*Handler)(nil)
	_ dealsapi.StrictServerInterface         = (*Handler)(nil)
	_ chatapi.StrictServerInterface          = (*Handler)(nil)
	_ notificationsapi.StrictServerInterface = (*Handler)(nil)
	_ adminapi.StrictServerInterface         = (*Handler)(nil)
	_ systemapi.StrictServerInterface        = (*Handler)(nil)
)

func New(d Deps) *Handler {
	return &Handler{db: d.DB, dicts: d.Dicts}
}
