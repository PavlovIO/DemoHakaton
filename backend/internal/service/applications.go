package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Действия audit_log для заявок (BE-14: «создание / отправка заявки», плюс отмена).
const (
	AuditApplicationCreate = "application.create"
	AuditApplicationSubmit = "application.submit"
	AuditApplicationCancel = "application.cancel"
)

const applicationNotFound = "Заявка не найдена"

// Пагинация и сортировка по умолчанию — как в контракте (Page, PageSize, sort).
const (
	defaultPage     = 1
	defaultPageSize = 20
	defaultSort     = "created_at_desc"
)

// OrgShort — компания в карточке заявки.
type OrgShort struct {
	ID          uuid.UUID
	Name        string
	CountryCode string
}

// Application — заявка, как её видит конкретный пользователь: вычисляемые поля
// (OffersCount, MyOfferID, ModerationComment) уже отфильтрованы по роли.
type Application struct {
	ID                    uuid.UUID
	Number                int64
	Borrower              OrgShort
	Status                ApplicationStatus
	Amount                decimal.Decimal
	Currency              string
	TargetCountryCode     string
	Purpose               string
	TermMonths            int
	TransferMethod        string
	TransferMethodComment *string
	ModerationComment     *string // только владельцу и admin
	AcceptedOfferID       *uuid.UUID
	OffersCount           *int64     // только владельцу и admin
	MyOfferID             *uuid.UUID // только lender: его активный оффер
	SubmittedAt           *time.Time
	PublishedAt           *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// CreateApplicationInput — новая заявка; формат полей уже проверен валидатором по контракту.
type CreateApplicationInput struct {
	Amount                decimal.Decimal
	Currency              string
	TargetCountryCode     string
	Purpose               string
	TermMonths            int
	TransferMethod        string
	TransferMethodComment *string
}

// UpdateApplicationInput — частичное обновление заявки. nil — поле не трогать;
// TransferMethodComment: "" — очистить (DECISIONS п. 7).
type UpdateApplicationInput struct {
	Amount                *decimal.Decimal
	Currency              *string
	TargetCountryCode     *string
	Purpose               *string
	TermMonths            *int
	TransferMethod        *string
	TransferMethodComment *string
}

// StatusHistoryEntry — строка журнала переходов заявки и её офферов.
type StatusHistoryEntry struct {
	ID          int64
	EntityType  string
	EntityID    uuid.UUID
	FromStatus  *string // nil — сущность создана
	ToStatus    string
	ActorUserID *uuid.UUID // nil — переход сделала система
	ActorRole   *string
	Comment     *string
	CreatedAt   time.Time
}

// ApplicationFilter — фильтры списка заявок. nil / пустой срез — фильтр не задан.
// Page и PageSize: 0 — по умолчанию (1 и 20).
type ApplicationFilter struct {
	Statuses        []ApplicationStatus
	TargetCountries []string
	TransferMethods []string
	Currency        *string
	AmountMin       *decimal.Decimal // только вместе с Currency
	AmountMax       *decimal.Decimal // только вместе с Currency
	TermMin         *int
	TermMax         *int
	CreatedFrom     *time.Time
	CreatedTo       *time.Time
	BorrowerOrgID   *uuid.UUID // учитывается только для admin
	HasMyOffer      *bool      // учитывается только для lender
	Sort            string     // значение ListApplicationsParamsSort; "" — created_at_desc
	Page            int
	PageSize        int
}

// ApplicationPage — страница списка заявок.
type ApplicationPage struct {
	Items    []Application
	Total    int64
	Page     int
	PageSize int
}

// ApplicationStore — запросы вне транзакции (чтение).
type ApplicationStore interface {
	GetApplicationVisible(ctx context.Context, arg repository.GetApplicationVisibleParams) (repository.GetApplicationVisibleRow, error)
	ListApplicationsVisible(ctx context.Context, arg repository.ListApplicationsVisibleParams) ([]repository.ListApplicationsVisibleRow, error)
	CountApplicationsVisible(ctx context.Context, arg repository.CountApplicationsVisibleParams) (int64, error)
	ListApplicationHistory(ctx context.Context, arg repository.ListApplicationHistoryParams) ([]repository.StatusHistory, error)
}

// TxRunner — транзакция, в которой выполняются мутации заявки.
type TxRunner interface {
	InTx(ctx context.Context, fn func(q repository.Querier) error) error
}

// ApplicationScreener — AML-скрининг заявки при отправке на модерацию (aml.Screener).
type ApplicationScreener interface {
	ScreenApplication(ctx context.Context, s aml.ApplicationSubject) ([]aml.Finding, error)
}

// ApplicationsDeps — зависимости сервиса заявок.
type ApplicationsDeps struct {
	Store    ApplicationStore
	Tx       TxRunner
	Screener ApplicationScreener // обязателен: без AML submit не проходит
}

// Applications — заявки заёмщиков: CRUD, витрина, статусы, история.
type Applications struct {
	store    ApplicationStore
	tx       TxRunner
	screener ApplicationScreener
	notifier Notifier
}

// NewApplications создаёт сервис заявок. Без Screener паникует, как NewKyc: AML
// обязателен (fail-closed), ошибку конфигурации лучше поймать при старте.
func NewApplications(d ApplicationsDeps) *Applications {
	if d.Screener == nil {
		panic("service.NewApplications: Screener обязателен")
	}
	return &Applications{store: d.Store, tx: d.Tx, screener: d.Screener}
}

// List — витрина и списки заявок. Видимость задаёт SQL по (viewer_role, viewer_org) из p;
// фильтры её только сужают. Фильтры чужих ролей отбрасываются: заёмщик не выберет чужую
// компанию через borrower_organization_id, has_my_offer не имеет смысла без оффера.
func (s *Applications) List(ctx context.Context, p Principal, f ApplicationFilter) (ApplicationPage, error) {
	// Сумма без валюты не сравнима: 100 RUB и 100 USD — разные деньги.
	if f.Currency == nil {
		var details []apperr.FieldError
		if f.AmountMin != nil {
			details = append(details, apperr.FieldError{Field: "amount_min", Message: "только вместе с currency"})
		}
		if f.AmountMax != nil {
			details = append(details, apperr.FieldError{Field: "amount_max", Message: "только вместе с currency"})
		}
		if len(details) > 0 {
			return ApplicationPage{}, apperr.Validation("Фильтр по сумме требует currency", details...)
		}
	}
	if p.ViewerRole() != repository.UserRoleAdmin {
		f.BorrowerOrgID = nil
	}
	if p.ViewerRole() != repository.UserRoleLender {
		f.HasMyOffer = nil
	}
	page, pageSize := f.Page, f.PageSize
	if page < 1 {
		page = defaultPage
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	sort := f.Sort
	if sort == "" {
		sort = defaultSort
	}

	count := repository.CountApplicationsVisibleParams{
		ViewerRole:        p.ViewerRole(),
		ViewerOrg:         p.ViewerOrg(),
		Statuses:          statusFilter(f.Statuses),
		TargetCountries:   f.TargetCountries,
		TransferMethods:   f.TransferMethods,
		Currency:          optionalText(f.Currency),
		AmountMin:         optionalDecimal(f.AmountMin),
		AmountMax:         optionalDecimal(f.AmountMax),
		TermMin:           optionalInt4(f.TermMin),
		TermMax:           optionalInt4(f.TermMax),
		CreatedFrom:       optionalTime(f.CreatedFrom),
		CreatedTo:         optionalTime(f.CreatedTo),
		BorrowerOrgFilter: optionalUUID(f.BorrowerOrgID),
		HasMyOffer:        optionalBool(f.HasMyOffer),
	}
	rows, err := s.store.ListApplicationsVisible(ctx, repository.ListApplicationsVisibleParams{
		ViewerRole:        count.ViewerRole,
		ViewerOrg:         count.ViewerOrg,
		Statuses:          count.Statuses,
		TargetCountries:   count.TargetCountries,
		TransferMethods:   count.TransferMethods,
		Currency:          count.Currency,
		AmountMin:         count.AmountMin,
		AmountMax:         count.AmountMax,
		TermMin:           count.TermMin,
		TermMax:           count.TermMax,
		CreatedFrom:       count.CreatedFrom,
		CreatedTo:         count.CreatedTo,
		BorrowerOrgFilter: count.BorrowerOrgFilter,
		HasMyOffer:        count.HasMyOffer,
		Sort:              sort,
		PageLimit:         int32(pageSize),              //nolint:gosec // ≤ 100 по контракту
		PageOffset:        int32((page - 1) * pageSize), //nolint:gosec // page ограничен контрактом
	})
	if err != nil {
		return ApplicationPage{}, fmt.Errorf("list applications: %w", err)
	}
	total, err := s.store.CountApplicationsVisible(ctx, count)
	if err != nil {
		return ApplicationPage{}, fmt.Errorf("count applications: %w", err)
	}
	items := make([]Application, len(rows))
	for i, row := range rows {
		items[i] = applicationFromVisible(p, row)
	}
	return ApplicationPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// Get — заявка, если p её видит; чужая и несуществующая — одинаково 404 (IDOR).
func (s *Applications) Get(ctx context.Context, p Principal, id uuid.UUID) (Application, error) {
	return s.reread(ctx, s.store, p, id)
}

// History — переходы заявки и её офферов по времени. Кредитор видит только свои офферы
// (фильтр в SQL) и не видит комментарии модерации: moderation_comment ему скрыт в
// Application, а в истории та же причина лежит в comment (DECISIONS п. 10).
func (s *Applications) History(ctx context.Context, p Principal, id uuid.UUID) ([]StatusHistoryEntry, error) {
	if _, err := s.Get(ctx, p, id); err != nil {
		return nil, err
	}
	rows, err := s.store.ListApplicationHistory(ctx, repository.ListApplicationHistoryParams{
		ApplicationID: id, ViewerRole: p.ViewerRole(), ViewerOrg: p.ViewerOrg(),
	})
	if err != nil {
		return nil, fmt.Errorf("application history: %w", err)
	}
	hideModeration := p.ViewerRole() == repository.UserRoleLender
	out := make([]StatusHistoryEntry, len(rows))
	for i, h := range rows {
		e := StatusHistoryEntry{
			ID:          h.ID,
			EntityType:  string(h.EntityType),
			EntityID:    h.EntityID,
			FromStatus:  nullableText(h.FromStatus),
			ToStatus:    h.ToStatus,
			ActorUserID: nullableUUID(h.ActorUserID),
			Comment:     nullableText(h.Comment),
			CreatedAt:   h.CreatedAt.Time,
		}
		if h.ActorRole.Valid {
			role := string(h.ActorRole.UserRole)
			e.ActorRole = &role
		}
		if hideModeration && h.EntityType == repository.EntityTypeApplication {
			e.Comment = nil
		}
		out[i] = e
	}
	return out, nil
}

// Create — новая заявка в draft. Роль и KYC проверяются и в middleware, и здесь
// (DECISIONS п. 36). Создание тоже пишется в историю: NULL → draft.
func (s *Applications) Create(ctx context.Context, p Principal, meta ClientMeta, in CreateApplicationInput) (Application, error) {
	if err := p.RequireRole(repository.UserRoleBorrower); err != nil {
		return Application{}, err
	}
	if err := p.RequireKYCVerified(); err != nil {
		return Application{}, err
	}
	var out Application
	err := s.tx.InTx(ctx, func(q repository.Querier) error {
		created, err := q.CreateApplication(ctx, repository.CreateApplicationParams{
			BorrowerOrganizationID: p.ViewerOrg(),
			CreatedByUserID:        p.UserID,
			Amount:                 in.Amount,
			Currency:               in.Currency,
			TargetCountryCode:      in.TargetCountryCode,
			Purpose:                in.Purpose,
			TermMonths:             int32(in.TermMonths), //nolint:gosec // 1..120 по контракту
			TransferMethod:         in.TransferMethod,
			TransferMethodComment:  optionalText(in.TransferMethodComment),
		})
		if err != nil {
			return pgerr.Map(err, applicationNotFound)
		}
		if err := recordStatusChange(ctx, q, StatusChange{
			EntityType: repository.EntityTypeApplication, EntityID: created.ID,
			To: string(created.Status), Actor: &p,
		}); err != nil {
			return err
		}
		if err := writeAudit(ctx, q, meta, applicationAudit(AuditApplicationCreate, p, created.ID)); err != nil {
			return err
		}
		out, err = s.reread(ctx, q, p, created.ID)
		return err
	})
	return out, err
}

// Update — правка заявки владельцем в draft или rejected; статус не меняется, история не
// пишется. Блокировка строки сериализует PATCH с submit и cancel той же заявки.
func (s *Applications) Update(ctx context.Context, p Principal, id uuid.UUID, in UpdateApplicationInput) (Application, error) {
	if err := p.RequireRole(repository.UserRoleBorrower); err != nil {
		return Application{}, err
	}
	var out Application
	err := s.tx.InTx(ctx, func(q repository.Querier) error {
		cur, err := lockOwnedApplication(ctx, q, p, id)
		if err != nil {
			return err
		}
		status := ApplicationStatus(cur.Status)
		if !canEditApplication(status) {
			return apperr.New(apperr.CodeInvalidTransition, "Заявку можно менять только в статусах draft и rejected")
		}
		_, err = q.UpdateApplication(ctx, repository.UpdateApplicationParams{
			ID:                    id,
			FromStatus:            cur.Status,
			Amount:                optionalDecimal(in.Amount),
			Currency:              optionalText(in.Currency),
			TargetCountryCode:     optionalText(in.TargetCountryCode),
			Purpose:               optionalText(in.Purpose),
			TermMonths:            optionalInt4(in.TermMonths),
			TransferMethod:        optionalText(in.TransferMethod),
			TransferMethodComment: optionalText(in.TransferMethodComment), // "" доходит до SQL и очищает
		})
		if err != nil {
			return transitionErr(err, status, status)
		}
		out, err = s.reread(ctx, q, p, id)
		return err
	})
	return out, err
}

// Submit — отправка на модерацию из draft или rejected. В одной транзакции с переходом:
// AML-скрининг (флаги атомарны с переходом), история, аудит и уведомления админам —
// application_submitted и одно aml_flag_created на весь прогон (DECISIONS п. 14).
func (s *Applications) Submit(ctx context.Context, p Principal, meta ClientMeta, id uuid.UUID) (Application, error) {
	if err := p.RequireRole(repository.UserRoleBorrower); err != nil {
		return Application{}, err
	}
	var out Application
	err := s.tx.InTx(ctx, func(q repository.Querier) error {
		cur, err := lockOwnedApplication(ctx, q, p, id)
		if err != nil {
			return err
		}
		from := ApplicationStatus(cur.Status)
		to, err := NextApplicationStatus(from, AppSubmit)
		if err != nil {
			return err
		}
		if _, err := q.SubmitApplication(ctx, repository.SubmitApplicationParams{ID: id, FromStatus: cur.Status}); err != nil {
			return transitionErr(err, from, to)
		}
		findings, err := s.screener.ScreenApplication(ctx, aml.ApplicationSubject{
			TargetCountryCode: cur.TargetCountryCode, Amount: cur.Amount, CurrencyCode: cur.Currency,
		})
		if err != nil {
			return fmt.Errorf("aml screening: %w", err)
		}
		flags, err := saveAmlFindings(ctx, q, cur.BorrowerOrganizationID, &cur.ID, findings)
		if err != nil {
			return err
		}
		if err := recordStatusChange(ctx, q, StatusChange{
			EntityType: repository.EntityTypeApplication, EntityID: id, From: string(from), To: string(to), Actor: &p,
		}); err != nil {
			return err
		}
		if err := writeAudit(ctx, q, meta, applicationAudit(AuditApplicationSubmit, p, id)); err != nil {
			return err
		}
		number := ApplicationNumber(cur.Number.Int64)
		if err := s.notifier.NotifyAdmins(ctx, q, applicationNotification(
			repository.NotificationTypeApplicationSubmitted, "Заявка на модерации: "+number, id)); err != nil {
			return err
		}
		if len(flags) > 0 {
			if err := s.notifier.NotifyAdmins(ctx, q, applicationNotification(
				repository.NotificationTypeAmlFlagCreated, fmt.Sprintf("AML: %d новых флагов — %s", len(flags), number), id)); err != nil {
				return err
			}
		}
		out, err = s.reread(ctx, q, p, id)
		return err
	})
	return out, err
}

// Cancel — отмена заявки владельцем. Все sent-офферы отклоняются в той же транзакции
// (application_cancelled): иначе кредитор ждал бы ответа по отменённой заявке. Админов не
// уведомляем даже из moderation (DECISIONS п. 14): отмена видна в истории и очереди.
func (s *Applications) Cancel(ctx context.Context, p Principal, meta ClientMeta, id uuid.UUID) (Application, error) {
	if err := p.RequireRole(repository.UserRoleBorrower); err != nil {
		return Application{}, err
	}
	var out Application
	err := s.tx.InTx(ctx, func(q repository.Querier) error {
		cur, err := lockOwnedApplication(ctx, q, p, id)
		if err != nil {
			return err
		}
		from := ApplicationStatus(cur.Status)
		to, err := NextApplicationStatus(from, AppCancel)
		if err != nil {
			return err
		}
		if _, err := s.rejectSentOffers(ctx, q, p, cur, repository.OfferRejectReasonApplicationCancelled); err != nil {
			return err
		}
		if _, err := q.CancelApplication(ctx, repository.CancelApplicationParams{ID: id, FromStatus: cur.Status}); err != nil {
			return transitionErr(err, from, to)
		}
		if err := recordStatusChange(ctx, q, StatusChange{
			EntityType: repository.EntityTypeApplication, EntityID: id, From: string(from), To: string(to), Actor: &p,
		}); err != nil {
			return err
		}
		if err := writeAudit(ctx, q, meta, applicationAudit(AuditApplicationCancel, p, id)); err != nil {
			return err
		}
		out, err = s.reread(ctx, q, p, id)
		return err
	})
	return out, err
}

// RejectAllOffers — заёмщик отклоняет все sent-офферы опубликованной заявки
// (rejected_by_borrower); заявка остаётся published и принимает новые офферы.
func (s *Applications) RejectAllOffers(ctx context.Context, p Principal, id uuid.UUID) (int, error) {
	if err := p.RequireRole(repository.UserRoleBorrower); err != nil {
		return 0, err
	}
	var n int
	err := s.tx.InTx(ctx, func(q repository.Querier) error {
		cur, err := lockOwnedApplication(ctx, q, p, id)
		if err != nil {
			return err
		}
		if !canRejectAllOffers(ApplicationStatus(cur.Status)) {
			return apperr.New(apperr.CodeInvalidTransition, "Отклонить офферы можно только у опубликованной заявки")
		}
		n, err = s.rejectSentOffers(ctx, q, p, cur, repository.OfferRejectReasonRejectedByBorrower)
		return err
	})
	return n, err
}

// rejectSentOffers — каскадное отклонение sent-офферов заблокированной заявки: история
// каждого оффера и offer_rejected его кредитору. actor — заёмщик: отклонение вызвало его
// действие, причина — в comment (DECISIONS п. 12). Возвращает число отклонённых.
func (s *Applications) rejectSentOffers(ctx context.Context, q repository.Querier, p Principal,
	app repository.Application, reason repository.OfferRejectReason,
) (int, error) {
	rejected, err := q.RejectSentOffersByApplication(ctx, repository.RejectSentOffersByApplicationParams{
		ApplicationID: app.ID, Reason: reason,
	})
	if err != nil {
		return 0, fmt.Errorf("reject offers of application %s: %w", app.ID, err)
	}
	title := "Оффер отклонён: заявка " + ApplicationNumber(app.Number.Int64)
	for _, o := range rejected {
		if err := recordStatusChange(ctx, q, StatusChange{
			EntityType: repository.EntityTypeOffer, EntityID: o.ID,
			From: string(repository.OfferStatusSent), To: string(repository.OfferStatusRejected),
			Actor: &p, Comment: string(reason),
		}); err != nil {
			return 0, err
		}
		if err := s.notifier.NotifyOrganization(ctx, q, o.LenderOrganizationID, Notification{
			Type: repository.NotificationTypeOfferRejected, Title: title,
			EntityType: repository.EntityTypeOffer, EntityID: o.ID.String(),
		}); err != nil {
			return 0, err
		}
	}
	return len(rejected), nil
}

// ApplicationNumber — номер заявки для людей: APP-000123. Единственное место формата:
// им пользуются хендлеры (BE-07, BE-08) и заголовки уведомлений (BE-11).
func ApplicationNumber(n int64) string {
	return fmt.Sprintf("APP-%06d", n)
}

func applicationNotification(t repository.NotificationType, title string, id uuid.UUID) Notification {
	return Notification{Type: t, Title: title, EntityType: repository.EntityTypeApplication, EntityID: id.String()}
}

// lockOwnedApplication — FOR UPDATE на заявку компании p. Чужая и несуществующая — 404.
// Первый шаг каждой мутации: сначала строка заявки, потом её офферы (порядок блокировок
// общий с acceptOffer, BE-09 — иначе deadlock).
func lockOwnedApplication(ctx context.Context, q repository.Querier, p Principal, id uuid.UUID) (repository.Application, error) {
	a, err := q.LockApplicationForBorrower(ctx, repository.LockApplicationForBorrowerParams{
		ID: id, BorrowerOrganizationID: p.ViewerOrg(),
	})
	if err != nil {
		return repository.Application{}, pgerr.Map(err, applicationNotFound)
	}
	return a, nil
}

// transitionErr — ошибка переходного UPDATE … AND status = from_status. Строка уже
// заблокирована и видна, поэтому 0 строк значит, что статус сменился, — это 409, а не
// 404, как сделал бы pgerr.Map.
func transitionErr(err error, from, to ApplicationStatus) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.InvalidTransition(string(from), string(to))
	}
	return pgerr.Map(err, applicationNotFound)
}

// reread — заявка глазами p на том же q: ответ мутации собирает borrower, offers_count и
// остальные вычисляемые поля той же конвертацией, что и Get.
func (s *Applications) reread(ctx context.Context, q ApplicationStore, p Principal, id uuid.UUID) (Application, error) {
	row, err := q.GetApplicationVisible(ctx, repository.GetApplicationVisibleParams{
		ID: id, ViewerRole: p.ViewerRole(), ViewerOrg: p.ViewerOrg(),
	})
	if err != nil {
		return Application{}, pgerr.Map(err, applicationNotFound)
	}
	return applicationFromVisible(p, repository.ListApplicationsVisibleRow(row)), nil
}

// applicationFromVisible — единственная конвертация строки *Visible в Application.
// Скрывает поля по роли: moderation_comment и offers_count — только владельцу и admin
// (кредитор не должен видеть ни причину отказа модерации, ни конкуренцию), my_offer_id —
// только кредитору.
func applicationFromVisible(p Principal, row repository.ListApplicationsVisibleRow) Application {
	a := row.Application
	out := Application{
		ID:                    a.ID,
		Number:                a.Number.Int64,
		Borrower:              OrgShort{ID: a.BorrowerOrganizationID, Name: row.BorrowerName, CountryCode: row.BorrowerCountryCode},
		Status:                ApplicationStatus(a.Status),
		Amount:                a.Amount,
		Currency:              a.Currency,
		TargetCountryCode:     a.TargetCountryCode,
		Purpose:               a.Purpose,
		TermMonths:            int(a.TermMonths),
		TransferMethod:        a.TransferMethod,
		TransferMethodComment: nullableText(a.TransferMethodComment),
		AcceptedOfferID:       nullableUUID(a.AcceptedOfferID),
		SubmittedAt:           nullableTime(a.SubmittedAt),
		PublishedAt:           nullableTime(a.PublishedAt),
		CreatedAt:             a.CreatedAt.Time,
		UpdatedAt:             a.UpdatedAt.Time,
	}
	if p.ViewerRole() == repository.UserRoleLender {
		if row.MyOfferID != uuid.Nil { // NULL подзапроса читается как uuid.Nil
			id := row.MyOfferID
			out.MyOfferID = &id
		}
		return out
	}
	out.ModerationComment = nullableText(a.ModerationComment)
	count := row.OffersCount
	out.OffersCount = &count
	return out
}

func applicationAudit(action string, p Principal, id uuid.UUID) AuditEvent {
	return AuditEvent{Action: action, UserID: &p.UserID, EntityType: repository.EntityTypeApplication, EntityID: id.String()}
}

// statusFilter — статусы заявок или офферов для narg statuses (text[]: см. ListApplicationsVisible).
func statusFilter[S ~string](in []S) []string {
	if len(in) == 0 {
		return nil // NULL — фильтр не задан
	}
	out := make([]string, len(in))
	for i, st := range in {
		out[i] = string(st)
	}
	return out
}

// optional* — nil → NULL (narg-фильтр не задан).
func optionalDecimal(d *decimal.Decimal) decimal.NullDecimal {
	if d == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: *d, Valid: true}
}

func optionalInt4(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true} //nolint:gosec // границы — в контракте
}

func optionalTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func optionalUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func optionalBool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

// nullableText, nullableUUID, nullableTime — NULL → nil.
func nullableText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func nullableUUID(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}

func nullableTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
