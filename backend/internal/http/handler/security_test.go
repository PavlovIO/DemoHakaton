package handler_test

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
)

// Защитные заголовки — на любом ответе API, включая 404 и отбитые Authenticate/Authorize.
func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	srv := newTestServer(t, handler.Deps{DB: okDB{}})
	want := map[string]string{
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
		"X-Frame-Options":         "DENY",
	}

	for _, tc := range []struct {
		name, method, path string
		wantStatus         int
	}{
		{"healthz", http.MethodGet, "/api/v1/healthz", http.StatusOK},
		{"unknown path", http.MethodGet, "/api/v1/nope", http.StatusNotFound},
		{"logout without session", http.MethodPost, "/api/v1/auth/logout", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := request(t, srv, tc.method, tc.path, "")
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, tc.wantStatus, body)
			}
			for name, value := range want {
				if got := resp.Header.Get(name); got != value {
					t.Errorf("%s = %q, want %q", name, got, value)
				}
			}
		})
	}
}

// HSTS — только для запросов по HTTPS: напрямую (TLS) или через прокси (X-Forwarded-Proto).
func TestSecurityHeadersHSTS(t *testing.T) {
	router := handler.NewRouter(handler.New(handler.Deps{DB: okDB{}, Auth: testAuthService()}))
	const wantHSTS = "max-age=31536000; includeSubDomains"

	for _, tc := range []struct {
		name    string
		prepare func(*http.Request)
		want    string
	}{
		{"plain http", func(*http.Request) {}, ""},
		{"forwarded https", func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") }, wantHSTS},
		{"forwarded http", func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "http") }, ""},
		{"direct tls", func(r *http.Request) { r.TLS = &tls.ConnectionState{} }, wantHSTS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
			tc.prepare(req)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if got := rec.Header().Get("Strict-Transport-Security"); got != tc.want {
				t.Fatalf("Strict-Transport-Security = %q, want %q", got, tc.want)
			}
		})
	}
}
