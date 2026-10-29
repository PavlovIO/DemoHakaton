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
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/kyc"
)

// Действия audit_log решений по KYC.
const (
	AuditKycApprove = "kyc.approve"
	AuditKycReject  = "kyc.reject"
)

const organizationNotFound = "Организация не найдена"

// ApproveKyc верифицирует компанию: under_review → verified.
//
// Порядок проверок под блокировкой строки: переход, затем флаги. Approve уже
// верифицированной компании — INVALID_STATUS_TRANSITION, а не CONFLICT: фронт различает
// «решение уже принято» и «данные мешают решению» по code (DECISIONS п. 6).
func (a *Admin) ApproveKyc(ctx context.Context, p service.Principal, orgID uuid.UUID, comment *string, meta service.ClientMeta) (kyc.OrganizationProfile, error) {
	return a.decideKyc(ctx, p, meta, orgID, kycDecision{
		action:  kyc.KycApprove,
		comment: optionalComment(comment),
		audit:   AuditKycApprove,
		notice:  repository.NotificationTypeKycApproved,
		title:   "Компания подтверждена",
		check: func(ctx context.Context, q repository.Querier) error {
			// Открытый high-флаг — риск, который админ ещё не разобрал (BE-13): верифицировать
			// до решения по флагу нельзя. Флаги ниже high видны в карточке, решение за админом.
			n, err := q.CountOpenHighAmlFlags(ctx, orgID)
			if err != nil {
				return fmt.Errorf("count open high aml flags: %w", err)
			}
			if n > 0 {
				return apperr.Conflict("У компании есть открытые AML-флаги высокой серьёзности")
			}
			return nil
		},
		update: func(ctx context.Context, q repository.Querier) error {
			_, err := q.ApproveKyc(ctx, orgID)
			return err
		},
	})
}

// RejectKyc отклоняет верификацию: under_review → rejected. Причина видна компании
// (kyc_reject_reason в getMyOrganization и /auth/me) и пишется в историю и аудит.
// Флаги не проверяются: отказ риск не увеличивает.
func (a *Admin) RejectKyc(ctx context.Context, p service.Principal, orgID uuid.UUID, reason string, meta service.ClientMeta) (kyc.OrganizationProfile, error) {
	return a.decideKyc(ctx, p, meta, orgID, kycDecision{
		action:  kyc.KycReject,
		comment: reason,
		audit:   AuditKycReject,
		meta:    map[string]any{"reason": reason},
		notice:  repository.NotificationTypeKycRejected,
		title:   "Проверка компании не пройдена: " + reason,
		update: func(ctx context.Context, q repository.Querier) error {
			_, err := q.RejectKyc(ctx, repository.RejectKycParams{
				ID: orgID, KycRejectReason: pgtype.Text{String: reason, Valid: true},
			})
			return err
		},
	})
}

// kycDecision — то, чем approve и reject отличаются; общий порядок шагов — в decideKyc.
type kycDecision struct {
	action  kyc.KycAction
	comment string // в status_history.comment; у reject — причина
	audit   string
	meta    map[string]any
	notice  repository.NotificationType
	title   string
	check   func(ctx context.Context, q repository.Querier) error // nil — проверок сверх перехода нет
	update  func(ctx context.Context, q repository.Querier) error
}

// decideKyc — решение по KYC в одной транзакции: FOR UPDATE → переход → проверки → UPDATE
// с охраной статуса → история, уведомление компании, аудит → профиль для ответа. Блокировка
// сериализует параллельные решения: второе увидит новый статус и получит 409.
func (a *Admin) decideKyc(ctx context.Context, p service.Principal, meta service.ClientMeta, orgID uuid.UUID, d kycDecision) (kyc.OrganizationProfile, error) {
	if err := requireAdmin(p); err != nil {
		return kyc.OrganizationProfile{}, err
	}
	var out kyc.OrganizationProfile
	err := a.tx.InTx(ctx, func(q repository.Querier) error {
		org, err := q.GetOrganizationForUpdate(ctx, orgID)
		if err != nil {
			return pgerr.Map(err, organizationNotFound)
		}
		to, err := kyc.NextKycStatus(org.KycStatus, d.action)
		if err != nil {
			return err
		}
		if d.check != nil {
			if err := d.check(ctx, q); err != nil {
				return err
			}
		}
		if err := d.update(ctx, q); err != nil {
			if errors.Is(err, pgx.ErrNoRows) { // охрана статуса в WHERE; под блокировкой не должна сработать
				return apperr.InvalidTransition(string(org.KycStatus), string(to))
			}
			return fmt.Errorf("kyc %s: %w", d.action, err)
		}
		if err := service.RecordStatusChange(ctx, q, service.StatusChange{
			EntityType: repository.EntityTypeOrganization, EntityID: orgID,
			From: string(org.KycStatus), To: string(to), Actor: &p, Comment: d.comment,
		}); err != nil {
			return err
		}
		if err := a.notifier.NotifyOrganization(ctx, q, orgID, service.Notification{
			Type: d.notice, Title: d.title, EntityType: repository.EntityTypeOrganization, EntityID: orgID.String(),
		}); err != nil {
			return err
		}
		if err := service.WriteAudit(ctx, q, meta, service.AuditEvent{
			Action: d.audit, UserID: &p.UserID, Meta: d.meta,
			EntityType: repository.EntityTypeOrganization, EntityID: orgID.String(),
		}); err != nil {
			return err
		}
		out, err = kyc.LoadProfile(ctx, q, orgID)
		return err
	})
	return out, err
}

// optionalComment — необязательный comment из AdminComment; nil → "" (NULL в истории).
func optionalComment(c *string) string {
	if c == nil {
		return ""
	}
	return *c
}
