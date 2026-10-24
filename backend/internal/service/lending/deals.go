package lending

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/pgconv"
)

// AuditOfferAccept — действие audit_log принятия оффера; в meta — deal_id (BE-14).
const AuditOfferAccept = "offer.accept"

const dealNotFound = "Сделка не найдена"

// Уникальные страховки миграции 00001 от второй сделки по заявке: один accepted-оффер на
// заявку, одна сделка на заявку и на оффер. При блокировке заявки не срабатывают.
var acceptUniqueConstraints = []string{"offers_one_accepted_uq", "deals_application_id_key", "deals_offer_id_key"}

// Deal — сделка: снапшот условий принятого оффера, не меняется после создания
// (у lp_app на deals только SELECT, INSERT).
type Deal struct {
	ID                 uuid.UUID
	Number             int64
	ApplicationID      uuid.UUID
	OfferID            uuid.UUID
	Borrower           OrgShort
	Lender             OrgShort
	Amount             decimal.Decimal
	Currency           string
	InterestRateAnnual decimal.Decimal
	FeePercent         *decimal.Decimal
	FeeAmount          *decimal.Decimal
	TermMonths         int
	TargetCountryCode  string
	TransferMethod     string
	RepaymentSchedule  string
	Collateral         *string
	AdditionalTerms    *string
	AgreedAt           time.Time
}

// DealFilter — фильтры списка сделок. nil — фильтр не задан. Период — по agreed_at.
// Page и PageSize: 0 — по умолчанию (1 и 20).
type DealFilter struct {
	BorrowerOrgID *uuid.UUID // учитывается только для admin
	LenderOrgID   *uuid.UUID // учитывается только для admin
	CreatedFrom   *time.Time
	CreatedTo     *time.Time
	Page          int
	PageSize      int
}

// DealPage — страница списка сделок.
type DealPage struct {
	Items    []Deal
	Total    int64
	Page     int
	PageSize int
}

// DealStore — запросы вне транзакции (чтение).
type DealStore interface {
	GetDealVisible(ctx context.Context, arg repository.GetDealVisibleParams) (repository.GetDealVisibleRow, error)
	ListDealsVisible(ctx context.Context, arg repository.ListDealsVisibleParams) ([]repository.ListDealsVisibleRow, error)
	CountDealsVisible(ctx context.Context, arg repository.CountDealsVisibleParams) (int64, error)
}

// DealsDeps — зависимости сервиса сделок. Now — часы для valid_until; nil — time.Now.
type DealsDeps struct {
	Store DealStore
	Tx    TxRunner
	Now   func() time.Time
}

// Deals — сделки и принятие оффера: операция нескольких доменов живёт в домене-результате.
type Deals struct {
	store    DealStore
	tx       TxRunner
	notifier service.Notifier
	now      func() time.Time
}

// NewDeals создаёт сервис сделок.
func NewDeals(d DealsDeps) *Deals {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &Deals{store: d.Store, tx: d.Tx, now: now}
}

// Get — сделка, если p её сторона или admin; остальным — 404.
func (s *Deals) Get(ctx context.Context, p service.Principal, id uuid.UUID) (Deal, error) {
	return s.reread(ctx, s.store, p, id)
}

