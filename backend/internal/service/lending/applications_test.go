package lending_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/lending"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// fakeAppStore — заявки и офферы в памяти. Видимость и охрана статусом в WHERE повторяют
// SQL (applications.sql): 0 строк — pgx.ErrNoRows. Транзакция (fakeAppTx) работает с той
// же памятью: откат не моделируется, тесты проверяют, что дошло до вызовов.
type fakeAppStore struct {
	repository.Querier
	orgs          map[uuid.UUID]repository.Organization
	apps          map[uuid.UUID]repository.Application
	offers        []repository.Offer
	history       []repository.CreateStatusHistoryParams
	adminNotices  []repository.CreateNotificationsForAdminsParams
	orgNotices    []repository.CreateNotificationsForOrganizationParams
	audit         []repository.CreateAuditLogParams
	listArg       *repository.ListApplicationsVisibleParams
	countArg      *repository.CountApplicationsVisibleParams
	rejectArg     *repository.RejectSentOffersByApplicationParams
	submitArg     *repository.SubmitApplicationParams
	updateArg     *repository.UpdateApplicationParams
	lostRace      bool // переходный UPDATE видит другой статус: «параллельный» запрос успел первым
	amlFlags      []repository.CreateAmlFlagParams
	nextNumber    int64
	storedHistory []repository.StatusHistory // ответ ListApplicationHistory
	historyArg    *repository.ListApplicationHistoryParams
	// Офферные запросы — offers_test.go.
	createArg      *repository.CreateOfferParams
	createErr      error
	rejectOfferArg *repository.RejectOfferParams
	withdrawArg    *repository.WithdrawOfferParams
	listOffersArg  *repository.ListOffersVisibleParams
	countOffersArg *repository.CountOffersVisibleParams
	appOffersArg   *repository.ListApplicationOffersVisibleParams
	unread         int64 // unread_messages каждой строки *OffersVisible
	// Сделочные запросы — deals_test.go.
	calls         []string // порядок вызовов транзакции принятия оффера
	deals         []repository.Deal
	agreeArg      *repository.AgreeApplicationParams
	createDealArg *repository.CreateDealParams
	createDealErr error
	// beforeAccept — оффер меняется между чтением и UPDATE: снапшот должен взять RETURNING.
	beforeAccept  func(*repository.Offer)
	listDealsArg  *repository.ListDealsVisibleParams
	countDealsArg *repository.CountDealsVisibleParams
	// Чат — chat_test.go.
	messages  []repository.Message
	authors   map[uuid.UUID]service.Principal // автор сообщения → роль и организация
	readMarks []repository.MarkMessagesReadParams
}

func newFakeAppStore() *fakeAppStore {
	return &fakeAppStore{
		orgs:       map[uuid.UUID]repository.Organization{},
		apps:       map[uuid.UUID]repository.Application{},
		nextNumber: 123,
	}
}

// org — компания заданного типа и её пользователь.
func (s *fakeAppStore) org(role repository.UserRole) service.Principal {
	o := repository.Organization{ID: uuid.New(), Name: "ООО " + string(role), CountryCode: "RU", KycStatus: repository.KycStatusVerified}
	s.orgs[o.ID] = o
	return service.Principal{UserID: uuid.New(), OrgID: &o.ID, Role: string(role), KycStatus: string(o.KycStatus)}
}

// app — заявка компании p в статусе status.
func (s *fakeAppStore) app(p service.Principal, status repository.ApplicationStatus) uuid.UUID {
	a := repository.Application{
		ID: uuid.New(), Number: pgtype.Int8{Int64: s.nextNumber, Valid: true}, BorrowerOrganizationID: *p.OrgID, CreatedByUserID: p.UserID,
		Status: status, Amount: decimal.RequireFromString("500000.00"), Currency: "EUR",
		TargetCountryCode: "DE", Purpose: "Закупка оборудования", TermMonths: 12, TransferMethod: "sepa",
	}
	s.nextNumber++
	s.apps[a.ID] = a
	return a.ID
}

// offer — оффер компании lender по заявке.
func (s *fakeAppStore) offer(appID uuid.UUID, lender service.Principal, status repository.OfferStatus) uuid.UUID {
	o := repository.Offer{ID: uuid.New(), ApplicationID: appID, LenderOrganizationID: *lender.OrgID, Status: status}
	s.offers = append(s.offers, o)
	return o.ID
}

