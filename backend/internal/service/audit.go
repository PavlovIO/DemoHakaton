package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Все записи audit_log идут через WriteAudit (BE-14). Константы действий живут в доменных
// пакетах: auth, kyc, lending, admin.

// ClientMeta содержит сетевую информацию клиента для сессии и аудита.
type ClientMeta struct {
	IP        *netip.Addr
	UserAgent string
}

// IPKey — IP клиента как ключ лимитов; без IP все такие запросы делят один ключ.
func (m ClientMeta) IPKey() string {
	if m.IP == nil {
		return "unknown"
	}
	return m.IP.String()
}

// AuditEvent — одно действие для audit_log.
type AuditEvent struct {
	Action     string                // константа Audit*
	UserID     *uuid.UUID            // nil — аноним
	EntityType repository.EntityType // "" — без сущности
	EntityID   string
	Meta       map[string]any // только идентификаторы и причины: без токенов и содержимого документов
}

// AuditWriter — часть repository.Querier для записи аудита; в транзакции действия — её q.
type AuditWriter interface {
	CreateAuditLog(ctx context.Context, arg repository.CreateAuditLogParams) (repository.AuditLog, error)
}

// WriteAudit пишет действие с IP и User-Agent клиента. Внутри транзакции действия
// ошибка откатывает действие: действие без следа в журнале хуже отказа (BE-14).
func WriteAudit(ctx context.Context, w AuditWriter, meta ClientMeta, e AuditEvent) error {
	fields := e.Meta
	if fields == nil {
		fields = map[string]any{} // колонка NOT NULL DEFAULT '{}', а json.Marshal(nil) даёт null
	}
	metaJSON, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("audit %s: meta: %w", e.Action, err)
	}
	arg := repository.CreateAuditLogParams{
		Action:    e.Action,
		Ip:        meta.IP,
		UserAgent: pgtype.Text{String: meta.UserAgent, Valid: meta.UserAgent != ""},
		Meta:      metaJSON,
	}
	if e.UserID != nil {
		arg.UserID = pgtype.UUID{Bytes: *e.UserID, Valid: true}
	}
	if e.EntityType != "" {
		arg.EntityType = repository.NullEntityType{EntityType: e.EntityType, Valid: true}
		arg.EntityID = pgtype.Text{String: e.EntityID, Valid: true}
	}
	if _, err := w.CreateAuditLog(ctx, arg); err != nil {
		return fmt.Errorf("audit %s: %w", e.Action, err)
	}
	return nil
}
