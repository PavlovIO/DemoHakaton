package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// Офферные запросы fakeAppStore (applications_test.go): та же память заявок и офферов,
// видимость повторяет offers.sql.

// testNow — часы сервиса офферов в тестах: valid_until сравнивается с ними.
var testNow = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func newOffers(s *fakeAppStore) *service.Offers {
	return service.NewOffers(service.OffersDeps{Store: s, Tx: fakeAppTx{s}, Now: func() time.Time { return testNow }})
}

func validOfferInput() service.CreateOfferInput {
	return service.CreateOfferInput{
		Amount: decimal.RequireFromString("100000.00"), InterestRateAnnual: decimal.RequireFromString("8.00"),
		RepaymentSchedule: "annuity",
	}
}

func TestOffersCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("borrower — FORBIDDEN", func(t *testing.T) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		appID := s.app(borrower, repository.ApplicationStatusPublished)
		_, err := newOffers(s).Create(ctx, borrower, service.ClientMeta{}, appID, validOfferInput())
		wantCode(t, err, apperr.CodeForbidden)
		if len(s.offers) != 0 {
			t.Fatalf("оффер создан: %+v", s.offers)
		}
	})

	t.Run("lender under_review — KYC_NOT_VERIFIED", func(t *testing.T) {
		s := newFakeAppStore()
		appID := s.app(s.org(repository.UserRoleBorrower), repository.ApplicationStatusPublished)
		lender := s.org(repository.UserRoleLender)
		lender.KycStatus = string(repository.KycStatusUnderReview)
		_, err := newOffers(s).Create(ctx, lender, service.ClientMeta{}, appID, validOfferInput())
		wantCode(t, err, apperr.CodeKYCNotVerified)
		if len(s.offers) != 0 {
			t.Fatalf("оффер создан: %+v", s.offers)
		}
	})

	t.Run("заявка moderation с моим старым оффером — CONFLICT", func(t *testing.T) {
		s := newFakeAppStore()
		appID := s.app(s.org(repository.UserRoleBorrower), repository.ApplicationStatusModeration)
		lender := s.org(repository.UserRoleLender)
		s.offer(appID, lender, repository.OfferStatusWithdrawn) // заявка видна кредитору
		_, err := newOffers(s).Create(ctx, lender, service.ClientMeta{}, appID, validOfferInput())
		wantCode(t, err, apperr.CodeConflict)
		if len(s.offers) != 1 {
			t.Fatalf("оффер создан: %+v", s.offers)
		}
	})

	t.Run("невидимая заявка — NOT_FOUND", func(t *testing.T) {
		s := newFakeAppStore()
		appID := s.app(s.org(repository.UserRoleBorrower), repository.ApplicationStatusDraft)
		_, err := newOffers(s).Create(ctx, s.org(repository.UserRoleLender), service.ClientMeta{}, appID, validOfferInput())
		wantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("сумма больше суммы заявки на копейку — VALIDATION_ERROR amount", func(t *testing.T) {
		s := newFakeAppStore()
		appID := s.app(s.org(repository.UserRoleBorrower), repository.ApplicationStatusPublished) // 500000.00
		in := validOfferInput()
		in.Amount = decimal.RequireFromString("500000.01")
		_, err := newOffers(s).Create(ctx, s.org(repository.UserRoleLender), service.ClientMeta{}, appID, in)
		wantValidationField(t, err, "amount")
		if len(s.offers) != 0 {
			t.Fatalf("оффер создан: %+v", s.offers)
		}
	})

	t.Run("valid_until в прошлом — VALIDATION_ERROR valid_until", func(t *testing.T) {
		s := newFakeAppStore()
		appID := s.app(s.org(repository.UserRoleBorrower), repository.ApplicationStatusPublished)
		in := validOfferInput()
		in.ValidUntil = ptr(testNow.Add(-time.Second))
		_, err := newOffers(s).Create(ctx, s.org(repository.UserRoleLender), service.ClientMeta{}, appID, in)
		wantValidationField(t, err, "valid_until")
	})

	t.Run("сумма = сумме заявки — sent в валюте заявки, история, уведомление, аудит", func(t *testing.T) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		appID := s.app(borrower, repository.ApplicationStatusPublished) // 500000.00 EUR, APP-000123
		lender := s.org(repository.UserRoleLender)
		in := validOfferInput()
		in.Amount = decimal.RequireFromString("500000.00")
		in.ValidUntil = ptr(testNow.Add(time.Hour))
		got, err := newOffers(s).Create(ctx, lender, service.ClientMeta{}, appID, in)
		if err != nil {
			t.Fatal(err)
		}
		if s.createArg == nil || s.createArg.Currency != "EUR" || s.createArg.LenderOrganizationID != *lender.OrgID ||
			s.createArg.CreatedByUserID != lender.UserID || !s.createArg.ValidUntil.Valid {
			t.Fatalf("create arg = %+v", s.createArg)
		}
		if got.Status != service.OfferStatusSent || got.Currency != "EUR" || got.ApplicationNumber != 123 ||
			got.Lender.ID != *lender.OrgID || got.Lender.Name != "ООО lender" {
			t.Fatalf("got %+v", got)
		}
		if len(s.history) != 1 {
			t.Fatalf("history = %+v, want 1", s.history)
		}
		h := s.history[0]
		if h.EntityType != repository.EntityTypeOffer || h.EntityID != got.ID || h.FromStatus.Valid || h.ToStatus != "sent" {
			t.Fatalf("history = %+v", h)
		}
		if len(s.orgNotices) != 1 || s.orgNotices[0].OrganizationID != *borrower.OrgID ||
			s.orgNotices[0].Type != repository.NotificationTypeOfferReceived || s.orgNotices[0].EntityID != got.ID.String() {
			t.Fatalf("notices = %+v", s.orgNotices)
		}
		if len(s.audit) != 1 || s.audit[0].Action != service.AuditOfferCreate {
			t.Fatalf("audit = %+v", s.audit)
		}
	})

	t.Run("второй активный оффер компании — CONFLICT с текстом про активный оффер", func(t *testing.T) {
		s := newFakeAppStore()
		appID := s.app(s.org(repository.UserRoleBorrower), repository.ApplicationStatusPublished)
		s.createErr = &pgconn.PgError{Code: "23505", ConstraintName: "offers_one_active_per_lender_uq"}
		_, err := newOffers(s).Create(ctx, s.org(repository.UserRoleLender), service.ClientMeta{}, appID, validOfferInput())
		wantCode(t, err, apperr.CodeConflict)
		if e, _ := apperr.As(err); e.Message != "У компании уже есть активный оффер по этой заявке" {
			t.Fatalf("message = %q", e.Message)
		}
		if len(s.history) != 0 || len(s.orgNotices) != 0 {
			t.Fatalf("побочные эффекты после ошибки: %+v %+v", s.history, s.orgNotices)
		}
	})
}

