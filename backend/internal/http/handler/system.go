package handler

import (
	"context"
	"log/slog"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/systemapi"
)

const healthzDBTimeout = 2 * time.Second

// Healthz — liveness/readiness: 200, если БД отвечает, иначе 503 UNAVAILABLE.
func (h *Handler) Healthz(ctx context.Context, _ systemapi.HealthzRequestObject) (systemapi.HealthzResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, healthzDBTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		slog.WarnContext(ctx, "healthz: database unavailable", "err", err)
		_, body := ErrorBody(apperr.Unavailable("База данных недоступна", err))
		return systemapi.Healthz503JSONResponse(body), nil
	}
	return systemapi.Healthz200JSONResponse{Status: "ok"}, nil
}
