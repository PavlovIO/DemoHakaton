package handler_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
)

// CSRF на бизнес-ручке с валидной сессией: без X-CSRF-Token или с чужим значением
// мутация не доходит до сервиса.
func TestCSRFOnBusinessEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     string
		path       string
		csrfHeader string // "" — заголовка нет
		wantStatus int
	}{
		{"post without header", http.MethodPost, "/api/v1/notifications/read-all", "", http.StatusForbidden},
		{"post with mismatched header", http.MethodPost, "/api/v1/notifications/read-all", "other", http.StatusForbidden},
		{"post with matching header", http.MethodPost, "/api/v1/notifications/read-all", "csrf", http.StatusNoContent},
		{"get without header", http.MethodGet, "/api/v1/notifications", "", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeNotifications{}
			srv := newNotificationsServer(t, fake)
			req, err := http.NewRequest(tc.method, srv.URL+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: "lender-verified"})
			req.AddCookie(&http.Cookie{Name: middleware.CSRFCookieName, Value: "csrf"})
			if tc.csrfHeader != "" {
				req.Header.Set(middleware.CSRFHeaderName, tc.csrfHeader)
			}
			resp, body := do(t, req)

			if tc.wantStatus != http.StatusForbidden {
				if resp.StatusCode != tc.wantStatus {
					t.Fatalf("status = %d, want %d; body %s", resp.StatusCode, tc.wantStatus, body)
				}
				if tc.method == http.MethodPost && !fake.markedAll {
					t.Fatal("MarkAllRead not called with valid CSRF token")
				}
				return
			}
			wantError(t, resp, body, http.StatusForbidden, "CSRF_INVALID", "")
			if fake.markedAll {
				t.Fatal("MarkAllRead called without valid CSRF token")
			}
		})
	}
}

// Публичные POST (register, login) CSRF не проверяют — cookie ещё нет. Кросс-сайтовая
// HTML-форма не умеет слать application/json, поэтому text/plain отбивает валидатор.
func TestPublicPostRejectsNonJSONContentType(t *testing.T) {
	fake, deps := authFixture()
	srv := newTestServer(t, deps)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, body := do(t, req)

	wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "")
	if fake.loginCalls != 0 {
		t.Fatalf("Login called %d times for text/plain body", fake.loginCalls)
	}
}

func do(t *testing.T, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, body
}
