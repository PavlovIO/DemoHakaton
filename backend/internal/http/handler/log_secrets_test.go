package handler_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
)

// Пароль и токены сессии не попадают в лог даже на уровне debug — ни при ошибке
// валидации (её текст WriteError пишет в лог), ни при успешном входе.
func TestLogsDoNotContainSecrets(t *testing.T) {
	const (
		shortPassword = "short-pw1" // 9 символов: minLength 10 → 400 VALIDATION_ERROR
		validPassword = "valid-password-42"
		sessionToken  = "session-token-secret-value"
		csrfToken     = "csrf-token-secret-value"
	)
	registerWith := func(password string) string {
		return `{"email":"user@example.com","password":"` + password + `","role":"lender",` +
			`"organization":{"name":"Example","country_code":"CY","registration_number":"REG-1","legal_address":"Address"}}`
	}
	loginWith := func(password string) string {
		return `{"email":"user@example.com","password":"` + password + `"}`
	}

	for _, tc := range []struct {
		name       string
		path       string
		body       string
		loginErr   error
		session    bool
		wantStatus int
	}{
		{name: "register short password", path: "/api/v1/auth/register", body: registerWith(shortPassword), wantStatus: http.StatusBadRequest},
		{name: "register", path: "/api/v1/auth/register", body: registerWith(validPassword), wantStatus: http.StatusCreated},
		{name: "login wrong password", path: "/api/v1/auth/login", body: loginWith(shortPassword + "0"),
			loginErr: apperr.Unauthorized("Неверный email или пароль"), wantStatus: http.StatusUnauthorized},
		{name: "login", path: "/api/v1/auth/login", body: loginWith(validPassword), wantStatus: http.StatusOK},
		{name: "logout", path: "/api/v1/auth/logout", session: true, wantStatus: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(middleware.ContextHandler{
				Handler: slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}),
			}))
			t.Cleanup(func() { slog.SetDefault(prev) })

			fake, deps := authFixture()
			fake.session.Token, fake.session.CSRFToken = sessionToken, csrfToken
			fake.loginErr = tc.loginErr
			srv := newTestServer(t, deps)

			req, err := http.NewRequest(http.MethodPost, srv.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tc.session {
				req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: sessionToken})
				req.AddCookie(&http.Cookie{Name: middleware.CSRFCookieName, Value: csrfToken})
				req.Header.Set(middleware.CSRFHeaderName, csrfToken)
			}
			resp, body := do(t, req)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", resp.StatusCode, tc.wantStatus, body)
			}

			for _, secret := range []string{shortPassword, validPassword, sessionToken, csrfToken} {
				if strings.Contains(logs.String(), secret) {
					t.Errorf("log contains secret %q:\n%s", secret, logs.String())
				}
			}
		})
	}
}
