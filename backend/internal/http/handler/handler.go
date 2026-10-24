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
//     строки в generatedTags (routes_test.go), в routes() (router.go) и в проверках ниже.
//  3. Бизнес-логику держать в internal/service; хендлер только переводит api-типы ⇄ доменные.
//     У операции в спеке — x-roles (и x-kyc-required): их проверяет middleware.Authorize,
//     а сервис повторяет через p.RequireRole(...) и p.RequireKYCVerified().
//     Principal брать через currentPrincipal(ctx); сущность читать запросами *Visible
//     с p.ViewerRole(), p.ViewerOrg() — чужое и несуществующее дают 404.
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
	"io"

	"github.com/google/uuid"

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
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
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

// AuthService — используемые HTTP-слоем операции аутентификации.
type AuthService interface {
	Register(ctx context.Context, in service.RegisterInput, meta service.ClientMeta) (*service.Session, error)
	Login(ctx context.Context, email, password string, meta service.ClientMeta) (*service.Session, error)
	Logout(ctx context.Context, token string) error
	Me(ctx context.Context, principal service.Principal) (*service.Me, error)
	// Authenticate — проверка сессии для middleware.Authenticate.
	Authenticate(ctx context.Context, token string) (*service.Principal, error)
}

// RegistryService — публичный поиск компании в реестре (service.Registry).
type RegistryService interface {
	Lookup(ctx context.Context, meta service.ClientMeta, countryCode, number string) (registry.Result, error)
}

// KycService — профиль компании, бенефициары, KYC-документы (service.Kyc).
type KycService interface {
	MyOrganization(ctx context.Context, p service.Principal) (service.OrganizationProfile, error)
	UpdateMyOrganization(ctx context.Context, p service.Principal, in service.OrganizationUpdateInput) (service.OrganizationProfile, error)
	ListDocuments(ctx context.Context, p service.Principal) ([]repository.KycDocument, error)
	UploadDocument(ctx context.Context, p service.Principal, meta service.ClientMeta, docType repository.KycDocumentType, fileName string, content io.Reader) (repository.KycDocument, error)
	DeleteDocument(ctx context.Context, p service.Principal, id uuid.UUID) error
	OpenDocument(ctx context.Context, p service.Principal, meta service.ClientMeta, id uuid.UUID) (service.DocumentFile, error)
	SubmitKyc(ctx context.Context, p service.Principal, meta service.ClientMeta) (service.OrganizationProfile, error)
}

// ApplicationService — заявки: CRUD, витрина, статусы, история (service.Applications).
type ApplicationService interface {
	List(ctx context.Context, p service.Principal, f service.ApplicationFilter) (service.ApplicationPage, error)
	Get(ctx context.Context, p service.Principal, id uuid.UUID) (service.Application, error)
	Create(ctx context.Context, p service.Principal, meta service.ClientMeta, in service.CreateApplicationInput) (service.Application, error)
	Update(ctx context.Context, p service.Principal, id uuid.UUID, in service.UpdateApplicationInput) (service.Application, error)
	Submit(ctx context.Context, p service.Principal, meta service.ClientMeta, id uuid.UUID) (service.Application, error)
	Cancel(ctx context.Context, p service.Principal, meta service.ClientMeta, id uuid.UUID) (service.Application, error)
	RejectAllOffers(ctx context.Context, p service.Principal, id uuid.UUID) (int, error)
	History(ctx context.Context, p service.Principal, id uuid.UUID) ([]service.StatusHistoryEntry, error)
}

// Deps — всё, от чего зависят хендлеры. Новые сервисы добавляются сюда.
type Deps struct {
	DB           Pinger
	Dicts        DictionaryService
	Auth         AuthService
	Kyc          KycService
	Registry     RegistryService
	Applications ApplicationService
	CookieSecure bool
}

// Handler логирует через slog.Default() — его настраивает cmd/server.
type Handler struct {
	db           Pinger
	dicts        DictionaryService
	auth         AuthService
	kyc          KycService
	registry     RegistryService
	applications ApplicationService
	cookieSecure bool
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
	return &Handler{db: d.DB, dicts: d.Dicts, auth: d.Auth, kyc: d.Kyc, registry: d.Registry, applications: d.Applications, cookieSecure: d.CookieSecure}
}
