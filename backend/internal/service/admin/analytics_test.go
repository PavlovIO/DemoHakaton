package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// analyticsDB — шесть запросов сводки; запоминает top_n и число снимков.
type analyticsDB struct {
	repository.Querier
	byStatus   []repository.CountApplicationsByStatusRow
	demandTopN int32
	supplyTopN int32
	snapshots  int
}

func (d *analyticsDB) InTx(_ context.Context, fn func(q repository.Querier) error) error {
	return fn(d)
}

func (d *analyticsDB) ReadSnapshot(_ context.Context, fn func(q repository.Querier) error) error {
	d.snapshots++
	return fn(d)
}

func (d *analyticsDB) CountApplicationsByStatus(context.Context, repository.CountApplicationsByStatusParams) ([]repository.CountApplicationsByStatusRow, error) {
	return d.byStatus, nil
}

func (d *analyticsDB) CountOffersInPeriod(context.Context, repository.CountOffersInPeriodParams) (int64, error) {
	return 0, nil
}

func (d *analyticsDB) CountDealsInPeriod(context.Context, repository.CountDealsInPeriodParams) (int64, error) {
	return 0, nil
}

func (d *analyticsDB) GetAnalyticsSummaryByCurrency(context.Context, repository.GetAnalyticsSummaryByCurrencyParams) ([]repository.GetAnalyticsSummaryByCurrencyRow, error) {
	return nil, nil
}

func (d *analyticsDB) GetTopDemandCountries(_ context.Context, arg repository.GetTopDemandCountriesParams) ([]repository.GetTopDemandCountriesRow, error) {
	d.demandTopN = arg.TopN
	return nil, nil
}

func (d *analyticsDB) GetTopSupplyCountries(_ context.Context, arg repository.GetTopSupplyCountriesParams) ([]repository.GetTopSupplyCountriesRow, error) {
	d.supplyTopN = arg.TopN
	return nil, nil
}

func TestAnalyticsSummary(t *testing.T) {
	t.Run("total — сумма статусов, пустые выборки — пустые массивы", func(t *testing.T) {
		db := &analyticsDB{byStatus: []repository.CountApplicationsByStatusRow{
			{Status: repository.ApplicationStatusPublished, Count: 4},
			{Status: repository.ApplicationStatusDraft, Count: 3},
		}}
		got, err := admin.New(admin.Deps{Tx: db}).AnalyticsSummary(context.Background(), adminPrincipal(), admin.AnalyticsFilter{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.ApplicationsTotal != 7 || got.ApplicationsByStatus["published"] != 4 || got.ApplicationsByStatus["draft"] != 3 {
			t.Fatalf("applications = %d %v, want 7 {published:4 draft:3}", got.ApplicationsTotal, got.ApplicationsByStatus)
		}
		if got.AmountsByCurrency == nil || got.TopDemandCountries == nil || got.TopSupplyCountries == nil {
			t.Fatalf("nil slices in %+v: в JSON будут null", got)
		}
		if db.demandTopN != admin.DefaultTopN || db.supplyTopN != admin.DefaultTopN {
			t.Errorf("top_n = %d/%d, want default %d", db.demandTopN, db.supplyTopN, admin.DefaultTopN)
		}
		if db.snapshots != 1 {
			t.Errorf("snapshots = %d, want 1", db.snapshots)
		}
	})

	t.Run("from > to — 400 created_to", func(t *testing.T) {
		from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		_, err := admin.New(admin.Deps{Tx: &analyticsDB{}}).AnalyticsSummary(context.Background(), adminPrincipal(),
			admin.AnalyticsFilter{CreatedFrom: &from, CreatedTo: &to})
		servicetest.WantValidationField(t, err, "created_to")
	})

	t.Run("не админ — 403", func(t *testing.T) {
		_, err := admin.New(admin.Deps{Tx: &analyticsDB{}}).AnalyticsSummary(context.Background(), borrowerPrincipal(), admin.AnalyticsFilter{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})
}
