package middleware

import (
	"context"
	"log/slog"
	"slices"
	"sync"
)

type logAttrsContextKey struct{}

// logAttrs — атрибуты лога одного запроса. Изменяемый, потому что RequestLog снаружи
// цепочки пишет строку после того, как Authenticate внутри узнал пользователя: значение
// в новом контексте до RequestLog не дошло бы.
type logAttrs struct {
	mu    sync.Mutex
	attrs []slog.Attr
}

// WithLogAttrs дописывает атрибуты во все дальнейшие строки лога текущего запроса,
// включая строку RequestLog. Вне запроса (нет RequestID) ничего не делает.
func WithLogAttrs(ctx context.Context, attrs ...slog.Attr) {
	holder, ok := ctx.Value(logAttrsContextKey{}).(*logAttrs)
	if !ok {
		return
	}
	holder.mu.Lock()
	defer holder.mu.Unlock()
	holder.attrs = append(holder.attrs, attrs...)
}

func logAttrsFromContext(ctx context.Context) []slog.Attr {
	holder, ok := ctx.Value(logAttrsContextKey{}).(*logAttrs)
	if !ok {
		return nil
	}
	holder.mu.Lock()
	defer holder.mu.Unlock()
	return slices.Clone(holder.attrs)
}

// ContextHandler дописывает в каждую запись request_id и атрибуты запроса из контекста:
// так строки RequestLog и WriteError одного запроса связываются. Вызовы slog без
// контекста (slog.Info вместо slog.InfoContext) этих атрибутов не получат.
type ContextHandler struct {
	slog.Handler
}

// Handle добавляет атрибуты контекста и передаёт запись обёрнутому хендлеру.
func (h ContextHandler) Handle(ctx context.Context, rec slog.Record) error {
	if id, ok := RequestIDFromContext(ctx); ok {
		rec.AddAttrs(slog.String("request_id", id))
	}
	rec.AddAttrs(logAttrsFromContext(ctx)...)
	return h.Handler.Handle(ctx, rec)
}

// WithAttrs сохраняет обёртку: иначе logger.With(...) терял бы request_id.
func (h ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup сохраняет обёртку, как WithAttrs.
func (h ContextHandler) WithGroup(name string) slog.Handler {
	return ContextHandler{Handler: h.Handler.WithGroup(name)}
}
