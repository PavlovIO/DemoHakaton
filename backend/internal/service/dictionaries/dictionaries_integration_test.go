package dictionaries_test

// Интеграционный тест: настоящий Postgres с применёнными миграциями (make db / CI).
// Без DATABASE_URL или с -short (pre-push хук) пропускается.

import (
	"context"
	"testing"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/dictionaries"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

func TestDictionariesIntegration(t *testing.T) {
	svc := dictionaries.NewDictionaries(repository.New(servicetest.NewPool(t)))
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
