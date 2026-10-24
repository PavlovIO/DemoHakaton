package handler

// Операции тега organizations (organizationsapi.StrictServerInterface).
// Ещё не реализованные отвечают 501 NOT_IMPLEMENTED в формате Error. Реализуя операцию,
// замените тело заглушки (образец — dictionaries.go + dictionaries_test.go).

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/organizationsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// Поля формы загрузки документа.
const (
	formDocType = "doc_type"
	formFile    = "file"
	// maxDocTypeLen — значения enum короче; длиннее — заведомо не из enum.
	maxDocTypeLen = 64
)

// DownloadDocument отдаёт файл документа владельцу или admin. Content-Disposition:
// attachment — браузер скачивает, а не открывает; имя кодирует mime.FormatMediaType
// (RFC 2231 для кириллицы, кавычки экранируются) — инъекция в заголовок невозможна.
// nosniff ставится явно, не полагаясь на глобальный SecurityHeaders.
func (h *Handler) DownloadDocument(ctx context.Context, req organizationsapi.DownloadDocumentRequestObject) (organizationsapi.DownloadDocumentResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	f, err := h.kyc.OpenDocument(ctx, p, middleware.ClientMetaFromContext(ctx), req.DocumentID)
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": f.FileName})
	nosniff := "nosniff"
	return organizationsapi.DownloadDocument200ApplicationOctetStreamResponse{
		Body:          f.Body, // закрывает сгенерированный VisitDownloadDocumentResponse
		ContentLength: f.Size,
		Headers: organizationsapi.DownloadDocument200ResponseHeaders{
			ContentDisposition:  &disposition,
			XContentTypeOptions: &nosniff,
		},
	}, nil
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

// ListMyDocuments — документы своей компании.
func (h *Handler) ListMyDocuments(ctx context.Context, _ organizationsapi.ListMyDocumentsRequestObject) (organizationsapi.ListMyDocumentsResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	docs, err := h.kyc.ListDocuments(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make(organizationsapi.ListMyDocuments200JSONResponse, len(docs))
	for i, d := range docs {
		out[i] = documentToAPI(d)
	}
	return out, nil
}

// UploadMyDocument читает multipart-форму потоком: файл не попадает в память целиком.
// Валидатор загрузку пропускает (skipValidation), поэтому doc_type и наличие file
// проверяются здесь — единственное место ручной валидации в хендлерах. doc_type должен
// идти до file (описание операции в контракте): иначе тип пришлось бы проверять после
// записи файла во временное хранилище.
func (h *Handler) UploadMyDocument(ctx context.Context, req organizationsapi.UploadMyDocumentRequestObject) (organizationsapi.UploadMyDocumentResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var docType repository.KycDocumentType
	for {
		part, err := req.Body.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, formFieldError(formFile, "обязательное поле")
		}
		if err != nil {
			return nil, uploadReadError(err)
		}
		switch part.FormName() {
		case formDocType:
			raw, err := io.ReadAll(io.LimitReader(part, maxDocTypeLen))
			if err != nil {
				return nil, uploadReadError(err)
			}
			if !api.KYCDocumentType(raw).Valid() {
				return nil, formFieldError(formDocType, "недопустимый тип документа")
			}
			docType = repository.KycDocumentType(raw)
		case formFile:
			if docType == "" {
				return nil, formFieldError(formDocType, "обязательное поле, передаётся до file")
			}
			// part.FileName() — только базовое имя, без пути с машины клиента.
			doc, err := h.kyc.UploadDocument(ctx, p, middleware.ClientMetaFromContext(ctx), docType, part.FileName(), uploadReader{part})
			if err != nil {
				return nil, err
			}
			return organizationsapi.UploadMyDocument201JSONResponse(documentToAPI(doc)), nil
		}
		// Прочие поля формы игнорируются: NextPart дочитает их сам.
	}
}

// DeleteMyDocument удаляет свой документ (только draft / rejected).
func (h *Handler) DeleteMyDocument(ctx context.Context, req organizationsapi.DeleteMyDocumentRequestObject) (organizationsapi.DeleteMyDocumentResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.kyc.DeleteDocument(ctx, p, req.DocumentID); err != nil {
		return nil, err
	}
	return organizationsapi.DeleteMyDocument204Response{}, nil
}

func formFieldError(field, msg string) error {
	return apperr.Validation("Запрос не соответствует контракту", apperr.FieldError{Field: field, Message: msg})
}

// uploadReadError — ошибка чтения формы. Лимит тела (middleware.MaxBody) → 413 из
// контракта; остальное — битая форма, 400.
func uploadReadError(err error) error {
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		return apperr.FileTooLarge()
	}
	return apperr.Wrap(apperr.CodeValidation, "Некорректная multipart-форма", err)
}

// uploadReader переводит превышение лимита тела в 413 прямо при чтении файла: сервис
// не знает про net/http, а ошибки apperr пропускает как есть.
type uploadReader struct{ r io.Reader }

func (u uploadReader) Read(p []byte) (int, error) {
	n, err := u.r.Read(p)
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		return n, apperr.FileTooLarge()
	}
	return n, err
}

func documentToAPI(d repository.KycDocument) api.KYCDocument {
	return api.KYCDocument{
		ID:         d.ID,
		DocType:    api.KYCDocumentType(d.DocType),
		FileName:   d.FileName,
		MimeType:   d.MimeType,
		SizeBytes:  d.SizeBytes,
		UploadedAt: d.UploadedAt.Time,
	}
}

// SubmitKYC отправляет компанию на проверку: draft|rejected → under_review.
func (h *Handler) SubmitKYC(ctx context.Context, _ organizationsapi.SubmitKYCRequestObject) (organizationsapi.SubmitKYCResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := h.kyc.SubmitKyc(ctx, p, middleware.ClientMetaFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return organizationsapi.SubmitKYC200JSONResponse(profileToAPI(profile)), nil
}

// RegistryLookup ищет компанию в реестре для автозаполнения формы регистрации.
// Маршрут публичный (security: []); лимиты частоты — в service.Registry.
func (h *Handler) RegistryLookup(ctx context.Context, req organizationsapi.RegistryLookupRequestObject) (organizationsapi.RegistryLookupResponseObject, error) {
	result, err := h.registry.Lookup(ctx, middleware.ClientMetaFromContext(ctx), req.Params.CountryCode, req.Params.Number)
	if err != nil {
		return nil, err
	}
	return organizationsapi.RegistryLookup200JSONResponse(registryResultToAPI(result)), nil
}

// registryResultToAPI: при found=false поля компании — null, checked_at есть всегда.
func registryResultToAPI(r registry.Result) api.RegistryLookupResult {
	checkedAt := r.CheckedAt
	out := api.RegistryLookupResult{
		Source:    api.RegistryLookupResultSource(r.Source),
		Found:     r.Found,
		CheckedAt: &checkedAt,
	}
	if !r.Found {
		return out
	}
	out.Name = optionalString(r.Name)
	out.RegistrationNumber = optionalString(r.RegistrationNumber)
	out.LegalAddress = optionalString(r.LegalAddress)
	out.RegistryStatus = optionalString(r.Status)
	return out
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
