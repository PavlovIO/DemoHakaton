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
//  3. Бизнес-логику держать в internal/service/<домен> (ADR-0005); хендлер только переводит
//     api-типы ⇄ доменные.
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
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/auth"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/kyc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/lending"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/notifications"
)

// Pinger — проверка соединения с БД для /healthz (*pgxpool.Pool подходит).
type Pinger interface {
	Ping(ctx context.Context) error
}

// DictionaryService — то, что хендлерам нужно от dictionaries.Dictionaries.
type DictionaryService interface {
	Countries(ctx context.Context) ([]repository.ListCountriesRow, error)
	Currencies(ctx context.Context) ([]repository.ListCurrenciesRow, error)
	TransferMethods(ctx context.Context) ([]repository.TransferMethod, error)
}

// AuthService — используемые HTTP-слоем операции аутентификации.
type AuthService interface {
	Register(ctx context.Context, in auth.RegisterInput, meta service.ClientMeta) (*auth.Session, error)
	Login(ctx context.Context, email, password string, meta service.ClientMeta) (*auth.Session, error)
	Logout(ctx context.Context, token string) error
	Me(ctx context.Context, principal service.Principal) (*auth.Me, error)
	// Authenticate — проверка сессии для middleware.Authenticate.
	Authenticate(ctx context.Context, token string) (*service.Principal, error)
}

// RegistryService — публичный поиск компании в реестре (kyc.Registry).
type RegistryService interface {
	Lookup(ctx context.Context, meta service.ClientMeta, countryCode, number string) (registry.Result, error)
}

// KycService — профиль компании, бенефициары, KYC-документы (kyc.Kyc).
type KycService interface {
	MyOrganization(ctx context.Context, p service.Principal) (kyc.OrganizationProfile, error)
	UpdateMyOrganization(ctx context.Context, p service.Principal, in kyc.OrganizationUpdateInput) (kyc.OrganizationProfile, error)
	ListDocuments(ctx context.Context, p service.Principal) ([]repository.KycDocument, error)
	UploadDocument(ctx context.Context, p service.Principal, meta service.ClientMeta, docType repository.KycDocumentType, fileName string, content io.Reader) (repository.KycDocument, error)
	DeleteDocument(ctx context.Context, p service.Principal, id uuid.UUID) error
	OpenDocument(ctx context.Context, p service.Principal, meta service.ClientMeta, id uuid.UUID) (kyc.DocumentFile, error)
	SubmitKyc(ctx context.Context, p service.Principal, meta service.ClientMeta) (kyc.OrganizationProfile, error)
}

// ApplicationService — заявки: CRUD, витрина, статусы, история (lending.Applications).
type ApplicationService interface {
	List(ctx context.Context, p service.Principal, f lending.ApplicationFilter) (lending.ApplicationPage, error)
	Get(ctx context.Context, p service.Principal, id uuid.UUID) (lending.Application, error)
	Create(ctx context.Context, p service.Principal, meta service.ClientMeta, in lending.CreateApplicationInput) (lending.Application, error)
	Update(ctx context.Context, p service.Principal, id uuid.UUID, in lending.UpdateApplicationInput) (lending.Application, error)
	Submit(ctx context.Context, p service.Principal, meta service.ClientMeta, id uuid.UUID) (lending.Application, error)
	Cancel(ctx context.Context, p service.Principal, meta service.ClientMeta, id uuid.UUID) (lending.Application, error)
	RejectAllOffers(ctx context.Context, p service.Principal, id uuid.UUID) (int, error)
	History(ctx context.Context, p service.Principal, id uuid.UUID) ([]lending.StatusHistoryEntry, error)
}

// OfferService — офферы: создание, просмотр, отклонение, отзыв (lending.Offers).
type OfferService interface {
	Create(ctx context.Context, p service.Principal, meta service.ClientMeta, applicationID uuid.UUID, in lending.CreateOfferInput) (lending.Offer, error)
	Get(ctx context.Context, p service.Principal, id uuid.UUID) (lending.Offer, error)
	List(ctx context.Context, p service.Principal, f lending.OfferFilter) (lending.OfferPage, error)
	ListByApplication(ctx context.Context, p service.Principal, applicationID uuid.UUID, statuses []lending.OfferStatus) ([]lending.Offer, error)
	Reject(ctx context.Context, p service.Principal, meta service.ClientMeta, id uuid.UUID, comment *string) (lending.Offer, error)
	Withdraw(ctx context.Context, p service.Principal, meta service.ClientMeta, id uuid.UUID) (lending.Offer, error)
}

