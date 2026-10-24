package handler_test

import (
	"context"
	"encoding/json"
	"io"
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

// fakeOffers запоминает, с чем его вызвали, и отдаёт заданный оффер или ошибку.
type fakeOffers struct {
	offer         service.Offer
	err           error
	gotID         uuid.UUID
	gotCreate     *service.CreateOfferInput
	gotComment    *string
	gotFilter     *service.OfferFilter
	gotStatuses   []service.OfferStatus
	rejectCalled  bool
	withdrawnByID uuid.UUID
}

func (f *fakeOffers) Create(_ context.Context, _ service.Principal, _ service.ClientMeta, applicationID uuid.UUID, in service.CreateOfferInput) (service.Offer, error) {
	f.gotID, f.gotCreate = applicationID, &in
	return f.offer, f.err
}

func (f *fakeOffers) Get(_ context.Context, _ service.Principal, id uuid.UUID) (service.Offer, error) {
	f.gotID = id
	return f.offer, f.err
}

func (f *fakeOffers) List(_ context.Context, _ service.Principal, filter service.OfferFilter) (service.OfferPage, error) {
	f.gotFilter = &filter
	return service.OfferPage{Items: []service.Offer{f.offer}, Total: 1, Page: 1, PageSize: 20}, f.err
}

func (f *fakeOffers) ListByApplication(_ context.Context, _ service.Principal, applicationID uuid.UUID, statuses []service.OfferStatus) ([]service.Offer, error) {
	f.gotID, f.gotStatuses = applicationID, statuses
	return []service.Offer{f.offer}, f.err
}

func (f *fakeOffers) Reject(_ context.Context, _ service.Principal, _ service.ClientMeta, id uuid.UUID, comment *string) (service.Offer, error) {
	f.gotID, f.gotComment, f.rejectCalled = id, comment, true
	return f.offer, f.err
}

func (f *fakeOffers) Withdraw(_ context.Context, _ service.Principal, _ service.ClientMeta, id uuid.UUID) (service.Offer, error) {
	f.gotID, f.withdrawnByID = id, id
	return f.offer, f.err
}

func testOffer() service.Offer {
	unread := int64(3)
	return service.Offer{
		ID: uuid.New(), ApplicationID: uuid.New(), ApplicationNumber: 123,
		Lender: service.OrgShort{ID: uuid.New(), Name: "Bank Beta", CountryCode: "AE"},
		Status: service.OfferStatusSent, Amount: decimal.RequireFromString("100.00"), Currency: "EUR",
		InterestRateAnnual: decimal.RequireFromString("8.00"), RepaymentSchedule: "annuity",
		UnreadMessages: &unread, CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
}

func newOffersServer(t *testing.T, offers *fakeOffers) *httptest.Server {
	t.Helper()
	fixture, deps := authFixture()
	deps.Auth = &cookieAuth{fakeAuth: *fixture}
	deps.Dicts, deps.DB, deps.Offers = fakeDicts{}, okDB{}, offers
	return newTestServer(t, deps)
}

func TestCreateOffer(t *testing.T) {
	t.Run("201: currency из ответа сервиса, amount со scale", func(t *testing.T) {
		offers := &fakeOffers{offer: testOffer()}
		srv := newOffersServer(t, offers)
		appID := uuid.New()
		resp, body := sessionRequest(t, srv, "lender-verified", "POST", "/api/v1/applications/"+appID.String()+"/offers",
			"application/json", strings.NewReader(`{"amount":"100.00","interest_rate_annual":"8.00","repayment_schedule":"annuity"}`))
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d; body %s", resp.StatusCode, body)
		}
		in := offers.gotCreate
		if offers.gotID != appID || in.Amount.String() != "100.00" || in.RepaymentSchedule != "annuity" || in.FeePercent != nil {
			t.Fatalf("create input = %+v", in)
		}
		var got api.Offer
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got.Currency != "EUR" || *got.ApplicationNumber != "APP-000123" || got.Lender.Name != "Bank Beta" || *got.UnreadMessages != 3 {
			t.Fatalf("got %+v", got)
		}
		if !strings.Contains(string(body), `"amount":"100.00"`) {
			t.Fatalf("amount не со scale: %s", body)
		}
	})

	t.Run("без repayment_schedule — 400 VALIDATION_ERROR", func(t *testing.T) {
		offers := &fakeOffers{}
		srv := newOffersServer(t, offers)
		resp, body := sessionRequest(t, srv, "lender-verified", "POST", "/api/v1/applications/"+uuid.NewString()+"/offers",
			"application/json", strings.NewReader(`{"amount":"100.00","interest_rate_annual":"8.00"}`))
		wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "repayment_schedule")
		if offers.gotCreate != nil {
			t.Fatal("сервис вызван")
		}
	})
}

