package handler

// Операции тега applications (applicationsapi.StrictServerInterface): перевод api ⇄ service.
// Роль, KYC, видимость и статусы проверяет service.Applications.

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/applicationsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

func (h *Handler) ListApplications(ctx context.Context, req applicationsapi.ListApplicationsRequestObject) (applicationsapi.ListApplicationsResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	page, err := h.applications.List(ctx, p, applicationFilter(req.Params))
	if err != nil {
		return nil, err
	}
	items := make([]api.Application, len(page.Items))
	for i, a := range page.Items {
		items[i] = toAPIApplication(a)
	}
	return applicationsapi.ListApplications200JSONResponse{
		Items: items, Total: int(page.Total), Page: page.Page, PageSize: page.PageSize,
	}, nil
}

func (h *Handler) CreateApplication(ctx context.Context, req applicationsapi.CreateApplicationRequestObject) (applicationsapi.CreateApplicationResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	a, err := h.applications.Create(ctx, p, middleware.ClientMetaFromContext(ctx), service.CreateApplicationInput{
		Amount:                b.Amount,
		Currency:              b.Currency,
		TargetCountryCode:     b.TargetCountryCode,
		Purpose:               b.Purpose,
		TermMonths:            b.TermMonths,
		TransferMethod:        string(b.TransferMethod),
		TransferMethodComment: b.TransferMethodComment,
	})
	if err != nil {
		return nil, err
	}
	return applicationsapi.CreateApplication201JSONResponse(toAPIApplication(a)), nil
}

func (h *Handler) GetApplication(ctx context.Context, req applicationsapi.GetApplicationRequestObject) (applicationsapi.GetApplicationResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	a, err := h.applications.Get(ctx, p, req.ApplicationID)
	if err != nil {
		return nil, err
	}
	return applicationsapi.GetApplication200JSONResponse(toAPIApplication(a)), nil
}

func (h *Handler) UpdateApplication(ctx context.Context, req applicationsapi.UpdateApplicationRequestObject) (applicationsapi.UpdateApplicationResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	in := service.UpdateApplicationInput{
		Amount:                b.Amount,
		Currency:              b.Currency,
		TargetCountryCode:     b.TargetCountryCode,
		Purpose:               b.Purpose,
		TermMonths:            b.TermMonths,
		TransferMethodComment: b.TransferMethodComment, // "" — очистить (DECISIONS п. 7)
	}
	if b.TransferMethod != nil {
		m := string(*b.TransferMethod)
		in.TransferMethod = &m
	}
	a, err := h.applications.Update(ctx, p, req.ApplicationID, in)
	if err != nil {
		return nil, err
	}
	return applicationsapi.UpdateApplication200JSONResponse(toAPIApplication(a)), nil
}

func (h *Handler) SubmitApplication(ctx context.Context, req applicationsapi.SubmitApplicationRequestObject) (applicationsapi.SubmitApplicationResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	a, err := h.applications.Submit(ctx, p, middleware.ClientMetaFromContext(ctx), req.ApplicationID)
	if err != nil {
		return nil, err
	}
	return applicationsapi.SubmitApplication200JSONResponse(toAPIApplication(a)), nil
}

func (h *Handler) CancelApplication(ctx context.Context, req applicationsapi.CancelApplicationRequestObject) (applicationsapi.CancelApplicationResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	a, err := h.applications.Cancel(ctx, p, middleware.ClientMetaFromContext(ctx), req.ApplicationID)
	if err != nil {
		return nil, err
	}
	return applicationsapi.CancelApplication200JSONResponse(toAPIApplication(a)), nil
}

func (h *Handler) RejectAllOffers(ctx context.Context, req applicationsapi.RejectAllOffersRequestObject) (applicationsapi.RejectAllOffersResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	n, err := h.applications.RejectAllOffers(ctx, p, req.ApplicationID)
	if err != nil {
		return nil, err
	}
	return applicationsapi.RejectAllOffers200JSONResponse{RejectedCount: n}, nil
}

func (h *Handler) GetApplicationHistory(ctx context.Context, req applicationsapi.GetApplicationHistoryRequestObject) (applicationsapi.GetApplicationHistoryResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := h.applications.History(ctx, p, req.ApplicationID)
	if err != nil {
		return nil, err
	}
	out := make(applicationsapi.GetApplicationHistory200JSONResponse, len(entries))
	for i, e := range entries {
		out[i] = api.StatusHistoryEntry{
			ID:          e.ID,
			EntityType:  api.EntityType(e.EntityType),
			EntityID:    e.EntityID.String(),
			FromStatus:  e.FromStatus,
			ToStatus:    e.ToStatus,
			ActorUserID: e.ActorUserID,
			Comment:     e.Comment,
			CreatedAt:   e.CreatedAt,
		}
		if e.ActorRole != nil {
			role := api.Role(*e.ActorRole)
			out[i].ActorRole = &role
		}
	}
	return out, nil
}

// applicationFilter — query-параметры списка в фильтр сервиса. Формат сумм уже проверен
// валидатором по паттерну Money, поэтому ошибка разбора здесь невозможна.
func applicationFilter(q api.ListApplicationsParams) service.ApplicationFilter {
	f := service.ApplicationFilter{
		TargetCountries: q.TargetCountry,
		Currency:        q.Currency,
		AmountMin:       parseMoney(q.AmountMin),
		AmountMax:       parseMoney(q.AmountMax),
		TermMin:         q.TermMin,
		TermMax:         q.TermMax,
		CreatedFrom:     q.CreatedFrom,
		CreatedTo:       q.CreatedTo,
		BorrowerOrgID:   q.BorrowerOrganizationID,
		HasMyOffer:      q.HasMyOffer,
	}
	for _, s := range q.Status {
		f.Statuses = append(f.Statuses, service.ApplicationStatus(s))
	}
	for _, m := range q.TransferMethod {
		f.TransferMethods = append(f.TransferMethods, string(m))
	}
	if q.Sort != nil {
		f.Sort = string(*q.Sort)
	}
	f.Page, f.PageSize = pageParams(q.Page, q.PageSize)
	return f
}

func parseMoney(s *string) *decimal.Decimal {
	if s == nil {
		return nil
	}
	d, err := decimal.NewFromString(*s)
	if err != nil {
		return nil
	}
	return &d
}

func toAPIApplication(a service.Application) api.Application {
	out := api.Application{
		ID:                    a.ID,
		Number:                service.ApplicationNumber(a.Number),
		Borrower:              api.OrganizationShort{ID: a.Borrower.ID, Name: a.Borrower.Name, CountryCode: a.Borrower.CountryCode},
		Status:                api.ApplicationStatus(a.Status),
		Amount:                a.Amount,
		Currency:              a.Currency,
		TargetCountryCode:     a.TargetCountryCode,
		Purpose:               a.Purpose,
		TermMonths:            a.TermMonths,
		TransferMethod:        api.TransferMethod(a.TransferMethod),
		TransferMethodComment: a.TransferMethodComment,
		ModerationComment:     a.ModerationComment,
		AcceptedOfferID:       a.AcceptedOfferID,
		MyOfferID:             a.MyOfferID,
		PublishedAt:           a.PublishedAt,
		CreatedAt:             a.CreatedAt,
		UpdatedAt:             a.UpdatedAt,
	}
	if a.OffersCount != nil {
		n := int(*a.OffersCount)
		out.OffersCount = &n
	}
	return out
}
