package middleware

import (
	"context"
	"log/slog"
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
)

// Authenticator проверяет токен сессии и возвращает principal.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*service.Principal, error)
}

// Authenticate кладёт principal в контекст, если сессия валидна. Он «мягкий»: без cookie
// или с невалидной сессией запрос идёт дальше без principal — отвечать ли 401, решает
// Authorize, потому что только он знает, публичная ли операция (протухшая cookie не должна
// мешать login). Сбой проверки (БД) — 500.
func Authenticate(a Authenticator, writeErr ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}

			principal, err := a.Authenticate(r.Context(), cookie.Value)
			if err != nil {
				if appErr, ok := apperr.As(err); ok && appErr.Code == apperr.CodeUnauthorized {
					next.ServeHTTP(w, r)
					return
				}
				writeErr(w, r, err)
				return
			}
			if principal == nil {
				next.ServeHTTP(w, r)
				return
			}

			logPrincipal(r.Context(), *principal)
			ctx := context.WithValue(r.Context(), principalContextKey, *principal)
			ctx = context.WithValue(ctx, sessionTokenContextKey, cookie.Value)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// logPrincipal дописывает пользователя в строки лога запроса; у admin организации нет.
func logPrincipal(ctx context.Context, p service.Principal) {
	attrs := []slog.Attr{slog.String("user_id", p.UserID.String())}
	if p.OrgID != nil {
		attrs = append(attrs, slog.String("org_id", p.OrgID.String()))
	}
	WithLogAttrs(ctx, attrs...)
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