func TestRejectOffer(t *testing.T) {
	t.Run("без тела — comment = nil", func(t *testing.T) {
		offers := &fakeOffers{offer: testOffer()}
		srv := newOffersServer(t, offers)
		id := uuid.New()
		resp, body := sessionRequest(t, srv, "borrower-verified", "POST", "/api/v1/offers/"+id.String()+"/reject", "", nil)
		if resp.StatusCode != http.StatusOK || !offers.rejectCalled || offers.gotID != id || offers.gotComment != nil {
			t.Fatalf("status = %d, comment = %v; body %s", resp.StatusCode, offers.gotComment, body)
		}
	})

	t.Run("с comment — доходит до сервиса", func(t *testing.T) {
		offers := &fakeOffers{offer: testOffer()}
		srv := newOffersServer(t, offers)
		resp, body := sessionRequest(t, srv, "borrower-verified", "POST", "/api/v1/offers/"+uuid.NewString()+"/reject",
			"application/json", strings.NewReader(`{"comment":"дорого"}`))
		if resp.StatusCode != http.StatusOK || offers.gotComment == nil || *offers.gotComment != "дорого" {
			t.Fatalf("status = %d, comment = %v; body %s", resp.StatusCode, offers.gotComment, body)
		}
	})
}

func TestWithdrawOffer(t *testing.T) {
	offers := &fakeOffers{offer: testOffer()}
	srv := newOffersServer(t, offers)
	id := uuid.New()
	resp, body := sessionRequest(t, srv, "lender-verified", "POST", "/api/v1/offers/"+id.String()+"/withdraw", "", nil)
	if resp.StatusCode != http.StatusOK || offers.withdrawnByID != id {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
}

func TestListOffers(t *testing.T) {
	offers := &fakeOffers{offer: testOffer()}
	srv := newOffersServer(t, offers)
	appID := uuid.New()
	resp, body := sessionRequest(t, srv, "admin", "GET",
		"/api/v1/offers?status=sent&status=accepted&application_id="+appID.String()+"&page=2&page_size=10", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
	f := offers.gotFilter
	if len(f.Statuses) != 2 || f.Statuses[0] != service.OfferStatusSent || f.Statuses[1] != service.OfferStatusAccepted ||
		f.ApplicationID == nil || *f.ApplicationID != appID || f.Page != 2 || f.PageSize != 10 {
		t.Fatalf("filter = %+v", f)
	}
	var page api.OfferPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Total != 1 || page.PageSize != 20 {
		t.Fatalf("page = %+v", page)
	}
}

func TestListApplicationOffers(t *testing.T) {
	offers := &fakeOffers{offer: testOffer()}
	srv := newOffersServer(t, offers)
	appID := uuid.New()
	resp, body := sessionRequest(t, srv, "borrower-verified", "GET", "/api/v1/applications/"+appID.String()+"/offers?status=sent", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
	if offers.gotID != appID || len(offers.gotStatuses) != 1 || offers.gotStatuses[0] != service.OfferStatusSent {
		t.Fatalf("id = %s, statuses = %v", offers.gotID, offers.gotStatuses)
	}
	var got []api.Offer
	if err := json.Unmarshal(body, &got); err != nil || len(got) != 1 {
		t.Fatalf("body %s, err %v", body, err)
	}
}

func TestOfferErrors(t *testing.T) {
	tests := []struct {
		name, role, method, path, body string
		err                            error
		status                         int
		code                           string
	}{
		{"getOffer NotFound — 404", "lender-verified", "GET", "/api/v1/offers/" + uuid.NewString(), "",
			apperr.NotFound("Оффер не найден"), http.StatusNotFound, "NOT_FOUND"},
		{"withdrawOffer InvalidTransition — 409", "lender-verified", "POST", "/api/v1/offers/" + uuid.NewString() + "/withdraw", "",
			apperr.InvalidTransition("accepted", "withdrawn"), http.StatusConflict, "INVALID_STATUS_TRANSITION"},
		{"createOffer Conflict — 409", "lender-verified", "POST", "/api/v1/applications/" + uuid.NewString() + "/offers",
			`{"amount":"100.00","interest_rate_annual":"8.00","repayment_schedule":"annuity"}`,
			apperr.Conflict("У компании уже есть активный оффер по этой заявке"), http.StatusConflict, "CONFLICT"},
		{"rejectOffer Forbidden — 403", "borrower-verified", "POST", "/api/v1/offers/" + uuid.NewString() + "/reject", "",
			apperr.Forbidden("Недостаточно прав для операции"), http.StatusForbidden, "FORBIDDEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newOffersServer(t, &fakeOffers{err: tt.err})
			contentType, body := "", io.Reader(nil)
			if tt.body != "" {
				contentType, body = "application/json", strings.NewReader(tt.body)
			}
			resp, got := sessionRequest(t, srv, tt.role, tt.method, tt.path, contentType, body)
			wantError(t, resp, got, tt.status, tt.code, "")
		})
	}
}
