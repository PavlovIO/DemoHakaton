package service_test

import (
	"context"
	"encoding/json"
	"math"
	"slices"
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

// Сделочные запросы fakeAppStore: та же память заявок и офферов (applications_test.go,
// offers_test.go), плюс сделки; calls — порядок вызовов в транзакции принятия.

func newDeals(s *fakeAppStore) *service.Deals {
	return service.NewDeals(service.DealsDeps{Store: s, Tx: fakeAppTx{s}, Now: func() time.Time { return testNow }})
}

// termsOffer — sent-оффер lender с полными условиями: 500 000.00 EUR, 8 %, комиссия 1 %.
func (s *fakeAppStore) termsOffer(appID uuid.UUID, lender service.Principal) uuid.UUID {
	id := s.offer(appID, lender, repository.OfferStatusSent)
	o := &s.offers[len(s.offers)-1]
	o.Amount, o.Currency = decimal.RequireFromString("500000.00"), "EUR"
	o.InterestRateAnnual = decimal.RequireFromString("8.0000")
	o.FeePercent = decimal.NewNullDecimal(decimal.RequireFromString("1"))
	o.RepaymentSchedule = repository.RepaymentScheduleAnnuity
	o.Collateral = pgtype.Text{String: "Залог оборудования", Valid: true}
	return id
}

func TestDealsAcceptOffer(t *testing.T) {
	ctx := context.Background()

	for _, role := range []repository.UserRole{repository.UserRoleLender, repository.UserRoleAdmin} {
		t.Run(string(role)+" — FORBIDDEN, стор не тронут", func(t *testing.T) {
			s := newFakeAppStore()
			borrower := s.org(repository.UserRoleBorrower)
			lender := s.org(repository.UserRoleLender)
			offerID := s.offer(s.app(borrower, repository.ApplicationStatusPublished), lender, repository.OfferStatusSent)
			_, err := newDeals(s).AcceptOffer(ctx, s.org(role), service.ClientMeta{}, offerID)
			wantCode(t, err, apperr.CodeForbidden)
			if len(s.calls) != 0 {
				t.Fatalf("вызовы стора: %v", s.calls)
			}
		})
	}

	// DECISIONS п. 36: KYC проверяет и сервис — сделка обязывает стороны, а KYC могли
	// отозвать уже после публикации заявки.
	t.Run("borrower без verified KYC — KYC_NOT_VERIFIED, стор не тронут", func(t *testing.T) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		offerID := s.termsOffer(s.app(borrower, repository.ApplicationStatusPublished), s.org(repository.UserRoleLender))
		borrower.KycStatus = string(repository.KycStatusRejected)
		_, err := newDeals(s).AcceptOffer(ctx, borrower, service.ClientMeta{}, offerID)
		wantCode(t, err, apperr.CodeKYCNotVerified)
		if len(s.calls) != 0 {
			t.Fatalf("вызовы стора: %v", s.calls)
		}
	})

	// Сделка неизменна: её условия — из строки, которую UPDATE перевёл в accepted, а не из
	// более раннего чтения, после которого оффер могли изменить.
	t.Run("снапшот — из строки AcceptOffer", func(t *testing.T) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		offerID := s.termsOffer(s.app(borrower, repository.ApplicationStatusPublished), s.org(repository.UserRoleLender))
		s.beforeAccept = func(o *repository.Offer) { o.Amount = decimal.RequireFromString("400000.00") }
		if _, err := newDeals(s).AcceptOffer(ctx, borrower, service.ClientMeta{}, offerID); err != nil {
			t.Fatal(err)
		}
		if a := s.createDealArg; a.Amount.StringFixed(2) != "400000.00" || a.FeeAmount.Decimal.StringFixed(2) != "4000.00" {
			t.Fatalf("create deal arg = %+v", a)
		}
	})

	t.Run("valid_until в прошлом — INVALID_STATUS_TRANSITION «срок истёк», оффер остаётся sent", func(t *testing.T) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		offerID := s.termsOffer(s.app(borrower, repository.ApplicationStatusPublished), s.org(repository.UserRoleLender))
		s.offers[0].ValidUntil = pgtype.Timestamptz{Time: testNow.Add(-time.Second), Valid: true}
		_, err := newDeals(s).AcceptOffer(ctx, borrower, service.ClientMeta{}, offerID)
		wantCode(t, err, apperr.CodeInvalidTransition)
		if e, _ := apperr.As(err); e.Message != "Срок действия оффера истёк" {
			t.Fatalf("message = %q", e.Message)
		}
		if slices.Contains(s.calls, "AcceptOffer") || s.offers[0].Status != repository.OfferStatusSent {
			t.Fatalf("оффер тронут: calls %v, offer %+v", s.calls, s.offers[0])
		}
	})

	for name, validUntil := range map[string]pgtype.Timestamptz{
		"valid_until через час": {Time: testNow.Add(time.Hour), Valid: true},
		"valid_until NULL":      {},
	} {
		t.Run(name+" — принят", func(t *testing.T) {
			s := newFakeAppStore()
			borrower := s.org(repository.UserRoleBorrower)
			offerID := s.termsOffer(s.app(borrower, repository.ApplicationStatusPublished), s.org(repository.UserRoleLender))
			s.offers[0].ValidUntil = validUntil
			if _, err := newDeals(s).AcceptOffer(ctx, borrower, service.ClientMeta{}, offerID); err != nil {
				t.Fatal(err)
			}
		})
	}

	t.Run("чужой оффер — NOT_FOUND", func(t *testing.T) {
		s := newFakeAppStore()
		owner := s.org(repository.UserRoleBorrower)
		offerID := s.termsOffer(s.app(owner, repository.ApplicationStatusPublished), s.org(repository.UserRoleLender))
		_, err := newDeals(s).AcceptOffer(ctx, s.org(repository.UserRoleBorrower), service.ClientMeta{}, offerID)
		wantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("несуществующий оффер — NOT_FOUND", func(t *testing.T) {
		s := newFakeAppStore()
		_, err := newDeals(s).AcceptOffer(ctx, s.org(repository.UserRoleBorrower), service.ClientMeta{}, uuid.New())
		wantCode(t, err, apperr.CodeNotFound)
	})

	for _, status := range []repository.ApplicationStatus{
		repository.ApplicationStatusAgreed, repository.ApplicationStatusCancelled, repository.ApplicationStatusModeration,
	} {
		t.Run("заявка "+string(status)+" — INVALID_STATUS_TRANSITION, оффер не тронут", func(t *testing.T) {
			s := newFakeAppStore()
			borrower := s.org(repository.UserRoleBorrower)
			offerID := s.termsOffer(s.app(borrower, status), s.org(repository.UserRoleLender))
			_, err := newDeals(s).AcceptOffer(ctx, borrower, service.ClientMeta{}, offerID)
			wantCode(t, err, apperr.CodeInvalidTransition)
			if slices.Contains(s.calls, "AcceptOffer") || len(s.deals) != 0 {
				t.Fatalf("calls %v, deals %v", s.calls, s.deals)
			}
		})
	}

	t.Run("оффер withdrawn — INVALID_STATUS_TRANSITION", func(t *testing.T) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		offerID := s.offer(s.app(borrower, repository.ApplicationStatusPublished), s.org(repository.UserRoleLender), repository.OfferStatusWithdrawn)
		_, err := newDeals(s).AcceptOffer(ctx, borrower, service.ClientMeta{}, offerID)
		wantCode(t, err, apperr.CodeInvalidTransition)
		if slices.Contains(s.calls, "AcceptOffer") {
			t.Fatalf("calls %v", s.calls)
		}
	})

	t.Run("AcceptOffer 0 строк (гонка) — INVALID_STATUS_TRANSITION, сделки нет", func(t *testing.T) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		offerID := s.termsOffer(s.app(borrower, repository.ApplicationStatusPublished), s.org(repository.UserRoleLender))
		s.lostRace = true
		_, err := newDeals(s).AcceptOffer(ctx, borrower, service.ClientMeta{}, offerID)
		wantCode(t, err, apperr.CodeInvalidTransition)
		if len(s.deals) != 0 {
			t.Fatalf("deals = %v", s.deals)
		}
	})

	// Страховки БД при правильной блокировке не срабатывают; если сработали — ответ тот же,
	// что у проигравшего гонку.
	for _, constraint := range []string{"offers_one_accepted_uq", "deals_application_id_key", "deals_offer_id_key"} {
		t.Run("CreateDeal 23505 "+constraint+" — INVALID_STATUS_TRANSITION", func(t *testing.T) {
			s := newFakeAppStore()
			borrower := s.org(repository.UserRoleBorrower)
			offerID := s.termsOffer(s.app(borrower, repository.ApplicationStatusPublished), s.org(repository.UserRoleLender))
			s.createDealErr = &pgconn.PgError{Code: "23505", ConstraintName: constraint}
			_, err := newDeals(s).AcceptOffer(ctx, borrower, service.ClientMeta{}, offerID)
			wantCode(t, err, apperr.CodeInvalidTransition)
		})
	}

	t.Run("успех — сделка-снапшот, заявка agreed, конкурент rejected, история, уведомления, аудит", func(t *testing.T) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		appID := s.app(borrower, repository.ApplicationStatusPublished) // 500000.00 EUR, DE, 12 мес., sepa
		winner, rival := s.org(repository.UserRoleLender), s.org(repository.UserRoleLender)
		offerID := s.termsOffer(appID, winner)
		rivalID := s.termsOffer(appID, rival)

		got, err := newDeals(s).AcceptOffer(ctx, borrower, service.ClientMeta{}, offerID)
		if err != nil {
			t.Fatal(err)
		}

		// Порядок блокировок: заявка → принимаемый оффер → остальные офферы (общий с cancel
		// и createOffer, иначе deadlock); заявка agreed — до сделки; ответ — перечитанная сделка.
		lockOrder := []string{"LockApplicationByOfferForBorrower", "LockOfferForAccept", "AcceptOffer", "RejectSentOffersByApplication"}
		if len(s.calls) < len(lockOrder) || !slices.Equal(s.calls[:len(lockOrder)], lockOrder) ||
			slices.Index(s.calls, "AgreeApplication") > slices.Index(s.calls, "CreateDeal") ||
			s.calls[len(s.calls)-1] != "GetDealVisible" {
			t.Fatalf("calls = %v", s.calls)
		}

		// Снапшот: условия — из оффера, срок/страна/способ перевода — из заявки.
		if got.Number != 45 || got.ApplicationID != appID || got.OfferID != offerID ||
			got.Borrower.ID != *borrower.OrgID || got.Borrower.Name != "ООО borrower" ||
			got.Lender.ID != *winner.OrgID || got.Lender.Name != "ООО lender" ||
			got.Amount.StringFixed(2) != "500000.00" || got.Currency != "EUR" ||
			got.InterestRateAnnual.String() != "8" || got.FeePercent == nil || got.FeePercent.String() != "1" ||
			got.FeeAmount == nil || got.FeeAmount.StringFixed(2) != "5000.00" ||
			got.TermMonths != 12 || got.TargetCountryCode != "DE" || got.TransferMethod != "sepa" ||
			got.RepaymentSchedule != "annuity" || got.Collateral == nil || *got.Collateral != "Залог оборудования" ||
			got.AdditionalTerms != nil || !got.AgreedAt.Equal(testNow) {
			t.Fatalf("deal = %+v", got)
		}

		a := s.apps[appID]
		if a.Status != repository.ApplicationStatusAgreed || a.AcceptedOfferID != (pgtype.UUID{Bytes: offerID, Valid: true}) {
			t.Fatalf("app = %+v", a)
		}
		if o := s.findOffer(offerID); o.Status != repository.OfferStatusAccepted {
			t.Fatalf("winner = %+v", o)
		}
		if o := s.findOffer(rivalID); o.Status != repository.OfferStatusRejected ||
			o.RejectReason.OfferRejectReason != repository.OfferRejectReasonAnotherOfferAccepted {
			t.Fatalf("rival = %+v", o)
		}

		// actor у всех записей — заёмщик (DECISIONS п. 12).
		wantHistory := []struct {
			entity   repository.EntityType
			id       uuid.UUID
			from, to string
			comment  string
		}{
			{repository.EntityTypeOffer, offerID, "sent", "accepted", ""},
			{repository.EntityTypeOffer, rivalID, "sent", "rejected", "another_offer_accepted"},
			{repository.EntityTypeApplication, appID, "published", "agreed", ""},
		}
		if len(s.history) != len(wantHistory) {
			t.Fatalf("history = %+v", s.history)
		}
		for _, w := range wantHistory {
			i := slices.IndexFunc(s.history, func(h repository.CreateStatusHistoryParams) bool { return h.EntityID == w.id })
			if i < 0 {
				t.Fatalf("нет истории %s %s", w.entity, w.id)
			}
			h := s.history[i]
			if h.EntityType != w.entity || h.FromStatus.String != w.from || h.ToStatus != w.to ||
				h.Comment.String != w.comment || h.ActorUserID != (pgtype.UUID{Bytes: borrower.UserID, Valid: true}) {
				t.Fatalf("history = %+v, want %+v", h, w)
			}
		}

		// Заёмщику ничего (DECISIONS п. 14): сделку он получает в ответе.
		if len(s.orgNotices) != 2 {
			t.Fatalf("notices = %+v", s.orgNotices)
		}
		byType := func(nt repository.NotificationType) repository.CreateNotificationsForOrganizationParams {
			i := slices.IndexFunc(s.orgNotices, func(n repository.CreateNotificationsForOrganizationParams) bool { return n.Type == nt })
			if i < 0 {
				t.Fatalf("нет уведомления %s: %+v", nt, s.orgNotices)
			}
			return s.orgNotices[i]
		}
		accepted, rejected := byType(repository.NotificationTypeOfferAccepted), byType(repository.NotificationTypeOfferRejected)
		if accepted.OrganizationID != *winner.OrgID || accepted.Type != repository.NotificationTypeOfferAccepted ||
			accepted.EntityType != repository.EntityTypeDeal || accepted.EntityID != got.ID.String() ||
			accepted.Title != "Оффер принят: сделка DEAL-000045" {
			t.Fatalf("offer_accepted = %+v", accepted)
		}
		if rejected.OrganizationID != *rival.OrgID || rejected.Type != repository.NotificationTypeOfferRejected ||
			rejected.EntityType != repository.EntityTypeOffer || rejected.EntityID != rivalID.String() {
			t.Fatalf("offer_rejected = %+v", rejected)
		}

		au := s.audit[0]
		var meta map[string]string
		if err := json.Unmarshal(au.Meta, &meta); err != nil {
			t.Fatal(err)
		}
		if au.Action != service.AuditOfferAccept || au.EntityType.EntityType != repository.EntityTypeOffer ||
			au.EntityID.String != offerID.String() || meta["deal_id"] != got.ID.String() {
			t.Fatalf("audit = %+v meta %v", au, meta)
		}
	})
}

