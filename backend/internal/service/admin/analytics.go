package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/pgconv"
)

// DefaultTopN — длина топов стран, если top_n не задан (default в контракте).
const DefaultTopN = 10

// AnalyticsFilter — период сводки [CreatedFrom, CreatedTo) и длина топов стран.
// nil — граница не задана; TopN <= 0 — DefaultTopN.
type AnalyticsFilter struct {
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	TopN        int
}

// AnalyticsSummary — сводка админа за период.
type AnalyticsSummary struct {
	ApplicationsTotal    int64
	ApplicationsByStatus map[string]int64 // только статусы, в которых есть заявки
	OffersTotal          int64
	DealsTotal           int64
	AmountsByCurrency    []CurrencyStat
	TopDemandCountries   []CountryStat
	TopSupplyCountries   []CountryStat
}

// CurrencyStat — заявки одной валюты: суммы без конвертации.
type CurrencyStat struct {
	Currency      string
	Count         int64
	TotalAmount   decimal.Decimal
	AverageAmount decimal.Decimal
}

// CountryStat — страна и число заявок (спрос) или офферов (предложение).
type CountryStat struct {
	CountryCode string
	Count       int64
}

// AnalyticsSummary считает сводку за период шестью запросами в одном снимке: иначе заявка,
// созданная между запросами, попала бы в applications_total, но не в суммы по валютам.
func (a *Admin) AnalyticsSummary(ctx context.Context, p service.Principal, f AnalyticsFilter) (AnalyticsSummary, error) {
	if err := requireAdmin(p); err != nil {
		return AnalyticsSummary{}, err
	}
	if err := validatePeriod(f.CreatedFrom, f.CreatedTo); err != nil {
		return AnalyticsSummary{}, err
	}
	topN := int32(DefaultTopN)
	if f.TopN > 0 {
		topN = int32(f.TopN)
	}
	from, to := pgconv.Timestamptz(f.CreatedFrom), pgconv.Timestamptz(f.CreatedTo)

	out := AnalyticsSummary{ApplicationsByStatus: map[string]int64{}}
	err := a.tx.ReadSnapshot(ctx, func(q repository.Querier) error {
		var err error
		if out.ApplicationsTotal, out.ApplicationsByStatus, err = applicationsByStatus(ctx, q, from, to); err != nil {
			return err
		}
		if out.OffersTotal, err = q.CountOffersInPeriod(ctx, repository.CountOffersInPeriodParams{CreatedFrom: from, CreatedTo: to}); err != nil {
			return fmt.Errorf("count offers: %w", err)
		}
		if out.DealsTotal, err = q.CountDealsInPeriod(ctx, repository.CountDealsInPeriodParams{CreatedFrom: from, CreatedTo: to}); err != nil {
			return fmt.Errorf("count deals: %w", err)
		}
		if out.AmountsByCurrency, err = amountsByCurrency(ctx, q, from, to); err != nil {
			return err
		}
		if out.TopDemandCountries, err = topDemand(ctx, q, from, to, topN); err != nil {
			return err
		}
		out.TopSupplyCountries, err = topSupply(ctx, q, from, to, topN)
		return err
	})
	if err != nil {
		return AnalyticsSummary{}, err
	}
	return out, nil
}

// applicationsByStatus — заявки по статусам и их сумма: total не считается отдельным
// запросом, чтобы всегда совпадать с суммой статусов.
func applicationsByStatus(ctx context.Context, q repository.Querier, from, to pgtype.Timestamptz) (int64, map[string]int64, error) {
	rows, err := q.CountApplicationsByStatus(ctx, repository.CountApplicationsByStatusParams{CreatedFrom: from, CreatedTo: to})
	if err != nil {
		return 0, nil, fmt.Errorf("count applications by status: %w", err)
	}
	var total int64
	byStatus := make(map[string]int64, len(rows))
	for _, row := range rows {
		byStatus[string(row.Status)] = row.Count
		total += row.Count
	}
	return total, byStatus, nil
}

func amountsByCurrency(ctx context.Context, q repository.Querier, from, to pgtype.Timestamptz) ([]CurrencyStat, error) {
	rows, err := q.GetAnalyticsSummaryByCurrency(ctx, repository.GetAnalyticsSummaryByCurrencyParams{CreatedFrom: from, CreatedTo: to})
	if err != nil {
		return nil, fmt.Errorf("amounts by currency: %w", err)
	}
	stats := make([]CurrencyStat, 0, len(rows))
	for _, row := range rows {
		stats = append(stats, CurrencyStat{Currency: row.Currency, Count: row.Count, TotalAmount: row.TotalAmount, AverageAmount: row.AverageAmount})
	}
	return stats, nil
}

func topDemand(ctx context.Context, q repository.Querier, from, to pgtype.Timestamptz, topN int32) ([]CountryStat, error) {
	rows, err := q.GetTopDemandCountries(ctx, repository.GetTopDemandCountriesParams{CreatedFrom: from, CreatedTo: to, TopN: topN})
	if err != nil {
		return nil, fmt.Errorf("top demand countries: %w", err)
	}
	stats := make([]CountryStat, 0, len(rows))
	for _, row := range rows {
		stats = append(stats, CountryStat{CountryCode: row.CountryCode, Count: int64(row.Count)})
	}
	return stats, nil
}

func topSupply(ctx context.Context, q repository.Querier, from, to pgtype.Timestamptz, topN int32) ([]CountryStat, error) {
	rows, err := q.GetTopSupplyCountries(ctx, repository.GetTopSupplyCountriesParams{CreatedFrom: from, CreatedTo: to, TopN: topN})
	if err != nil {
		return nil, fmt.Errorf("top supply countries: %w", err)
	}
	stats := make([]CountryStat, 0, len(rows))
	for _, row := range rows {
		stats = append(stats, CountryStat{CountryCode: row.CountryCode, Count: int64(row.Count)})
	}
	return stats, nil
}

// validatePeriod отклоняет обратный период: [from, to) с from > to всегда пуст, и пустой
// ответ выглядел бы как «за период ничего не было».
func validatePeriod(from, to *time.Time) error {
	if from != nil && to != nil && from.After(*to) {
		return apperr.Validation("Некорректный период",
			apperr.FieldError{Field: "created_to", Message: "должно быть не раньше created_from"})
	}
	return nil
}
