// Package handler реализует api.StrictServerInterface (сгенерирован из api/openapi.yaml).
//
// Как добавить операцию (образец — dictionaries.go + dictionaries_test.go):
//  1. Удалить заглушку метода из unimplemented.go.
//  2. Написать метод в файле своего домена: applications.go, offers.go, ...
//     Все методы принадлежат одному типу *Handler, Go разрешает раскладывать их по файлам.
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

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
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

// Компилятор проверяет, что Handler реализует все операции спеки.
var _ api.StrictServerInterface = (*Handler)(nil)

func New(d Deps) *Handler {
	return &Handler{db: d.DB, dicts: d.Dicts}
}
