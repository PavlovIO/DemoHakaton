package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeApplications запоминает, с чем его вызвали, и отдаёт заданную заявку или ошибку.
type fakeApplications struct {
	app       service.Application
	err       error
	rejected  int
	history   []service.StatusHistoryEntry
	gotFilter *service.ApplicationFilter
	gotCreate *service.CreateApplicationInput
	gotUpdate *service.UpdateApplicationInput
	gotID     uuid.UUID
}

func (f *fakeApplications) List(_ context.Context, _ service.Principal, filter service.ApplicationFilter) (service.ApplicationPage, error) {
	f.gotFilter = &filter
	return service.ApplicationPage{Items: []service.Application{f.app}, Total: 1, Page: 1, PageSize: 20}, f.err
}

func (f *fakeApplications) Get(_ context.Context, _ service.Principal, id uuid.UUID) (service.Application, error) {
	f.gotID = id
	return f.app, f.err
}

func (f *fakeApplications) Create(_ context.Context, _ service.Principal, _ service.ClientMeta, in service.CreateApplicationInput) (service.Application, error) {
	f.gotCreate = &in
	return f.app, f.err
}

func (f *fakeApplications) Update(_ context.Context, _ service.Principal, id uuid.UUID, in service.UpdateApplicationInput) (service.Application, error) {
	f.gotID, f.gotUpdate = id, &in
	return f.app, f.err
}

func (f *fakeApplications) Submit(_ context.Context, _ service.Principal, _ service.ClientMeta, id uuid.UUID) (service.Application, error) {
	f.gotID = id
	return f.app, f.err
}

func (f *fakeApplications) Cancel(_ context.Context, _ service.Principal, _ service.ClientMeta, id uuid.UUID) (service.Application, error) {
	f.gotID = id
	return f.app, f.err
}

func (f *fakeApplications) RejectAllOffers(_ context.Context, _ service.Principal, id uuid.UUID) (int, error) {
	f.gotID = id
	return f.rejected, f.err
}

func (f *fakeApplications) History(_ context.Context, _ service.Principal, id uuid.UUID) ([]service.StatusHistoryEntry, error) {
	f.gotID = id
	return f.history, f.err
}

func testApplication() service.Application {
	created := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	// Сумма с масштабом, как её отдаёт pgx: в JSON — "500000.00".
	amount, _ := decimal.NewFromString("500000.00")
	return service.Application{
		ID: uuid.New(), Number: 123,
		Borrower: service.OrgShort{ID: uuid.New(), Name: "ООО Альфа", CountryCode: "RU"},
		Status:   service.AppStatusPublished, Amount: amount, Currency: "EUR", TargetCountryCode: "DE",
		Purpose: "Закупка оборудования", TermMonths: 12, TransferMethod: "sepa",
		CreatedAt: created, UpdatedAt: created,
	}
}

func newApplicationsServer(t *testing.T, apps *fakeApplications) *httptest.Server {
	t.Helper()
	fixture, deps := authFixture()
	deps.Auth = &cookieAuth{fakeAuth: *fixture}
	deps.Dicts, deps.DB, deps.Applications = fakeDicts{}, okDB{}, apps
	return newTestServer(t, deps)
}

func TestListApplications(t *testing.T) {
	apps := &fakeApplications{app: testApplication()}
	srv := newApplicationsServer(t, apps)

	resp, body := sessionRequest(t, srv, "lender-verified", "GET",
		"/api/v1/applications?target_country=DE&target_country=AE&status=published&sort=published_at_desc"+
			"&transfer_method=sepa&has_my_offer=true&currency=EUR&amount_min=100.50&page=2&page_size=10", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
	f := apps.gotFilter
	if strings.Join(f.TargetCountries, ",") != "DE,AE" || len(f.Statuses) != 1 || f.Statuses[0] != service.AppStatusPublished ||
		f.Sort != "published_at_desc" || strings.Join(f.TransferMethods, ",") != "sepa" ||
		f.HasMyOffer == nil || !*f.HasMyOffer || *f.Currency != "EUR" || f.AmountMin.String() != "100.50" ||
		f.Page != 2 || f.PageSize != 10 {
		t.Fatalf("filter = %+v", f)
	}
	var page api.ApplicationPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Number != "APP-000123" || page.Total != 1 || page.Items[0].Borrower.Name != "ООО Альфа" {
		t.Fatalf("page = %+v", page)
	}
	if !strings.Contains(string(body), `"amount":"500000.00"`) {
		t.Fatalf("amount не со scale: %s", body)
	}
}

func TestListApplicationsBadAmount(t *testing.T) {
	srv := newApplicationsServer(t, &fakeApplications{})
	resp, body := sessionRequest(t, srv, "lender-verified", "GET", "/api/v1/applications?amount_min=abc", "", nil)
	wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "amount_min")
}

