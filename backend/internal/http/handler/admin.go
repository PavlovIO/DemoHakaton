package handler

// Операции тега admin (adminapi.StrictServerInterface). Решения и карточка компании —
// service/admin (BE-12), AML-флаги — BE-13, аналитика и журнал аудита — BE-14.

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/adminapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
)

// AdminListAMLFlags — AML-флаги всех компаний с фильтрами.
func (h *Handler) AdminListAMLFlags(ctx context.Context, req adminapi.AdminListAMLFlagsRequestObject) (adminapi.AdminListAMLFlagsResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	q := req.Params
	f := admin.AmlFlagFilter{OrganizationID: q.OrganizationID, ApplicationID: q.ApplicationID}
	if q.Status != nil {
		st := string(*q.Status)
		f.Status = &st
	}
	if q.Severity != nil {
		sev := string(*q.Severity)
		f.Severity = &sev
	}
	f.Page, f.PageSize = pageParams(q.Page, q.PageSize)
	page, err := h.admin.ListAmlFlags(ctx, p, f)
	if err != nil {
		return nil, err
	}
	items := make([]api.AMLFlag, len(page.Items))
	for i, flag := range page.Items {
		items[i] = amlFlagToAPI(flag)
	}
	return adminapi.AdminListAMLFlags200JSONResponse{
		Items: items, Total: int(page.Total), Page: page.Page, PageSize: page.PageSize,
	}, nil
}

