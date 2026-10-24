// Package middleware — обёртки net/http вокруг всего API.
//
// Порядок в handler.NewRouter: RequestID → RequestLog → Recover → SecurityHeaders → CORS → MaxBody →
// WithRequestMeta → Authenticate → Authorize → CSRF. RequestID — первым: его id и атрибуты
// пользователя (WithLogAttrs из Authenticate) ContextHandler пишет во все строки лога запроса,
// включая строку RequestLog.
package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// ErrorWriter пишет ошибку в едином формате API (в проде — handler.WriteError).
// middleware не может импортировать handler (цикл импортов), поэтому получает его параметром.
type ErrorWriter func(w http.ResponseWriter, r *http.Request, err error)

// Chain применяет middleware так, что первый в списке — самый внешний.
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// Recover ловит панику в хендлере, пишет стек в лог и отдаёт ответ через onPanic
// (в проде — 500 INTERNAL в формате Error). http.ErrAbortHandler пробрасывается дальше.
func Recover(onPanic func(w http.ResponseWriter, r *http.Request)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}
				slog.ErrorContext(r.Context(), "panic recovered",
					"method", r.Method, "path", r.URL.Path, "panic", rec, "stack", string(debug.Stack()))
				onPanic(w, r)
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequestLog пишет одну строку на запрос: метод, путь, статус, длительность.
func RequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		level := slog.LevelInfo
		if rec.status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		slog.Log(r.Context(), level, "http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap нужен http.ResponseController (Flush, SetDeadline и т.п.).
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// MaxBody ограничивает размер тела запроса: валидатор по спеке (handler/validate.go)
// читает тело целиком в память, без лимита любой POST — неограниченный буфер.
// limitFor возвращает лимит в байтах для конкретного запроса (загрузка файлов больше остальных).
// Превышение всплывает как *http.MaxBytesError при чтении тела — валидатор и strict-декодер
// переводят его в 400 VALIDATION_ERROR.
func MaxBody(limitFor func(*http.Request) int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && r.Body != http.NoBody {
				r.Body = http.MaxBytesReader(w, r.Body, limitFor(r))
			}
			next.ServeHTTP(w, r)
		})
	}
}
