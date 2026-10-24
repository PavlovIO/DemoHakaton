package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// cookieAuth выдаёт principal по значению cookie session: "borrower-verified",
// "borrower-draft", "lender-verified", "admin"; "expired" и прочее — UNAUTHORIZED.
type cookieAuth struct{ fakeAuth }

func (cookieAuth) Authenticate(_ context.Context, token string) (*service.Principal, error) {
	org := uuid.New()
	switch token {
	case "borrower-verified":
		return &service.Principal{UserID: uuid.New(), OrgID: &org, Role: "borrower", KycStatus: "verified"}, nil
	case "borrower-draft":
		return &service.Principal{UserID: uuid.New(), OrgID: &org, Role: "borrower", KycStatus: "draft"}, nil
	case "lender-verified":
		return &service.Principal{UserID: uuid.New(), OrgID: &org, Role: "lender", KycStatus: "verified"}, nil
	case "admin":
		return &service.Principal{UserID: uuid.New(), Role: "admin"}, nil
	default:
		return nil, apperr.Unauthorized("Сессия недействительна")
	}
}

const validApplicationBody = `{"amount":"1000000.00","currency":"USD","target_country_code":"DE",` +
	`"purpose":"Оплата поставки оборудования","term_months":12,"transfer_method":"swift"}`

// TestAccessOrder — порядок проверок на настоящем роутере: доступ (401/403) раньше CSRF,
// CSRF раньше валидатора, несуществующий путь — 404 независимо от сессии.
func TestAccessOrder(t *testing.T) {
	fixture, deps := authFixture()
	deps.Auth = &cookieAuth{fakeAuth: *fixture}
	deps.Dicts, deps.DB, deps.Applications = fakeDicts{}, okDB{}, &fakeApplications{app: testApplication()}
	srv := newTestServer(t, deps)

	session := func(v string) *http.Cookie { return &http.Cookie{Name: middleware.SessionCookieName, Value: v} }
	csrf := &http.Cookie{Name: middleware.CSRFCookieName, Value: "csrf"}

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		cookies    []*http.Cookie
		withCSRF   bool
		wantStatus int
		wantCode   string
	}{
		{name: "без cookie", method: "GET", path: "/api/v1/applications", wantStatus: 401, wantCode: "UNAUTHORIZED"},
		{name: "протухшая cookie", method: "GET", path: "/api/v1/applications", cookies: []*http.Cookie{session("expired")}, wantStatus: 401, wantCode: "UNAUTHORIZED"},
		{name: "протухшая cookie не мешает login", method: "POST", path: "/api/v1/auth/login", body: loginBody, cookies: []*http.Cookie{session("expired")}, wantStatus: 200},
		{name: "KYC раньше CSRF", method: "POST", path: "/api/v1/applications", body: validApplicationBody, cookies: []*http.Cookie{session("borrower-draft")}, wantStatus: 403, wantCode: "KYC_NOT_VERIFIED"},
		{name: "валидатор после доступа", method: "POST", path: "/api/v1/applications", body: `{}`, cookies: []*http.Cookie{session("borrower-verified"), csrf}, withCSRF: true, wantStatus: 400, wantCode: "VALIDATION_ERROR"},
		{name: "всё прошло — хендлер", method: "POST", path: "/api/v1/applications", body: validApplicationBody, cookies: []*http.Cookie{session("borrower-verified"), csrf}, withCSRF: true, wantStatus: 201},
		{name: "нет пути, с cookie", method: "GET", path: "/api/v1/nope", cookies: []*http.Cookie{session("admin")}, wantStatus: 404, wantCode: "NOT_FOUND"},
		{name: "нет пути, без cookie", method: "GET", path: "/api/v1/nope", wantStatus: 404, wantCode: "NOT_FOUND"},
		{name: "HEAD healthz", method: "HEAD", path: "/api/v1/healthz", wantStatus: 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, srv.URL+tt.path, strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tt.withCSRF {
				req.Header.Set(middleware.CSRFHeaderName, csrf.Value)
			}
			for _, c := range tt.cookies {
				req.AddCookie(c)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", resp.StatusCode, tt.wantStatus, body)
			}
			if tt.wantCode == "" {
				return
			}
			var e api.Error
			if err := json.Unmarshal(body, &e); err != nil {
				t.Fatalf("decode %s: %v", body, err)
			}
			if e.Error.Code != tt.wantCode {
				t.Fatalf("code = %s, want %s; body %s", e.Error.Code, tt.wantCode, body)
			}
		})
	}
}
