package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// SaveAmlFindings сохраняет находки скрининга флагами (BE-13, шаг 6) в транзакции
// перехода статуса: флаги атомарны с переходом. Находка, похожая на уже открытый или
// ложный флаг, пропускается (HasSimilarAmlFlag по Finding.MatchKey, пустой — по Details). Возвращает только созданные флаги;
// уведомление шлёт вызывающий.
func SaveAmlFindings(ctx context.Context, q repository.Querier, orgID uuid.UUID, appID *uuid.UUID, findings []aml.Finding) ([]repository.AmlFlag, error) {
	var application pgtype.UUID
	if appID != nil {
		application = pgtype.UUID{Bytes: *appID, Valid: true}
	}
	var created []repository.AmlFlag
	for _, f := range findings {
		key := f.MatchKey
		if key == "" {
			key = f.Details
		}
		exists, err := q.HasSimilarAmlFlag(ctx, repository.HasSimilarAmlFlagParams{
			OrganizationID: orgID, ApplicationID: application, Rule: f.Rule, MatchKey: key,
		})
		if err != nil {
			return nil, fmt.Errorf("aml flag dedup: %w", err)
		}
		if exists {
			continue
		}
		flag, err := q.CreateAmlFlag(ctx, repository.CreateAmlFlagParams{
			OrganizationID: orgID, ApplicationID: application, Rule: f.Rule, Severity: f.Severity, Details: f.Details,
			MatchKey: pgtype.Text{String: key, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("create aml flag: %w", err)
		}
		created = append(created, flag)
	}
	return created, nil
}
