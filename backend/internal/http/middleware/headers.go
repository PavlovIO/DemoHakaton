package middleware

import (
	"net/http"
	"strings"
)

// hstsValue — год, с поддоменами, без preload: preload-список не откатить быстро.
const hstsValue = "max-age=31536000; includeSubDomains"

// SecurityHeaders устанавливает защитные HTTP-заголовки.
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			w.Header().Set("Cache-Control", "no-store")
			// Подделанный X-Forwarded-Proto по HTTP безвреден: браузер игнорирует HSTS,
			// пришедший не по HTTPS (RFC 6797 §8.1).
			if isHTTPS(r) {
				w.Header().Set("Strict-Transport-Security", hstsValue)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isHTTPS — запрос пришёл по TLS напрямую или через терминирующий TLS прокси.
func isHTTPS(r *http.Request) bool {
	// Цепочка прокси шлёт «https, http»: схема клиента — первое значение.
	proto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(proto), "https")
}