func (s *fakeAppStore) CreateOffer(_ context.Context, arg repository.CreateOfferParams) (repository.Offer, error) {
	s.createArg = &arg
	if s.createErr != nil {
		return repository.Offer{}, s.createErr
	}
	o := repository.Offer{
		ID: uuid.New(), ApplicationID: arg.ApplicationID, LenderOrganizationID: arg.LenderOrganizationID,
		CreatedByUserID: arg.CreatedByUserID, Status: repository.OfferStatusSent, Amount: arg.Amount,
		Currency: arg.Currency, InterestRateAnnual: arg.InterestRateAnnual, RepaymentSchedule: arg.RepaymentSchedule,
		ValidUntil: arg.ValidUntil,
	}
	s.offers = append(s.offers, o)
	return o, nil
}

// offerVisible — правило видимости GetOfferVisible: admin — все, lender — свои, borrower — на свои заявки.
func (s *fakeAppStore) offerVisible(o repository.Offer, role repository.UserRole, org uuid.UUID) bool {
	switch role {
	case repository.UserRoleAdmin:
		return true
	case repository.UserRoleLender:
		return o.LenderOrganizationID == org
	case repository.UserRoleBorrower:
		return s.apps[o.ApplicationID].BorrowerOrganizationID == org
	}
	return false
}

