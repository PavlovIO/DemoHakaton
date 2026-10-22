package handler

// Операции тега applications (applicationsapi.StrictServerInterface).
// Ещё не реализованные отвечают 501 NOT_IMPLEMENTED в формате Error. Реализуя операцию,
// замените тело заглушки (образец — dictionaries.go + dictionaries_test.go).

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/applicationsapi"
)

func (h *Handler) ListApplications(context.Context, applicationsapi.ListApplicationsRequestObject) (applicationsapi.ListApplicationsResponseObject, error) {
	return nil, apperr.NotImplemented("listApplications")
}

func (h *Handler) CreateApplication(context.Context, applicationsapi.CreateApplicationRequestObject) (applicationsapi.CreateApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("createApplication")
}

func (h *Handler) GetApplication(context.Context, applicationsapi.GetApplicationRequestObject) (applicationsapi.GetApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("getApplication")
}

func (h *Handler) UpdateApplication(context.Context, applicationsapi.UpdateApplicationRequestObject) (applicationsapi.UpdateApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("updateApplication")
}

func (h *Handler) CancelApplication(context.Context, applicationsapi.CancelApplicationRequestObject) (applicationsapi.CancelApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("cancelApplication")
}

func (h *Handler) GetApplicationHistory(context.Context, applicationsapi.GetApplicationHistoryRequestObject) (applicationsapi.GetApplicationHistoryResponseObject, error) {
	return nil, apperr.NotImplemented("getApplicationHistory")
}

func (h *Handler) RejectAllOffers(context.Context, applicationsapi.RejectAllOffersRequestObject) (applicationsapi.RejectAllOffersResponseObject, error) {
	return nil, apperr.NotImplemented("rejectAllOffers")
}

func (h *Handler) SubmitApplication(context.Context, applicationsapi.SubmitApplicationRequestObject) (applicationsapi.SubmitApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("submitApplication")
}
