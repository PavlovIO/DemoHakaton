package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Минимальная версия хелпера из BE-07, шаг 2 (README планов, «Общие куски кода»).

// StatusChange — один переход статуса сущности для журнала status_history.
type StatusChange struct {
	EntityType repository.EntityType
	EntityID   uuid.UUID
	From       string // "" — сущность создана (from_status = NULL)
	To         string
	Actor      *Principal // nil — переход сделала система
	Comment    string     // "" → NULL
}

// StatusHistoryWriter — подмножество Querier для записи истории.
type StatusHistoryWriter interface {
	CreateStatusHistory(ctx context.Context, arg repository.CreateStatusHistoryParams) (repository.StatusHistory, error)
}

// RecordStatusChange пишет переход. Вызывать только внутри InTx с тем же q, что и сам
// переход: история атомарна со сменой статуса.
func RecordStatusChange(ctx context.Context, q StatusHistoryWriter, c StatusChange) error {
	arg := repository.CreateStatusHistoryParams{
		EntityType: c.EntityType,
		EntityID:   c.EntityID,
		FromStatus: pgtype.Text{String: c.From, Valid: c.From != ""},
		ToStatus:   c.To,
		Comment:    pgtype.Text{String: c.Comment, Valid: c.Comment != ""},
	}
	if c.Actor != nil {
		arg.ActorUserID = pgtype.UUID{Bytes: c.Actor.UserID, Valid: true}
		arg.ActorRole = repository.NullUserRole{UserRole: c.Actor.ViewerRole(), Valid: true}
	}
	if _, err := q.CreateStatusHistory(ctx, arg); err != nil {
		return fmt.Errorf("status history %s %s: %w", c.EntityType, c.EntityID, err)
	}
	return nil
}
