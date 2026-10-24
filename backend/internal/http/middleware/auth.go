package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

const (
	// SessionCookieName — имя cookie с токеном сессии.
	SessionCookieName = "session"
)

type contextKey struct {
	name string
}

var (
	principalContextKey    = contextKey{name: "principal"}
	sessionTokenContextKey = contextKey{name: "session_token"}

	publicRoutes = map[string]map[string]struct{}{
		http.MethodGet: {
			"/api/v1/healthz":                       {},
			"/api/v1/dictionaries/countries":        {},
			"/api/v1/dictionaries/currencies":       {},
			"/api/v1/dictionaries/transfer-methods": {},
			"/api/v1/registry/lookup":               {},
		},
		http.MethodPost: {
			"/api/v1/auth/register": {},
			"/api/v1/auth/login":    {},
		},
	}
)

// Authenticator проверяет токен сессии и возвращает principal.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*service.Principal, error)
}

// Authenticate пропускает публичные маршруты и проверяет сессию на остальных.
// Отказ пишется через writeErr (handler.WriteError) — тот же формат и статусы, что у хендлеров.
func Authenticate(a Authenticator, writeErr ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublic(r) {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				writeErr(w, r, apperr.Unauthorized("Сессия недействительна"))
				return
			}

			principal, err := a.Authenticate(r.Context(), cookie.Value)
			if err != nil {
				if _, ok := apperr.As(err); !ok || errors.Is(err, &apperr.Error{Code: apperr.CodeInternal}) {
					writeErr(w, r, err)
					return
				}
				writeErr(w, r, apperr.Unauthorized("Сессия недействительна"))
				return
			}
			if principal == nil {
				writeErr(w, r, apperr.Unauthorized("Сессия недействительна"))
				return
			}

			ctx := context.WithValue(r.Context(), principalContextKey, *principal)
			ctx = context.WithValue(ctx, sessionTokenContextKey, cookie.Value)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// PrincipalFromContext возвращает principal из контекста запроса.
func PrincipalFromContext(ctx context.Context) (service.Principal, bool) {
	principal, ok := ctx.Value(principalContextKey).(service.Principal)
	return principal, ok
}

// SessionTokenFromContext возвращает исходный токен сессии из контекста запроса.
func SessionTokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(sessionTokenContextKey).(string)
	return token, ok
}

func isPublic(r *http.Request) bool {
	method := r.Method
	if method == http.MethodHead { // HEAD обслуживается теми же маршрутами, что GET
		method = http.MethodGet
	}
	methods, ok := publicRoutes[method]
	if !ok {
		return false
	}
	_, ok = methods[r.URL.Path]
	return ok
}
