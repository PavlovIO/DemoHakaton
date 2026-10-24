package handler_test

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
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

// CORS — запасной путь для фронта на другом origin: только точное совпадение со списком,
// с credentials; preflight отвечает сам, не доходя до Authenticate и валидатора спеки.
func TestCORS(t *testing.T) {
	const allowed = "http://localhost:5173"

	for _, tc := range []struct {
		name        string
		allowList   []string
		method      string
		origin      string
		preflight   bool
		wantStatus  int
		wantAllowed bool
	}{
		{"allowed origin", []string{allowed}, http.MethodGet, allowed, false, http.StatusOK, true},
		{"foreign origin", []string{allowed}, http.MethodGet, "https://evil.example", false, http.StatusOK, false},
		{"no origin", []string{allowed}, http.MethodGet, "", false, http.StatusOK, false},
		{"empty allow list", nil, http.MethodGet, allowed, false, http.StatusOK, false},
		{"preflight from allowed", []string{allowed}, http.MethodOptions, allowed, true, http.StatusNoContent, true},
		// Чужой preflight проходит дальше как обычный OPTIONS — OPTIONS в спеке нет.
		{"preflight from foreign", []string{allowed}, http.MethodOptions, "https://evil.example", true, http.StatusNotFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t, handler.Deps{DB: okDB{}, CORSAllowedOrigins: tc.allowList})
			// /notifications требует сессию: preflight без cookie не должен получить 401.
			path := "/api/v1/healthz"
			if tc.preflight {
				path = "/api/v1/notifications/read-all"
			}
			req, err := http.NewRequest(tc.method, srv.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.preflight {
				req.Header.Set("Access-Control-Request-Method", http.MethodPost)
				req.Header.Set("Access-Control-Request-Headers", "content-type, x-csrf-token")
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			gotOrigin := resp.Header.Get("Access-Control-Allow-Origin")
			if !tc.wantAllowed {
				if gotOrigin != "" || resp.Header.Get("Access-Control-Allow-Credentials") != "" {
					t.Fatalf("CORS headers for disallowed request: %v", resp.Header)
				}
				return
			}
			if gotOrigin != tc.origin {
				t.Fatalf("Access-Control-Allow-Origin = %q, want %q", gotOrigin, tc.origin)
			}
			if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
				t.Errorf("Access-Control-Allow-Credentials = %q, want true", got)
			}
			if !headerHasToken(resp.Header, "Vary", "Origin") {
				t.Errorf("Vary = %q, want Origin", resp.Header.Values("Vary"))
			}
			if !tc.preflight {
				if got := resp.Header.Get("Access-Control-Expose-Headers"); !strings.Contains(got, "Retry-After") {
					t.Errorf("Access-Control-Expose-Headers = %q, want Retry-After", got)
				}
				return
			}
			if got := resp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-CSRF-Token") {
				t.Errorf("Access-Control-Allow-Headers = %q, want X-CSRF-Token", got)
			}
			if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPost) {
				t.Errorf("Access-Control-Allow-Methods = %q, want POST", got)
			}
			if got := resp.Header.Get("Access-Control-Max-Age"); got != "600" {
				t.Errorf("Access-Control-Max-Age = %q, want 600", got)
			}
		})
	}
}

func headerHasToken(h http.Header, name, token string) bool {
	for _, value := range h.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}