func (s *fakeAppStore) offerRow(o repository.Offer) repository.ListOffersVisibleRow {
	a := s.apps[o.ApplicationID]
	lender := s.orgs[o.LenderOrganizationID]
	return repository.ListOffersVisibleRow{
		Offer: o, ApplicationNumber: a.Number, BorrowerOrganizationID: a.BorrowerOrganizationID,
		LenderName: lender.Name, LenderCountryCode: lender.CountryCode, UnreadMessages: s.unread,
	}
}

func (s *fakeAppStore) GetOfferVisible(_ context.Context, arg repository.GetOfferVisibleParams) (repository.GetOfferVisibleRow, error) {
	s.calls = append(s.calls, "GetOfferVisible")
	for _, o := range s.offers {
		if o.ID == arg.ID && s.offerVisible(o, arg.ViewerRole, arg.ViewerOrg) {
			return repository.GetOfferVisibleRow(s.offerRow(o)), nil
		}
	}
	return repository.GetOfferVisibleRow{}, pgx.ErrNoRows
}

// LockApplicationForLender — видимость заявки для lender, как в offers.sql.
func (s *fakeAppStore) LockApplicationForLender(_ context.Context, arg repository.LockApplicationForLenderParams) (repository.LockApplicationForLenderRow, error) {
	a, ok := s.apps[arg.ID]
	if !ok || !s.visible(a, repository.UserRoleLender, arg.LenderOrg) {
		return repository.LockApplicationForLenderRow{}, pgx.ErrNoRows
	}
	return repository.LockApplicationForLenderRow{
		ID: a.ID, Number: a.Number, Status: a.Status, Amount: a.Amount, Currency: a.Currency,
		BorrowerOrganizationID: a.BorrowerOrganizationID,
	}, nil
}

