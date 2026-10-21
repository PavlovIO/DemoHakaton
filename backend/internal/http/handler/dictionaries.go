package handler

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
)

// Образец сквозной операции: handler → service → sqlc → БД.
// Хендлер только вызывает сервис и переводит строки sqlc в типы спеки;
// ошибка возвращается как есть и превращается в Error в WriteError.

func (h *Handler) ListCountries(ctx context.Context, _ api.ListCountriesRequestObject) (api.ListCountriesResponseObject, error) {
	rows, err := h.dicts.Countries(ctx)
	if err != nil {
		return nil, err
	}
	resp := make(api.ListCountries200JSONResponse, len(rows))
	for i, r := range rows {
		resp[i] = api.Country{
			Code:         r.Code,
			NameRu:       r.NameRu,
			NameEn:       r.NameEn,
			FATFHighRisk: r.FatfHighRisk,
		}
	}
	return resp, nil
}

func (h *Handler) ListCurrencies(ctx context.Context, _ api.ListCurrenciesRequestObject) (api.ListCurrenciesResponseObject, error) {
	rows, err := h.dicts.Currencies(ctx)
	if err != nil {
		return nil, err
	}
	resp := make(api.ListCurrencies200JSONResponse, len(rows))
	for i, r := range rows {
		resp[i] = api.Currency{Code: r.Code, Name: r.Name}
	}
	return resp, nil
}

func (h *Handler) ListTransferMethods(ctx context.Context, _ api.ListTransferMethodsRequestObject) (api.ListTransferMethodsResponseObject, error) {
	rows, err := h.dicts.TransferMethods(ctx)
	if err != nil {
		return nil, err
	}
	resp := make(api.ListTransferMethods200JSONResponse, len(rows))
	for i, r := range rows {
		resp[i] = api.DictionaryItem{Code: r.Code, Name: r.Name}
	}
	return resp, nil
}
