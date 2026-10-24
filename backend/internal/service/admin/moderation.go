package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/lending"
)

// Действия audit_log модерации заявок.
const (
	AuditApplicationApprove = "application.approve"
	AuditApplicationReject  = "application.reject"
)

const applicationNotFound = "Заявка не найдена"

// ApproveApplication публикует заявку: moderation → published. Открытые AML-флаги
// публикацию не блокируют (DECISIONS п. 13): они видны на экране модерации, решение за админом.
func (a *Admin) ApproveApplication(ctx context.Context, p service.Principal, appID uuid.UUID, comment *string, meta service.ClientMeta) (lending.Application, error) {
	return a.moderate(ctx, p, meta, appID, moderation{
		action:  lending.AppApprove,
		comment: optionalComment(comment),
		audit:   AuditApplicationApprove,
		notice:  repository.NotificationTypeApplicationPublished,
		title:   "Заявка опубликована: ",
		update: func(ctx context.Context, q repository.Querier, from repository.ApplicationStatus) error {
			_, err := q.ApproveApplication(ctx, repository.ApproveApplicationParams{ID: appID, FromStatus: from})
			return err
		},
	})
}

// RejectApplication возвращает заявку на доработку: moderation → rejected. Причина видна
// заёмщику в moderation_comment (кредиторам — нет, см. lending.applicationFromVisible).
func (a *Admin) RejectApplication(ctx context.Context, p service.Principal, appID uuid.UUID, reason string, meta service.ClientMeta) (lending.Application, error) {
	return a.moderate(ctx, p, meta, appID, moderation{
		action:  lending.AppReject,
		comment: reason,
		audit:   AuditApplicationReject,
		meta:    map[string]any{"reason": reason},
		notice:  repository.NotificationTypeApplicationRejected,
		title:   "Заявка возвращена на доработку: ",
		update: func(ctx context.Context, q repository.Querier, from repository.ApplicationStatus) error {
			_, err := q.RejectApplication(ctx, repository.RejectApplicationParams{
				ID: appID, ModerationComment: pgtype.Text{String: reason, Valid: true}, FromStatus: from,
			})
			return err
		},
	})
}

// moderation — то, чем approve и reject заявки отличаются; порядок шагов — в moderate.
type moderation struct {
	action  lending.ApplicationAction
	comment string
	audit   string
	meta    map[string]any
	notice  repository.NotificationType
	title   string // префикс; дополняется номером заявки
	update  func(ctx context.Context, q repository.Querier, from repository.ApplicationStatus) error
}

// moderate — решение по заявке в одной транзакции: FOR UPDATE → переход → UPDATE с охраной
// from_status → история, уведомление заёмщику, аудит → заявка глазами админа.
func (a *Admin) moderate(ctx context.Context, p service.Principal, meta service.ClientMeta, appID uuid.UUID, m moderation) (lending.Application, error) {
	if err := requireAdmin(p); err != nil {
		return lending.Application{}, err
	}
	var out lending.Application
	err := a.tx.InTx(ctx, func(q repository.Querier) error {
		cur, err := q.LockApplicationByID(ctx, appID)
		if err != nil {
			return pgerr.Map(err, applicationNotFound)
		}
		from := lending.ApplicationStatus(cur.Status)
		to, err := lending.NextApplicationStatus(from, m.action)
		if err != nil {
			return err
		}
		if err := m.update(ctx, q, cur.Status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) { // охрана статуса в WHERE; под блокировкой не должна сработать
				return apperr.InvalidTransition(string(from), string(to))
			}
			return fmt.Errorf("application %s: %w", m.action, err)
		}
		if err := service.RecordStatusChange(ctx, q, service.StatusChange{
			EntityType: repository.EntityTypeApplication, EntityID: appID,
			From: string(from), To: string(to), Actor: &p, Comment: m.comment,
		}); err != nil {
			return err
		}
		if err := a.notifier.NotifyOrganization(ctx, q, cur.BorrowerOrganizationID, service.Notification{
			Type: m.notice, Title: m.title + lending.ApplicationNumber(cur.Number.Int64),
			EntityType: repository.EntityTypeApplication, EntityID: appID.String(),
		}); err != nil {
			return err
		}
		if err := service.WriteAudit(ctx, q, meta, service.AuditEvent{
			Action: m.audit, UserID: &p.UserID, Meta: m.meta,
			EntityType: repository.EntityTypeApplication, EntityID: appID.String(),
		}); err != nil {
			return err
		}
		out, err = lending.LoadApplication(ctx, q, p, appID)
		return err
	})
	return out, err
}
