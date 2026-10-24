package middleware

import (
	"context"
	"net/http"
	"regexp"

	"github.com/google/uuid"
)

// RequestIDHeader — заголовок с id запроса: клиент может прислать свой, сервер всегда возвращает.
const RequestIDHeader = "X-Request-ID"

// validRequestID — id клиента принимается только из безопасных символов и не длиннее 64:
// перевод строки или произвольная длина позволили бы подделать или раздуть строку лога.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type requestIDContextKey struct{}

// RequestID берёт X-Request-ID клиента или генерирует новый, кладёт его в контекст
// (его пишет в каждую строку лога ContextHandler) и в заголовок ответа. Заголовок ставится
// до next: ответ ошибкой из любого слоя его уже несёт. Ещё кладёт в контекст изменяемый
// набор атрибутов лога запроса — его дополняют внутренние middleware (WithLogAttrs).
// Стоит первым в цепочке, до RequestLog.
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(RequestIDHeader)
			if !validRequestID.MatchString(id) {
				id = uuid.NewString()
			}
			w.Header().Set(RequestIDHeader, id)
			ctx := context.WithValue(r.Context(), requestIDContextKey{}, id)
			ctx = context.WithValue(ctx, logAttrsContextKey{}, &logAttrs{})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestIDFromContext возвращает id текущего запроса.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDContextKey{}).(string)
	return id, ok
}
