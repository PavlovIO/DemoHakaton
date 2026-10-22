package middleware

import (
	"context"
	"net/http"
	"net/netip"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

type requestMetaContextKey struct{}

// WithRequestMeta кладёт сетевые метаданные запроса в контекст.
func WithRequestMeta() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			meta := service.ClientMeta{UserAgent: truncateRunes(r.UserAgent(), 255)}
			if addrPort, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
				addr := addrPort.Addr()
				meta.IP = &addr
			}
			ctx := context.WithValue(r.Context(), requestMetaContextKey{}, meta)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClientMetaFromContext возвращает клиентские метаданные из контекста запроса.
func ClientMetaFromContext(ctx context.Context) service.ClientMeta {
	meta, _ := ctx.Value(requestMetaContextKey{}).(service.ClientMeta)
	return meta
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
