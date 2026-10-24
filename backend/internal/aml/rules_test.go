package aml

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// fakeStore отдаёт заданных кандидатов нужного entity_kind (как SQL) и FATF-статусы по коду страны.
type fakeStore struct {
	candidates []repository.SearchSanctionsRow
	fatf       map[string]repository.FatfList
	err        error
	queries    []string // нормализованные запросы SearchSanctions
}

func (s *fakeStore) SearchSanctions(_ context.Context, p repository.SearchSanctionsParams) ([]repository.SearchSanctionsRow, error) {
	s.queries = append(s.queries, p.Query)
	var out []repository.SearchSanctionsRow
	for _, c := range s.candidates {
		if slices.Contains(p.EntityKinds, c.EntityKind) {
			out = append(out, c)
		}
	}
	return out, s.err
}

func (s *fakeStore) ListCountriesFatf(_ context.Context, codes []string) ([]repository.ListCountriesFatfRow, error) {
	if s.err != nil {
		return nil, s.err
	}
	var rows []repository.ListCountriesFatfRow
	for _, c := range codes {
		if list, ok := s.fatf[c]; ok {
			rows = append(rows, repository.ListCountriesFatfRow{
				Code:     c,
				FatfList: repository.NullFatfList{FatfList: list, Valid: true},
				FatfAsOf: pgtype.Date{Time: fatfAsOf, Valid: true},
			})
		}
	}
	return rows, nil
}

func candidate(id, kind, name string) repository.SearchSanctionsRow {
	return repository.SearchSanctionsRow{Source: "mock", ExternalID: id, EntityKind: kind, Name: name, NameNormalized: Normalize(name)}
}

// Пары с известным JW: a целиком входит в b со сдвигом на 1 — префикса и транспозиций нет,
// jaro = (2 + |a|/|b|)/3.
const (
	shortSubject = "bcdefgh"     // 7 букв
	score090     = "xbcdefghyz"  // 7/10 → 0.90
	score088     = "xbcdefghyzq" // 7/11 → 0.8788

	longSubject = "bcdefghijkl"           // 11 букв
	score085    = "xbcdefghijklyyyyyyyy"  // 11/20 → 0.85
	score084    = "xbcdefghijklyyyyyyyyy" // 11/21 → 0.8413
)

var fatfAsOf = time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)

func beneficiary(name string) OrganizationSubject {
	return OrganizationSubject{Name: "Zeta", CountryCode: "RU", Beneficiaries: []Person{{FullName: name, CitizenshipCountryCode: "RU"}}}
}

func TestScreenOrganizationSanctions(t *testing.T) {
	cases := []struct {
		name       string
		subject    OrganizationSubject
		candidates []repository.SearchSanctionsRow
		want       []Finding
		wantDetail []string // подстроки details единственной находки
	}{
		{
			name:       "точное совпадение бенефициара",
			subject:    beneficiary("Ivanov Ivan"),
			candidates: []repository.SearchSanctionsRow{candidate("M-002", "person", "IVANOV Ivan")},
			want:       []Finding{{Rule: repository.AmlRuleSanctionsMatch, Severity: repository.AmlSeverityHigh}},
			wantDetail: []string{"score 1.00", "mock", "M-002", "«Ivanov Ivan»"},
		},
		{
			name:       "score ровно 0.90 — high",
			subject:    beneficiary(shortSubject),
			candidates: []repository.SearchSanctionsRow{candidate("M-1", "person", score090)},
			want:       []Finding{{Rule: repository.AmlRuleSanctionsMatch, Severity: repository.AmlSeverityHigh}},
			wantDetail: []string{"score 0.90"},
		},
		{
			name:       "ниже 0.90 — medium",
			subject:    beneficiary(shortSubject),
			candidates: []repository.SearchSanctionsRow{candidate("M-1", "person", score088)},
			want:       []Finding{{Rule: repository.AmlRuleSanctionsMatch, Severity: repository.AmlSeverityMedium}},
			wantDetail: []string{"score 0.88"},
		},
		{
			name:       "score ровно 0.85 — medium",
			subject:    beneficiary(longSubject),
			candidates: []repository.SearchSanctionsRow{candidate("M-1", "person", score085)},
			want:       []Finding{{Rule: repository.AmlRuleSanctionsMatch, Severity: repository.AmlSeverityMedium}},
			wantDetail: []string{"score 0.85"},
		},
		{
			name:       "ниже 0.85 — нет находки",
			subject:    beneficiary(longSubject),
			candidates: []repository.SearchSanctionsRow{candidate("M-1", "person", score084)},
		},
		{
			name:       "совпадение компании",
			subject:    OrganizationSubject{Name: "ООО «Омега»", CountryCode: "RU"},
			candidates: []repository.SearchSanctionsRow{candidate("M-5", "organization", "OMEGA LLC")},
			want:       []Finding{{Rule: repository.AmlRuleSanctionsMatch, Severity: repository.AmlSeverityHigh}},
			wantDetail: []string{"компании «ООО «Омега»»", "M-5", "score 1.00"},
		},
		{
			name:       "компания не сравнивается с персоной",
			subject:    OrganizationSubject{Name: "Ivanov Ivan", CountryCode: "RU"},
			candidates: []repository.SearchSanctionsRow{candidate("M-2", "person", "Ivanov Ivan")},
		},
		{
			name:    "один флаг на субъект — по лучшему кандидату",
			subject: beneficiary(shortSubject),
			candidates: []repository.SearchSanctionsRow{
				candidate("M-1", "person", score088),
				candidate("M-2", "other", score090),
			},
			want:       []Finding{{Rule: repository.AmlRuleSanctionsMatch, Severity: repository.AmlSeverityHigh}},
			wantDetail: []string{"M-2", "score 0.90"},
		},
		{
			name:       "кириллица через BGN/PCGN: Юрий Петров ↔ Yuri Petrov",
			subject:    beneficiary("Юрий Петров"),
			candidates: []repository.SearchSanctionsRow{candidate("M-3", "person", "Yuri Petrov")},
			want:       []Finding{{Rule: repository.AmlRuleSanctionsMatch, Severity: repository.AmlSeverityHigh}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := &fakeStore{candidates: c.candidates}
			got, err := RuleScreener{}.ScreenOrganization(context.Background(), store, c.subject)
			if err != nil {
				t.Fatalf("ScreenOrganization: %v", err)
			}
			assertFindings(t, got, c.want, c.wantDetail)
		})
	}
}

