package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Действия audit_log для офферов (BE-14).
const (
	AuditOfferCreate   = "offer.create"
	AuditOfferReject   = "offer.reject"
	AuditOfferWithdraw = "offer.withdraw"
)

const offerNotFound = "Оффер не найден"

// offersOneActivePerLender — частичный уникальный индекс миграции 00001: один sent-оффер
// компании на заявку. После withdrawn/rejected компания может сделать новый.
const offersOneActivePerLender = "offers_one_active_per_lender_uq"

// Offer — оффер, как его видит конкретный пользователь.
type Offer struct {
	ID                 uuid.UUID
	ApplicationID      uuid.UUID
	ApplicationNumber  int64
	Lender             OrgShort
	Status             OfferStatus
	RejectReason       *string
	RejectComment      *string
	Amount             decimal.Decimal
	Currency           string
	InterestRateAnnual decimal.Decimal
	FeePercent         *decimal.Decimal
	RepaymentSchedule  string
	Collateral         *string
	AdditionalTerms    *string
	ValidUntil         *time.Time
	UnreadMessages     *int64 // nil у admin: он не сторона чата, указателя прочтения нет
	CreatedAt          time.Time
	DecidedAt          *time.Time
}

// CreateOfferInput — новый оффер; формат полей уже проверен валидатором по контракту.
// Валюты нет: оффер всегда в валюте заявки.
type CreateOfferInput struct {
	Amount             decimal.Decimal
	InterestRateAnnual decimal.Decimal
	FeePercent         *decimal.Decimal
	RepaymentSchedule  string
	Collateral         *string
	AdditionalTerms    *string
	ValidUntil         *time.Time
}

// OfferFilter — фильтры списка офферов. nil / пустой срез — фильтр не задан.
// Page и PageSize: 0 — по умолчанию (1 и 20).
type OfferFilter struct {
	Statuses      []OfferStatus
	ApplicationID *uuid.UUID
	LenderOrgID   *uuid.UUID // учитывается только для admin
	BorrowerOrgID *uuid.UUID // учитывается только для admin
	CreatedFrom   *time.Time
	CreatedTo     *time.Time
	Page          int
	PageSize      int
}

// OfferPage — страница списка офферов.
type OfferPage struct {
	Items    []Offer
	Total    int64
	Page     int
	PageSize int
}

// OfferStore — запросы вне транзакции (чтение).
type OfferStore interface {
	GetOfferVisible(ctx context.Context, arg repository.GetOfferVisibleParams) (repository.GetOfferVisibleRow, error)
	ListOffersVisible(ctx context.Context, arg repository.ListOffersVisibleParams) ([]repository.ListOffersVisibleRow, error)
	CountOffersVisible(ctx context.Context, arg repository.CountOffersVisibleParams) (int64, error)
	ListApplicationOffersVisible(ctx context.Context, arg repository.ListApplicationOffersVisibleParams) ([]repository.ListApplicationOffersVisibleRow, error)
	GetApplicationVisible(ctx context.Context, arg repository.GetApplicationVisibleParams) (repository.GetApplicationVisibleRow, error)
}

// OffersDeps — зависимости сервиса офферов. Now — часы для valid_until; nil — time.Now.
type OffersDeps struct {
	Store OfferStore
	Tx    TxRunner
	Now   func() time.Time
}

// Offers — офферы кредиторов на заявки: создание, просмотр, отклонение, отзыв.
type Offers struct {
	store    OfferStore
	tx       TxRunner
	notifier Notifier
	now      func() time.Time
}

// NewOffers создаёт сервис офферов.
func NewOffers(d OffersDeps) *Offers {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &Offers{store: d.Store, tx: d.Tx, now: now}
}

// Get — оффер, если p его видит: автор, заёмщик-владелец заявки или admin; остальным — 404.
func (s *Offers) Get(ctx context.Context, p Principal, id uuid.UUID) (Offer, error) {
	return s.reread(ctx, s.store, p, id)
}

