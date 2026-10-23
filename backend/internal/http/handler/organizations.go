package handler

// Операции тега organizations (organizationsapi.StrictServerInterface).
// Ещё не реализованные отвечают 501 NOT_IMPLEMENTED в формате Error. Реализуя операцию,
// замените тело заглушки (образец — dictionaries.go + dictionaries_test.go).

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/organizationsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

func (h *Handler) DownloadDocument(context.Context, organizationsapi.DownloadDocumentRequestObject) (organizationsapi.DownloadDocumentResponseObject, error) {
	return nil, apperr.NotImplemented("downloadDocument")
}

// GetMyOrganization — профиль своей компании с бенефициарами.
func (h *Handler) GetMyOrganization(ctx context.Context, _ organizationsapi.GetMyOrganizationRequestObject) (organizationsapi.GetMyOrganizationResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := h.kyc.MyOrganization(ctx, p)
	if err != nil {
		return nil, err
	}
	return organizationsapi.GetMyOrganization200JSONResponse(profileToAPI(profile)), nil
}

// UpdateMyOrganization — частичная правка профиля (только draft / rejected).
func (h *Handler) UpdateMyOrganization(ctx context.Context, req organizationsapi.UpdateMyOrganizationRequestObject) (organizationsapi.UpdateMyOrganizationResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	body := req.Body
	in := service.OrganizationUpdateInput{
		Name:               body.Name,
		RegistrationNumber: body.RegistrationNumber,
		LegalAddress:       body.LegalAddress,
		LEI:                body.LEI,
	}
	// Срез без указателя (prefer-skip-optional-pointer): encoding/json оставляет его nil,
	// если поля нет или оно null, а для [] даёт пустой не-nil. nil — список не трогать.
	if body.Beneficiaries != nil {
		list := make([]service.BeneficiaryInput, len(body.Beneficiaries))
		for i, b := range body.Beneficiaries {
			list[i] = service.BeneficiaryInput{
				FullName:               b.FullName,
				CitizenshipCountryCode: b.CitizenshipCountryCode,
				OwnershipPercent:       b.OwnershipPercent,
			}
		}
		in.Beneficiaries = &list
	}
	profile, err := h.kyc.UpdateMyOrganization(ctx, p, in)
	if err != nil {
		return nil, err
	}
	return organizationsapi.UpdateMyOrganization200JSONResponse(profileToAPI(profile)), nil
}

func profileToAPI(p service.OrganizationProfile) api.Organization {
	return organizationToAPI(p.Org, p.Beneficiaries)
}

// organizationToAPI — организация в ответе; beneficiaries всегда массив, не null.
func organizationToAPI(o *service.Org, beneficiaries []service.Beneficiary) api.Organization {
	out := api.Organization{
		ID:                 o.ID,
		Type:               api.OrganizationType(o.Type),
		Name:               o.Name,
		CountryCode:        o.CountryCode,
		RegistrationNumber: o.RegistrationNumber,
		LEI:                o.LEI,
		LegalAddress:       o.LegalAddress,
		KYCStatus:          api.KYCStatus(o.KycStatus),
		CreatedAt:          o.CreatedAt,
		VerifiedAt:         o.VerifiedAt,
		KYCRejectReason:    o.KycRejectReason,
		Beneficiaries:      make([]api.Beneficiary, len(beneficiaries)),
	}
	for i, b := range beneficiaries {
		out.Beneficiaries[i] = api.Beneficiary{
			FullName:               b.FullName,
			CitizenshipCountryCode: b.CitizenshipCountryCode,
			OwnershipPercent:       b.OwnershipPercent,
		}
	}
	return out
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
