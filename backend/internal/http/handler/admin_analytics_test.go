package handler_test

import (
	"context"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
)

func (f *fakeAdmin) AnalyticsSummary(_ context.Context, _ service.Principal, filter admin.AnalyticsFilter) (admin.AnalyticsSummary, error) {
	f.gotSummary = &filter
	return admin.AnalyticsSummary{
		ApplicationsTotal:    3,
		ApplicationsByStatus: map[string]int64{"published": 2, "draft": 1},
		OffersTotal:          4,
		DealsTotal:           1,
		AmountsByCurrency: []admin.CurrencyStat{{
			Currency: "EUR", Count: 2,
			TotalAmount: decimal.RequireFromString("300.50"), AverageAmount: decimal.RequireFromString("150.25"),
		}},
		TopDemandCountries: []admin.CountryStat{{CountryCode: "DE", Count: 2}},
		TopSupplyCountries: []admin.CountryStat{},
	}, f.err
}

func (f *fakeAdmin) ListAuditLog(_ context.Context, _ service.Principal, filter admin.AuditLogFilter) (admin.AuditLogPage, error) {
	f.gotAudit = &filter
	return f.auditPage, f.err
}

func TestAdminAnalyticsSummary(t *testing.T) {
	t.Run("период и top_n доходят до сервиса, деньги — строки", func(t *testing.T) {
		fake := &fakeAdmin{}
		srv := newAdminServer(t, fake)
		resp, body := sessionRequest(t, srv, "admin", "GET",
			"/api/v1/admin/analytics/summary?created_from=2026-10-01T00:00:00Z&created_to=2026-10-10T00:00:00Z&top_n=3", "", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d; body %s", resp.StatusCode, body)
		}
		f := fake.gotSummary
		if f == nil || f.TopN != 3 || f.CreatedFrom == nil || !f.CreatedFrom.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) ||
			f.CreatedTo == nil || !f.CreatedTo.Equal(time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("filter = %+v", f)
		}
		raw := decodeObject(t, body)
		if raw["applications_total"] != float64(3) || raw["offers_total"] != float64(4) || raw["deals_total"] != float64(1) {
			t.Fatalf("totals in %s", body)
		}
		if byStatus, _ := raw["applications_by_status"].(map[string]any); byStatus["published"] != float64(2) {
			t.Fatalf("applications_by_status in %s", body)
		}
		amounts, _ := raw["amounts_by_currency"].([]any)
		eur, _ := amounts[0].(map[string]any)
		if eur["total_amount"] != "300.50" || eur["average_amount"] != "150.25" || eur["count"] != float64(2) {
			t.Fatalf("amounts = %v", amounts)
		}
		if supply, ok := raw["top_supply_countries"].([]any); !ok || len(supply) != 0 {
			t.Fatalf("top_supply_countries = %v, want []", raw["top_supply_countries"])
		}
	})

	t.Run("без top_n — 0, дефолт выбирает сервис", func(t *testing.T) {
		fake := &fakeAdmin{}
		srv := newAdminServer(t, fake)
		resp, body := sessionRequest(t, srv, "admin", "GET", "/api/v1/admin/analytics/summary", "", nil)
		if resp.StatusCode != http.StatusOK || fake.gotSummary == nil || fake.gotSummary.CreatedFrom != nil {
			t.Fatalf("status = %d, filter = %+v; body %s", resp.StatusCode, fake.gotSummary, body)
		}
	})

	t.Run("top_n=0 — 400", func(t *testing.T) {
		srv := newAdminServer(t, &fakeAdmin{})
		resp, body := sessionRequest(t, srv, "admin", "GET", "/api/v1/admin/analytics/summary?top_n=0", "", nil)
		wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "top_n")
	})
}

func TestAdminListAuditLog(t *testing.T) {
	ip := netip.MustParseAddr("192.0.2.1")
	userID := uuid.New()
	entityType, entityID, ua := "offer", "offer-1", "Mozilla"
	page := admin.AuditLogPage{Items: []admin.AuditLogEntry{
		{ID: 2, UserID: &userID, Action: "offer.accept", EntityType: &entityType, EntityID: &entityID, IP: &ip, UserAgent: &ua,
			Meta: map[string]any{"deal_id": "d-1"}, CreatedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)},
		{ID: 1, Action: "auth.login_failed", Meta: map[string]any{"reason": "unknown_email"}, CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
	}, Total: 7, Page: 1, PageSize: 5}

	t.Run("фильтры и маппинг", func(t *testing.T) {
		fake := &fakeAdmin{auditPage: page}
		srv := newAdminServer(t, fake)
		resp, body := sessionRequest(t, srv, "admin", "GET",
			"/api/v1/admin/audit-log?entity_type=offer&entity_id=offer-1&action=offer.accept&user_id="+userID.String()+
				"&created_from=2026-10-01T00:00:00Z&created_to=2026-10-03T00:00:00Z&page_size=5", "", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d; body %s", resp.StatusCode, body)
		}
		f := fake.gotAudit
		if f == nil || *f.EntityType != "offer" || *f.EntityID != "offer-1" || *f.Action != "offer.accept" || *f.UserID != userID ||
			f.CreatedFrom == nil || f.CreatedTo == nil || f.PageSize != 5 {
			t.Fatalf("filter = %+v", f)
		}
		raw := decodeObject(t, body)
		items, _ := raw["items"].([]any)
		if raw["total"] != float64(7) || len(items) != 2 {
			t.Fatalf("body = %s", body)
		}
		first, _ := items[0].(map[string]any)
		meta, _ := first["meta"].(map[string]any)
		if first["ip"] != "192.0.2.1" || first["user_agent"] != "Mozilla" || first["entity_type"] != "offer" || meta["deal_id"] != "d-1" {
			t.Fatalf("first = %v", first)
		}
		// Поля nullable и необязательные: сгенерированная модель опускает их (omitempty).
		second, _ := items[1].(map[string]any)
		if second["ip"] != nil || second["user_id"] != nil || second["entity_type"] != nil {
			t.Fatalf("anonymous = %v, want no ip/user_id/entity_type", second)
		}
	})

	t.Run("entity_type=bogus — 400", func(t *testing.T) {
		srv := newAdminServer(t, &fakeAdmin{})
		resp, body := sessionRequest(t, srv, "admin", "GET", "/api/v1/admin/audit-log?entity_type=bogus", "", nil)
		wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "entity_type")
	})

	t.Run("компании — 403", func(t *testing.T) {
		srv := newAdminServer(t, &fakeAdmin{})
		for _, path := range []string{"/api/v1/admin/audit-log", "/api/v1/admin/analytics/summary"} {
			resp, body := sessionRequest(t, srv, "borrower-verified", "GET", path, "", nil)
			wantError(t, resp, body, http.StatusForbidden, "FORBIDDEN", "")
		}
	})
}
