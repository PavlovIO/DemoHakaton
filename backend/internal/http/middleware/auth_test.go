package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/google/uuid"
)

type fakeAuthenticator struct {
	principal *service.Principal
	err       error
	token     string
}

func (a *fakeAuthenticator) Authenticate(_ context.Context, token string) (*service.Principal, error) {
	a.token = token
	return a.principal, a.err
}

func TestAuthenticateMiddleware(t *testing.T) {
	userID := uuid.New()
	orgID := uuid.New()
	// Authenticate «мягкий»: 401 решает Authorize, потому что только он знает, публичная ли
	// операция. Сюда доходит только решение «есть principal или нет» и 500 при сбое БД.
	cases := []struct {
		name          string
		method        string
		path          string
		cookie        string
		authErr       error
		wantStatus    int
		wantCode      string
		wantNext      bool
		wantPrincipal bool
	}{
		{name: "no cookie", path: "/api/v1/users/me", wantStatus: http.StatusNoContent, wantNext: true},
		{name: "expired session", path: "/api/v1/users/me", cookie: "bad", authErr: apperr.Unauthorized("expired"), wantStatus: http.StatusNoContent, wantNext: true},
		{name: "non application error", path: "/api/v1/users/me", cookie: "bad", authErr: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError, wantCode: string(apperr.CodeInternal)},
		{name: "valid token", path: "/api/v1/users/me", cookie: "valid-token", wantStatus: http.StatusNoContent, wantNext: true, wantPrincipal: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authenticator := &fakeAuthenticator{
				principal: &service.Principal{UserID: userID, OrgID: &orgID, Role: "lender"},
				err:       tc.authErr,
			}
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				_, hasPrincipal := PrincipalFromContext(r.Context())
				if hasPrincipal != tc.wantPrincipal {
					t.Errorf("principal in context = %v, want %v", hasPrincipal, tc.wantPrincipal)
				}
				if tc.wantPrincipal {
					principal, ok := PrincipalFromContext(r.Context())
					if !ok || principal.UserID != userID || principal.OrgID == nil || *principal.OrgID != orgID || principal.Role != "lender" {
						t.Errorf("PrincipalFromContext() = %#v, %v", principal, ok)
					}
					token, ok := SessionTokenFromContext(r.Context())
					if !ok || token != tc.cookie {
						t.Errorf("SessionTokenFromContext() = %q, %v", token, ok)
					}
				}
				w.WriteHeader(http.StatusNoContent)
			})
			handler := Authenticate(authenticator, testWriteError)(next)
			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, tc.path, nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tc.cookie})
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if nextCalled != tc.wantNext {
				t.Fatalf("next called = %v, want %v", nextCalled, tc.wantNext)
			}
			if tc.cookie != "" && authenticator.token != tc.cookie {
				t.Fatalf("Authenticate() token = %q, want %q", authenticator.token, tc.cookie)
			}
			if tc.wantCode != "" {
				var body struct {
					Error struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if body.Error.Code != tc.wantCode || body.Error.Message == "" {
					t.Fatalf("error body = %#v, want code %q and message", body.Error, tc.wantCode)
				}
			}
		})
	}
}
