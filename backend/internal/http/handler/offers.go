package handler

// Операции тега offers (offersapi.StrictServerInterface).
// Ещё не реализованные отвечают 501 NOT_IMPLEMENTED в формате Error. Реализуя операцию,
// замените тело заглушки (образец — dictionaries.go + dictionaries_test.go).

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/offersapi"
)

func (h *Handler) ListApplicationOffers(context.Context, offersapi.ListApplicationOffersRequestObject) (offersapi.ListApplicationOffersResponseObject, error) {
	return nil, apperr.NotImplemented("listApplicationOffers")
}

func (h *Handler) CreateOffer(context.Context, offersapi.CreateOfferRequestObject) (offersapi.CreateOfferResponseObject, error) {
	return nil, apperr.NotImplemented("createOffer")
}

func (h *Handler) ListOffers(context.Context, offersapi.ListOffersRequestObject) (offersapi.ListOffersResponseObject, error) {
	return nil, apperr.NotImplemented("listOffers")
}

func (h *Handler) GetOffer(context.Context, offersapi.GetOfferRequestObject) (offersapi.GetOfferResponseObject, error) {
	return nil, apperr.NotImplemented("getOffer")
}

func (h *Handler) AcceptOffer(context.Context, offersapi.AcceptOfferRequestObject) (offersapi.AcceptOfferResponseObject, error) {
	return nil, apperr.NotImplemented("acceptOffer")
}

func (h *Handler) RejectOffer(context.Context, offersapi.RejectOfferRequestObject) (offersapi.RejectOfferResponseObject, error) {
	return nil, apperr.NotImplemented("rejectOffer")
}

func (h *Handler) WithdrawOffer(context.Context, offersapi.WithdrawOfferRequestObject) (offersapi.WithdrawOfferResponseObject, error) {
	return nil, apperr.NotImplemented("withdrawOffer")
}
