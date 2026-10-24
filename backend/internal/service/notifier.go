package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Минимальная версия Notifier из BE-11, шаг 1: NotifyAdmins (submitKyc, submit заявки) и
// NotifyOrganization (offer_rejected кредитору, BE-07). NotifyUser добавляет BE-11.

// Notification — одно уведомление; получателей выбирает метод Notifier.
type Notification struct {
	Type       repository.NotificationType
	Title      string
	EntityType repository.EntityType
	EntityID   string // uuid.String() или strconv.FormatInt для bigint-сущностей
}

// NotificationWriter — часть repository.Querier, которая нужна Notifier.
type NotificationWriter interface {
	CreateNotificationsForAdmins(ctx context.Context, arg repository.CreateNotificationsForAdminsParams) (int64, error)
	CreateNotificationsForOrganization(ctx context.Context, arg repository.CreateNotificationsForOrganizationParams) (int64, error)
}

// Notifier пишет уведомления querier'ом транзакции бизнес-события: вызывать только
// внутри InTx, передавая q из колбэка, — уведомление атомарно с событием (ADR-0001).
type Notifier struct{}

// NotifyAdmins — уведомление каждому активному админу.
func (Notifier) NotifyAdmins(ctx context.Context, q NotificationWriter, n Notification) error {
	_, err := q.CreateNotificationsForAdmins(ctx, repository.CreateNotificationsForAdminsParams{
		Type: n.Type, Title: n.Title, EntityType: n.EntityType, EntityID: n.EntityID,
	})
	if err != nil {
		return fmt.Errorf("notify admins %s: %w", n.Type, err)
	}
	return nil
}

// NotifyOrganization — уведомление каждому активному пользователю компании.
func (Notifier) NotifyOrganization(ctx context.Context, q NotificationWriter, orgID uuid.UUID, n Notification) error {
	_, err := q.CreateNotificationsForOrganization(ctx, repository.CreateNotificationsForOrganizationParams{
		Type: n.Type, Title: n.Title, EntityType: n.EntityType, EntityID: n.EntityID, OrganizationID: orgID,
	})
	if err != nil {
		return fmt.Errorf("notify organization %s %s: %w", orgID, n.Type, err)
	}
	return nil
}
