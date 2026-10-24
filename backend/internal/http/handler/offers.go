package handler

// Операции тега offers (offersapi.StrictServerInterface): перевод api ⇄ service.
// Роль, KYC, видимость и статусы проверяет service.Offers; AcceptOffer — service.Deals
// (результат принятия — сделка).

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/offersapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

func (h *Handler) ListApplicationOffers(ctx context.Context, req offersapi.ListApplicationOffersRequestObject) (offersapi.ListApplicationOffersResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	offers, err := h.offers.ListByApplication(ctx, p, req.ApplicationID, offerStatuses(req.Params.Status))
	if err != nil {
		return nil, err
	}
	out := make(offersapi.ListApplicationOffers200JSONResponse, len(offers))
	for i, o := range offers {
		out[i] = toAPIOffer(o)
	}
	return out, nil
}

func (h *Handler) CreateOffer(ctx context.Context, req offersapi.CreateOfferRequestObject) (offersapi.CreateOfferResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	o, err := h.offers.Create(ctx, p, middleware.ClientMetaFromContext(ctx), req.ApplicationID, service.CreateOfferInput{
		Amount:             b.Amount,
		InterestRateAnnual: b.InterestRateAnnual,
		FeePercent:         b.FeePercent,
		RepaymentSchedule:  string(b.RepaymentSchedule),
		Collateral:         b.Collateral,
		AdditionalTerms:    b.AdditionalTerms,
		ValidUntil:         b.ValidUntil,
	})
	if err != nil {
		return nil, err
	}
	return offersapi.CreateOffer201JSONResponse(toAPIOffer(o)), nil
}

func (h *Handler) ListOffers(ctx context.Context, req offersapi.ListOffersRequestObject) (offersapi.ListOffersResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	page, err := h.offers.List(ctx, p, offerFilter(req.Params))
	if err != nil {
		return nil, err
	}
	items := make([]api.Offer, len(page.Items))
	for i, o := range page.Items {
		items[i] = toAPIOffer(o)
	}
	return offersapi.ListOffers200JSONResponse{
		Items: items, Total: int(page.Total), Page: page.Page, PageSize: page.PageSize,
	}, nil
}

func (h *Handler) GetOffer(ctx context.Context, req offersapi.GetOfferRequestObject) (offersapi.GetOfferResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	o, err := h.offers.Get(ctx, p, req.OfferID)
	if err != nil {
		return nil, err
	}
	return offersapi.GetOffer200JSONResponse(toAPIOffer(o)), nil
}

func (h *Handler) AcceptOffer(ctx context.Context, req offersapi.AcceptOfferRequestObject) (offersapi.AcceptOfferResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.deals.AcceptOffer(ctx, p, middleware.ClientMetaFromContext(ctx), req.OfferID)
	if err != nil {
		return nil, err
	}
	return offersapi.AcceptOffer200JSONResponse(toAPIDeal(d)), nil
}

func (h *Handler) RejectOffer(ctx context.Context, req offersapi.RejectOfferRequestObject) (offersapi.RejectOfferResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var comment *string // тело необязательно (requestBody.required: false)
	if req.Body != nil {
		comment = req.Body.Comment
	}
	o, err := h.offers.Reject(ctx, p, middleware.ClientMetaFromContext(ctx), req.OfferID, comment)
	if err != nil {
		return nil, err
	}
	return offersapi.RejectOffer200JSONResponse(toAPIOffer(o)), nil
}

func (h *Handler) WithdrawOffer(ctx context.Context, req offersapi.WithdrawOfferRequestObject) (offersapi.WithdrawOfferResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	o, err := h.offers.Withdraw(ctx, p, middleware.ClientMetaFromContext(ctx), req.OfferID)
	if err != nil {
		return nil, err
	}
	return offersapi.WithdrawOffer200JSONResponse(toAPIOffer(o)), nil
}

// offerFilter — query-параметры списка в фильтр сервиса.
func offerFilter(q api.ListOffersParams) service.OfferFilter {
	f := service.OfferFilter{
		Statuses:      offerStatuses(q.Status),
		ApplicationID: q.ApplicationID,
		LenderOrgID:   q.LenderOrganizationID,
		BorrowerOrgID: q.BorrowerOrganizationID,
		CreatedFrom:   q.CreatedFrom,
		CreatedTo:     q.CreatedTo,
	}
	f.Page, f.PageSize = pageParams(q.Page, q.PageSize)
	return f
}

func offerStatuses(in []api.OfferStatus) []service.OfferStatus {
	out := make([]service.OfferStatus, len(in))
	for i, s := range in {
		out[i] = service.OfferStatus(s)
	}
	return out
}

func toAPIOffer(o service.Offer) api.Offer {
	number := service.ApplicationNumber(o.ApplicationNumber)
	out := api.Offer{
		ID:                 o.ID,
		ApplicationID:      o.ApplicationID,
		ApplicationNumber:  &number,
		Lender:             api.OrganizationShort{ID: o.Lender.ID, Name: o.Lender.Name, CountryCode: o.Lender.CountryCode},
		Status:             api.OfferStatus(o.Status),
		RejectComment:      o.RejectComment,
		Amount:             o.Amount,
		Currency:           o.Currency,
		InterestRateAnnual: o.InterestRateAnnual,
		FeePercent:         o.FeePercent,
		RepaymentSchedule:  api.RepaymentSchedule(o.RepaymentSchedule),
		Collateral:         o.Collateral,
		AdditionalTerms:    o.AdditionalTerms,
		ValidUntil:         o.ValidUntil,
		CreatedAt:          o.CreatedAt,
		DecidedAt:          o.DecidedAt,
	}
	if o.RejectReason != nil {
		reason := api.OfferRejectReason(*o.RejectReason)
		out.RejectReason = &reason
	}
	if o.UnreadMessages != nil {
		n := int(*o.UnreadMessages)
		out.UnreadMessages = &n
	}
	return out
}
