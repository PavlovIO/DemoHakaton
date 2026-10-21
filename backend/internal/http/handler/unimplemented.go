package handler

// Заглушки ещё не реализованных операций: отвечают 501 NOT_IMPLEMENTED в формате Error.
// Реализуя операцию, удалите её заглушку отсюда и напишите метод в файле своего домена
// (компилятор подскажет "method redeclared", если забыть).

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
)

func (h *Handler) AcceptOffer(context.Context, api.AcceptOfferRequestObject) (api.AcceptOfferResponseObject, error) {
	return nil, apperr.NotImplemented("acceptOffer")
}

func (h *Handler) AdminAnalyticsSummary(context.Context, api.AdminAnalyticsSummaryRequestObject) (api.AdminAnalyticsSummaryResponseObject, error) {
	return nil, apperr.NotImplemented("adminAnalyticsSummary")
}

func (h *Handler) AdminApproveApplication(context.Context, api.AdminApproveApplicationRequestObject) (api.AdminApproveApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("adminApproveApplication")
}

func (h *Handler) AdminApproveKYC(context.Context, api.AdminApproveKYCRequestObject) (api.AdminApproveKYCResponseObject, error) {
	return nil, apperr.NotImplemented("adminApproveKYC")
}

func (h *Handler) AdminBlockUser(context.Context, api.AdminBlockUserRequestObject) (api.AdminBlockUserResponseObject, error) {
	return nil, apperr.NotImplemented("adminBlockUser")
}

func (h *Handler) AdminGetOrganization(context.Context, api.AdminGetOrganizationRequestObject) (api.AdminGetOrganizationResponseObject, error) {
	return nil, apperr.NotImplemented("adminGetOrganization")
}

func (h *Handler) AdminListAMLFlags(context.Context, api.AdminListAMLFlagsRequestObject) (api.AdminListAMLFlagsResponseObject, error) {
	return nil, apperr.NotImplemented("adminListAMLFlags")
}

func (h *Handler) AdminListAuditLog(context.Context, api.AdminListAuditLogRequestObject) (api.AdminListAuditLogResponseObject, error) {
	return nil, apperr.NotImplemented("adminListAuditLog")
}

func (h *Handler) AdminListOrganizations(context.Context, api.AdminListOrganizationsRequestObject) (api.AdminListOrganizationsResponseObject, error) {
	return nil, apperr.NotImplemented("adminListOrganizations")
}

func (h *Handler) AdminListUsers(context.Context, api.AdminListUsersRequestObject) (api.AdminListUsersResponseObject, error) {
	return nil, apperr.NotImplemented("adminListUsers")
}

func (h *Handler) AdminRejectApplication(context.Context, api.AdminRejectApplicationRequestObject) (api.AdminRejectApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("adminRejectApplication")
}

func (h *Handler) AdminRejectKYC(context.Context, api.AdminRejectKYCRequestObject) (api.AdminRejectKYCResponseObject, error) {
	return nil, apperr.NotImplemented("adminRejectKYC")
}

func (h *Handler) AdminResolveAMLFlag(context.Context, api.AdminResolveAMLFlagRequestObject) (api.AdminResolveAMLFlagResponseObject, error) {
	return nil, apperr.NotImplemented("adminResolveAMLFlag")
}

func (h *Handler) AdminUnblockUser(context.Context, api.AdminUnblockUserRequestObject) (api.AdminUnblockUserResponseObject, error) {
	return nil, apperr.NotImplemented("adminUnblockUser")
}

func (h *Handler) CancelApplication(context.Context, api.CancelApplicationRequestObject) (api.CancelApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("cancelApplication")
}

func (h *Handler) CreateApplication(context.Context, api.CreateApplicationRequestObject) (api.CreateApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("createApplication")
}

func (h *Handler) CreateOffer(context.Context, api.CreateOfferRequestObject) (api.CreateOfferResponseObject, error) {
	return nil, apperr.NotImplemented("createOffer")
}

func (h *Handler) DeleteMyDocument(context.Context, api.DeleteMyDocumentRequestObject) (api.DeleteMyDocumentResponseObject, error) {
	return nil, apperr.NotImplemented("deleteMyDocument")
}

func (h *Handler) DownloadDocument(context.Context, api.DownloadDocumentRequestObject) (api.DownloadDocumentResponseObject, error) {
	return nil, apperr.NotImplemented("downloadDocument")
}

func (h *Handler) GetApplication(context.Context, api.GetApplicationRequestObject) (api.GetApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("getApplication")
}

func (h *Handler) GetApplicationHistory(context.Context, api.GetApplicationHistoryRequestObject) (api.GetApplicationHistoryResponseObject, error) {
	return nil, apperr.NotImplemented("getApplicationHistory")
}

func (h *Handler) GetDeal(context.Context, api.GetDealRequestObject) (api.GetDealResponseObject, error) {
	return nil, apperr.NotImplemented("getDeal")
}

