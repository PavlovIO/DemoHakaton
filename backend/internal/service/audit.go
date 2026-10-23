package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Минимальная версия хелпера аудита из BE-14, шаг 1 (README планов, «Общие куски кода»):
// BE-14 переносит сюда ClientMeta и переводит Auth.audit на writeAudit.

// Действия audit_log, которые пишет BE-05. Формат — <сущность>.<глагол>.
const (
	AuditDocumentUpload   = "document.upload"
	AuditDocumentDownload = "document.download" // только скачивание админом (DECISIONS п. 16)
	AuditKycSubmit        = "kyc.submit"
)

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

// writeAudit пишет действие с IP и User-Agent клиента. Внутри транзакции действия
// ошибка откатывает действие: действие без следа в журнале хуже отказа (BE-14).
func writeAudit(ctx context.Context, w AuditWriter, meta ClientMeta, e AuditEvent) error {
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