func (s *fakeAppStore) findOffer(id uuid.UUID) repository.Offer {
	for _, o := range s.offers {
		if o.ID == id {
			return o
		}
	}
	return repository.Offer{}
}

// LockApplicationByOfferForBorrower — заявка оффера, если она компании borrower_org; статус не фильтруется.
func (s *fakeAppStore) LockApplicationByOfferForBorrower(_ context.Context, arg repository.LockApplicationByOfferForBorrowerParams) (repository.Application, error) {
	s.calls = append(s.calls, "LockApplicationByOfferForBorrower")
	a, ok := s.apps[s.findOffer(arg.OfferID).ApplicationID]
	if !ok || a.BorrowerOrganizationID != arg.BorrowerOrg {
		return repository.Application{}, pgx.ErrNoRows
	}
	return a, nil
}

func (s *fakeAppStore) LockOfferForAccept(_ context.Context, arg repository.LockOfferForAcceptParams) (repository.Offer, error) {
	s.calls = append(s.calls, "LockOfferForAccept")
	if o := s.findOffer(arg.ID); o.ID == arg.ID && o.ApplicationID == arg.ApplicationID {
		return o, nil
	}
	return repository.Offer{}, pgx.ErrNoRows
}

func (s *fakeAppStore) AcceptOffer(_ context.Context, arg repository.AcceptOfferParams) (repository.Offer, error) {
	s.calls = append(s.calls, "AcceptOffer")
	if s.beforeAccept != nil {
		for i := range s.offers {
			if s.offers[i].ID == arg.ID {
				s.beforeAccept(&s.offers[i])
			}
		}
	}
	return s.decide(arg.ID, func(o repository.Offer) bool { return o.ApplicationID == arg.ApplicationID },
		func(o *repository.Offer) { o.Status = repository.OfferStatusAccepted })
}