func (s *fakeAppStore) visible(a repository.Application, role repository.UserRole, org uuid.UUID) bool {
	switch role {
	case repository.UserRoleAdmin:
		return true
	case repository.UserRoleBorrower:
		return a.BorrowerOrganizationID == org
	case repository.UserRoleLender:
		if a.Status == repository.ApplicationStatusPublished {
			return true
		}
		for _, o := range s.offers {
			if o.ApplicationID == a.ID && o.LenderOrganizationID == org {
				return true
			}
		}
	}
	return false
}

func (s *fakeAppStore) GetApplicationVisible(_ context.Context, arg repository.GetApplicationVisibleParams) (repository.GetApplicationVisibleRow, error) {
	a, ok := s.apps[arg.ID]
	if !ok || !s.visible(a, arg.ViewerRole, arg.ViewerOrg) {
		return repository.GetApplicationVisibleRow{}, pgx.ErrNoRows
	}
	row := repository.GetApplicationVisibleRow{Application: a}
	org := s.orgs[a.BorrowerOrganizationID]
	row.BorrowerName, row.BorrowerCountryCode = org.Name, org.CountryCode
	for _, o := range s.offers {
		if o.ApplicationID != a.ID {
			continue
		}
		row.OffersCount++
		if o.Status == repository.OfferStatusSent && o.LenderOrganizationID == arg.ViewerOrg {
			row.MyOfferID = o.ID
		}
	}
	return row, nil
}

func (s *fakeAppStore) CreateApplication(_ context.Context, arg repository.CreateApplicationParams) (repository.Application, error) {
	a := repository.Application{
		ID: uuid.New(), Number: pgtype.Int8{Int64: s.nextNumber, Valid: true}, BorrowerOrganizationID: arg.BorrowerOrganizationID,
		CreatedByUserID: arg.CreatedByUserID, Status: repository.ApplicationStatusDraft, Amount: arg.Amount,
		Currency: arg.Currency, TargetCountryCode: arg.TargetCountryCode, Purpose: arg.Purpose,
		TermMonths: arg.TermMonths, TransferMethod: arg.TransferMethod, TransferMethodComment: arg.TransferMethodComment,
	}
	s.nextNumber++
	s.apps[a.ID] = a
	return a, nil
}

func (s *fakeAppStore) CreateStatusHistory(_ context.Context, arg repository.CreateStatusHistoryParams) (repository.StatusHistory, error) {
	s.calls = append(s.calls, "CreateStatusHistory")
	s.history = append(s.history, arg)
	return repository.StatusHistory{}, nil
}

func (s *fakeAppStore) CreateAuditLog(_ context.Context, arg repository.CreateAuditLogParams) (repository.AuditLog, error) {
	s.calls = append(s.calls, "CreateAuditLog")
	s.audit = append(s.audit, arg)
	return repository.AuditLog{}, nil
}

// fakeAppTx — транзакция поверх той же памяти.
type fakeAppTx struct{ store *fakeAppStore }

// appInTx — querier внутри транзакции: отдельный тип, чтобы тест отличил его от store пула.
type appInTx struct{ *fakeAppStore }

func (t fakeAppTx) InTx(_ context.Context, fn func(q repository.Querier) error) error {
	return fn(appInTx{t.store})
}

// fakeAppScreener — AML-скрининг заявки: запоминает субъект, возвращает findings.
type fakeAppScreener struct {
	aml.NoopScreener
	screened []aml.ApplicationSubject
	stores   []aml.Store
	findings []aml.Finding
}

func (f *fakeAppScreener) ScreenApplication(_ context.Context, store aml.Store, s aml.ApplicationSubject) ([]aml.Finding, error) {
	f.screened, f.stores = append(f.screened, s), append(f.stores, store)
	return f.findings, nil
}

func newApplications(s *fakeAppStore) (*lending.Applications, *fakeAppScreener) {
	screener := &fakeAppScreener{}
	return lending.NewApplications(lending.ApplicationsDeps{Store: s, Tx: fakeAppTx{s}, Screener: screener}), screener
}

func validCreateInput() lending.CreateApplicationInput {
	return lending.CreateApplicationInput{
		Amount: decimal.RequireFromString("500000.00"), Currency: "EUR", TargetCountryCode: "DE",
		Purpose: "Закупка оборудования для завода", TermMonths: 12, TransferMethod: "sepa",
	}
}

