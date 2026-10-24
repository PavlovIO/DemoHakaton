package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// NotificationStore — запросы ленты уведомлений пользователя.
type NotificationStore interface {
	ListNotifications(ctx context.Context, arg repository.ListNotificationsParams) ([]repository.Notification, error)
	CountNotifications(ctx context.Context, arg repository.CountNotificationsParams) (int64, error)
	CountUnreadNotifications(ctx context.Context, userID uuid.UUID) (int64, error)
	MarkNotificationRead(ctx context.Context, arg repository.MarkNotificationReadParams) (int64, error)
	MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error
}

// Notifications — лента уведомлений текущего пользователя. Владение — в SQL:
// каждый запрос фильтрует по p.UserID, user_id из запроса не принимается.
type Notifications struct {
	store NotificationStore
}

// NewNotifications создаёт сервис ленты уведомлений.
func NewNotifications(store NotificationStore) *Notifications {
	return &Notifications{store: store}
}

// NotificationFilter — параметры ленты. Page и PageSize: 0 — по умолчанию (1 и 20).
type NotificationFilter struct {
	UnreadOnly bool
	Page       int
	PageSize   int
}

// NotificationPage — страница ленты. Total считается с тем же фильтром, что и Items;
// UnreadTotal — все непрочитанные пользователя, от UnreadOnly не зависит (счётчик в шапке).
type NotificationPage struct {
	Items       []repository.Notification
	Total       int64
	UnreadTotal int64
	Page        int
	PageSize    int
}

// List — лента уведомлений p, новые сверху.
func (s *Notifications) List(ctx context.Context, p Principal, f NotificationFilter) (NotificationPage, error) {
	page, pageSize, limit, offset := pageWindow(f.Page, f.PageSize)
	var unreadOnly pgtype.Bool // NULL — без фильтра
	if f.UnreadOnly {
		unreadOnly = pgtype.Bool{Bool: true, Valid: true}
	}
	items, err := s.store.ListNotifications(ctx, repository.ListNotificationsParams{
		UserID: p.UserID, UnreadOnly: unreadOnly, Limit: limit, Offset: offset,
	})
	if err != nil {
		return NotificationPage{}, fmt.Errorf("list notifications: %w", err)
	}
	total, err := s.store.CountNotifications(ctx, repository.CountNotificationsParams{UserID: p.UserID, UnreadOnly: unreadOnly})
	if err != nil {
		return NotificationPage{}, fmt.Errorf("count notifications: %w", err)
	}
	unreadTotal, err := s.store.CountUnreadNotifications(ctx, p.UserID)
	if err != nil {
		return NotificationPage{}, fmt.Errorf("count unread notifications: %w", err)
	}
	return NotificationPage{Items: items, Total: total, UnreadTotal: unreadTotal, Page: page, PageSize: pageSize}, nil
}

// MarkRead отмечает своё уведомление прочитанным. Идемпотентно: повторная отметка
// обновляет ту же строку. 0 строк — уведомление чужое или его нет → 404, не 403 (IDOR).
func (s *Notifications) MarkRead(ctx context.Context, p Principal, id int64) error {
	rows, err := s.store.MarkNotificationRead(ctx, repository.MarkNotificationReadParams{ID: id, UserID: p.UserID})
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	if rows == 0 {
		return apperr.NotFound("Уведомление не найдено")
	}
	return nil
}

// MarkAllRead отмечает прочитанными все уведомления p.
func (s *Notifications) MarkAllRead(ctx context.Context, p Principal) error {
	if err := s.store.MarkAllNotificationsRead(ctx, p.UserID); err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}
	return nil
}
