package handler

// Операции тега admin (adminapi.StrictServerInterface).
// Ещё не реализованные отвечают 501 NOT_IMPLEMENTED в формате Error. Реализуя операцию,
// замените тело заглушки (образец — dictionaries.go + dictionaries_test.go).

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/adminapi"
)

func (h *Handler) AdminListAMLFlags(context.Context, adminapi.AdminListAMLFlagsRequestObject) (adminapi.AdminListAMLFlagsResponseObject, error) {
	return nil, apperr.NotImplemented("adminListAMLFlags")
}

func (h *Handler) AdminResolveAMLFlag(context.Context, adminapi.AdminResolveAMLFlagRequestObject) (adminapi.AdminResolveAMLFlagResponseObject, error) {
	return nil, apperr.NotImplemented("adminResolveAMLFlag")
}

func (h *Handler) AdminAnalyticsSummary(context.Context, adminapi.AdminAnalyticsSummaryRequestObject) (adminapi.AdminAnalyticsSummaryResponseObject, error) {
	return nil, apperr.NotImplemented("adminAnalyticsSummary")
}

func (h *Handler) AdminApproveApplication(context.Context, adminapi.AdminApproveApplicationRequestObject) (adminapi.AdminApproveApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("adminApproveApplication")
}

func (h *Handler) AdminRejectApplication(context.Context, adminapi.AdminRejectApplicationRequestObject) (adminapi.AdminRejectApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("adminRejectApplication")
}

func (h *Handler) AdminListAuditLog(context.Context, adminapi.AdminListAuditLogRequestObject) (adminapi.AdminListAuditLogResponseObject, error) {
	return nil, apperr.NotImplemented("adminListAuditLog")
}

func (h *Handler) AdminListOrganizations(context.Context, adminapi.AdminListOrganizationsRequestObject) (adminapi.AdminListOrganizationsResponseObject, error) {
	return nil, apperr.NotImplemented("adminListOrganizations")
}

func (h *Handler) AdminGetOrganization(context.Context, adminapi.AdminGetOrganizationRequestObject) (adminapi.AdminGetOrganizationResponseObject, error) {
	return nil, apperr.NotImplemented("adminGetOrganization")
}

func (h *Handler) AdminApproveKYC(context.Context, adminapi.AdminApproveKYCRequestObject) (adminapi.AdminApproveKYCResponseObject, error) {
	return nil, apperr.NotImplemented("adminApproveKYC")
}

func (h *Handler) AdminRejectKYC(context.Context, adminapi.AdminRejectKYCRequestObject) (adminapi.AdminRejectKYCResponseObject, error) {
	return nil, apperr.NotImplemented("adminRejectKYC")
}

func (h *Handler) AdminListUsers(context.Context, adminapi.AdminListUsersRequestObject) (adminapi.AdminListUsersResponseObject, error) {
	return nil, apperr.NotImplemented("adminListUsers")
}

func (h *Handler) AdminBlockUser(context.Context, adminapi.AdminBlockUserRequestObject) (adminapi.AdminBlockUserResponseObject, error) {
	return nil, apperr.NotImplemented("adminBlockUser")
}

func (h *Handler) AdminUnblockUser(context.Context, adminapi.AdminUnblockUserRequestObject) (adminapi.AdminUnblockUserResponseObject, error) {
	return nil, apperr.NotImplemented("adminUnblockUser")
}
