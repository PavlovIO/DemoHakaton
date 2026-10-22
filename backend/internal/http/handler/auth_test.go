package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/authapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/google/uuid"
)

type fakeAuth struct {
	session     *service.Session
	me          *service.Me
	principal   *service.Principal
	registerErr error
	loginErr    error
	logoutErr   error
	meErr       error
	loginCalls  int
	logoutCalls int
}

func (f *fakeAuth) Register(context.Context, service.RegisterInput, service.ClientMeta) (*service.Session, error) {
	return f.session, f.registerErr
}

func (f *fakeAuth) Login(context.Context, string, string, service.ClientMeta) (*service.Session, error) {
	f.loginCalls++
	return f.session, f.loginErr
}

func (f *fakeAuth) Logout(context.Context, string) error {
	f.logoutCalls++
	return f.logoutErr
}

func (f *fakeAuth) Me(context.Context, service.Principal) (*service.Me, error) {
	return f.me, f.meErr
}

func (f *fakeAuth) Authenticate(context.Context, string) (*service.Principal, error) {
	return f.principal, nil
}

func addValidTestSession(req *http.Request) {
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: "test-session"})
	req.AddCookie(&http.Cookie{Name: middleware.CSRFCookieName, Value: "test-csrf"})
	req.Header.Set(middleware.CSRFHeaderName, "test-csrf")
}

func testAuthService() *fakeAuth {
	return &fakeAuth{
		principal: &service.Principal{UserID: uuid.New(), Role: "borrower"},
	}
}

func authFixture() (*fakeAuth, handler.Deps) {
	userID := uuid.New()
	orgID := uuid.New()
	me := &service.Me{
		User: service.User{
			ID: userID, Email: "user@example.com", Role: "lender",
			Status: "active", OrganizationID: &orgID, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		Organization: &service.Org{
			ID: orgID, Type: "lender", Name: "Example", CountryCode: "CY",
			RegistrationNumber: "REG-1", LegalAddress: "Address", KycStatus: "draft",
		},
	}
	auth := &fakeAuth{
		me:        me,
		principal: &service.Principal{UserID: userID, OrgID: &orgID, Role: "lender"},
		session: &service.Session{
			Token: "session-value", CSRFToken: "csrf-value", ExpiresAt: time.Now().Add(time.Hour), Me: me,
		},
	}
	return auth, handler.Deps{Auth: auth, CookieSecure: false}
}

func request(t *testing.T, server *httptest.Server, method, path, body string, cookies ...*http.Cookie) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, responseBody
}

const registerBody = `{"email":"user@example.com","password":"password1234","role":"lender","organization":{"name":"Example","country_code":"CY","registration_number":"REG-1","legal_address":"Address"}}`
const loginBody = `{"email":"user@example.com","password":"password1234"}`

func TestRegisterAndLoginSetSessionCookies(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantCalls  int
	}{
		{name: "register", method: http.MethodPost, path: "/api/v1/auth/register", body: registerBody, wantStatus: http.StatusCreated},
		{name: "login", method: http.MethodPost, path: "/api/v1/auth/login", body: loginBody, wantStatus: http.StatusOK, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth, deps := authFixture()
			server := newTestServer(t, deps)
			resp, body := request(t, server, tc.method, tc.path, tc.body)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, tc.wantStatus, body)
			}
			cookies := resp.Cookies()
			if len(cookies) != 2 {
				t.Fatalf("Set-Cookie count = %d, want 2: %#v", len(cookies), cookies)
			}
			got := map[string]*http.Cookie{}
			for _, cookie := range cookies {
				got[cookie.Name] = cookie
			}
			sessionCookie := got[middleware.SessionCookieName]
			csrfCookie := got[middleware.CSRFCookieName]
			if sessionCookie == nil || !sessionCookie.HttpOnly || sessionCookie.Secure || sessionCookie.SameSite != http.SameSiteLaxMode {
				t.Fatalf("session cookie = %#v", sessionCookie)
			}
			if csrfCookie == nil || csrfCookie.HttpOnly || csrfCookie.Secure || csrfCookie.SameSite != http.SameSiteLaxMode {
				t.Fatalf("CSRF cookie = %#v", csrfCookie)
			}
			if sessionCookie.Path != "/" || csrfCookie.Path != "/" || sessionCookie.Expires.IsZero() || csrfCookie.Expires.IsZero() {
				t.Fatalf("cookie attributes: session=%#v csrf=%#v", sessionCookie, csrfCookie)
			}
			if auth.loginCalls != tc.wantCalls {
				t.Fatalf("Login calls = %d, want %d", auth.loginCalls, tc.wantCalls)
			}
			var gotMe api.Me
			if err := json.Unmarshal(body, &gotMe); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if gotMe.Organization == nil || gotMe.Organization.Beneficiaries == nil {
				t.Fatalf("organization/beneficiaries = %#v, want organization and empty slice", gotMe.Organization)
			}
		})
	}
}