func (s *fakeAppStore) AgreeApplication(_ context.Context, arg repository.AgreeApplicationParams) (repository.Application, error) {
	s.calls = append(s.calls, "AgreeApplication")
	s.agreeArg = &arg
	a, err := s.guard(arg.ID, arg.FromStatus)
	if err != nil {
		return a, err
	}
	a.Status, a.AcceptedOfferID = repository.ApplicationStatusAgreed, arg.AcceptedOfferID
	s.apps[a.ID] = a
	return a, nil
}

func (s *fakeAppStore) CreateDeal(_ context.Context, arg repository.CreateDealParams) (repository.Deal, error) {
	s.calls = append(s.calls, "CreateDeal")
	s.createDealArg = &arg
	if s.createDealErr != nil {
		return repository.Deal{}, s.createDealErr
	}
	d := repository.Deal{
		ID: uuid.New(), Number: pgtype.Int8{Int64: 45, Valid: true}, ApplicationID: arg.ApplicationID, OfferID: arg.OfferID,
		BorrowerOrganizationID: arg.BorrowerOrganizationID, LenderOrganizationID: arg.LenderOrganizationID,
		Amount: arg.Amount, Currency: arg.Currency, InterestRateAnnual: arg.InterestRateAnnual,
		FeePercent: arg.FeePercent, FeeAmount: arg.FeeAmount, TermMonths: arg.TermMonths,
		TargetCountryCode: arg.TargetCountryCode, TransferMethod: arg.TransferMethod,
		RepaymentSchedule: arg.RepaymentSchedule, Collateral: arg.Collateral, AdditionalTerms: arg.AdditionalTerms,
		AgreedAt: pgtype.Timestamptz{Time: testNow, Valid: true},
	}
	s.deals = append(s.deals, d)
	return d, nil
}

