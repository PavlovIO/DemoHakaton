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
//
// Атрибуты запроса всегда в корне строки. rec.AddAttrs после WithGroup положил бы их
// в группу, поэтому с первого WithGroup обёртка копит группы и атрибуты в scope и
// применяет их к обёрнутому хендлеру в Handle — уже после атрибутов запроса.
type ContextHandler struct {
	slog.Handler
	scope []func(slog.Handler) slog.Handler
}

// Handle добавляет атрибуты контекста и передаёт запись обёрнутому хендлеру.
func (h ContextHandler) Handle(ctx context.Context, rec slog.Record) error {
	var attrs []slog.Attr
	if id, ok := RequestIDFromContext(ctx); ok {
		attrs = append(attrs, slog.String("request_id", id))
	}
	attrs = append(attrs, logAttrsFromContext(ctx)...)
	if len(h.scope) == 0 {
		rec.AddAttrs(attrs...)
		return h.Handler.Handle(ctx, rec)
	}
	next := h.Handler
	if len(attrs) > 0 {
		next = next.WithAttrs(attrs)
	}
	for _, apply := range h.scope {
		next = apply(next)
	}
	return next.Handle(ctx, rec)
}

// WithAttrs сохраняет обёртку: иначе logger.With(...) терял бы request_id.
func (h ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(h.scope) == 0 {
		return ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
	}
	return h.withScope(func(next slog.Handler) slog.Handler { return next.WithAttrs(attrs) })
}

// WithGroup сохраняет обёртку и откладывает группу до Handle (см. ContextHandler).
func (h ContextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.withScope(func(next slog.Handler) slog.Handler { return next.WithGroup(name) })
}

func (h ContextHandler) withScope(apply func(slog.Handler) slog.Handler) ContextHandler {
	return ContextHandler{Handler: h.Handler, scope: append(slices.Clip(h.scope), apply)}
}