func TestAuthErrorsAreMapped(t *testing.T) {
	t.Run("login unauthorized", func(t *testing.T) {
		auth, deps := authFixture()
		auth.loginErr = apperr.Unauthorized("Неверный email или пароль")
		server := newTestServer(t, deps)
		resp, body := request(t, server, http.MethodPost, "/api/v1/auth/login", loginBody)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
		decoded := decodeError(t, body)
		if decoded.Error.Code != string(apperr.CodeUnauthorized) {
			t.Fatalf("error code = %q", decoded.Error.Code)
		}
	})

	t.Run("register conflict", func(t *testing.T) {
		auth, deps := authFixture()
		auth.registerErr = apperr.Conflict("Уже существует")
		server := newTestServer(t, deps)
		resp, body := request(t, server, http.MethodPost, "/api/v1/auth/register", registerBody)
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body: %s", resp.StatusCode, body)
		}
		if decoded := decodeError(t, body); decoded.Error.Code != string(apperr.CodeConflict) {
			t.Fatalf("error code = %q", decoded.Error.Code)
		}
	})
}

func TestLogoutClearsCookies(t *testing.T) {
	auth, deps := authFixture()
	server := newTestServer(t, deps)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/auth/logout", nil)
	if err != nil {
		t.Fatalf("new logout request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: "session-value"})
	req.AddCookie(&http.Cookie{Name: middleware.CSRFCookieName, Value: "csrf-value"})
	req.Header.Set(middleware.CSRFHeaderName, "csrf-value")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("logout request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read logout body: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent || len(body) != 0 {
		t.Fatalf("status/body = %d/%q, want 204/empty", resp.StatusCode, body)
	}
	cookies := resp.Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cleared cookie count = %d, want 2", len(cookies))
	}
	for _, cookie := range cookies {
		if cookie.MaxAge >= 0 {
			t.Errorf("cookie %q MaxAge = %d, want negative", cookie.Name, cookie.MaxAge)
		}
	}
	if auth.logoutCalls != 1 {
		t.Fatalf("Logout calls = %d, want 1", auth.logoutCalls)
	}
}

func TestGetMeRequiresPrincipalAndMapsAdmin(t *testing.T) {
	auth, deps := authFixture()
	h := handler.New(deps)
	_, err := h.GetMe(context.Background(), authapi.GetMeRequestObject{})
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeUnauthorized {
		t.Fatalf("GetMe() error = %v, want unauthorized", err)
	}

	adminID := uuid.New()
	auth.principal = &service.Principal{UserID: adminID, Role: "admin"}
	auth.me = &service.Me{User: service.User{ID: adminID, Email: "admin@example.com", Role: "admin", Status: "active"}}
	server := newTestServer(t, deps)
	resp, body := request(t, server, http.MethodGet, "/api/v1/auth/me", "",
		&http.Cookie{Name: middleware.SessionCookieName, Value: "session-value"},
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, body)
	}
	if !bytes.Contains(body, []byte(`"organization":null`)) {
		t.Fatalf("body = %s, want organization null", body)
	}
}

func TestRouterAuthAndCSRFOrder(t *testing.T) {
	auth, deps := authFixture()
	server := newTestServer(t, deps)

	resp, body := request(t, server, http.MethodPost, "/api/v1/auth/login", loginBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200; body: %s", resp.StatusCode, body)
	}
	if auth.loginCalls != 1 {
		t.Fatalf("login calls = %d, want 1", auth.loginCalls)
	}

	resp, body = request(t, server, http.MethodPost, "/api/v1/auth/logout", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("logout without session status = %d, want 401; body: %s", resp.StatusCode, body)
	}

	resp, body = request(t, server, http.MethodPost, "/api/v1/auth/logout", "",
		&http.Cookie{Name: middleware.SessionCookieName, Value: "session-value"},
	)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("logout without CSRF status = %d, want 403; body: %s", resp.StatusCode, body)
	}
	if auth.logoutCalls != 0 {
		t.Fatalf("Logout calls = %d, want 0", auth.logoutCalls)
	}
}

func TestAuthErrorResponseBodyHasNoRawError(t *testing.T) {
	auth, deps := authFixture()
	auth.loginErr = errors.New("database connection detail")
	server := newTestServer(t, deps)
	resp, body := request(t, server, http.MethodPost, "/api/v1/auth/login", loginBody)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if bytes.Contains(body, []byte("database connection detail")) {
		t.Fatalf("internal error leaked: %s", body)
	}
}
