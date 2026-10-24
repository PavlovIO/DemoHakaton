package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
)

const (
	// CSRFCookieName — имя cookie с CSRF-токеном.
	CSRFCookieName = "csrf_token"
	// CSRFHeaderName — имя HTTP-заголовка с CSRF-токеном.
	CSRFHeaderName = "X-CSRF-Token"
)

// CSRF проверяет совпадение CSRF-токена cookie и заголовка (double-submit) у изменяющих
// запросов к защищённым операциям. Стоит после Authorize: правило операции берётся из
// контекста. Не проверяются: путь не из спеки (404 отдаст валидатор), публичные операции
// (register, login — DECISIONS п. 1) и безопасные методы.
func CSRF(writeErr ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rule, ok := accessRuleFromContext(r.Context())
			if !ok || rule.Public || isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(CSRFCookieName)
			headerToken := r.Header.Get(CSRFHeaderName)
			if err != nil || cookie.Value == "" || headerToken == "" ||
				subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(headerToken)) != 1 {
				writeErr(w, r, apperr.CSRFInvalid())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
