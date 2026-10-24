package middleware

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// Что разрешено фронту на другом origin. Методы и заголовки — ровно те, что шлёт SPA:
// X-CSRF-Token — double-submit, X-Request-ID — сквозной id запроса.
var (
	corsAllowMethods = strings.Join([]string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete}, ", ")
	corsAllowHeaders = strings.Join([]string{"Content-Type", CSRFHeaderName, RequestIDHeader}, ", ")
	// Content-Disposition — имя файла у GET /documents/{id}/file: без него фронт на другом
	// origin не прочитает исходное имя документа.
	corsExposeHeaders   = strings.Join([]string{"Retry-After", RequestIDHeader, "Content-Disposition"}, ", ")
	corsPreflightMaxAge = strconv.Itoa(600) // секунд: браузер не повторяет preflight 10 минут
)

// CORS открывает API origin'ам из allowed — запасной путь, когда фронт не за тем же
// origin (основной путь — общий origin через прокси, тогда список пуст и CORS выключен).
//
// Origin сравнивается точно: «*» с credentials браузер не примет, а отражение любого
// Origin открыло бы API с cookie сессии любому сайту. Запрос с чужим или без Origin
// проходит дальше без CORS-заголовков — браузер сам не отдаст ответ чужой странице.
func CORS(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(allowed) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Ответ зависит от Origin — кэш не должен отдать его другому origin.
			w.Header().Add("Vary", "Origin")
			origin := r.Header.Get("Origin")
			if origin == "" || !slices.Contains(allowed, origin) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			// Preflight отвечаем здесь: дальше Authenticate отдал бы 401 (cookie в preflight
			// нет), а валидатор — 404 (OPTIONS в спеке нет).
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
				w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
				w.Header().Set("Access-Control-Max-Age", corsPreflightMaxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Access-Control-Expose-Headers", corsExposeHeaders)
			next.ServeHTTP(w, r)
		})
	}
}