// List — офферы, которые p видит (правило в SQL). Фильтры по организациям — инструмент
// admin: кредитору и заёмщику они ничего бы не открыли, но и смысла не имеют — отбрасываются.
func (s *Offers) List(ctx context.Context, p Principal, f OfferFilter) (OfferPage, error) {
	if p.ViewerRole() != repository.UserRoleAdmin {
		f.LenderOrgID, f.BorrowerOrgID = nil, nil
	}
	page, pageSize := f.Page, f.PageSize
	if page < 1 {
		page = defaultPage
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	count := repository.CountOffersVisibleParams{
		ViewerRole:        p.ViewerRole(),
		ViewerOrg:         p.ViewerOrg(),
		Statuses:          statusFilter(f.Statuses),
		ApplicationID:     optionalUUID(f.ApplicationID),
		LenderOrgFilter:   optionalUUID(f.LenderOrgID),
		BorrowerOrgFilter: optionalUUID(f.BorrowerOrgID),
		CreatedFrom:       optionalTime(f.CreatedFrom),
		CreatedTo:         optionalTime(f.CreatedTo),
	}
	rows, err := s.store.ListOffersVisible(ctx, repository.ListOffersVisibleParams{
		ViewerUser:        p.UserID,
		ViewerRole:        count.ViewerRole,
		ViewerOrg:         count.ViewerOrg,
		Statuses:          count.Statuses,
		ApplicationID:     count.ApplicationID,
		LenderOrgFilter:   count.LenderOrgFilter,
		BorrowerOrgFilter: count.BorrowerOrgFilter,
		CreatedFrom:       count.CreatedFrom,
		CreatedTo:         count.CreatedTo,
		PageLimit:         int32(pageSize),              //nolint:gosec // ≤ 100 по контракту
		PageOffset:        int32((page - 1) * pageSize), //nolint:gosec // page ограничен контрактом
	})
	if err != nil {
		return OfferPage{}, fmt.Errorf("list offers: %w", err)
	}
	total, err := s.store.CountOffersVisible(ctx, count)
	if err != nil {
		return OfferPage{}, fmt.Errorf("count offers: %w", err)
	}
	items := make([]Offer, len(rows))
	for i, row := range rows {
		items[i] = offerFromVisible(p, row)
	}
	return OfferPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// ListByApplication — офферы заявки. Сначала видимость самой заявки (чужая — 404, а не
// пустой список); дальше SQL оставляет кредитору только его оффер (DECISIONS п. 11):
// суммы и ставки конкурентов ему не видны.
func (s *Offers) ListByApplication(ctx context.Context, p Principal, applicationID uuid.UUID, statuses []OfferStatus) ([]Offer, error) {
	if _, err := s.store.GetApplicationVisible(ctx, repository.GetApplicationVisibleParams{
		ID: applicationID, ViewerRole: p.ViewerRole(), ViewerOrg: p.ViewerOrg(),
	}); err != nil {
		return nil, pgerr.Map(err, applicationNotFound)
	}
	rows, err := s.store.ListApplicationOffersVisible(ctx, repository.ListApplicationOffersVisibleParams{
		ApplicationID: applicationID, ViewerRole: p.ViewerRole(), ViewerOrg: p.ViewerOrg(), ViewerUser: p.UserID,
		Statuses: statusFilter(statuses),
	})
	if err != nil {
		return nil, fmt.Errorf("list application offers: %w", err)
	}
	out := make([]Offer, len(rows))
	for i, row := range rows {
		out[i] = offerFromVisible(p, repository.ListOffersVisibleRow(row))
	}
	return out, nil
}

// Create — оффер верифицированного кредитора на опубликованную заявку. В одной транзакции:
// FOR SHARE на заявку (не даёт параллельно отменить или принять её), INSERT, история
// NULL → sent, offer_received заёмщику и аудит.
func (s *Offers) Create(ctx context.Context, p Principal, meta ClientMeta, applicationID uuid.UUID, in CreateOfferInput) (Offer, error) {
	if err := p.RequireRole(repository.UserRoleLender); err != nil {
		return Offer{}, err
	}
	if err := p.RequireKYCVerified(); err != nil {
		return Offer{}, err
	}
	// Просроченный оффер нельзя было бы принять (BE-09 отвечает 409) — не создаём его.
	if in.ValidUntil != nil && !in.ValidUntil.After(s.now()) {
		return Offer{}, apperr.Validation("Срок действия оффера уже истёк",
			apperr.FieldError{Field: "valid_until", Message: "должен быть в будущем"})
	}
	var out Offer
	err := s.tx.InTx(ctx, func(q repository.Querier) error {
		app, err := q.LockApplicationForLender(ctx, repository.LockApplicationForLenderParams{
			ID: applicationID, LenderOrg: p.ViewerOrg(),
		})
		if err != nil {
			return pgerr.Map(err, applicationNotFound)
		}
		// 409 CONFLICT, а не INVALID_STATUS_TRANSITION (DECISIONS п. 6): статус мешает
		// действию, но перехода у самой заявки нет.
		if ApplicationStatus(app.Status) != AppStatusPublished {
			return apperr.Conflict("Заявка не принимает офферы")
		}
		// Инвариант docs/db/schema.md: кредитор не даёт больше, чем просили. Валюты
		// совпадают по построению — оффер берёт валюту заявки.
		if in.Amount.GreaterThan(app.Amount) {
			return apperr.Validation("Сумма оффера больше суммы заявки",
				apperr.FieldError{Field: "amount", Message: "больше суммы заявки"})
		}
		created, err := q.CreateOffer(ctx, repository.CreateOfferParams{
			ApplicationID:        app.ID,
			LenderOrganizationID: p.ViewerOrg(),
			CreatedByUserID:      p.UserID,
			Amount:               in.Amount,
			Currency:             app.Currency,
			InterestRateAnnual:   in.InterestRateAnnual,
			FeePercent:           optionalDecimal(in.FeePercent),
			RepaymentSchedule:    repository.RepaymentSchedule(in.RepaymentSchedule),
			Collateral:           optionalText(in.Collateral),
			AdditionalTerms:      optionalText(in.AdditionalTerms),
			ValidUntil:           optionalTime(in.ValidUntil),
		})
		if err != nil {
			return createOfferErr(err)
		}
		if err := recordStatusChange(ctx, q, StatusChange{
			EntityType: repository.EntityTypeOffer, EntityID: created.ID, To: string(created.Status), Actor: &p,
		}); err != nil {
			return err
		}
		if err := s.notifier.NotifyOrganization(ctx, q, app.BorrowerOrganizationID, offerNotification(
			repository.NotificationTypeOfferReceived, "Новый оффер по заявке "+ApplicationNumber(app.Number.Int64), created.ID)); err != nil {
			return err
		}
		if err := writeAudit(ctx, q, meta, offerAudit(AuditOfferCreate, p, created.ID)); err != nil {
			return err
		}
		out, err = s.reread(ctx, q, p, created.ID)
		return err
	})
	return out, err
}

// Reject — заёмщик-владелец заявки отклоняет sent-оффер (rejected_by_borrower);
// offer_rejected — кредитору.
func (s *Offers) Reject(ctx context.Context, p Principal, meta ClientMeta, id uuid.UUID, comment *string) (Offer, error) {
	d := offerDecision{
		role: repository.UserRoleBorrower, action: OfferReject, audit: AuditOfferReject,
		notice: repository.NotificationTypeOfferRejected, title: "Оффер отклонён: заявка ",
		recipient: func(row repository.GetOfferVisibleRow) uuid.UUID { return row.Offer.LenderOrganizationID },
		update: func(q repository.Querier) error {
			_, err := q.RejectOffer(ctx, repository.RejectOfferParams{ID: id, BorrowerOrg: p.ViewerOrg(), Comment: optionalText(comment)})
			return err
		},
	}
	if comment != nil {
		d.comment = *comment
	}
	return s.decide(ctx, p, meta, id, d)
}

// Withdraw — кредитор-автор отзывает sent-оффер; offer_withdrawn — заёмщику.
func (s *Offers) Withdraw(ctx context.Context, p Principal, meta ClientMeta, id uuid.UUID) (Offer, error) {
	return s.decide(ctx, p, meta, id, offerDecision{
		role: repository.UserRoleLender, action: OfferWithdraw, audit: AuditOfferWithdraw,
		notice: repository.NotificationTypeOfferWithdrawn, title: "Оффер отозван: заявка ",
		recipient: func(row repository.GetOfferVisibleRow) uuid.UUID { return row.BorrowerOrganizationID },
		update: func(q repository.Querier) error {
			_, err := q.WithdrawOffer(ctx, repository.WithdrawOfferParams{ID: id, LenderOrg: p.ViewerOrg()})
			return err
		},
	})
}

// offerDecision — решение одной стороны по sent-офферу: кто вправе, какой переход,
// переходный UPDATE (с условием владельца в SQL) и кого уведомить.
type offerDecision struct {
	role      repository.UserRole
	action    OfferAction
	update    func(q repository.Querier) error
	comment   string // в status_history; "" — без комментария
	notice    repository.NotificationType
	title     string // + номер заявки
	recipient func(row repository.GetOfferVisibleRow) uuid.UUID
	audit     string
}

// decide — общий ход reject и withdraw в одной транзакции. Порядок проверок задаёт коды:
// невидимый оффер — 404 (IDOR), видимый, но не та сторона (admin, вторая сторона) — 403,
// не sent — 409; UPDATE с 0 строк после проверки статуса — гонка, тоже 409.
func (s *Offers) decide(ctx context.Context, p Principal, meta ClientMeta, id uuid.UUID, d offerDecision) (Offer, error) {
	var out Offer
	err := s.tx.InTx(ctx, func(q repository.Querier) error {
		row, err := s.visible(ctx, q, p, id)
		if err != nil {
			return err
		}
		if err := p.RequireRole(d.role); err != nil {
			return err
		}
		from := OfferStatus(row.Offer.Status)
		to, err := NextOfferStatus(from, d.action)
		if err != nil {
			return err
		}
		if err := d.update(q); err != nil {
			return offerTransitionErr(err, from, to)
		}
		if err := recordStatusChange(ctx, q, StatusChange{
			EntityType: repository.EntityTypeOffer, EntityID: id, From: string(from), To: string(to), Actor: &p, Comment: d.comment,
		}); err != nil {
			return err
		}
		if err := s.notifier.NotifyOrganization(ctx, q, d.recipient(row), offerNotification(
			d.notice, d.title+ApplicationNumber(row.ApplicationNumber.Int64), id)); err != nil {
			return err
		}
		if err := writeAudit(ctx, q, meta, offerAudit(d.audit, p, id)); err != nil {
			return err
		}
		out, err = s.reread(ctx, q, p, id)
		return err
	})
	return out, err
}

// offerTransitionErr — ошибка переходного UPDATE … AND status = 'sent'. Оффер уже прочитан
// как видимый и sent, поэтому 0 строк значит, что параллельное решение успело первым, — 409.
func offerTransitionErr(err error, from, to OfferStatus) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.InvalidTransition(string(from), string(to))
	}
	return pgerr.Map(err, offerNotFound)
}