func TestApplicationsCreate(t *testing.T) {
	t.Run("lender — FORBIDDEN", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		_, err := svc.Create(context.Background(), s.org(repository.UserRoleLender), service.ClientMeta{}, validCreateInput())
		servicetest.WantCode(t, err, apperr.CodeForbidden)
		if len(s.apps) != 0 {
			t.Fatalf("заявка создана: %v", s.apps)
		}
	})

	t.Run("KYC не verified — KYC_NOT_VERIFIED", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		p.KycStatus = string(repository.KycStatusUnderReview)
		_, err := svc.Create(context.Background(), p, service.ClientMeta{}, validCreateInput())
		servicetest.WantCode(t, err, apperr.CodeKYCNotVerified)
		if len(s.apps) != 0 {
			t.Fatalf("заявка создана: %v", s.apps)
		}
	})

	t.Run("verified — draft, история NULL→draft, аудит", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		got, err := svc.Create(context.Background(), p, service.ClientMeta{}, validCreateInput())
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != lending.AppStatusDraft || got.Borrower.ID != *p.OrgID || got.Borrower.Name != "ООО borrower" {
			t.Fatalf("got %+v", got)
		}
		if !got.Amount.Equal(decimal.RequireFromString("500000")) {
			t.Fatalf("amount = %s", got.Amount)
		}
		if len(s.history) != 1 {
			t.Fatalf("history = %+v, want 1", s.history)
		}
		h := s.history[0]
		if h.EntityType != repository.EntityTypeApplication || h.EntityID != got.ID ||
			h.FromStatus.Valid || h.ToStatus != "draft" || h.ActorUserID.Bytes != p.UserID {
			t.Fatalf("history = %+v", h)
		}
		if len(s.audit) != 1 || s.audit[0].Action != lending.AuditApplicationCreate {
			t.Fatalf("audit = %+v", s.audit)
		}
	})
}

func (s *fakeAppStore) ListApplicationsVisible(_ context.Context, arg repository.ListApplicationsVisibleParams) ([]repository.ListApplicationsVisibleRow, error) {
	s.listArg = &arg
	return []repository.ListApplicationsVisibleRow{}, nil
}

func (s *fakeAppStore) CountApplicationsVisible(_ context.Context, arg repository.CountApplicationsVisibleParams) (int64, error) {
	s.countArg = &arg
	return 0, nil
}

func TestApplicationsList(t *testing.T) {
	ctx := context.Background()

	t.Run("amount_min без currency — VALIDATION_ERROR", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		min := decimal.RequireFromString("100")
		_, err := svc.List(ctx, s.org(repository.UserRoleLender), lending.ApplicationFilter{AmountMin: &min})
		servicetest.WantValidationField(t, err, "amount_min")
		if s.listArg != nil {
			t.Fatal("запрос ушёл в БД")
		}
	})

	t.Run("borrower: видимость его компании, чужой BorrowerOrgID и HasMyOffer отброшены", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		other := uuid.New()
		page, err := svc.List(ctx, p, lending.ApplicationFilter{BorrowerOrgID: &other, HasMyOffer: servicetest.Ptr(true)})
		if err != nil {
			t.Fatal(err)
		}
		a := s.listArg
		if a.ViewerRole != repository.UserRoleBorrower || a.ViewerOrg != *p.OrgID || a.BorrowerOrgFilter.Valid || a.HasMyOffer.Valid {
			t.Fatalf("list arg = %+v", a)
		}
		if s.countArg.ViewerOrg != *p.OrgID || s.countArg.BorrowerOrgFilter.Valid {
			t.Fatalf("count arg = %+v", s.countArg)
		}
		if page.Page != 1 || page.PageSize != 20 || a.PageLimit != 20 || a.PageOffset != 0 || a.Sort != "created_at_desc" {
			t.Fatalf("page = %+v, arg = %+v", page, a)
		}
		if page.Items == nil {
			t.Fatal("items = nil, want []")
		}
	})

	t.Run("lender: has_my_offer и фильтры доходят до запроса", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleLender)
		_, err := svc.List(ctx, p, lending.ApplicationFilter{
			HasMyOffer: servicetest.Ptr(true), Statuses: []lending.ApplicationStatus{lending.AppStatusPublished},
			TargetCountries: []string{"DE", "AE"}, TransferMethods: []string{"sepa"},
			Sort: "published_at_desc", Page: 3, PageSize: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		a := s.listArg
		if !a.HasMyOffer.Valid || !a.HasMyOffer.Bool || a.ViewerRole != repository.UserRoleLender {
			t.Fatalf("has_my_offer = %+v", a.HasMyOffer)
		}
		if len(a.Statuses) != 1 || a.Statuses[0] != "published" ||
			len(a.TargetCountries) != 2 || a.TargetCountries[1] != "AE" || a.TransferMethods[0] != "sepa" {
			t.Fatalf("filters = %+v", a)
		}
		if a.Sort != "published_at_desc" || a.PageLimit != 10 || a.PageOffset != 20 {
			t.Fatalf("sort/page = %+v", a)
		}
	})

	t.Run("admin: borrower_organization_id доходит до запроса", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		org := uuid.New()
		_, err := svc.List(ctx, service.Principal{UserID: uuid.New(), Role: "admin"}, lending.ApplicationFilter{BorrowerOrgID: &org, HasMyOffer: servicetest.Ptr(false)})
		if err != nil {
			t.Fatal(err)
		}
		a := s.listArg
		if !a.BorrowerOrgFilter.Valid || a.BorrowerOrgFilter.Bytes != org || a.HasMyOffer.Valid || a.ViewerOrg != uuid.Nil {
			t.Fatalf("list arg = %+v", a)
		}
	})
}