// dealVisible — правило видимости GetDealVisible: admin — все, стороны — свои.
func dealVisible(d repository.Deal, role repository.UserRole, org uuid.UUID) bool {
	switch role {
	case repository.UserRoleAdmin:
		return true
	case repository.UserRoleBorrower:
		return d.BorrowerOrganizationID == org
	case repository.UserRoleLender:
		return d.LenderOrganizationID == org
	}
	return false
}

func (s *fakeAppStore) dealRow(d repository.Deal) repository.ListDealsVisibleRow {
	bo, lo := s.orgs[d.BorrowerOrganizationID], s.orgs[d.LenderOrganizationID]
	return repository.ListDealsVisibleRow{
		Deal: d, BorrowerName: bo.Name, BorrowerCountryCode: bo.CountryCode, LenderName: lo.Name, LenderCountryCode: lo.CountryCode,
	}
}

func (s *fakeAppStore) GetDealVisible(_ context.Context, arg repository.GetDealVisibleParams) (repository.GetDealVisibleRow, error) {
	s.calls = append(s.calls, "GetDealVisible")
	for _, d := range s.deals {
		if d.ID == arg.ID && dealVisible(d, arg.ViewerRole, arg.ViewerOrg) {
			return repository.GetDealVisibleRow(s.dealRow(d)), nil
		}
	}
	return repository.GetDealVisibleRow{}, pgx.ErrNoRows
}