// createOfferErr — ошибка INSERT оффера. Второй активный оффер компании ловит частичный
// уникальный индекс: свой текст вместо общего «Запись уже существует» из pgerr.Map.
func createOfferErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == offersOneActivePerLender {
		return apperr.Wrap(apperr.CodeConflict, "У компании уже есть активный оффер по этой заявке", err)
	}
	return pgerr.Map(err, applicationNotFound)
}

// visible — строка оффера, если p его видит; чужой и несуществующий — одинаково 404 (IDOR).
func (s *Offers) visible(ctx context.Context, q OfferStore, p Principal, id uuid.UUID) (repository.GetOfferVisibleRow, error) {
	row, err := q.GetOfferVisible(ctx, repository.GetOfferVisibleParams{
		ID: id, ViewerRole: p.ViewerRole(), ViewerOrg: p.ViewerOrg(), ViewerUser: p.UserID,
	})
	if err != nil {
		return repository.GetOfferVisibleRow{}, pgerr.Map(err, offerNotFound)
	}
	return row, nil
}

// reread — оффер глазами p на том же q: ответ мутации собирается той же конвертацией, что и Get.
func (s *Offers) reread(ctx context.Context, q OfferStore, p Principal, id uuid.UUID) (Offer, error) {
	row, err := s.visible(ctx, q, p, id)
	if err != nil {
		return Offer{}, err
	}
	return offerFromVisible(p, repository.ListOffersVisibleRow(row)), nil
}

