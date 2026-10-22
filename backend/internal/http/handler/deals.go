package handler

// Операции тега deals (dealsapi.StrictServerInterface).
// Ещё не реализованные отвечают 501 NOT_IMPLEMENTED в формате Error. Реализуя операцию,
// замените тело заглушки (образец — dictionaries.go + dictionaries_test.go).

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/dealsapi"
)

func (h *Handler) ListDeals(context.Context, dealsapi.ListDealsRequestObject) (dealsapi.ListDealsResponseObject, error) {
	return nil, apperr.NotImplemented("listDeals")
}

func (h *Handler) GetDeal(context.Context, dealsapi.GetDealRequestObject) (dealsapi.GetDealResponseObject, error) {
	return nil, apperr.NotImplemented("getDeal")
}
