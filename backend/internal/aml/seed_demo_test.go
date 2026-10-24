package aml

import (
	"context"
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml/sanctionslist"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

const seedCommonPath = "../../migrations/testdata/seed_demo_common.sql"

// Строки VALUES сида BE-16: организация, бенефициар, AML-флаг. Бенефициары ищутся только
// внутри INSERT INTO beneficiaries, а их число сверяется со строками VALUES (seedValuesRow):
// строка, которую регулярка не разобрала, — ошибка теста, а не молча нескриненный человек.
var (
	seedBeneficiaries = regexp.MustCompile(`(?s)INSERT INTO beneficiaries .*?;`)
	seedValuesRow     = regexp.MustCompile(`(?m)^\s*\(`)
	seedOrgRow        = regexp.MustCompile(
		`\('([0-9a-f-]{36})', '(?:borrower|lender)', '([^']+)', '([A-Z]{2})', '([^']+)', (?:'([A-Z0-9]{20})'|NULL)`)
	seedBeneficiaryRow = regexp.MustCompile(`\(\s*'[0-9a-f-]{36}',\s*'([0-9a-f-]{36})',\s*'([^']+)',\s*'([A-Z]{2})'`)
	seedFlagRow        = regexp.MustCompile(
		`\('([0-9a-f-]{36})', '(sanctions_match|high_risk_jurisdiction|amount_threshold|registry_mismatch|other)', '(low|medium|high)',\s*'([^']+)',\s*'([^']+)'`)
)

// seedFATF — списки FATF стран демо-компаний и бенефициаров на 2026-06-19 (миграция 00005).
var seedFATF = map[string]repository.FatfList{"KW": repository.FatfListGrey}

// Сид вставляет флаги SQL-ом, не запуская скрининг (DECISIONS п. 45). Флаг должен быть
// ровно тем, что дал бы RuleScreener на submitKyc: иначе повторная подача на демо не узнает
// его по match_key и создаст дубль, а лишняя находка у компании без флага в сиде (high на
// «Альфе» или Beta) заблокировала бы одобрение KYC в US-14. Тест прогоняет скрининг по
// каждой компании сида с мок-списком санкций и мок-реестром и сверяет с её флагами.
func TestSeedFlagDetails(t *testing.T) {
	sql, err := os.ReadFile(seedCommonPath)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{candidates: mockSanctionsCandidates(t), fatf: seedFATF}
	block := seedBeneficiaries.FindString(string(sql))
	parsed := seedBeneficiaryRow.FindAllStringSubmatch(block, -1)
	if rows := len(seedValuesRow.FindAllString(block, -1)); block == "" || len(parsed) != rows {
		t.Fatalf("parsed %d beneficiaries of %d VALUES rows: update seedBeneficiaryRow", len(parsed), rows)
	}
	beneficiaries := map[string][]Person{}
	for _, b := range parsed {
		beneficiaries[b[1]] = append(beneficiaries[b[1]], Person{FullName: b[2], CitizenshipCountryCode: b[3]})
	}
	seedFlags := map[string][]Finding{}
	for _, f := range seedFlagRow.FindAllStringSubmatch(string(sql), -1) {
		seedFlags[f[1]] = append(seedFlags[f[1]], Finding{
			Rule: repository.AmlRule(f[2]), Severity: repository.AmlSeverity(f[3]), Details: f[4], MatchKey: f[5],
		})
	}

	orgs := seedOrgRow.FindAllStringSubmatch(string(sql), -1)
	if len(orgs) != len(demoCompanies) {
		t.Fatalf("seed organizations = %d, want %d", len(orgs), len(demoCompanies))
	}
	for _, org := range orgs {
		id, name, country, regNumber, lei := org[1], org[2], org[3], org[4], org[5]
		t.Run(name, func(t *testing.T) {
			if len(beneficiaries[id]) == 0 {
				t.Fatal("no beneficiaries: submitKyc requires at least one")
			}
			subj := OrganizationSubject{
				Name: name, CountryCode: country, Beneficiaries: beneficiaries[id],
				Registry: mockRegistryResult(t, country, regNumber, lei),
			}
			findings, err := RuleScreener{}.ScreenOrganization(context.Background(), store, subj)
			if err != nil {
				t.Fatal(err)
			}
			got := withMatchKeys(findings)
			want := seedFlags[id]
			slices.SortFunc(got, compareFindings)
			slices.SortFunc(want, compareFindings)
			if !slices.Equal(got, want) {
				t.Errorf("screening findings:\n  %+v\nseed flags:\n  %+v", got, want)
			}
		})
	}
}

// mockSanctionsCandidates — мок-список BE-13, как его загружает make load-sanctions.
func mockSanctionsCandidates(t *testing.T) []repository.SearchSanctionsRow {
	t.Helper()
	entries, err := sanctionslist.Mock()
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]repository.SearchSanctionsRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, candidate(e.ExternalID, e.EntityKind, e.Name))
	}
	return rows
}

// mockRegistryResult — сверка с мок-реестром BE-06 так, как её делает submitKyc: ИНН для
// RU, LEI для остальных.
func mockRegistryResult(t *testing.T, country, regNumber, lei string) *RegistryResult {
	t.Helper()
	number := lei
	if country == "RU" {
		number = regNumber
	}
	r, err := registry.NewMock().Lookup(context.Background(), country, number)
	if err != nil {
		t.Fatal(err)
	}
	return &RegistryResult{Outcome: RegistryChecked, Found: r.Found, Names: []string{r.Name, r.FullName}, Status: r.Status}
}

// withMatchKeys — пустой MatchKey сохраняется как details (SaveAmlFindings).
func withMatchKeys(findings []Finding) []Finding {
	out := slices.Clone(findings)
	for i := range out {
		if out[i].MatchKey == "" {
			out[i].MatchKey = out[i].Details
		}
	}
	return out
}

func compareFindings(a, b Finding) int {
	switch {
	case a.MatchKey < b.MatchKey:
		return -1
	case a.MatchKey > b.MatchKey:
		return 1
	}
	return 0
}
