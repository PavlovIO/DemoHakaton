package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/pgconv"
)

// AuditLogFilter — фильтры журнала аудита. nil — фильтр не задан; период —
// полуинтервал [CreatedFrom, CreatedTo).
type AuditLogFilter struct {
	UserID      *uuid.UUID
	Action      *string
	EntityType  *string
	EntityID    *string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Page        int
	PageSize    int
}

// AuditLogEntry — строка журнала. nil — значение не записано (аноним, действие без сущности).
type AuditLogEntry struct {
	ID         int64
	UserID     *uuid.UUID
	Action     string
	EntityType *string
	EntityID   *string
	IP         *netip.Addr
	UserAgent  *string
	Meta       map[string]any
	CreatedAt  time.Time
}

// AuditLogPage — страница журнала.
type AuditLogPage struct {
	Items    []AuditLogEntry
	Total    int64
	Page     int
	PageSize int
}

// ListAuditLog — журнал действий пользователей, новые сверху.
func (a *Admin) ListAuditLog(ctx context.Context, p service.Principal, f AuditLogFilter) (AuditLogPage, error) {
	if err := requireAdmin(p); err != nil {
		return AuditLogPage{}, err
	}
	if err := validatePeriod(f.CreatedFrom, f.CreatedTo); err != nil {
		return AuditLogPage{}, err
	}
	page, size, limit, offset := service.PageWindow(f.Page, f.PageSize)
	count := repository.CountAuditLogParams{
		UserID:      pgconv.UUID(f.UserID),
		Action:      pgconv.Text(f.Action),
		EntityID:    pgconv.Text(f.EntityID),
		CreatedFrom: pgconv.Timestamptz(f.CreatedFrom),
		CreatedTo:   pgconv.Timestamptz(f.CreatedTo),
	}
	if f.EntityType != nil {
		count.EntityType = repository.NullEntityType{EntityType: repository.EntityType(*f.EntityType), Valid: true}
	}
	out := AuditLogPage{Items: []AuditLogEntry{}, Page: page, PageSize: size}
	// Страница и total — в одном снимке: журнал пишется постоянно, и запись между
	// запросами иначе попала бы в total, но не в items.
	err := a.tx.ReadSnapshot(ctx, func(q repository.Querier) error {
		rows, err := q.ListAuditLog(ctx, repository.ListAuditLogParams{
			UserID: count.UserID, Action: count.Action, EntityType: count.EntityType, EntityID: count.EntityID,
			CreatedFrom: count.CreatedFrom, CreatedTo: count.CreatedTo,
			Limit: limit, Offset: offset,
		})
		if err != nil {
			return fmt.Errorf("list audit log: %w", err)
		}
		for _, row := range rows {
			entry, err := auditLogEntryFromRow(row)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, entry)
		}
		out.Total, err = q.CountAuditLog(ctx, count)
		if err != nil {
			return fmt.Errorf("count audit log: %w", err)
		}
		return nil
	})
	if err != nil {
		return AuditLogPage{}, err
	}
	return out, nil
}

func auditLogEntryFromRow(row repository.AuditLog) (AuditLogEntry, error) {
	entry := AuditLogEntry{
		ID:        row.ID,
		UserID:    pgconv.UUIDPtr(row.UserID),
		Action:    row.Action,
		EntityID:  pgconv.TextPtr(row.EntityID),
		IP:        row.Ip,
		UserAgent: pgconv.TextPtr(row.UserAgent),
		Meta:      map[string]any{},
		CreatedAt: row.CreatedAt.Time,
	}
	if row.EntityType.Valid {
		entityType := string(row.EntityType.EntityType)
		entry.EntityType = &entityType
	}
	if err := json.Unmarshal(row.Meta, &entry.Meta); err != nil {
		return AuditLogEntry{}, fmt.Errorf("audit log %d: meta: %w", row.ID, err)
	}
	return entry, nil
}
