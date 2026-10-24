package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/notifications"
)

// fakeNotifications запоминает аргументы и отдаёт заданную ошибку.
type fakeNotifications struct {
	err       error
	gotFilter *notifications.NotificationFilter
	gotMarkID int64
	markedAll bool
}

func (f *fakeNotifications) List(_ context.Context, _ service.Principal, filter notifications.NotificationFilter) (notifications.NotificationPage, error) {
	f.gotFilter = &filter
	n := repository.Notification{
		ID: 7, Type: repository.NotificationTypeOfferReceived, Title: "Новое предложение по заявке APP-000123",
		EntityType: repository.EntityTypeOffer, EntityID: "0b6f7c4e-0000-4000-8000-000000000001",
		CreatedAt: pgtype.Timestamptz{Time: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), Valid: true},
	}
	return notifications.NotificationPage{Items: []repository.Notification{n}, Total: 3, UnreadTotal: 5, Page: 1, PageSize: 2}, f.err
}

func (f *fakeNotifications) MarkRead(_ context.Context, _ service.Principal, id int64) error {
	f.gotMarkID = id
	return f.err
}

func (f *fakeNotifications) MarkAllRead(context.Context, service.Principal) error {
	f.markedAll = true
	return f.err
}

func newNotificationsServer(t *testing.T, fake *fakeNotifications) *httptest.Server {
	t.Helper()
	fixture, deps := authFixture()
	deps.Auth = &cookieAuth{fakeAuth: *fixture}
	deps.Dicts, deps.DB, deps.Notifications = fakeDicts{}, okDB{}, fake
	return newTestServer(t, deps)
}

func TestListNotifications(t *testing.T) {
	fake := &fakeNotifications{}
	srv := newNotificationsServer(t, fake)

	resp, body := sessionRequest(t, srv, "lender-verified", "GET", "/api/v1/notifications?unread_only=true&page=1&page_size=2", "", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
	if f := fake.gotFilter; f == nil || !f.UnreadOnly || f.Page != 1 || f.PageSize != 2 {
		t.Fatalf("filter = %+v", fake.gotFilter)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	items, _ := raw["items"].([]any)
	if raw["total"] != float64(3) || raw["unread_total"] != float64(5) || raw["page_size"] != float64(2) || len(items) != 1 {
		t.Fatalf("body = %s", body)
	}
	item, _ := items[0].(map[string]any)
	if item["id"] != float64(7) || item["type"] != "offer_received" || item["entity_type"] != "offer" ||
		item["is_read"] != false || item["created_at"] != "2026-10-24T12:00:00Z" {
		t.Fatalf("item = %v", item)
	}
}

func TestListNotificationsWithoutSession(t *testing.T) {
	srv := newNotificationsServer(t, &fakeNotifications{})

	resp, body := sessionRequest(t, srv, "", "GET", "/api/v1/notifications", "", nil)

	wantError(t, resp, body, http.StatusUnauthorized, "UNAUTHORIZED", "")
}

func TestMarkNotificationRead(t *testing.T) {
	t.Run("своё — 204", func(t *testing.T) {
		fake := &fakeNotifications{}
		srv := newNotificationsServer(t, fake)

		resp, body := sessionRequest(t, srv, "borrower-verified", "POST", "/api/v1/notifications/42/read", "", nil)

		if resp.StatusCode != http.StatusNoContent || len(body) != 0 || fake.gotMarkID != 42 {
			t.Fatalf("status = %d, id %d; body %s", resp.StatusCode, fake.gotMarkID, body)
		}
	})
	t.Run("чужое — 404", func(t *testing.T) {
		srv := newNotificationsServer(t, &fakeNotifications{err: apperr.NotFound("Уведомление не найдено")})

		resp, body := sessionRequest(t, srv, "borrower-verified", "POST", "/api/v1/notifications/999/read", "", nil)

		wantError(t, resp, body, http.StatusNotFound, "NOT_FOUND", "")
	})
	t.Run("не число — 400", func(t *testing.T) {
		srv := newNotificationsServer(t, &fakeNotifications{})

		resp, body := sessionRequest(t, srv, "borrower-verified", "POST", "/api/v1/notifications/abc/read", "", nil)

		wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "notificationId")
	})
}

func TestMarkAllNotificationsRead(t *testing.T) {
	t.Run("204 без тела", func(t *testing.T) {
		fake := &fakeNotifications{}
		srv := newNotificationsServer(t, fake)

		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/notifications/read-all", "", nil)

		if resp.StatusCode != http.StatusNoContent || len(body) != 0 || !fake.markedAll {
			t.Fatalf("status = %d, marked %v; body %s", resp.StatusCode, fake.markedAll, body)
		}
	})
	t.Run("без X-CSRF-Token — 403", func(t *testing.T) {
		fake := &fakeNotifications{}
		srv := newNotificationsServer(t, fake)
		req, err := http.NewRequest("POST", srv.URL+"/api/v1/notifications/read-all", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: "admin"})
		req.AddCookie(&http.Cookie{Name: middleware.CSRFCookieName, Value: "csrf"})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden || fake.markedAll {
			t.Fatalf("status = %d, marked %v", resp.StatusCode, fake.markedAll)
		}
	})
}
