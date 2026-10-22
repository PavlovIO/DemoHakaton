package middleware

import (
	"context"
	"encoding/json"
	"errors"
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
func Authenticate(a Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublic(r) {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				writeMiddlewareError(w, r, apperr.Unauthorized("Сессия недействительна"))
				return
			}

			principal, err := a.Authenticate(r.Context(), cookie.Value)
			if err != nil {
				if _, ok := apperr.As(err); !ok || errors.Is(err, &apperr.Error{Code: apperr.CodeInternal}) {
					writeMiddlewareError(w, r, err)
					return
				}
				writeMiddlewareError(w, r, apperr.Unauthorized("Сессия недействительна"))
				return
			}
			if principal == nil {
				writeMiddlewareError(w, r, apperr.Unauthorized("Сессия недействительна"))
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
	methods, ok := publicRoutes[r.Method]
	if !ok {
		return false
	}
	_, ok = methods[r.URL.Path]
	return ok
}

type middlewareErrorEnvelope struct {
	Error middlewareErrorBody `json:"error"`
}

type middlewareErrorBody struct {
	Code    string                  `json:"code"`
	Message string                  `json:"message"`
	Details []middlewareErrorDetail `json:"details,omitempty"`
}

type middlewareErrorDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func writeMiddlewareError(w http.ResponseWriter, r *http.Request, err error) {
	appErr, ok := apperr.As(err)
	if !ok || errors.Is(err, &apperr.Error{Code: apperr.CodeInternal}) {
		appErr = apperr.New(apperr.CodeInternal, "Внутренняя ошибка сервера")
	}

	status := http.StatusInternalServerError
	switch appErr.Code {
	case apperr.CodeUnauthorized:
		status = http.StatusUnauthorized
	case apperr.CodeForbidden, apperr.CodeCSRFInvalid, apperr.CodeKYCNotVerified:
		status = http.StatusForbidden
	case apperr.CodeValidation:
		status = http.StatusBadRequest
	case apperr.CodeNotFound:
		status = http.StatusNotFound
	case apperr.CodeConflict, apperr.CodeInvalidTransition:
		status = http.StatusConflict
	case apperr.CodeRateLimited:
		status = http.StatusTooManyRequests
	case apperr.CodeNotImplemented:
		status = http.StatusNotImplemented
	case apperr.CodeUnavailable:
		status = http.StatusServiceUnavailable
	}

	details := make([]middlewareErrorDetail, len(appErr.Details))
	for i, detail := range appErr.Details {
		details[i] = middlewareErrorDetail{Field: detail.Field, Message: detail.Message}
	}
	body := middlewareErrorEnvelope{Error: middlewareErrorBody{
		Code:    string(appErr.Code),
		Message: appErr.Message,
		Details: details,
	}}
	attrs := []any{"method", r.Method, "path", r.URL.Path, "status", status, "code", body.Error.Code, "err", err}
	if status >= http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "request failed", attrs...)
	} else {
		slog.DebugContext(r.Context(), "request rejected", attrs...)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
