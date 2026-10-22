package handler

// Операции тега organizations (organizationsapi.StrictServerInterface).
// Ещё не реализованные отвечают 501 NOT_IMPLEMENTED в формате Error. Реализуя операцию,
// замените тело заглушки (образец — dictionaries.go + dictionaries_test.go).

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/organizationsapi"
)

func (h *Handler) DownloadDocument(context.Context, organizationsapi.DownloadDocumentRequestObject) (organizationsapi.DownloadDocumentResponseObject, error) {
	return nil, apperr.NotImplemented("downloadDocument")
}

func (h *Handler) GetMyOrganization(context.Context, organizationsapi.GetMyOrganizationRequestObject) (organizationsapi.GetMyOrganizationResponseObject, error) {
	return nil, apperr.NotImplemented("getMyOrganization")
}

func (h *Handler) UpdateMyOrganization(context.Context, organizationsapi.UpdateMyOrganizationRequestObject) (organizationsapi.UpdateMyOrganizationResponseObject, error) {
	return nil, apperr.NotImplemented("updateMyOrganization")
}

func (h *Handler) ListMyDocuments(context.Context, organizationsapi.ListMyDocumentsRequestObject) (organizationsapi.ListMyDocumentsResponseObject, error) {
	return nil, apperr.NotImplemented("listMyDocuments")
}

func (h *Handler) UploadMyDocument(context.Context, organizationsapi.UploadMyDocumentRequestObject) (organizationsapi.UploadMyDocumentResponseObject, error) {
	return nil, apperr.NotImplemented("uploadMyDocument")
}

func (h *Handler) DeleteMyDocument(context.Context, organizationsapi.DeleteMyDocumentRequestObject) (organizationsapi.DeleteMyDocumentResponseObject, error) {
	return nil, apperr.NotImplemented("deleteMyDocument")
}

func (h *Handler) SubmitKYC(context.Context, organizationsapi.SubmitKYCRequestObject) (organizationsapi.SubmitKYCResponseObject, error) {
	return nil, apperr.NotImplemented("submitKYC")
}

func (h *Handler) RegistryLookup(context.Context, organizationsapi.RegistryLookupRequestObject) (organizationsapi.RegistryLookupResponseObject, error) {
	return nil, apperr.NotImplemented("registryLookup")
}