// offerFromVisible — единственная конвертация строки *OffersVisible в Offer.
func offerFromVisible(p Principal, row repository.ListOffersVisibleRow) Offer {
	o := row.Offer
	out := Offer{
		ID:                 o.ID,
		ApplicationID:      o.ApplicationID,
		ApplicationNumber:  row.ApplicationNumber.Int64,
		Lender:             OrgShort{ID: o.LenderOrganizationID, Name: row.LenderName, CountryCode: row.LenderCountryCode},
		Status:             OfferStatus(o.Status),
		RejectComment:      nullableText(o.RejectComment),
		Amount:             o.Amount,
		Currency:           o.Currency,
		InterestRateAnnual: o.InterestRateAnnual,
		RepaymentSchedule:  string(o.RepaymentSchedule),
		Collateral:         nullableText(o.Collateral),
		AdditionalTerms:    nullableText(o.AdditionalTerms),
		ValidUntil:         nullableTime(o.ValidUntil),
		CreatedAt:          o.CreatedAt.Time,
		DecidedAt:          nullableTime(o.DecidedAt),
	}
	if o.RejectReason.Valid {
		reason := string(o.RejectReason.OfferRejectReason)
		out.RejectReason = &reason
	}
	if o.FeePercent.Valid {
		fee := o.FeePercent.Decimal
		out.FeePercent = &fee
	}
	if p.ViewerRole() != repository.UserRoleAdmin {
		unread := row.UnreadMessages
		out.UnreadMessages = &unread
	}
	return out
}

func offerNotification(t repository.NotificationType, title string, id uuid.UUID) Notification {
	return Notification{Type: t, Title: title, EntityType: repository.EntityTypeOffer, EntityID: id.String()}
}

func offerAudit(action string, p Principal, id uuid.UUID) AuditEvent {
	return AuditEvent{Action: action, UserID: &p.UserID, EntityType: repository.EntityTypeOffer, EntityID: id.String()}
}