func TestDealsList(t *testing.T) {
	ctx := context.Background()
	from := testNow.Add(-24 * time.Hour)
	other := uuid.New()

	t.Run("borrower — фильтры организаций отброшены, страница 1/20, период передан", func(t *testing.T) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		_, err := newDeals(s).List(ctx, borrower, service.DealFilter{LenderOrgID: &other, BorrowerOrgID: &other, CreatedFrom: &from})
		if err != nil {
			t.Fatal(err)
		}
		a := s.listDealsArg
		if a.LenderOrgFilter.Valid || a.BorrowerOrgFilter.Valid || a.ViewerOrg != *borrower.OrgID ||
			a.ViewerRole != repository.UserRoleBorrower || a.PageLimit != 20 || a.PageOffset != 0 ||
			!a.CreatedFrom.Time.Equal(from) || a.CreatedTo.Valid {
			t.Fatalf("list arg = %+v", a)
		}
		if s.countDealsArg.LenderOrgFilter.Valid || !s.countDealsArg.CreatedFrom.Valid {
			t.Fatalf("count arg = %+v", s.countDealsArg)
		}
	})

	t.Run("admin — фильтр кредитора передан, страница 2", func(t *testing.T) {
		s := newFakeAppStore()
		admin := service.Principal{UserID: uuid.New(), Role: string(repository.UserRoleAdmin)}
		page, err := newDeals(s).List(ctx, admin, service.DealFilter{LenderOrgID: &other, Page: 2, PageSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		a := s.listDealsArg
		if a.LenderOrgFilter != (pgtype.UUID{Bytes: other, Valid: true}) || a.PageLimit != 10 || a.PageOffset != 10 {
			t.Fatalf("list arg = %+v", a)
		}
		if page.Page != 2 || page.PageSize != 10 {
			t.Fatalf("page = %+v", page)
		}
	})

	// page в контракте без maximum: смещение за пределом int32 — просто страница за концом
	// списка, а не 500 от отрицательного OFFSET или чужая страница после переполнения.
	t.Run("page за пределом int32 — пустая страница, total есть", func(t *testing.T) {
		s := newFakeAppStore()
		borrower, lender := s.org(repository.UserRoleBorrower), s.org(repository.UserRoleLender)
		s.deals = append(s.deals, repository.Deal{ID: uuid.New(),
			BorrowerOrganizationID: *borrower.OrgID, LenderOrganizationID: *lender.OrgID})
		page, err := newDeals(s).List(ctx, borrower, service.DealFilter{Page: 30_000_000, PageSize: 100})
		if err != nil {
			t.Fatal(err)
		}
		if a := s.listDealsArg; a.PageOffset != math.MaxInt32 || a.PageLimit != 100 {
			t.Fatalf("list arg = %+v", a)
		}
		if page.Total != 1 || page.Page != 30_000_000 {
			t.Fatalf("page = %+v", page)
		}
	})

	t.Run("строки и total — из запросов", func(t *testing.T) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		lender := s.org(repository.UserRoleLender)
		s.deals = append(s.deals, repository.Deal{ID: uuid.New(), Number: pgtype.Int8{Int64: 7, Valid: true},
			BorrowerOrganizationID: *borrower.OrgID, LenderOrganizationID: *lender.OrgID})
		page, err := newDeals(s).List(ctx, borrower, service.DealFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Number != 7 || page.Items[0].Lender.Name != "ООО lender" {
			t.Fatalf("page = %+v", page)
		}
	})
}

func TestDealsGet(t *testing.T) {
	ctx := context.Background()
	s := newFakeAppStore()
	borrower, lender := s.org(repository.UserRoleBorrower), s.org(repository.UserRoleLender)
	id := uuid.New()
	s.deals = append(s.deals, repository.Deal{ID: id, BorrowerOrganizationID: *borrower.OrgID, LenderOrganizationID: *lender.OrgID})

	for _, p := range []service.Principal{borrower, lender} {
		if got, err := newDeals(s).Get(ctx, p, id); err != nil || got.ID != id {
			t.Fatalf("%s: got %+v, err %v", p.Role, got, err)
		}
	}
	_, err := newDeals(s).Get(ctx, s.org(repository.UserRoleLender), id)
	wantCode(t, err, apperr.CodeNotFound)
}

func (s *fakeAppStore) ListDealsVisible(_ context.Context, arg repository.ListDealsVisibleParams) ([]repository.ListDealsVisibleRow, error) {
	s.listDealsArg = &arg
	rows := []repository.ListDealsVisibleRow{}
	for _, d := range s.deals {
		if dealVisible(d, arg.ViewerRole, arg.ViewerOrg) {
			rows = append(rows, s.dealRow(d))
		}
	}
	return rows, nil
}

func (s *fakeAppStore) CountDealsVisible(_ context.Context, arg repository.CountDealsVisibleParams) (int64, error) {
	s.countDealsArg = &arg
	var n int64
	for _, d := range s.deals {
		if dealVisible(d, arg.ViewerRole, arg.ViewerOrg) {
			n++
		}
	}
	return n, nil
}