// AdminResolveAMLFlag — open → false_positive | confirmed; 404 — нет флага, 409 — уже закрыт.
func (h *Handler) AdminResolveAMLFlag(ctx context.Context, req adminapi.AdminResolveAMLFlagRequestObject) (adminapi.AdminResolveAMLFlagResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	flag, err := h.admin.ResolveAmlFlag(ctx, p, req.FlagID, repository.AmlFlagStatus(req.Body.Resolution),
		req.Body.Comment, middleware.ClientMetaFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return adminapi.AdminResolveAMLFlag200JSONResponse(amlFlagToAPI(flag)), nil
}

// AdminAnalyticsSummary — сводка за период; from > to — 400 из сервиса.
func (h *Handler) AdminAnalyticsSummary(ctx context.Context, req adminapi.AdminAnalyticsSummaryRequestObject) (adminapi.AdminAnalyticsSummaryResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	f := admin.AnalyticsFilter{CreatedFrom: req.Params.CreatedFrom, CreatedTo: req.Params.CreatedTo}
	if req.Params.TopN != nil {
		f.TopN = *req.Params.TopN
	}
	summary, err := h.admin.AnalyticsSummary(ctx, p, f)
	if err != nil {
		return nil, err
	}
	byStatus := make(map[string]int, len(summary.ApplicationsByStatus))
	for status, n := range summary.ApplicationsByStatus {
		byStatus[status] = int(n)
	}
	amounts := make([]api.CurrencyStat, len(summary.AmountsByCurrency))
	for i, c := range summary.AmountsByCurrency {
		amounts[i] = api.CurrencyStat{Currency: c.Currency, Count: int(c.Count), TotalAmount: c.TotalAmount, AverageAmount: c.AverageAmount}
	}
	return adminapi.AdminAnalyticsSummary200JSONResponse{
		ApplicationsTotal:    int(summary.ApplicationsTotal),
		ApplicationsByStatus: byStatus,
		OffersTotal:          int(summary.OffersTotal),
		DealsTotal:           int(summary.DealsTotal),
		AmountsByCurrency:    amounts,
		TopDemandCountries:   countryStatsToAPI(summary.TopDemandCountries),
		TopSupplyCountries:   countryStatsToAPI(summary.TopSupplyCountries),
	}, nil
}

func countryStatsToAPI(stats []admin.CountryStat) []api.CountryStat {
	out := make([]api.CountryStat, len(stats))
	for i, s := range stats {
		out[i] = api.CountryStat{CountryCode: s.CountryCode, Count: int(s.Count)}
	}
	return out
}

// AdminListAuditLog — журнал действий с фильтрами контракта.
func (h *Handler) AdminListAuditLog(ctx context.Context, req adminapi.AdminListAuditLogRequestObject) (adminapi.AdminListAuditLogResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	q := req.Params
	f := admin.AuditLogFilter{
		UserID: q.UserID, Action: q.Action, EntityID: q.EntityID,
		CreatedFrom: q.CreatedFrom, CreatedTo: q.CreatedTo,
	}
	if q.EntityType != nil {
		entityType := string(*q.EntityType)
		f.EntityType = &entityType
	}
	f.Page, f.PageSize = pageParams(q.Page, q.PageSize)
	page, err := h.admin.ListAuditLog(ctx, p, f)
	if err != nil {
		return nil, err
	}
	items := make([]api.AuditLogEntry, len(page.Items))
	for i, e := range page.Items {
		items[i] = auditLogEntryToAPI(e)
	}
	return adminapi.AdminListAuditLog200JSONResponse{
		Items: items, Total: int(page.Total), Page: page.Page, PageSize: page.PageSize,
	}, nil
}

func auditLogEntryToAPI(e admin.AuditLogEntry) api.AuditLogEntry {
	out := api.AuditLogEntry{
		ID: e.ID, UserID: e.UserID, Action: e.Action, EntityID: e.EntityID,
		UserAgent: e.UserAgent, Meta: e.Meta, CreatedAt: e.CreatedAt,
	}
	if e.EntityType != nil {
		entityType := api.EntityType(*e.EntityType)
		out.EntityType = &entityType
	}
	if e.IP != nil {
		ip := e.IP.String()
		out.IP = &ip
	}
	return out
}

// AdminListOrganizations — очередь верификации и поиск компаний.
func (h *Handler) AdminListOrganizations(ctx context.Context, req adminapi.AdminListOrganizationsRequestObject) (adminapi.AdminListOrganizationsResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	q := req.Params
	f := admin.OrganizationFilter{Q: q.Q, CountryCode: q.CountryCode}
	for _, st := range q.KYCStatus {
		f.KycStatuses = append(f.KycStatuses, string(st))
	}
	if q.Type != nil {
		t := string(*q.Type)
		f.Type = &t
	}
	f.Page, f.PageSize = pageParams(q.Page, q.PageSize)
	page, err := h.admin.ListOrganizations(ctx, p, f)
	if err != nil {
		return nil, err
	}
	items := make([]api.Organization, len(page.Items))
	for i, o := range page.Items {
		items[i] = profileToAPI(o)
	}
	return adminapi.AdminListOrganizations200JSONResponse{
		Items: items, Total: int(page.Total), Page: page.Page, PageSize: page.PageSize,
	}, nil
}

// AdminGetOrganization — полная карточка компании для проверки KYC.
func (h *Handler) AdminGetOrganization(ctx context.Context, req adminapi.AdminGetOrganizationRequestObject) (adminapi.AdminGetOrganizationResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.admin.GetOrganization(ctx, p, req.OrganizationID)
	if err != nil {
		return nil, err
	}
	return adminapi.AdminGetOrganization200JSONResponse(organizationDetailsToAPI(d)), nil
}

// organizationDetailsToAPI — Organization плюс массивы карточки; массивы всегда [], не null.
func organizationDetailsToAPI(d admin.OrganizationDetails) api.OrganizationDetails {
	o := profileToAPI(d.Profile)
	out := api.OrganizationDetails{
		ID:                 o.ID,
		Type:               o.Type,
		Name:               o.Name,
		CountryCode:        o.CountryCode,
		RegistrationNumber: o.RegistrationNumber,
		LEI:                o.LEI,
		LegalAddress:       o.LegalAddress,
		KYCStatus:          o.KYCStatus,
		KYCRejectReason:    o.KYCRejectReason,
		CreatedAt:          o.CreatedAt,
		VerifiedAt:         o.VerifiedAt,
		Beneficiaries:      o.Beneficiaries,
		Documents:          make([]api.KYCDocument, len(d.Documents)),
		AMLFlags:           make([]api.AMLFlag, len(d.AmlFlags)),
		Users:              make([]api.User, len(d.Users)),
	}
	for i, doc := range d.Documents {
		out.Documents[i] = documentToAPI(doc)
	}
	for i, f := range d.AmlFlags {
		out.AMLFlags[i] = amlFlagToAPI(f)
	}
	for i, u := range d.Users {
		out.Users[i] = adminUserToAPI(u)
	}
	if d.RegistryCheck != nil {
		rc := registryResultToAPI(*d.RegistryCheck)
		out.RegistryCheck = &rc
	}
	return out
}

func amlFlagToAPI(f admin.AmlFlag) api.AMLFlag {
	return api.AMLFlag{
		ID:                f.ID,
		Organization:      api.OrganizationShort{ID: f.Organization.ID, Name: f.Organization.Name, CountryCode: f.Organization.CountryCode},
		ApplicationID:     f.ApplicationID,
		Rule:              api.AMLRule(f.Rule),
		Severity:          api.AMLSeverity(f.Severity),
		Details:           f.Details,
		Status:            api.AMLFlagStatus(f.Status),
		ResolvedByUserID:  f.ResolvedByUserID,
		ResolutionComment: f.ResolutionComment,
		CreatedAt:         f.CreatedAt,
		ResolvedAt:        f.ResolvedAt,
	}
}

// AdminApproveKYC — under_review → verified. 409 CONFLICT при открытых high-флагах.
func (h *Handler) AdminApproveKYC(ctx context.Context, req adminapi.AdminApproveKYCRequestObject) (adminapi.AdminApproveKYCResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := h.admin.ApproveKyc(ctx, p, req.OrganizationID, adminComment(req.Body), middleware.ClientMetaFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return adminapi.AdminApproveKYC200JSONResponse(profileToAPI(profile)), nil
}

// AdminRejectKYC — under_review → rejected; reason (minLength 3) проверил валидатор.
func (h *Handler) AdminRejectKYC(ctx context.Context, req adminapi.AdminRejectKYCRequestObject) (adminapi.AdminRejectKYCResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := h.admin.RejectKyc(ctx, p, req.OrganizationID, req.Body.Reason, middleware.ClientMetaFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return adminapi.AdminRejectKYC200JSONResponse(profileToAPI(profile)), nil
}

// AdminApproveApplication — moderation → published.
func (h *Handler) AdminApproveApplication(ctx context.Context, req adminapi.AdminApproveApplicationRequestObject) (adminapi.AdminApproveApplicationResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	a, err := h.admin.ApproveApplication(ctx, p, req.ApplicationID, adminComment(req.Body), middleware.ClientMetaFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return adminapi.AdminApproveApplication200JSONResponse(toAPIApplication(a)), nil
}

// AdminRejectApplication — moderation → rejected; причина — в moderation_comment.
func (h *Handler) AdminRejectApplication(ctx context.Context, req adminapi.AdminRejectApplicationRequestObject) (adminapi.AdminRejectApplicationResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	a, err := h.admin.RejectApplication(ctx, p, req.ApplicationID, req.Body.Reason, middleware.ClientMetaFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return adminapi.AdminRejectApplication200JSONResponse(toAPIApplication(a)), nil
}

// AdminListUsers — пользователи платформы.
func (h *Handler) AdminListUsers(ctx context.Context, req adminapi.AdminListUsersRequestObject) (adminapi.AdminListUsersResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	q := req.Params
	f := admin.UserFilter{Q: q.Q}
	if q.Role != nil {
		r := string(*q.Role)
		f.Role = &r
	}
	if q.Status != nil {
		s := string(*q.Status)
		f.Status = &s
	}
	f.Page, f.PageSize = pageParams(q.Page, q.PageSize)
	page, err := h.admin.ListUsers(ctx, p, f)
	if err != nil {
		return nil, err
	}
	items := make([]api.User, len(page.Items))
	for i, u := range page.Items {
		items[i] = adminUserToAPI(u)
	}
	return adminapi.AdminListUsers200JSONResponse{
		Items: items, Total: int(page.Total), Page: page.Page, PageSize: page.PageSize,
	}, nil
}

// AdminBlockUser — блокировка с удалением сессий; администратора — 403.
func (h *Handler) AdminBlockUser(ctx context.Context, req adminapi.AdminBlockUserRequestObject) (adminapi.AdminBlockUserResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.admin.BlockUser(ctx, p, req.UserID, req.Body.Reason, middleware.ClientMetaFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return adminapi.AdminBlockUser200JSONResponse(adminUserToAPI(u)), nil
}

// AdminUnblockUser — снятие блокировки; пользователь входит заново.
func (h *Handler) AdminUnblockUser(ctx context.Context, req adminapi.AdminUnblockUserRequestObject) (adminapi.AdminUnblockUserResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.admin.UnblockUser(ctx, p, req.UserID, middleware.ClientMetaFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return adminapi.AdminUnblockUser200JSONResponse(adminUserToAPI(u)), nil
}

// adminComment — необязательный comment: тело AdminComment необязательно целиком.
func adminComment(body *api.AdminComment) *string {
	if body == nil {
		return nil
	}
	return body.Comment
}

// adminUserToAPI — только поля User контракта: password_hash и blocked_reason не отдаются.
func adminUserToAPI(u admin.User) api.User {
	return api.User{
		ID:             u.ID,
		Email:          openapi_types.Email(u.Email),
		Role:           api.Role(u.Role),
		Status:         api.UserStatus(u.Status),
		OrganizationID: u.OrganizationID,
		CreatedAt:      u.CreatedAt,
	}
}
