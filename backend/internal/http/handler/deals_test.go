package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeDeals запоминает, с чем его вызвали, и отдаёт заданную сделку или ошибку.
type fakeDeals struct {
	deal       service.Deal
	err        error
	gotID      uuid.UUID
	gotFilter  *service.DealFilter
	acceptedID uuid.UUID
}

func (f *fakeDeals) AcceptOffer(_ context.Context, _ service.Principal, _ service.ClientMeta, offerID uuid.UUID) (service.Deal, error) {
	f.acceptedID = offerID
	return f.deal, f.err
}

func (f *fakeDeals) List(_ context.Context, _ service.Principal, filter service.DealFilter) (service.DealPage, error) {
	f.gotFilter = &filter
	return service.DealPage{Items: []service.Deal{f.deal}, Total: 1, Page: 2, PageSize: 20}, f.err
}

func (f *fakeDeals) Get(_ context.Context, _ service.Principal, id uuid.UUID) (service.Deal, error) {
	f.gotID = id
	return f.deal, f.err
}

// testDeal — сделка из кейса issue: 500 000.00 EUR, комиссия 1 % = 5 000.00.
func testDeal() service.Deal {
	fee, feeAmount := decimal.RequireFromString("1.0000"), decimal.RequireFromString("5000.00")
	return service.Deal{
		ID: uuid.New(), Number: 45, ApplicationID: uuid.New(), OfferID: uuid.New(),
		Borrower: service.OrgShort{ID: uuid.New(), Name: "ООО «Альфа»", CountryCode: "RU"},
		Lender:   service.OrgShort{ID: uuid.New(), Name: "Beta Corp Ltd", CountryCode: "CY"},
		Amount:   decimal.RequireFromString("500000.00"), Currency: "EUR",
		InterestRateAnnual: decimal.RequireFromString("8.0000"), FeePercent: &fee, FeeAmount: &feeAmount,
		TermMonths: 12, TargetCountryCode: "DE", TransferMethod: "sepa", RepaymentSchedule: "annuity",
		AgreedAt: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC),
	}
}

func newDealsServer(t *testing.T, deals *fakeDeals) *httptest.Server {
	t.Helper()
	fixture, deps := authFixture()
	deps.Auth = &cookieAuth{fakeAuth: *fixture}
	deps.Dicts, deps.DB, deps.Deals = fakeDicts{}, okDB{}, deals
	return newTestServer(t, deps)
}

func TestAcceptOffer(t *testing.T) {
	deals := &fakeDeals{deal: testDeal()}
	srv := newDealsServer(t, deals)
	offerID := uuid.New()
	resp, body := sessionRequest(t, srv, "borrower-verified", "POST", "/api/v1/offers/"+offerID.String()+"/accept", "", nil)
	if resp.StatusCode != http.StatusOK || deals.acceptedID != offerID {
		t.Fatalf("status = %d, accepted %s; body %s", resp.StatusCode, deals.acceptedID, body)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	borrower, _ := raw["borrower"].(map[string]any)
	lender, _ := raw["lender"].(map[string]any)
	if raw["number"] != "DEAL-000045" || raw["amount"] != "500000.00" || raw["fee_amount"] != "5000.00" ||
		raw["currency"] != "EUR" || borrower["name"] != "ООО «Альфа»" || lender["name"] != "Beta Corp Ltd" {
		t.Fatalf("body = %s", body)
	}
}

func TestListDeals(t *testing.T) {
	deals := &fakeDeals{deal: testDeal()}
	srv := newDealsServer(t, deals)
	lenderOrg := uuid.New()
	resp, body := sessionRequest(t, srv, "admin", "GET",
		"/api/v1/deals?created_from=2026-10-01T00:00:00Z&page=2&lender_organization_id="+lenderOrg.String(), "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
	f := deals.gotFilter
	if f == nil || f.Page != 2 || f.CreatedFrom == nil || !f.CreatedFrom.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) ||
		f.LenderOrgID == nil || *f.LenderOrgID != lenderOrg || f.CreatedTo != nil {
		t.Fatalf("filter = %+v", f)
	}
	var page api.DealPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Page != 2 || len(page.Items) != 1 || page.Items[0].Number != "DEAL-000045" {
		t.Fatalf("page = %+v", page)
	}
}

func TestGetDeal(t *testing.T) {
	deals := &fakeDeals{deal: testDeal()}
	srv := newDealsServer(t, deals)
	id := uuid.New()
	resp, body := sessionRequest(t, srv, "lender-verified", "GET", "/api/v1/deals/"+id.String(), "", nil)
	if resp.StatusCode != http.StatusOK || deals.gotID != id {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
}

func TestDealErrors(t *testing.T) {
	tests := []struct {
		name, role, method, path string
		err                      error
		status                   int
		code                     string
	}{
		{"acceptOffer InvalidTransition — 409", "borrower-verified", "POST", "/api/v1/offers/" + uuid.NewString() + "/accept",
			apperr.New(apperr.CodeInvalidTransition, "Срок действия оффера истёк"), http.StatusConflict, "INVALID_STATUS_TRANSITION"},
		{"acceptOffer NotFound — 404", "borrower-verified", "POST", "/api/v1/offers/" + uuid.NewString() + "/accept",
			apperr.NotFound("Оффер не найден"), http.StatusNotFound, "NOT_FOUND"},
		{"getDeal NotFound — 404", "lender-verified", "GET", "/api/v1/deals/" + uuid.NewString(),
			apperr.NotFound("Сделка не найдена"), http.StatusNotFound, "NOT_FOUND"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newDealsServer(t, &fakeDeals{err: tt.err})
			resp, got := sessionRequest(t, srv, tt.role, tt.method, tt.path, "", nil)
			wantError(t, resp, got, tt.status, tt.code, "")
		})
	}
}
