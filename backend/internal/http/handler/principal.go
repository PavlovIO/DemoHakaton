package handler

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// currentPrincipal — кто выполняет запрос. Хендлеры защищённых операций берут principal
// только отсюда. До хендлера без principal доходит лишь публичная операция (Authorize),
// так что 401 здесь — подстраховка на случай ошибки в спеке или цепочке middleware.
func currentPrincipal(ctx context.Context) (service.Principal, error) {
	p, ok := middleware.PrincipalFromContext(ctx)
	if !ok {
		return service.Principal{}, apperr.Unauthorized("Требуется вход")
	}
	return p, nil
}
