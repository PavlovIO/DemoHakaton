package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
)

func TestCSRFMiddleware(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		path       string
		cookie     string
		header     string
		wantStatus int
		wantCode   string
	}{
		{name: "protected post without cookie", method: http.MethodPost, path: "/api/v1/transfers", wantStatus: http.StatusForbidden, wantCode: string(apperr.CodeCSRFInvalid)},
		{name: "protected post without header", method: http.MethodPost, path: "/api/v1/transfers", cookie: "csrf", wantStatus: http.StatusForbidden, wantCode: string(apperr.CodeCSRFInvalid)},
		{name: "mismatched tokens", method: http.MethodPost, path: "/api/v1/transfers", cookie: "cookie-token", header: "header-token", wantStatus: http.StatusForbidden, wantCode: string(apperr.CodeCSRFInvalid)},
		{name: "matching tokens", method: http.MethodPost, path: "/api/v1/transfers", cookie: "same", header: "same", wantStatus: http.StatusNoContent},
		{name: "safe get", method: http.MethodGet, path: "/api/v1/transfers", wantStatus: http.StatusNoContent},
		{name: "public login", method: http.MethodPost, path: "/api/v1/auth/login", wantStatus: http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			handler := CSRF()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(tc.method, tc.path, nil)
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