// List — сделки, которые p видит (правило в SQL). Фильтры по организациям — инструмент
// admin: сторонам они ничего бы не открыли, но и смысла не имеют — отбрасываются.
func (s *Deals) List(ctx context.Context, p service.Principal, f DealFilter) (DealPage, error) {
	if p.ViewerRole() != repository.UserRoleAdmin {
		f.BorrowerOrgID, f.LenderOrgID = nil, nil
	}
	page, pageSize, limit, offset := service.PageWindow(f.Page, f.PageSize)
	count := repository.CountDealsVisibleParams{
		ViewerRole:        p.ViewerRole(),
		ViewerOrg:         p.ViewerOrg(),
		BorrowerOrgFilter: pgconv.UUID(f.BorrowerOrgID),
		LenderOrgFilter:   pgconv.UUID(f.LenderOrgID),
		CreatedFrom:       pgconv.Timestamptz(f.CreatedFrom),
		CreatedTo:         pgconv.Timestamptz(f.CreatedTo),
	}
	rows, err := s.store.ListDealsVisible(ctx, repository.ListDealsVisibleParams{
		ViewerRole:        count.ViewerRole,
		ViewerOrg:         count.ViewerOrg,
		BorrowerOrgFilter: count.BorrowerOrgFilter,
		LenderOrgFilter:   count.LenderOrgFilter,
		CreatedFrom:       count.CreatedFrom,
		CreatedTo:         count.CreatedTo,
		PageLimit:         limit,
		PageOffset:        offset,
	})
	if err != nil {
		return DealPage{}, fmt.Errorf("list deals: %w", err)
	}
	total, err := s.store.CountDealsVisible(ctx, count)
	if err != nil {
		return DealPage{}, fmt.Errorf("count deals: %w", err)
	}
	items := make([]Deal, len(rows))
	for i, row := range rows {
		items[i] = dealFromVisible(row)
	}
	return DealPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// AcceptOffer — заёмщик-владелец заявки принимает sent-оффер. Одна транзакция: оффер
// accepted, остальные sent → rejected (another_offer_accepted), заявка agreed, сделка-снапшот,
// история, уведомления кредиторам, аудит.
//
// Порядок блокировок фиксирован: сначала FOR UPDATE на заявку, потом офферы — тот же, что
// у cancel/reject-offers и createOffer (FOR SHARE), иначе параллельные операции ловят
// deadlock. Статус заявки проверяется уже после блокировки: в READ COMMITTED вторая
// параллельная транзакция дожидается первой и видит новую версию строки (agreed) — 409,
// и вторая сделка не появляется.
func (s *Deals) AcceptOffer(ctx context.Context, p service.Principal, meta service.ClientMeta, offerID uuid.UUID) (Deal, error) {
	if err := p.RequireRole(repository.UserRoleBorrower); err != nil {
		return Deal{}, err
	}
	if err := p.RequireKYCVerified(); err != nil {
		return Deal{}, err
	}
	var out Deal
	err := s.tx.InTx(ctx, func(q repository.Querier) error {
		// Без фильтра статуса: 0 строк — только «оффера нет или заявка чужая» (404).
		app, err := q.LockApplicationByOfferForBorrower(ctx, repository.LockApplicationByOfferForBorrowerParams{
			OfferID: offerID, BorrowerOrg: p.ViewerOrg(),
		})
		if err != nil {
			return pgerr.Map(err, offerNotFound)
		}
		appFrom := ApplicationStatus(app.Status)
		appTo, err := NextApplicationStatus(appFrom, AppAgree)
		if err != nil {
			return err
		}
		offer, err := q.LockOfferForAccept(ctx, repository.LockOfferForAcceptParams{ID: offerID, ApplicationID: app.ID})
		if err != nil {
			return pgerr.Map(err, offerNotFound)
		}
		offerFrom := OfferStatus(offer.Status)
		offerTo, err := NextOfferStatus(offerFrom, OfferAccept)
		if err != nil {
			return err
		}
		// Статуса expired нет: просроченный оффер остаётся sent, но принять его нельзя —
		// перехода в accepted больше нет, отсюда 409, а не 400 (DECISIONS п. 6).
		if offer.ValidUntil.Valid && offer.ValidUntil.Time.Before(s.now()) {
			return apperr.New(apperr.CodeInvalidTransition, "Срок действия оффера истёк")
		}
		// Снапшот — из строки, которую UPDATE перевёл в accepted: сделка неизменна и должна
		// повторять именно принятую версию оффера.
		accepted, err := q.AcceptOffer(ctx, repository.AcceptOfferParams{ID: offerID, ApplicationID: app.ID})
		if err != nil {
			return acceptErr(offerTransitionErr(err, offerFrom, offerTo), err)
		}
		if _, err := rejectSentOffers(ctx, q, s.notifier, p, app, repository.OfferRejectReasonAnotherOfferAccepted); err != nil {
			return err
		}
		if _, err := q.AgreeApplication(ctx, repository.AgreeApplicationParams{
			ID: app.ID, AcceptedOfferID: pgconv.UUID(&offerID), FromStatus: app.Status,
		}); err != nil {
			return transitionErr(err, appFrom, appTo)
		}
		deal, err := q.CreateDeal(ctx, dealSnapshot(app, accepted))
		if err != nil {
			return acceptErr(fmt.Errorf("create deal: %w", err), err)
		}
		for _, c := range []service.StatusChange{
			{EntityType: repository.EntityTypeOffer, EntityID: offerID, From: string(offerFrom), To: string(offerTo), Actor: &p},
			{EntityType: repository.EntityTypeApplication, EntityID: app.ID, From: string(appFrom), To: string(appTo), Actor: &p},
		} {
			if err := service.RecordStatusChange(ctx, q, c); err != nil {
				return err
			}
		}
		// Заёмщику уведомления нет (DECISIONS п. 14): сделку он получает в ответе.
		if err := s.notifier.NotifyOrganization(ctx, q, offer.LenderOrganizationID, service.Notification{
			Type: repository.NotificationTypeOfferAccepted, Title: "Оффер принят: сделка " + DealNumber(deal.Number.Int64),
			EntityType: repository.EntityTypeDeal, EntityID: deal.ID.String(),
		}); err != nil {
			return err
		}
		audit := offerAudit(AuditOfferAccept, p, offerID)
		audit.Meta = map[string]any{"deal_id": deal.ID.String()}
		if err := service.WriteAudit(ctx, q, meta, audit); err != nil {
			return err
		}
		out, err = s.reread(ctx, q, p, deal.ID)
		return err
	})
	return out, err
}

// acceptErr — сработавшая уникальная страховка от второй сделки отвечает так же, как
// проигранная гонка (409 INVALID_STATUS_TRANSITION), а не CONFLICT из pgerr.Map; прочие
// ошибки — fallback.
func acceptErr(fallback, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && slices.Contains(acceptUniqueConstraints, pgErr.ConstraintName) {
		return apperr.Wrap(apperr.CodeInvalidTransition, "Оффер по этой заявке уже принят", err)
	}
	return fallback
}

// dealSnapshot — условия сделки: финансовые — из оффера (сумма оффера может быть меньше
// суммы заявки), срок, страна и способ перевода — из заявки.
func dealSnapshot(app repository.Application, offer repository.Offer) repository.CreateDealParams {
	return repository.CreateDealParams{
		ApplicationID:          app.ID,
		OfferID:                offer.ID,
		BorrowerOrganizationID: app.BorrowerOrganizationID,
		LenderOrganizationID:   offer.LenderOrganizationID,
		Amount:                 offer.Amount,
		Currency:               offer.Currency,
		InterestRateAnnual:     offer.InterestRateAnnual,
		FeePercent:             offer.FeePercent,
		FeeAmount:              dealFee(offer.Amount, offer.FeePercent),
		TermMonths:             app.TermMonths,
		TargetCountryCode:      app.TargetCountryCode,
		TransferMethod:         app.TransferMethod,
		RepaymentSchedule:      offer.RepaymentSchedule,
		Collateral:             offer.Collateral,
		AdditionalTerms:        offer.AdditionalTerms,
	}
}

// reread — сделка глазами p на том же q: ответ собирается той же конвертацией, что и Get.
func (s *Deals) reread(ctx context.Context, q DealStore, p service.Principal, id uuid.UUID) (Deal, error) {
	row, err := q.GetDealVisible(ctx, repository.GetDealVisibleParams{
		ID: id, ViewerRole: p.ViewerRole(), ViewerOrg: p.ViewerOrg(),
	})
	if err != nil {
		return Deal{}, pgerr.Map(err, dealNotFound)
	}
	return dealFromVisible(repository.ListDealsVisibleRow(row)), nil
}

// dealFromVisible — единственная конвертация строки *DealsVisible в Deal.
func dealFromVisible(row repository.ListDealsVisibleRow) Deal {
	d := row.Deal
	out := Deal{
		ID:                 d.ID,
		Number:             d.Number.Int64,
		ApplicationID:      d.ApplicationID,
		OfferID:            d.OfferID,
		Borrower:           OrgShort{ID: d.BorrowerOrganizationID, Name: row.BorrowerName, CountryCode: row.BorrowerCountryCode},
		Lender:             OrgShort{ID: d.LenderOrganizationID, Name: row.LenderName, CountryCode: row.LenderCountryCode},
		Amount:             d.Amount,
		Currency:           d.Currency,
		InterestRateAnnual: d.InterestRateAnnual,
		TermMonths:         int(d.TermMonths),
		TargetCountryCode:  d.TargetCountryCode,
		TransferMethod:     d.TransferMethod,
		RepaymentSchedule:  string(d.RepaymentSchedule),
		Collateral:         pgconv.TextPtr(d.Collateral),
		AdditionalTerms:    pgconv.TextPtr(d.AdditionalTerms),
		AgreedAt:           d.AgreedAt.Time,
	}
	if d.FeePercent.Valid {
		fee := d.FeePercent.Decimal
		out.FeePercent = &fee
	}
	if d.FeeAmount.Valid {
		fee := d.FeeAmount.Decimal
		out.FeeAmount = &fee
	}
	return out
}

// DealNumber — номер сделки для людей: DEAL-000045. Единственное место формата: им
// пользуются хендлер и заголовок уведомления offer_accepted.
func DealNumber(n int64) string {
	return fmt.Sprintf("DEAL-%06d", n)
}

// dealFee — комиссия сделки: fee_amount = round(amount × fee_percent / 100, 2).
// Shift(-2) делит на 100 без потери точности; Round у shopspring округляет половину от
// нуля, как round(numeric) в Postgres: 0.50 × 1 % = 0.005 → 0.01. Без процента комиссии
// нет и суммы — deals_fee_chk требует fee_amount IS NULL ⇔ fee_percent IS NULL.
func dealFee(amount decimal.Decimal, feePercent decimal.NullDecimal) decimal.NullDecimal {
	if !feePercent.Valid {
		return decimal.NullDecimal{}
	}
	return decimal.NewNullDecimal(amount.Mul(feePercent.Decimal).Shift(-2).Round(2))
}