func TestScreenOrganizationFatf(t *testing.T) {
	store := &fakeStore{fatf: map[string]repository.FatfList{"KP": repository.FatfListBlack, "KW": repository.FatfListGrey}}
	subject := OrganizationSubject{
		Name: "Zeta", CountryCode: "KP",
		Beneficiaries: []Person{
			{FullName: "Petrov Petr", CitizenshipCountryCode: "RU"},
			{FullName: "Al Sabah", CitizenshipCountryCode: "KW"},
		},
	}
	got, err := RuleScreener{}.ScreenOrganization(context.Background(), store, subject)
	if err != nil {
		t.Fatalf("ScreenOrganization: %v", err)
	}
	want := []Finding{
		{Rule: repository.AmlRuleHighRiskJurisdiction, Severity: repository.AmlSeverityHigh,
			Details: "Страна регистрации KP — чёрный список FATF (на 2026-06-19)"},
		{Rule: repository.AmlRuleHighRiskJurisdiction, Severity: repository.AmlSeverityMedium,
			Details: "Гражданство бенефициара «Al Sabah» KW — серый список FATF (на 2026-06-19)"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("findings = %+v\nwant %+v", got, want)
	}
}

func TestScreenOrganizationRegistry(t *testing.T) {
	cases := []struct {
		name     string
		registry *RegistryResult
		want     []Finding
	}{
		{name: "сверка не подключена", registry: nil},
		{name: "найдена, активна, название совпало",
			registry: &RegistryResult{Outcome: RegistryChecked, Found: true, Status: "active", Names: []string{"ООО «Альфа»", "Общество с ограниченной ответственностью «Альфа-Трейдинг»"}}},
		{name: "найдена без названий — сверять название не с чем",
			registry: &RegistryResult{Outcome: RegistryChecked, Found: true, Status: "active"}},
		{name: "статус пустой — не ошибка",
			registry: &RegistryResult{Outcome: RegistryChecked, Found: true, Names: []string{"ООО «Альфа»"}}},
		{name: "не найдена",
			registry: &RegistryResult{Outcome: RegistryChecked, Found: false},
			want: []Finding{{Rule: repository.AmlRuleRegistryMismatch, Severity: repository.AmlSeverityMedium,
				Details: "Компания «Альфа» не найдена в реестре"}}},
		{name: "ликвидирована",
			registry: &RegistryResult{Outcome: RegistryChecked, Found: true, Status: "liquidated", Names: []string{"ООО «Альфа»"}},
			want: []Finding{{Rule: repository.AmlRuleRegistryMismatch, Severity: repository.AmlSeverityHigh,
				Details: "Название по реестру «ООО «Альфа»», статус liquidated"}}},
		{name: "в процессе ликвидации",
			registry: &RegistryResult{Outcome: RegistryChecked, Found: true, Status: "liquidating", Names: []string{"ООО «Альфа»"}},
			want: []Finding{{Rule: repository.AmlRuleRegistryMismatch, Severity: repository.AmlSeverityMedium,
				Details: "Название по реестру «ООО «Альфа»», статус liquidating"}}},
		{name: "название не совпало",
			registry: &RegistryResult{Outcome: RegistryChecked, Found: true, Status: "active", Names: []string{"ООО «Омега»"}},
			want: []Finding{{Rule: repository.AmlRuleRegistryMismatch, Severity: repository.AmlSeverityMedium,
				Details: "Название «Альфа» не совпадает с реестром «ООО «Омега»»"}}},
		{name: "реестр недоступен",
			registry: &RegistryResult{Outcome: RegistryUnavailable},
			want: []Finding{{Rule: repository.AmlRuleOther, Severity: repository.AmlSeverityLow,
				Details: "Реестр не ответил — нужна ручная сверка"}}},
		{name: "нет идентификатора",
			registry: &RegistryResult{Outcome: RegistryNoIdentifier},
			want: []Finding{{Rule: repository.AmlRuleOther, Severity: repository.AmlSeverityLow,
				Details: "Нет идентификатора для сверки с реестром — нужна ручная сверка"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			subject := OrganizationSubject{Name: "Альфа", CountryCode: "RU", Registry: c.registry}
			got, err := RuleScreener{}.ScreenOrganization(context.Background(), &fakeStore{}, subject)
			if err != nil {
				t.Fatalf("ScreenOrganization: %v", err)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("findings = %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestScreenOrganizationShortNameNotSearched(t *testing.T) {
	store := &fakeStore{candidates: []repository.SearchSanctionsRow{candidate("M-9", "person", "li")}}
	got, err := RuleScreener{}.ScreenOrganization(context.Background(), store, beneficiary("Li"))
	if err != nil {
		t.Fatalf("ScreenOrganization: %v", err)
	}
	if slices.Contains(store.queries, "li") {
		t.Errorf("SearchSanctions вызван с коротким запросом: %q", store.queries)
	}
	assertFindings(t, got, nil, nil)
}

func TestScreenOrganizationStoreError(t *testing.T) {
	store := &fakeStore{err: errors.New("db down")}
	got, err := RuleScreener{}.ScreenOrganization(context.Background(), store, beneficiary("Ivanov Ivan"))
	if err == nil {
		t.Fatal("ошибка Store проглочена")
	}
	if got != nil {
		t.Errorf("при ошибке есть находки: %+v", got)
	}
}

func assertFindings(t *testing.T, got, want []Finding, detailParts []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("findings = %+v, want %d", got, len(want))
	}
	for i := range want {
		if got[i].Rule != want[i].Rule || got[i].Severity != want[i].Severity {
			t.Errorf("finding[%d] = %s/%s, want %s/%s", i, got[i].Rule, got[i].Severity, want[i].Rule, want[i].Severity)
		}
	}
	for _, part := range detailParts {
		if !strings.Contains(got[0].Details, part) {
			t.Errorf("details %q не содержит %q", got[0].Details, part)
		}
	}
}

func TestScreenApplication(t *testing.T) {
	cases := []struct {
		name    string
		subject ApplicationSubject
		want    []Finding
	}{
		{name: "страна назначения в чёрном списке",
			subject: ApplicationSubject{TargetCountryCode: "IR", Amount: decimal.NewFromInt(1000), CurrencyCode: "USD"},
			want: []Finding{{Rule: repository.AmlRuleHighRiskJurisdiction, Severity: repository.AmlSeverityHigh,
				Details: "Страна назначения IR — чёрный список FATF (на 2026-06-19)"}}},
		{name: "сумма выше порога",
			subject: ApplicationSubject{TargetCountryCode: "DE", Amount: decimal.RequireFromString("100000000.01"), CurrencyCode: "RUB"},
			want: []Finding{{Rule: repository.AmlRuleAmountThreshold, Severity: repository.AmlSeverityMedium,
				Details: "Сумма 100000000.01 RUB выше порога 100000000 RUB"}}},
		{name: "ровно порог — нет",
			subject: ApplicationSubject{TargetCountryCode: "DE", Amount: decimal.NewFromInt(1_000_000), CurrencyCode: "EUR"}},
		{name: "демо-сделка 500 000 EUR — нет",
			subject: ApplicationSubject{TargetCountryCode: "DE", Amount: decimal.NewFromInt(500_000), CurrencyCode: "EUR"}},
		{name: "валюта без порога — нет",
			subject: ApplicationSubject{TargetCountryCode: "DE", Amount: decimal.NewFromInt(1_000_000_000), CurrencyCode: "AED"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := &fakeStore{fatf: map[string]repository.FatfList{"IR": repository.FatfListBlack}}
			got, err := RuleScreener{}.ScreenApplication(context.Background(), store, c.subject)
			if err != nil {
				t.Fatalf("ScreenApplication: %v", err)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("findings = %+v\nwant %+v", got, c.want)
			}
		})
	}
}
