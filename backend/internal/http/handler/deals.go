package handler

// Операции тега deals (dealsapi.StrictServerInterface): перевод api ⇄ service.
// Видимость (стороны сделки и admin) проверяет lending.Deals в SQL.

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/dealsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/lending"
)

func (h *Handler) ListDeals(ctx context.Context, req dealsapi.ListDealsRequestObject) (dealsapi.ListDealsResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	page, err := h.deals.List(ctx, p, dealFilter(req.Params))
	if err != nil {
		return nil, err
	}
	items := make([]api.Deal, len(page.Items))
	for i, d := range page.Items {
		items[i] = toAPIDeal(d)
	}
	return dealsapi.ListDeals200JSONResponse{
		Items: items, Total: int(page.Total), Page: page.Page, PageSize: page.PageSize,
	}, nil
}

func (h *Handler) GetDeal(ctx context.Context, req dealsapi.GetDealRequestObject) (dealsapi.GetDealResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.deals.Get(ctx, p, req.DealID)
	if err != nil {
		return nil, err
	}
	return dealsapi.GetDeal200JSONResponse(toAPIDeal(d)), nil
}

func dealFilter(q api.ListDealsParams) lending.DealFilter {
	f := lending.DealFilter{
		BorrowerOrgID: q.BorrowerOrganizationID,
		LenderOrgID:   q.LenderOrganizationID,
		CreatedFrom:   q.CreatedFrom,
		CreatedTo:     q.CreatedTo,
	}
	f.Page, f.PageSize = pageParams(q.Page, q.PageSize)
	return f
}

func toAPIDeal(d lending.Deal) api.Deal {
	targetCountry := d.TargetCountryCode
	transferMethod := api.TransferMethod(d.TransferMethod)
	return api.Deal{
		ID:                 d.ID,
		Number:             lending.DealNumber(d.Number),
		ApplicationID:      d.ApplicationID,
		OfferID:            d.OfferID,
		Borrower:           api.OrganizationShort{ID: d.Borrower.ID, Name: d.Borrower.Name, CountryCode: d.Borrower.CountryCode},
		Lender:             api.OrganizationShort{ID: d.Lender.ID, Name: d.Lender.Name, CountryCode: d.Lender.CountryCode},
		Amount:             d.Amount,
		Currency:           d.Currency,
		InterestRateAnnual: d.InterestRateAnnual,
		FeePercent:         d.FeePercent,
		FeeAmount:          d.FeeAmount,
		TermMonths:         d.TermMonths,
		TargetCountryCode:  &targetCountry,
		TransferMethod:     &transferMethod,
		RepaymentSchedule:  api.RepaymentSchedule(d.RepaymentSchedule),
		Collateral:         d.Collateral,
		AdditionalTerms:    d.AdditionalTerms,
		AgreedAt:           d.AgreedAt,
	}
}