func TestCreateApplication(t *testing.T) {
	t.Run("201 и вход сервиса", func(t *testing.T) {
		apps := &fakeApplications{app: testApplication()}
		srv := newApplicationsServer(t, apps)
		resp, body := sessionRequest(t, srv, "borrower-verified", "POST", "/api/v1/applications",
			"application/json", strings.NewReader(validApplicationBody))
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d; body %s", resp.StatusCode, body)
		}
		in := apps.gotCreate
		if in.Amount.String() != "1000000.00" || in.Currency != "USD" || in.TermMonths != 12 || in.TransferMethod != "swift" {
			t.Fatalf("create input = %+v", in)
		}
	})

	t.Run("сервис вернул KYCNotVerified — 403 KYC_NOT_VERIFIED", func(t *testing.T) {
		srv := newApplicationsServer(t, &fakeApplications{err: apperr.KYCNotVerified()})
		resp, body := sessionRequest(t, srv, "borrower-verified", "POST", "/api/v1/applications",
			"application/json", strings.NewReader(validApplicationBody))
		wantError(t, resp, body, http.StatusForbidden, "KYC_NOT_VERIFIED", "")
	})
}

func TestUpdateApplication(t *testing.T) {
	apps := &fakeApplications{app: testApplication()}
	srv := newApplicationsServer(t, apps)
	id := uuid.New()
	resp, body := sessionRequest(t, srv, "borrower-verified", "PATCH", "/api/v1/applications/"+id.String(),
		"application/json", strings.NewReader(`{"term_months":24,"transfer_method_comment":""}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
	in := apps.gotUpdate
	if apps.gotID != id || in.TermMonths == nil || *in.TermMonths != 24 || in.TransferMethodComment == nil ||
		*in.TransferMethodComment != "" || in.Amount != nil || in.Purpose != nil {
		t.Fatalf("update input = %+v", in)
	}
}

func TestRejectAllOffers(t *testing.T) {
	t.Run("200 rejected_count", func(t *testing.T) {
		srv := newApplicationsServer(t, &fakeApplications{rejected: 2})
		resp, body := sessionRequest(t, srv, "borrower-verified", "POST", "/api/v1/applications/"+uuid.NewString()+"/reject-offers", "", nil)
		if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != `{"rejected_count":2}` {
			t.Fatalf("status = %d; body %s", resp.StatusCode, body)
		}
	})

	t.Run("InvalidTransition — 409", func(t *testing.T) {
		srv := newApplicationsServer(t, &fakeApplications{err: apperr.InvalidTransition("moderation", "published")})
		resp, body := sessionRequest(t, srv, "borrower-verified", "POST", "/api/v1/applications/"+uuid.NewString()+"/reject-offers", "", nil)
		wantError(t, resp, body, http.StatusConflict, "INVALID_STATUS_TRANSITION", "")
	})
}

func TestGetApplicationNotFound(t *testing.T) {
	srv := newApplicationsServer(t, &fakeApplications{err: apperr.NotFound("Заявка не найдена")})
	resp, body := sessionRequest(t, srv, "lender-verified", "GET", "/api/v1/applications/"+uuid.NewString(), "", nil)
	wantError(t, resp, body, http.StatusNotFound, "NOT_FOUND", "")
}

func TestApplicationTransitions(t *testing.T) {
	for _, action := range []string{"submit", "cancel"} {
		t.Run(action, func(t *testing.T) {
			app := testApplication()
			app.Status = service.AppStatusCancelled
			apps := &fakeApplications{app: app}
			srv := newApplicationsServer(t, apps)
			id := uuid.New()
			resp, body := sessionRequest(t, srv, "borrower-verified", "POST", "/api/v1/applications/"+id.String()+"/"+action, "", nil)
			if resp.StatusCode != http.StatusOK || apps.gotID != id {
				t.Fatalf("status = %d, id = %s; body %s", resp.StatusCode, apps.gotID, body)
			}
			var got api.Application
			if err := json.Unmarshal(body, &got); err != nil || got.Status != api.ApplicationStatusCancelled {
				t.Fatalf("body %s, err %v", body, err)
			}
		})
	}
}

func TestApplicationHistory(t *testing.T) {
	from, role := "draft", "borrower"
	actor := uuid.New()
	id := uuid.New()
	apps := &fakeApplications{history: []service.StatusHistoryEntry{{
		ID: 7, EntityType: "application", EntityID: id, FromStatus: &from, ToStatus: "moderation",
		ActorUserID: &actor, ActorRole: &role, CreatedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
	}}}
	srv := newApplicationsServer(t, apps)
	resp, body := sessionRequest(t, srv, "borrower-verified", "GET", "/api/v1/applications/"+id.String()+"/history", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
	var got []api.StatusHistoryEntry
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].EntityID != id.String() || *got[0].FromStatus != "draft" || *got[0].ActorRole != api.RoleBorrower || got[0].Comment != nil {
		t.Fatalf("got %+v", got)
	}
}
