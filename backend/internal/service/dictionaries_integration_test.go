package service_test

// Интеграционный тест: настоящий Postgres с применёнными миграциями (make db / CI).
// Без DATABASE_URL или с -short (pre-push хук) пропускается.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestDictionariesIntegration(t *testing.T) {
	svc := service.NewDictionaries(repository.New(newPool(t)))
	ctx := context.Background()

	countries, err := svc.Countries(ctx)
	if err != nil {
		t.Fatalf("Countries: %v", err)
	}
	risk := map[string]bool{}
	for _, c := range countries {
		risk[c.Code] = c.FatfHighRisk
	}
	// fatf_high_risk = fatf_list IS NOT NULL (сид 00002: IR в чёрном списке, DE — нет).
	if v, ok := risk["IR"]; !ok || !v {
		t.Errorf("IR: present=%v fatf_high_risk=%v, want true", ok, v)
	}
	if v, ok := risk["DE"]; !ok || v {
		t.Errorf("DE: present=%v fatf_high_risk=%v, want false", ok, v)
	}

	currencies, err := svc.Currencies(ctx)
	if err != nil || len(currencies) == 0 {
		t.Fatalf("Currencies: %d rows, err %v", len(currencies), err)
	}

	methods, err := svc.TransferMethods(ctx)
	if err != nil || len(methods) == 0 {
		t.Fatalf("TransferMethods: %d rows, err %v", len(methods), err)
	}
}
