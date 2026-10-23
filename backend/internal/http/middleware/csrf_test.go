package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
)

// TestCSRFUsesAccessRule: CSRF проверяется у непубличной операции спеки с небезопасным
// методом. Правило операции кладёт Authorize; здесь его кладёт сам тест.
func TestCSRFUsesAccessRule(t *testing.T) {
	protected := &AccessRule{Roles: []string{"borrower"}}
	public := &AccessRule{Public: true}
	cases := []struct {
		name       string
		method     string
		path       string
		rule       *AccessRule // nil — путь не из спеки
		cookie     string
		header     string
		wantStatus int
		wantCode   string
	}{
		{name: "public login without header", method: http.MethodPost, path: "/api/v1/auth/login", rule: public, wantStatus: http.StatusNoContent},
		{name: "protected post without cookie", method: http.MethodPost, path: "/api/v1/applications", rule: protected, wantStatus: http.StatusForbidden, wantCode: string(apperr.CodeCSRFInvalid)},
		{name: "protected post without header", method: http.MethodPost, path: "/api/v1/applications", rule: protected, cookie: "csrf", wantStatus: http.StatusForbidden, wantCode: string(apperr.CodeCSRFInvalid)},
		{name: "mismatched tokens", method: http.MethodPost, path: "/api/v1/applications", rule: protected, cookie: "cookie-token", header: "header-token", wantStatus: http.StatusForbidden, wantCode: string(apperr.CodeCSRFInvalid)},
		{name: "matching tokens", method: http.MethodPost, path: "/api/v1/applications", rule: protected, cookie: "same", header: "same", wantStatus: http.StatusNoContent},
		{name: "safe get", method: http.MethodGet, path: "/api/v1/applications", rule: protected, wantStatus: http.StatusNoContent},
		{name: "path not in spec", method: http.MethodPost, path: "/api/v1/nope", wantStatus: http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			handler := CSRF(testWriteError)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.rule != nil {
				req = req.WithContext(context.WithValue(req.Context(), accessRuleContextKey{}, *tc.rule))
			}
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: tc.cookie})
			}
			if tc.header != "" {
				req.Header.Set(CSRFHeaderName, tc.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if nextCalled != (tc.wantStatus == http.StatusNoContent) {
				t.Fatalf("next called = %v", nextCalled)
			}
			if tc.wantCode != "" {
				var body struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if body.Error.Code != tc.wantCode {
					t.Fatalf("response code = %q, want %q", body.Error.Code, tc.wantCode)
				}
			}
		})
	}
}
