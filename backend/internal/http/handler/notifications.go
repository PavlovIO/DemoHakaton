package handler

// Операции тега notifications (notificationsapi.StrictServerInterface).
// Ещё не реализованные отвечают 501 NOT_IMPLEMENTED в формате Error. Реализуя операцию,
// замените тело заглушки (образец — dictionaries.go + dictionaries_test.go).

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/notificationsapi"
)

func (h *Handler) ListNotifications(context.Context, notificationsapi.ListNotificationsRequestObject) (notificationsapi.ListNotificationsResponseObject, error) {
	return nil, apperr.NotImplemented("listNotifications")
}

func (h *Handler) MarkAllNotificationsRead(context.Context, notificationsapi.MarkAllNotificationsReadRequestObject) (notificationsapi.MarkAllNotificationsReadResponseObject, error) {
	return nil, apperr.NotImplemented("markAllNotificationsRead")
}

func (h *Handler) MarkNotificationRead(context.Context, notificationsapi.MarkNotificationReadRequestObject) (notificationsapi.MarkNotificationReadResponseObject, error) {
	return nil, apperr.NotImplemented("markNotificationRead")
}