// DealService — сделки: принятие оффера, список, карточка (lending.Deals).
type DealService interface {
	AcceptOffer(ctx context.Context, p service.Principal, meta service.ClientMeta, offerID uuid.UUID) (lending.Deal, error)
	List(ctx context.Context, p service.Principal, f lending.DealFilter) (lending.DealPage, error)
	Get(ctx context.Context, p service.Principal, id uuid.UUID) (lending.Deal, error)
}

// ChatService — чат по офферу: polling и отправка (lending.Chat).
type ChatService interface {
	List(ctx context.Context, p service.Principal, offerID uuid.UUID, afterID int64, limit int32) ([]lending.Message, error)
	Send(ctx context.Context, p service.Principal, offerID uuid.UUID, text string) (lending.Message, error)
}

// NotificationService — лента уведомлений текущего пользователя (notifications.Notifications).
type NotificationService interface {
	List(ctx context.Context, p service.Principal, f notifications.NotificationFilter) (notifications.NotificationPage, error)
	MarkRead(ctx context.Context, p service.Principal, id int64) error
	MarkAllRead(ctx context.Context, p service.Principal) error
}

// AdminService — операции тега admin (admin.Admin).
type AdminService interface {
	ListOrganizations(ctx context.Context, p service.Principal, f admin.OrganizationFilter) (admin.OrganizationPage, error)
	GetOrganization(ctx context.Context, p service.Principal, id uuid.UUID) (admin.OrganizationDetails, error)
	ApproveKyc(ctx context.Context, p service.Principal, id uuid.UUID, comment *string, meta service.ClientMeta) (kyc.OrganizationProfile, error)
	RejectKyc(ctx context.Context, p service.Principal, id uuid.UUID, reason string, meta service.ClientMeta) (kyc.OrganizationProfile, error)
	ApproveApplication(ctx context.Context, p service.Principal, id uuid.UUID, comment *string, meta service.ClientMeta) (lending.Application, error)
	RejectApplication(ctx context.Context, p service.Principal, id uuid.UUID, reason string, meta service.ClientMeta) (lending.Application, error)
	ListUsers(ctx context.Context, p service.Principal, f admin.UserFilter) (admin.UserPage, error)
	BlockUser(ctx context.Context, p service.Principal, id uuid.UUID, reason string, meta service.ClientMeta) (admin.User, error)
	UnblockUser(ctx context.Context, p service.Principal, id uuid.UUID, meta service.ClientMeta) (admin.User, error)
	ListAmlFlags(ctx context.Context, p service.Principal, f admin.AmlFlagFilter) (admin.AmlFlagPage, error)
	ResolveAmlFlag(ctx context.Context, p service.Principal, id int64, resolution repository.AmlFlagStatus, comment string, meta service.ClientMeta) (admin.AmlFlag, error)
	AnalyticsSummary(ctx context.Context, p service.Principal, f admin.AnalyticsFilter) (admin.AnalyticsSummary, error)
	ListAuditLog(ctx context.Context, p service.Principal, f admin.AuditLogFilter) (admin.AuditLogPage, error)
}

// Deps — всё, от чего зависят хендлеры. Новые сервисы добавляются сюда.
type Deps struct {
	DB            Pinger
	Dicts         DictionaryService
	Auth          AuthService
	Kyc           KycService
	Registry      RegistryService
	Applications  ApplicationService
	Offers        OfferService
	Deals         DealService
	Chat          ChatService
	Notifications NotificationService
	Admin         AdminService
	CookieSecure  bool
}

// Handler логирует через slog.Default() — его настраивает cmd/server.
type Handler struct {
	db            Pinger
	dicts         DictionaryService
	auth          AuthService
	kyc           KycService
	registry      RegistryService
	applications  ApplicationService
	offers        OfferService
	deals         DealService
	chat          ChatService
	notifications NotificationService
	admin         AdminService
	cookieSecure  bool
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
	return &Handler{db: d.DB, dicts: d.Dicts, auth: d.Auth, kyc: d.Kyc, registry: d.Registry, applications: d.Applications, offers: d.Offers, deals: d.Deals, chat: d.Chat, notifications: d.Notifications, admin: d.Admin, cookieSecure: d.CookieSecure}
}