func (h *Handler) GetMe(context.Context, api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	return nil, apperr.NotImplemented("getMe")
}

func (h *Handler) GetMyOrganization(context.Context, api.GetMyOrganizationRequestObject) (api.GetMyOrganizationResponseObject, error) {
	return nil, apperr.NotImplemented("getMyOrganization")
}

func (h *Handler) GetOffer(context.Context, api.GetOfferRequestObject) (api.GetOfferResponseObject, error) {
	return nil, apperr.NotImplemented("getOffer")
}

func (h *Handler) ListApplicationOffers(context.Context, api.ListApplicationOffersRequestObject) (api.ListApplicationOffersResponseObject, error) {
	return nil, apperr.NotImplemented("listApplicationOffers")
}

func (h *Handler) ListApplications(context.Context, api.ListApplicationsRequestObject) (api.ListApplicationsResponseObject, error) {
	return nil, apperr.NotImplemented("listApplications")
}

func (h *Handler) ListDeals(context.Context, api.ListDealsRequestObject) (api.ListDealsResponseObject, error) {
	return nil, apperr.NotImplemented("listDeals")
}

func (h *Handler) ListMessages(context.Context, api.ListMessagesRequestObject) (api.ListMessagesResponseObject, error) {
	return nil, apperr.NotImplemented("listMessages")
}

func (h *Handler) ListMyDocuments(context.Context, api.ListMyDocumentsRequestObject) (api.ListMyDocumentsResponseObject, error) {
	return nil, apperr.NotImplemented("listMyDocuments")
}

func (h *Handler) ListNotifications(context.Context, api.ListNotificationsRequestObject) (api.ListNotificationsResponseObject, error) {
	return nil, apperr.NotImplemented("listNotifications")
}

func (h *Handler) ListOffers(context.Context, api.ListOffersRequestObject) (api.ListOffersResponseObject, error) {
	return nil, apperr.NotImplemented("listOffers")
}

func (h *Handler) Login(context.Context, api.LoginRequestObject) (api.LoginResponseObject, error) {
	return nil, apperr.NotImplemented("login")
}

func (h *Handler) Logout(context.Context, api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	return nil, apperr.NotImplemented("logout")
}

func (h *Handler) MarkAllNotificationsRead(context.Context, api.MarkAllNotificationsReadRequestObject) (api.MarkAllNotificationsReadResponseObject, error) {
	return nil, apperr.NotImplemented("markAllNotificationsRead")
}

func (h *Handler) MarkNotificationRead(context.Context, api.MarkNotificationReadRequestObject) (api.MarkNotificationReadResponseObject, error) {
	return nil, apperr.NotImplemented("markNotificationRead")
}

func (h *Handler) Register(context.Context, api.RegisterRequestObject) (api.RegisterResponseObject, error) {
	return nil, apperr.NotImplemented("register")
}

func (h *Handler) RegistryLookup(context.Context, api.RegistryLookupRequestObject) (api.RegistryLookupResponseObject, error) {
	return nil, apperr.NotImplemented("registryLookup")
}

func (h *Handler) RejectAllOffers(context.Context, api.RejectAllOffersRequestObject) (api.RejectAllOffersResponseObject, error) {
	return nil, apperr.NotImplemented("rejectAllOffers")
}

func (h *Handler) RejectOffer(context.Context, api.RejectOfferRequestObject) (api.RejectOfferResponseObject, error) {
	return nil, apperr.NotImplemented("rejectOffer")
}

func (h *Handler) SendMessage(context.Context, api.SendMessageRequestObject) (api.SendMessageResponseObject, error) {
	return nil, apperr.NotImplemented("sendMessage")
}

func (h *Handler) SubmitApplication(context.Context, api.SubmitApplicationRequestObject) (api.SubmitApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("submitApplication")
}

func (h *Handler) SubmitKYC(context.Context, api.SubmitKYCRequestObject) (api.SubmitKYCResponseObject, error) {
	return nil, apperr.NotImplemented("submitKYC")
}

func (h *Handler) UpdateApplication(context.Context, api.UpdateApplicationRequestObject) (api.UpdateApplicationResponseObject, error) {
	return nil, apperr.NotImplemented("updateApplication")
}

func (h *Handler) UpdateMyOrganization(context.Context, api.UpdateMyOrganizationRequestObject) (api.UpdateMyOrganizationResponseObject, error) {
	return nil, apperr.NotImplemented("updateMyOrganization")
}

func (h *Handler) UploadMyDocument(context.Context, api.UploadMyDocumentRequestObject) (api.UploadMyDocumentResponseObject, error) {
	return nil, apperr.NotImplemented("uploadMyDocument")
}

func (h *Handler) WithdrawOffer(context.Context, api.WithdrawOfferRequestObject) (api.WithdrawOfferResponseObject, error) {
	return nil, apperr.NotImplemented("withdrawOffer")
}
