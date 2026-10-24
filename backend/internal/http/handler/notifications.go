package handler

// Операции тега notifications (notificationsapi.StrictServerInterface): перевод api ⇄ service.
// Лента всегда своя: сервис фильтрует по пользователю из сессии.

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/notificationsapi"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

func (h *Handler) ListNotifications(ctx context.Context, req notificationsapi.ListNotificationsRequestObject) (notificationsapi.ListNotificationsResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	page, err := h.notifications.List(ctx, p, notificationFilter(req.Params))
	if err != nil {
		return nil, err
	}
	items := make([]api.Notification, len(page.Items))
	for i, n := range page.Items {
		items[i] = toAPINotification(n)
	}
	return notificationsapi.ListNotifications200JSONResponse{
		Items: items, Total: int(page.Total), UnreadTotal: int(page.UnreadTotal), Page: page.Page, PageSize: page.PageSize,
	}, nil
}

func (h *Handler) MarkAllNotificationsRead(ctx context.Context, _ notificationsapi.MarkAllNotificationsReadRequestObject) (notificationsapi.MarkAllNotificationsReadResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.notifications.MarkAllRead(ctx, p); err != nil {
		return nil, err
	}
	return notificationsapi.MarkAllNotificationsRead204Response{}, nil
}

func (h *Handler) MarkNotificationRead(ctx context.Context, req notificationsapi.MarkNotificationReadRequestObject) (notificationsapi.MarkNotificationReadResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.notifications.MarkRead(ctx, p, req.NotificationID); err != nil {
		return nil, err
	}
	return notificationsapi.MarkNotificationRead204Response{}, nil
}

func notificationFilter(q api.ListNotificationsParams) service.NotificationFilter {
	var f service.NotificationFilter
	if q.UnreadOnly != nil {
		f.UnreadOnly = *q.UnreadOnly
	}
	f.Page, f.PageSize = pageParams(q.Page, q.PageSize)
	return f
}

func toAPINotification(n repository.Notification) api.Notification {
	return api.Notification{
		ID:         n.ID,
		Type:       api.NotificationType(n.Type),
		Title:      n.Title,
		EntityType: api.EntityType(n.EntityType),
		EntityID:   n.EntityID,
		IsRead:     n.IsRead,
		CreatedAt:  n.CreatedAt.Time,
	}
}