func TestApplicationsGet(t *testing.T) {
	ctx := context.Background()
	s := newFakeAppStore()
	svc, _ := newApplications(s)
	borrower := s.org(repository.UserRoleBorrower)
	lender := s.org(repository.UserRoleLender)
	other := s.org(repository.UserRoleLender)
	id := s.app(borrower, repository.ApplicationStatusPublished)
	a := s.apps[id]
	a.ModerationComment = pgtype.Text{String: "Уточните цель", Valid: true}
	s.apps[id] = a
	myOffer := s.offer(id, lender, repository.OfferStatusSent)
	s.offer(id, other, repository.OfferStatusRejected)

	t.Run("владелец: offers_count и moderation_comment, без my_offer_id", func(t *testing.T) {
		got, err := svc.Get(ctx, borrower, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.OffersCount == nil || *got.OffersCount != 2 || got.ModerationComment == nil || got.MyOfferID != nil {
			t.Fatalf("got %+v", got)
		}
		if got.Number != 123 || got.Borrower.Name != "ООО borrower" || got.Borrower.CountryCode != "RU" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("кредитор: my_offer_id, без offers_count и moderation_comment", func(t *testing.T) {
		got, err := svc.Get(ctx, lender, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.MyOfferID == nil || *got.MyOfferID != myOffer || got.OffersCount != nil || got.ModerationComment != nil {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("кредитор без активного оффера: my_offer_id = nil", func(t *testing.T) {
		got, err := svc.Get(ctx, other, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.MyOfferID != nil {
			t.Fatalf("my_offer_id = %v", got.MyOfferID)
		}
	})

	t.Run("чужая заявка — NOT_FOUND", func(t *testing.T) {
		_, err := svc.Get(ctx, s.org(repository.UserRoleBorrower), id)
		servicetest.WantCode(t, err, apperr.CodeNotFound)
	})
}

func (s *fakeAppStore) LockApplicationForBorrower(_ context.Context, arg repository.LockApplicationForBorrowerParams) (repository.Application, error) {
	a, ok := s.apps[arg.ID]
	if !ok || a.BorrowerOrganizationID != arg.BorrowerOrganizationID {
		return repository.Application{}, pgx.ErrNoRows
	}
	return a, nil
}

// guard — охрана статусом в WHERE переходных UPDATE: другой статус или проигранная гонка — 0 строк.
func (s *fakeAppStore) guard(id uuid.UUID, from repository.ApplicationStatus) (repository.Application, error) {
	a, ok := s.apps[id]
	if !ok || a.Status != from || s.lostRace {
		return repository.Application{}, pgx.ErrNoRows
	}
	return a, nil
}

func (s *fakeAppStore) UpdateApplication(_ context.Context, arg repository.UpdateApplicationParams) (repository.Application, error) {
	s.updateArg = &arg
	a, err := s.guard(arg.ID, arg.FromStatus)
	if err != nil {
		return a, err
	}
	if arg.Purpose.Valid {
		a.Purpose = arg.Purpose.String
	}
	if arg.TransferMethodComment.Valid {
		a.TransferMethodComment = pgtype.Text{String: arg.TransferMethodComment.String, Valid: arg.TransferMethodComment.String != ""}
	}
	s.apps[a.ID] = a
	return a, nil
}

func TestApplicationsUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("draft: поля и from_status уходят в UPDATE, ответ — перечитанная заявка", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		id := s.app(p, repository.ApplicationStatusRejected)
		got, err := svc.Update(ctx, p, id, lending.UpdateApplicationInput{
			Purpose: servicetest.Ptr("Новая цель заявки"), TransferMethodComment: servicetest.Ptr(""),
		})
		if err != nil {
			t.Fatal(err)
		}
		a := s.updateArg
		if a.FromStatus != repository.ApplicationStatusRejected || a.Amount.Valid || !a.Purpose.Valid ||
			!a.TransferMethodComment.Valid || a.TransferMethodComment.String != "" {
			t.Fatalf("update arg = %+v", a)
		}
		if got.Purpose != "Новая цель заявки" || got.TransferMethodComment != nil || got.Borrower.Name == "" {
			t.Fatalf("got %+v", got)
		}
		if len(s.history) != 0 {
			t.Fatalf("PATCH записал историю: %+v", s.history)
		}
	})

	t.Run("published — INVALID_STATUS_TRANSITION, UPDATE не вызван", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		id := s.app(p, repository.ApplicationStatusPublished)
		_, err := svc.Update(ctx, p, id, lending.UpdateApplicationInput{Purpose: servicetest.Ptr("Новая цель заявки")})
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
		if s.updateArg != nil {
			t.Fatal("UPDATE вызван")
		}
	})

	t.Run("чужая заявка — NOT_FOUND", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		id := s.app(s.org(repository.UserRoleBorrower), repository.ApplicationStatusDraft)
		_, err := svc.Update(ctx, s.org(repository.UserRoleBorrower), id, lending.UpdateApplicationInput{})
		servicetest.WantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("UPDATE с from_status вернул 0 строк — INVALID_STATUS_TRANSITION, не NOT_FOUND", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		id := s.app(p, repository.ApplicationStatusDraft)
		s.lostRace = true
		_, err := svc.Update(ctx, p, id, lending.UpdateApplicationInput{Purpose: servicetest.Ptr("Новая цель заявки")})
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
	})

	t.Run("lender — FORBIDDEN", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		_, err := svc.Update(ctx, s.org(repository.UserRoleLender), uuid.New(), lending.UpdateApplicationInput{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})
}

func (s *fakeAppStore) SubmitApplication(_ context.Context, arg repository.SubmitApplicationParams) (repository.Application, error) {
	s.submitArg = &arg
	a, err := s.guard(arg.ID, arg.FromStatus)
	if err != nil {
		return a, err
	}
	a.Status = repository.ApplicationStatusModeration
	a.ModerationComment = pgtype.Text{}
	s.apps[a.ID] = a
	return a, nil
}

func (s *fakeAppStore) CreateNotificationsForAdmins(_ context.Context, arg repository.CreateNotificationsForAdminsParams) (int64, error) {
	s.adminNotices = append(s.adminNotices, arg)
	return 1, nil
}

func (s *fakeAppStore) HasSimilarAmlFlag(context.Context, repository.HasSimilarAmlFlagParams) (bool, error) {
	return false, nil
}

func (s *fakeAppStore) CreateAmlFlag(_ context.Context, arg repository.CreateAmlFlagParams) (repository.AmlFlag, error) {
	s.amlFlags = append(s.amlFlags, arg)
	return repository.AmlFlag{}, nil
}

func TestApplicationsSubmit(t *testing.T) {
	ctx := context.Background()

	t.Run("draft → moderation: AML, уведомление админам, история, аудит", func(t *testing.T) {
		s := newFakeAppStore()
		svc, screener := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		id := s.app(p, repository.ApplicationStatusDraft)
		got, err := svc.Submit(ctx, p, service.ClientMeta{}, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != lending.AppStatusModeration {
			t.Fatalf("status = %s", got.Status)
		}
		if s.submitArg == nil || s.submitArg.FromStatus != repository.ApplicationStatusDraft {
			t.Fatalf("submit arg = %+v", s.submitArg)
		}
		if len(screener.screened) != 1 || screener.screened[0].TargetCountryCode != "DE" || screener.screened[0].CurrencyCode != "EUR" {
			t.Fatalf("screened = %+v", screener.screened)
		}
		if _, ok := screener.stores[0].(appInTx); !ok {
			t.Fatalf("скрининг получил %T, а не querier транзакции", screener.stores[0])
		}
		if len(s.adminNotices) != 1 || s.adminNotices[0].Type != repository.NotificationTypeApplicationSubmitted ||
			s.adminNotices[0].EntityType != repository.EntityTypeApplication || s.adminNotices[0].EntityID != id.String() {
			t.Fatalf("admin notices = %+v", s.adminNotices)
		}
		if len(s.history) != 1 || s.history[0].FromStatus.String != "draft" || s.history[0].ToStatus != "moderation" {
			t.Fatalf("history = %+v", s.history)
		}
		if len(s.audit) != 1 || s.audit[0].Action != lending.AuditApplicationSubmit {
			t.Fatalf("audit = %+v", s.audit)
		}
	})

	t.Run("AML-находки — флаги по заявке и одно aml_flag_created", func(t *testing.T) {
		s := newFakeAppStore()
		svc, screener := newApplications(s)
		screener.findings = []aml.Finding{
			{Rule: repository.AmlRuleAmountThreshold, Severity: repository.AmlSeverityMedium, Details: "{}"},
			{Rule: repository.AmlRuleHighRiskJurisdiction, Severity: repository.AmlSeverityHigh, Details: "{}"},
		}
		p := s.org(repository.UserRoleBorrower)
		id := s.app(p, repository.ApplicationStatusRejected)
		if _, err := svc.Submit(ctx, p, service.ClientMeta{}, id); err != nil {
			t.Fatal(err)
		}
		if len(s.amlFlags) != 2 || s.amlFlags[0].ApplicationID.Bytes != id || s.amlFlags[0].OrganizationID != *p.OrgID {
			t.Fatalf("flags = %+v", s.amlFlags)
		}
		if len(s.adminNotices) != 2 || s.adminNotices[1].Type != repository.NotificationTypeAmlFlagCreated {
			t.Fatalf("admin notices = %+v", s.adminNotices)
		}
	})

	t.Run("из published — INVALID_STATUS_TRANSITION, ни одной записи", func(t *testing.T) {
		s := newFakeAppStore()
		svc, screener := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		id := s.app(p, repository.ApplicationStatusPublished)
		_, err := svc.Submit(ctx, p, service.ClientMeta{}, id)
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
		if s.submitArg != nil || len(screener.screened) != 0 || len(s.history)+len(s.adminNotices)+len(s.audit) != 0 {
			t.Fatalf("побочные эффекты: submit=%v screened=%v history=%v notices=%v audit=%v",
				s.submitArg, screener.screened, s.history, s.adminNotices, s.audit)
		}
	})

	t.Run("проигранная гонка — INVALID_STATUS_TRANSITION", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		id := s.app(p, repository.ApplicationStatusDraft)
		s.lostRace = true
		_, err := svc.Submit(ctx, p, service.ClientMeta{}, id)
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
	})
}

func (s *fakeAppStore) RejectSentOffersByApplication(_ context.Context, arg repository.RejectSentOffersByApplicationParams) ([]repository.RejectSentOffersByApplicationRow, error) {
	s.calls = append(s.calls, "RejectSentOffersByApplication")
	s.rejectArg = &arg
	rows := []repository.RejectSentOffersByApplicationRow{}
	for i, o := range s.offers {
		if o.ApplicationID != arg.ApplicationID || o.Status != repository.OfferStatusSent {
			continue
		}
		s.offers[i].Status = repository.OfferStatusRejected
		s.offers[i].RejectReason = repository.NullOfferRejectReason{OfferRejectReason: arg.Reason, Valid: true}
		rows = append(rows, repository.RejectSentOffersByApplicationRow{ID: o.ID, LenderOrganizationID: o.LenderOrganizationID})
	}
	return rows, nil
}

func (s *fakeAppStore) CancelApplication(_ context.Context, arg repository.CancelApplicationParams) (repository.Application, error) {
	a, err := s.guard(arg.ID, arg.FromStatus)
	if err != nil {
		return a, err
	}
	a.Status = repository.ApplicationStatusCancelled
	s.apps[a.ID] = a
	return a, nil
}

func (s *fakeAppStore) CreateNotificationsForOrganization(_ context.Context, arg repository.CreateNotificationsForOrganizationParams) (int64, error) {
	s.calls = append(s.calls, "CreateNotificationsForOrganization")
	s.orgNotices = append(s.orgNotices, arg)
	return 1, nil
}

func TestApplicationsCancel(t *testing.T) {
	ctx := context.Background()

	t.Run("published с двумя sent-офферами: каскад, 3 записи истории, 2 offer_rejected", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		l1, l2, l3 := s.org(repository.UserRoleLender), s.org(repository.UserRoleLender), s.org(repository.UserRoleLender)
		id := s.app(p, repository.ApplicationStatusPublished)
		o1 := s.offer(id, l1, repository.OfferStatusSent)
		o2 := s.offer(id, l2, repository.OfferStatusSent)
		s.offer(id, l3, repository.OfferStatusWithdrawn)

		got, err := svc.Cancel(ctx, p, service.ClientMeta{}, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != lending.AppStatusCancelled {
			t.Fatalf("status = %s", got.Status)
		}
		if s.rejectArg == nil || s.rejectArg.Reason != repository.OfferRejectReasonApplicationCancelled {
			t.Fatalf("reject arg = %+v", s.rejectArg)
		}
		if len(s.history) != 3 {
			t.Fatalf("history = %+v, want 3", s.history)
		}
		for i, offerID := range []uuid.UUID{o1, o2} {
			h := s.history[i]
			if h.EntityType != repository.EntityTypeOffer || h.EntityID != offerID || h.FromStatus.String != "sent" ||
				h.ToStatus != "rejected" || h.Comment.String != "application_cancelled" || h.ActorUserID.Bytes != p.UserID {
				t.Fatalf("offer history[%d] = %+v", i, h)
			}
		}
		if h := s.history[2]; h.EntityType != repository.EntityTypeApplication || h.FromStatus.String != "published" || h.ToStatus != "cancelled" {
			t.Fatalf("application history = %+v", h)
		}
		if len(s.orgNotices) != 2 || s.orgNotices[0].OrganizationID != *l1.OrgID || s.orgNotices[1].OrganizationID != *l2.OrgID ||
			s.orgNotices[0].Type != repository.NotificationTypeOfferRejected || s.orgNotices[0].EntityType != repository.EntityTypeOffer ||
			s.orgNotices[0].EntityID != o1.String() {
			t.Fatalf("org notices = %+v", s.orgNotices)
		}
		if len(s.audit) != 1 || s.audit[0].Action != lending.AuditApplicationCancel {
			t.Fatalf("audit = %+v", s.audit)
		}
	})

	t.Run("из moderation — админам не уведомляем", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		id := s.app(p, repository.ApplicationStatusModeration)
		if _, err := svc.Cancel(ctx, p, service.ClientMeta{}, id); err != nil {
			t.Fatal(err)
		}
		if len(s.adminNotices) != 0 || len(s.orgNotices) != 0 {
			t.Fatalf("notices: admins=%+v orgs=%+v", s.adminNotices, s.orgNotices)
		}
	})

	t.Run("из agreed — INVALID_STATUS_TRANSITION, офферы не тронуты", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		id := s.app(p, repository.ApplicationStatusAgreed)
		_, err := svc.Cancel(ctx, p, service.ClientMeta{}, id)
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
		if s.rejectArg != nil || len(s.history) != 0 {
			t.Fatalf("побочные эффекты: reject=%v history=%v", s.rejectArg, s.history)
		}
	})
}

func TestApplicationsRejectAllOffers(t *testing.T) {
	ctx := context.Background()

	t.Run("published: rejected_by_borrower, результат = число офферов, заявка published", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		id := s.app(p, repository.ApplicationStatusPublished)
		s.offer(id, s.org(repository.UserRoleLender), repository.OfferStatusSent)
		s.offer(id, s.org(repository.UserRoleLender), repository.OfferStatusSent)

		n, err := svc.RejectAllOffers(ctx, p, id)
		if err != nil {
			t.Fatal(err)
		}
		if n != 2 || s.rejectArg.Reason != repository.OfferRejectReasonRejectedByBorrower {
			t.Fatalf("n = %d, reject arg = %+v", n, s.rejectArg)
		}
		if s.apps[id].Status != repository.ApplicationStatusPublished {
			t.Fatalf("status = %s", s.apps[id].Status)
		}
		if len(s.history) != 2 || s.history[0].Comment.String != "rejected_by_borrower" || len(s.orgNotices) != 2 {
			t.Fatalf("history = %+v, notices = %+v", s.history, s.orgNotices)
		}
	})

	t.Run("без офферов — 0, не ошибка", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		n, err := svc.RejectAllOffers(ctx, p, s.app(p, repository.ApplicationStatusPublished))
		if err != nil || n != 0 {
			t.Fatalf("n = %d, err = %v", n, err)
		}
	})

	t.Run("из moderation — INVALID_STATUS_TRANSITION", func(t *testing.T) {
		s := newFakeAppStore()
		svc, _ := newApplications(s)
		p := s.org(repository.UserRoleBorrower)
		_, err := svc.RejectAllOffers(ctx, p, s.app(p, repository.ApplicationStatusModeration))
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
		if s.rejectArg != nil {
			t.Fatal("офферы отклонены")
		}
	})
}

func (s *fakeAppStore) ListApplicationHistory(_ context.Context, arg repository.ListApplicationHistoryParams) ([]repository.StatusHistory, error) {
	s.historyArg = &arg
	return s.storedHistory, nil
}

func TestApplicationsHistory(t *testing.T) {
	ctx := context.Background()
	s := newFakeAppStore()
	svc, _ := newApplications(s)
	borrower := s.org(repository.UserRoleBorrower)
	lender := s.org(repository.UserRoleLender)
	id := s.app(borrower, repository.ApplicationStatusPublished)
	offerID := s.offer(id, lender, repository.OfferStatusRejected)
	s.storedHistory = []repository.StatusHistory{
		{ID: 1, EntityType: repository.EntityTypeApplication, EntityID: id, FromStatus: pgtype.Text{String: "moderation", Valid: true},
			ToStatus: "rejected", ActorRole: repository.NullUserRole{UserRole: repository.UserRoleAdmin, Valid: true},
			Comment: pgtype.Text{String: "Уточните цель", Valid: true}},
		{ID: 2, EntityType: repository.EntityTypeOffer, EntityID: offerID, FromStatus: pgtype.Text{String: "sent", Valid: true},
			ToStatus: "rejected", Comment: pgtype.Text{String: "rejected_by_borrower", Valid: true}},
	}

	t.Run("lender: комментарий модерации скрыт, комментарий оффера сохранён", func(t *testing.T) {
		got, err := svc.History(ctx, lender, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Comment != nil || got[1].Comment == nil || *got[1].Comment != "rejected_by_borrower" {
			t.Fatalf("got %+v", got)
		}
		if s.historyArg.ViewerRole != repository.UserRoleLender || s.historyArg.ViewerOrg != *lender.OrgID {
			t.Fatalf("history arg = %+v", s.historyArg)
		}
		if got[0].ActorRole == nil || *got[0].ActorRole != "admin" || got[0].FromStatus == nil || got[0].EntityID != id {
			t.Fatalf("entry = %+v", got[0])
		}
	})

	for _, p := range []service.Principal{borrower, {UserID: uuid.New(), Role: "admin"}} {
		t.Run(p.Role+": комментарий модерации на месте", func(t *testing.T) {
			got, err := svc.History(ctx, p, id)
			if err != nil {
				t.Fatal(err)
			}
			if got[0].Comment == nil || *got[0].Comment != "Уточните цель" {
				t.Fatalf("got %+v", got[0])
			}
		})
	}

	t.Run("невидимая заявка — NOT_FOUND, история не читается", func(t *testing.T) {
		s.historyArg = nil
		_, err := svc.History(ctx, s.org(repository.UserRoleBorrower), id)
		servicetest.WantCode(t, err, apperr.CodeNotFound)
		if s.historyArg != nil {
			t.Fatal("история прочитана")
		}
	})
}
