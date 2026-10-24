package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/pgconv"
)

// AuditAmlFlagResolve — действие audit_log решения по AML-флагу.
const AuditAmlFlagResolve = "aml_flag.resolve"

// AmlFlagFilter — фильтры списка флагов. nil — фильтр не задан.
type AmlFlagFilter struct {
	Status         *string
	Severity       *string
	OrganizationID *uuid.UUID
	ApplicationID  *uuid.UUID
	Page           int
	PageSize       int
}

// AmlFlagPage — страница флагов.
type AmlFlagPage struct {
	Items    []AmlFlag
	Total    int64
	Page     int
	PageSize int
}

// ListAmlFlags — флаги всех компаний для разбора админом, новые сверху.
func (a *Admin) ListAmlFlags(ctx context.Context, p service.Principal, f AmlFlagFilter) (AmlFlagPage, error) {
	if err := requireAdmin(p); err != nil {
		return AmlFlagPage{}, err
	}
	page, size, limit, offset := service.PageWindow(f.Page, f.PageSize)
	count := repository.CountAmlFlagsParams{
		OrganizationID: pgconv.UUID(f.OrganizationID),
		ApplicationID:  pgconv.UUID(f.ApplicationID),
	}
	if f.Status != nil {
		count.Status = repository.NullAmlFlagStatus{AmlFlagStatus: repository.AmlFlagStatus(*f.Status), Valid: true}
	}
	if f.Severity != nil {
		count.Severity = repository.NullAmlSeverity{AmlSeverity: repository.AmlSeverity(*f.Severity), Valid: true}
	}
	out := AmlFlagPage{Items: []AmlFlag{}, Page: page, PageSize: size}
	// Страница и total — в одном снимке, как у списка компаний: флаг, созданный между
	// запросами, иначе попал бы в total, но не в items.
	err := a.tx.ReadSnapshot(ctx, func(q repository.Querier) error {
		rows, err := q.ListAmlFlags(ctx, repository.ListAmlFlagsParams{
			Status: count.Status, Severity: count.Severity,
			OrganizationID: count.OrganizationID, ApplicationID: count.ApplicationID,
			Limit: limit, Offset: offset,
		})
		if err != nil {
			return fmt.Errorf("list aml flags: %w", err)
		}
		for _, r := range rows {
			out.Items = append(out.Items, amlFlagFromParts(r.AmlFlag, r.OrganizationName, r.OrganizationCountryCode))
		}
		if out.Total, err = q.CountAmlFlags(ctx, count); err != nil {
			return fmt.Errorf("count aml flags: %w", err)
		}
		return nil
	})
	if err != nil {
		return AmlFlagPage{}, err
	}
	return out, nil
}

// ResolveAmlFlag закрывает открытый флаг: open → false_positive | confirmed. Закрытый флаг
// не переоткрывается и не меняет решение — 409 INVALID_STATUS_TRANSITION.
func (a *Admin) ResolveAmlFlag(ctx context.Context, p service.Principal, id int64,
	resolution repository.AmlFlagStatus, comment string, meta service.ClientMeta,
) (AmlFlag, error) {
	if err := requireAdmin(p); err != nil {
		return AmlFlag{}, err
	}
	var out AmlFlag
	err := a.tx.InTx(ctx, func(q repository.Querier) error {
		// Охрана status = 'open' в WHERE: два админа, закрывающие флаг одновременно, не
		// перезапишут решения друг друга — второй получит 409.
		_, err := q.ResolveAmlFlag(ctx, repository.ResolveAmlFlagParams{
			ID: id, Status: resolution,
			ResolvedByUserID:  pgtype.UUID{Bytes: p.UserID, Valid: true},
			ResolutionComment: pgtype.Text{String: comment, Valid: true},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return resolveMiss(ctx, q, id, resolution)
		}
		if err != nil {
			return fmt.Errorf("resolve aml flag: %w", err)
		}
		if err := service.WriteAudit(ctx, q, meta, service.AuditEvent{
			Action: AuditAmlFlagResolve, UserID: &p.UserID,
			EntityType: repository.EntityTypeAmlFlag, EntityID: fmt.Sprint(id),
			Meta: map[string]any{"resolution": string(resolution), "comment": comment},
		}); err != nil {
			return err
		}
		row, err := q.GetAmlFlagByID(ctx, id)
		if err != nil {
			return fmt.Errorf("get aml flag: %w", err)
		}
		out = amlFlagFromParts(row.AmlFlag, row.OrganizationName, row.OrganizationCountryCode)
		return nil
	})
	if err != nil {
		return AmlFlag{}, err
	}
	return out, nil
}

// resolveMiss различает, почему ResolveAmlFlag не нашёл строку: флага нет — 404, флаг уже
// закрыт — 409 (контракт adminResolveAmlFlag).
func resolveMiss(ctx context.Context, q repository.Querier, id int64, resolution repository.AmlFlagStatus) error {
	row, err := q.GetAmlFlagByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound("AML-флаг не найден")
	}
	if err != nil {
		return fmt.Errorf("get aml flag: %w", err)
	}
	return apperr.InvalidTransition(string(row.AmlFlag.Status), string(resolution))
}
