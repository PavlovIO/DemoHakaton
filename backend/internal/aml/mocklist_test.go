package aml

import (
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml/sanctionslist"
)

// demoCompanies — названия 8 демо-компаний (BE-16, шаг 1). Бенефициаров сида проверяет
// TestSeedFlagDetails.
var demoCompanies = []string{
	"ООО «Альфа»", "Beta Corp Ltd", "Gamma GmbH", "АО «Вектор»",
	"Delta Trading", "Epsilon Capital Pte", "ООО «Омега»", "ООО «Сигма»",
}

// Мок-список даёт на демо ровно одно совпадение: GAMA GmbH ↔ Gamma GmbH (high). Флаг
// high на другой компании заблокировал бы одобрение KYC в сценарии US-14.
func TestMockListAgainstDemoCompanies(t *testing.T) {
	entries, err := sanctionslist.Mock()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		for _, company := range demoCompanies {
			best := 0.0
			for _, q := range NormalizeVariants(company) {
				best = max(best, nameScore(q, Normalize(e.Name)))
			}
			isGamaPair := e.ExternalID == "demo-gama-001" && company == "Gamma GmbH"
			switch {
			case isGamaPair && best < sanctionsHighScore:
				t.Errorf("GAMA GmbH ↔ Gamma GmbH: score %.4f < %.2f", best, sanctionsHighScore)
			case !isGamaPair && best >= sanctionsMediumScore:
				t.Errorf("%s «%s» ↔ «%s»: score %.4f ≥ %.2f", e.ExternalID, e.Name, company, best, sanctionsMediumScore)
			}
		}
	}
}