func TestOffersReject(t *testing.T) {
	ctx := context.Background()
	admin := service.Principal{UserID: uuid.New(), Role: string(repository.UserRoleAdmin)}

	setup := func(status repository.OfferStatus) (*fakeAppStore, service.Principal, service.Principal, uuid.UUID) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		lender := s.org(repository.UserRoleLender)
		id := s.offer(s.app(borrower, repository.ApplicationStatusPublished), lender, status)
		return s, borrower, lender, id
	}

	t.Run("admin и lender видят оффер, но — FORBIDDEN", func(t *testing.T) {
		s, _, lender, id := setup(repository.OfferStatusSent)
		for _, p := range []service.Principal{admin, lender} {
			_, err := newOffers(s).Reject(ctx, p, service.ClientMeta{}, id, nil)
			wantCode(t, err, apperr.CodeForbidden)
		}
		if s.offers[0].Status != repository.OfferStatusSent {
			t.Fatalf("status = %s", s.offers[0].Status)
		}
	})

	t.Run("чужой заёмщик — NOT_FOUND", func(t *testing.T) {
		s, _, _, id := setup(repository.OfferStatusSent)
		_, err := newOffers(s).Reject(ctx, s.org(repository.UserRoleBorrower), service.ClientMeta{}, id, nil)
		wantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("из withdrawn — INVALID_STATUS_TRANSITION", func(t *testing.T) {
		s, borrower, _, id := setup(repository.OfferStatusWithdrawn)
		_, err := newOffers(s).Reject(ctx, borrower, service.ClientMeta{}, id, nil)
		wantCode(t, err, apperr.CodeInvalidTransition)
	})

	t.Run("UPDATE не нашёл sent (гонка) — INVALID_STATUS_TRANSITION", func(t *testing.T) {
		s, borrower, _, id := setup(repository.OfferStatusSent)
		s.lostRace = true
		_, err := newOffers(s).Reject(ctx, borrower, service.ClientMeta{}, id, nil)
		wantCode(t, err, apperr.CodeInvalidTransition)
		if len(s.history) != 0 {
			t.Fatalf("history = %+v", s.history)
		}
	})

	t.Run("владелец — rejected, история с comment, offer_rejected кредитору, аудит", func(t *testing.T) {
		s, borrower, lender, id := setup(repository.OfferStatusSent)
		got, err := newOffers(s).Reject(ctx, borrower, service.ClientMeta{}, id, ptr("дорого"))
		if err != nil {
			t.Fatal(err)
		}
		if s.rejectOfferArg == nil || s.rejectOfferArg.BorrowerOrg != *borrower.OrgID || s.rejectOfferArg.Comment.String != "дорого" {
			t.Fatalf("reject arg = %+v", s.rejectOfferArg)
		}
		if got.Status != service.OfferStatusRejected || got.RejectComment == nil || *got.RejectComment != "дорого" {
			t.Fatalf("got %+v", got)
		}
		if len(s.history) != 1 || s.history[0].FromStatus.String != "sent" || s.history[0].ToStatus != "rejected" ||
			s.history[0].Comment.String != "дорого" {
			t.Fatalf("history = %+v", s.history)
		}
		if len(s.orgNotices) != 1 || s.orgNotices[0].OrganizationID != *lender.OrgID ||
			s.orgNotices[0].Type != repository.NotificationTypeOfferRejected {
			t.Fatalf("notices = %+v", s.orgNotices)
		}
		if len(s.audit) != 1 || s.audit[0].Action != service.AuditOfferReject {
			t.Fatalf("audit = %+v", s.audit)
		}
	})
}

// decide — переходный UPDATE оффера: строка находится, только если она sent и проходит
// условие владельца (owns); lostRace — «параллельный» запрос успел первым.
func (s *fakeAppStore) decide(id uuid.UUID, owns func(repository.Offer) bool, apply func(*repository.Offer)) (repository.Offer, error) {
	for i := range s.offers {
		o := &s.offers[i]
		if o.ID != id || o.Status != repository.OfferStatusSent || !owns(*o) || s.lostRace {
			continue
		}
		apply(o)
		o.DecidedAt = pgtype.Timestamptz{Time: testNow, Valid: true}
		return *o, nil
	}
	return repository.Offer{}, pgx.ErrNoRows
}

func (s *fakeAppStore) RejectOffer(_ context.Context, arg repository.RejectOfferParams) (repository.Offer, error) {
	s.rejectOfferArg = &arg
	return s.decide(arg.ID, func(o repository.Offer) bool {
		return s.apps[o.ApplicationID].BorrowerOrganizationID == arg.BorrowerOrg
	}, func(o *repository.Offer) {
		o.Status = repository.OfferStatusRejected
		o.RejectReason = repository.NullOfferRejectReason{OfferRejectReason: repository.OfferRejectReasonRejectedByBorrower, Valid: true}
		o.RejectComment = arg.Comment
	})
}

func TestOffersWithdraw(t *testing.T) {
	ctx := context.Background()

	setup := func(status repository.OfferStatus) (*fakeAppStore, service.Principal, service.Principal, uuid.UUID) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		lender := s.org(repository.UserRoleLender)
		id := s.offer(s.app(borrower, repository.ApplicationStatusPublished), lender, status)
		return s, borrower, lender, id
	}

	t.Run("заёмщик-владелец — FORBIDDEN", func(t *testing.T) {
		s, borrower, _, id := setup(repository.OfferStatusSent)
		_, err := newOffers(s).Withdraw(ctx, borrower, service.ClientMeta{}, id)
		wantCode(t, err, apperr.CodeForbidden)
	})

	t.Run("чужой кредитор — NOT_FOUND", func(t *testing.T) {
		s, _, _, id := setup(repository.OfferStatusSent)
		_, err := newOffers(s).Withdraw(ctx, s.org(repository.UserRoleLender), service.ClientMeta{}, id)
		wantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("из accepted — INVALID_STATUS_TRANSITION", func(t *testing.T) {
		s, _, lender, id := setup(repository.OfferStatusAccepted)
		_, err := newOffers(s).Withdraw(ctx, lender, service.ClientMeta{}, id)
		wantCode(t, err, apperr.CodeInvalidTransition)
	})

	t.Run("автор — withdrawn, история, offer_withdrawn заёмщику, аудит", func(t *testing.T) {
		s, borrower, lender, id := setup(repository.OfferStatusSent)
		got, err := newOffers(s).Withdraw(ctx, lender, service.ClientMeta{}, id)
		if err != nil {
			t.Fatal(err)
		}
		if s.withdrawArg == nil || s.withdrawArg.LenderOrg != *lender.OrgID {
			t.Fatalf("withdraw arg = %+v", s.withdrawArg)
		}
		if got.Status != service.OfferStatusWithdrawn || got.DecidedAt == nil {
			t.Fatalf("got %+v", got)
		}
		if len(s.history) != 1 || s.history[0].FromStatus.String != "sent" || s.history[0].ToStatus != "withdrawn" {
			t.Fatalf("history = %+v", s.history)
		}
		if len(s.orgNotices) != 1 || s.orgNotices[0].OrganizationID != *borrower.OrgID ||
			s.orgNotices[0].Type != repository.NotificationTypeOfferWithdrawn {
			t.Fatalf("notices = %+v", s.orgNotices)
		}
		if len(s.audit) != 1 || s.audit[0].Action != service.AuditOfferWithdraw {
			t.Fatalf("audit = %+v", s.audit)
		}
	})
}

func (s *fakeAppStore) WithdrawOffer(_ context.Context, arg repository.WithdrawOfferParams) (repository.Offer, error) {
	s.withdrawArg = &arg
	return s.decide(arg.ID, func(o repository.Offer) bool { return o.LenderOrganizationID == arg.LenderOrg },
		func(o *repository.Offer) { o.Status = repository.OfferStatusWithdrawn })
}

func TestOffersGet(t *testing.T) {
	ctx := context.Background()
	s := newFakeAppStore()
	s.unread = 2
	borrower := s.org(repository.UserRoleBorrower)
	lender := s.org(repository.UserRoleLender)
	id := s.offer(s.app(borrower, repository.ApplicationStatusPublished), lender, repository.OfferStatusSent)

	t.Run("lender — unread_messages из строки", func(t *testing.T) {
		got, err := newOffers(s).Get(ctx, lender, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.UnreadMessages == nil || *got.UnreadMessages != 2 || got.Lender.Name != "ООО lender" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("admin — unread_messages не заполняется", func(t *testing.T) {
		got, err := newOffers(s).Get(ctx, service.Principal{UserID: uuid.New(), Role: string(repository.UserRoleAdmin)}, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.UnreadMessages != nil {
			t.Fatalf("unread = %d, want nil", *got.UnreadMessages)
		}
	})

	t.Run("чужой кредитор — NOT_FOUND", func(t *testing.T) {
		_, err := newOffers(s).Get(ctx, s.org(repository.UserRoleLender), id)
		wantCode(t, err, apperr.CodeNotFound)
	})
}

func TestOffersList(t *testing.T) {
	ctx := context.Background()

	t.Run("borrower: чужие фильтры организаций — NULL, пагинация по умолчанию", func(t *testing.T) {
		s := newFakeAppStore()
		p := s.org(repository.UserRoleBorrower)
		other := uuid.New()
		page, err := newOffers(s).List(ctx, p, service.OfferFilter{
			LenderOrgID: &other, BorrowerOrgID: &other,
			Statuses: []service.OfferStatus{service.OfferStatusSent, service.OfferStatusAccepted},
		})
		if err != nil {
			t.Fatal(err)
		}
		a := s.listOffersArg
		if a.ViewerRole != repository.UserRoleBorrower || a.ViewerOrg != *p.OrgID || a.ViewerUser != p.UserID ||
			a.LenderOrgFilter.Valid || a.BorrowerOrgFilter.Valid {
			t.Fatalf("list arg = %+v", a)
		}
		if len(a.Statuses) != 2 || a.Statuses[1] != "accepted" {
			t.Fatalf("statuses = %v", a.Statuses)
		}
		if s.countOffersArg.ViewerOrg != *p.OrgID || s.countOffersArg.LenderOrgFilter.Valid || len(s.countOffersArg.Statuses) != 2 {
			t.Fatalf("count arg = %+v", s.countOffersArg)
		}
		if page.Page != 1 || page.PageSize != 20 || a.PageLimit != 20 || a.PageOffset != 0 || page.Items == nil {
			t.Fatalf("page = %+v, arg = %+v", page, a)
		}
	})

	t.Run("admin: фильтры организаций и страница доходят до запроса", func(t *testing.T) {
		s := newFakeAppStore()
		lenderOrg, borrowerOrg := uuid.New(), uuid.New()
		_, err := newOffers(s).List(ctx, service.Principal{UserID: uuid.New(), Role: string(repository.UserRoleAdmin)}, service.OfferFilter{
			LenderOrgID: &lenderOrg, BorrowerOrgID: &borrowerOrg, Page: 3, PageSize: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		a := s.listOffersArg
		if a.LenderOrgFilter.Bytes != lenderOrg || a.BorrowerOrgFilter.Bytes != borrowerOrg || a.PageLimit != 10 || a.PageOffset != 20 {
			t.Fatalf("list arg = %+v", a)
		}
	})
}

func (s *fakeAppStore) ListOffersVisible(_ context.Context, arg repository.ListOffersVisibleParams) ([]repository.ListOffersVisibleRow, error) {
	s.listOffersArg = &arg
	return []repository.ListOffersVisibleRow{}, nil
}

func (s *fakeAppStore) CountOffersVisible(_ context.Context, arg repository.CountOffersVisibleParams) (int64, error) {
	s.countOffersArg = &arg
	return 0, nil
}

func TestOffersListByApplication(t *testing.T) {
	ctx := context.Background()
	s := newFakeAppStore()
	borrower := s.org(repository.UserRoleBorrower)
	l1, l2 := s.org(repository.UserRoleLender), s.org(repository.UserRoleLender)
	appID := s.app(borrower, repository.ApplicationStatusPublished)
	mine := s.offer(appID, l1, repository.OfferStatusSent)
	s.offer(appID, l2, repository.OfferStatusSent)

	t.Run("владелец — все офферы", func(t *testing.T) {
		got, err := newOffers(s).ListByApplication(ctx, borrower, appID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d, want 2", len(got))
		}
	})

	t.Run("кредитор — только свой", func(t *testing.T) {
		got, err := newOffers(s).ListByApplication(ctx, l1, appID, []service.OfferStatus{service.OfferStatusSent})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != mine {
			t.Fatalf("got %+v", got)
		}
		if a := s.appOffersArg; a.ViewerUser != l1.UserID || len(a.Statuses) != 1 || a.Statuses[0] != "sent" {
			t.Fatalf("arg = %+v", a)
		}
	})

	t.Run("кредитор без оффера на опубликованной — пустой массив", func(t *testing.T) {
		got, err := newOffers(s).ListByApplication(ctx, s.org(repository.UserRoleLender), appID, nil)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("got %+v, err %v", got, err)
		}
	})

	t.Run("чужой заёмщик — NOT_FOUND", func(t *testing.T) {
		_, err := newOffers(s).ListByApplication(ctx, s.org(repository.UserRoleBorrower), appID, nil)
		wantCode(t, err, apperr.CodeNotFound)
	})
}

func (s *fakeAppStore) ListApplicationOffersVisible(_ context.Context, arg repository.ListApplicationOffersVisibleParams) ([]repository.ListApplicationOffersVisibleRow, error) {
	s.appOffersArg = &arg
	out := []repository.ListApplicationOffersVisibleRow{}
	for _, o := range s.offers {
		if o.ApplicationID == arg.ApplicationID && s.offerVisible(o, arg.ViewerRole, arg.ViewerOrg) {
			out = append(out, repository.ListApplicationOffersVisibleRow(s.offerRow(o)))
		}
	}
	return out, nil
}
