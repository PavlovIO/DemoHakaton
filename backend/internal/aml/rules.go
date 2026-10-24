package aml

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Пороги санкционного скрининга (issue #29, docs/db/schema.md): score — Jaro-Winkler
// нормализованных имён. ≥ 0.90 — почти наверняка то же лицо, флаг high; 0.85–0.90 —
// похожее имя, нужен взгляд админа (medium); ниже — шум.
const (
	sanctionsHighScore   = 0.90
	sanctionsMediumScore = 0.85
)

// Кандидаты из pg_trgm (SearchSanctions). candidateMinSimilarity равен GUC
// pg_trgm.similarity_threshold роли lp_app (DB-SEC) и не меньше его — иначе «%» отрежет
// кандидатов раньше явного порога и результат начнёт зависеть от GUC.
const (
	candidateMinSimilarity = 0.4
	maxSanctionCandidates  = 20
	// minQueryLen — у строки короче 3 символов нет триграмм, а JW по ней — шум.
	minQueryLen = 3
	// scorePrecision — score округляется до 4 знаков: без этого 0.85 из float64 выходит то
	// 0.8499999…, то 0.8500001…, и находка на границе порога зависела бы от шума.
	scorePrecision = 1e4
)

// Store — то, что скринингу нужно от БД; *repository.Queries ему удовлетворяет.
type Store interface {
	SearchSanctions(ctx context.Context, arg repository.SearchSanctionsParams) ([]repository.SearchSanctionsRow, error)
	ListCountriesFatf(ctx context.Context, codes []string) ([]repository.ListCountriesFatfRow, error)
}

// RuleScreener — скрининг по правилам BE-13: FATF, санкционные списки, сверка с реестром,
// порог суммы. Только находит: флаги пишет вызывающий в своей транзакции.
type RuleScreener struct {
	store Store
}

var _ Screener = (*RuleScreener)(nil)

// NewRuleScreener создаёт скрининг поверх store.
func NewRuleScreener(store Store) *RuleScreener {
	return &RuleScreener{store: store}
}

// ScreenOrganization проверяет компанию и каждого её бенефициара.
func (s *RuleScreener) ScreenOrganization(ctx context.Context, subj OrganizationSubject) ([]Finding, error) {
	jurisdictions := []jurisdiction{{subj.CountryCode, "Страна регистрации " + subj.CountryCode}}
	for _, p := range subj.Beneficiaries {
		jurisdictions = append(jurisdictions, jurisdiction{
			p.CitizenshipCountryCode,
			fmt.Sprintf("Гражданство бенефициара «%s» %s", p.FullName, p.CitizenshipCountryCode),
		})
	}
	findings, err := s.screenFatf(ctx, jurisdictions)
	if err != nil {
		return nil, err
	}
	company, err := s.screenSanctions(ctx, subj.Name, organizationKinds, "компании")
	if err != nil {
		return nil, err
	}
	findings = append(findings, company...)
	for _, p := range subj.Beneficiaries {
		f, err := s.screenSanctions(ctx, p.FullName, personKinds, "бенефициара")
		if err != nil {
			return nil, err
		}
		findings = append(findings, f...)
	}
	return append(findings, screenRegistry(subj.Name, subj.Registry)...), nil
}

// ScreenApplication проверяет заявку: страну назначения по FATF и сумму по порогу.
func (s *RuleScreener) ScreenApplication(ctx context.Context, subj ApplicationSubject) ([]Finding, error) {
	findings, err := s.screenFatf(ctx, []jurisdiction{{subj.TargetCountryCode, "Страна назначения " + subj.TargetCountryCode}})
	if err != nil {
		return nil, err
	}
	return append(findings, screenAmount(subj)...), nil
}

// amountThresholds — крупная сделка для ручной проверки (DECISIONS п. 20): эквивалент
// ~100 млн ₽ в каждой валюте. Env-настройки нет: на демо порог не меняют. Валюта без
// порога правилом суммы не проверяется.
var amountThresholds = map[string]decimal.Decimal{
	"RUB": decimal.NewFromInt(100_000_000),
	"USD": decimal.NewFromInt(1_000_000),
	"EUR": decimal.NewFromInt(1_000_000),
	"CNY": decimal.NewFromInt(7_000_000),
}

// screenAmount — сумма строго выше порога: ровно порог — не флаг.
func screenAmount(subj ApplicationSubject) []Finding {
	threshold, ok := amountThresholds[subj.CurrencyCode]
	if !ok || !subj.Amount.GreaterThan(threshold) {
		return nil
	}
	return []Finding{{
		Rule:     repository.AmlRuleAmountThreshold,
		Severity: repository.AmlSeverityMedium,
		Details: fmt.Sprintf("Сумма %s %s выше порога %s %s",
			subj.Amount.String(), subj.CurrencyCode, threshold.String(), subj.CurrencyCode),
	}}
}

// jurisdiction — страна, связанная с субъектом, и как её назвать в details.
type jurisdiction struct {
	code  string
	label string // «Страна регистрации IR»
}

// fatfSeverity — FATF «black» (call for action) — high: с такими юрисдикциями сделки
// запрещены или требуют контрмер; «grey» (increased monitoring) — medium: нужна
// усиленная проверка, но не запрет. countries.is_unfriendly (430-р) — справочное поле,
// в правилах не участвует (DECISIONS п. 19).
var fatfSeverity = map[repository.FatfList]repository.AmlSeverity{
	repository.FatfListBlack: repository.AmlSeverityHigh,
	repository.FatfListGrey:  repository.AmlSeverityMedium,
}

var fatfListLabel = map[repository.FatfList]string{
	repository.FatfListBlack: "чёрный",
	repository.FatfListGrey:  "серый",
}

// screenFatf проверяет страны по спискам FATF одним запросом: находка на каждую страну
// из списка, в порядке jurisdictions.
func (s *RuleScreener) screenFatf(ctx context.Context, jurisdictions []jurisdiction) ([]Finding, error) {
	var codes []string
	for _, j := range jurisdictions {
		if j.code != "" && !slices.Contains(codes, j.code) {
			codes = append(codes, j.code)
		}
	}
	if len(codes) == 0 {
		return nil, nil
	}
	rows, err := s.store.ListCountriesFatf(ctx, codes)
	if err != nil {
		return nil, fmt.Errorf("list countries fatf: %w", err)
	}
	listed := make(map[string]repository.ListCountriesFatfRow, len(rows))
	for _, r := range rows {
		listed[r.Code] = r
	}
	var findings []Finding
	for _, j := range jurisdictions {
		r, ok := listed[j.code]
		if !ok || !r.FatfList.Valid {
			continue
		}
		details := fmt.Sprintf("%s — %s список FATF", j.label, fatfListLabel[r.FatfList.FatfList])
		if r.FatfAsOf.Valid {
			details += fmt.Sprintf(" (на %s)", r.FatfAsOf.Time.Format(time.DateOnly))
		}
		findings = append(findings, Finding{
			Rule: repository.AmlRuleHighRiskJurisdiction, Severity: fatfSeverity[r.FatfList.FatfList], Details: details,
		})
	}
	return findings, nil
}

// Сверка с реестром (BE-06). Статусы — нормализованные registry.Result.Status.
const (
	registryStatusActive     = "active"
	registryStatusLiquidated = "liquidated"
	// registryNameMinScore — название по реестру и в анкете считаются одним, если JW
	// нормализованных строк не ниже порога high санкций: «ООО «Альфа»» и «Альфа» совпадают
	// после удаления орг. формы, а опечатку в анкете стоит показать админу.
	registryNameMinScore = 0.90
)

// screenRegistry сверяет анкету с ответом реестра. Ликвидированная компания — high: с ней
// нельзя заключить договор. Не найдена, иной не-активный статус или чужое название —
// medium: возможна ошибка в анкете, решает админ. Реестр не ответил или искать не по чему —
// low (rule other): сверку админ делает руками. nil — сверка не подключена.
func screenRegistry(name string, r *RegistryResult) []Finding {
	if r == nil {
		return nil
	}
	mismatch := func(severity repository.AmlSeverity, details string) Finding {
		return Finding{Rule: repository.AmlRuleRegistryMismatch, Severity: severity, Details: details}
	}
	switch r.Outcome {
	case RegistryUnavailable:
		return []Finding{{Rule: repository.AmlRuleOther, Severity: repository.AmlSeverityLow,
			Details: "Реестр не ответил — нужна ручная сверка"}}
	case RegistryNoIdentifier:
		return []Finding{{Rule: repository.AmlRuleOther, Severity: repository.AmlSeverityLow,
			Details: "Нет идентификатора для сверки с реестром — нужна ручная сверка"}}
	case RegistryChecked:
	default:
		return nil
	}
	if !r.Found {
		return []Finding{mismatch(repository.AmlSeverityMedium, fmt.Sprintf("Компания «%s» не найдена в реестре", name))}
	}
	registryName := strings.Join(r.Names, " / ")
	var findings []Finding
	switch r.Status {
	case registryStatusActive, "":
	case registryStatusLiquidated:
		findings = append(findings, mismatch(repository.AmlSeverityHigh,
			fmt.Sprintf("Название по реестру «%s», статус %s", registryName, r.Status)))
	default:
		findings = append(findings, mismatch(repository.AmlSeverityMedium,
			fmt.Sprintf("Название по реестру «%s», статус %s", registryName, r.Status)))
	}
	if len(r.Names) > 0 && !registryNameMatches(name, r.Names) {
		findings = append(findings, mismatch(repository.AmlSeverityMedium,
			fmt.Sprintf("Название «%s» не совпадает с реестром «%s»", name, registryName)))
	}
	return findings
}

func registryNameMatches(name string, registryNames []string) bool {
	normalized := Normalize(name)
	for _, rn := range registryNames {
		if roundScore(nameScore(normalized, Normalize(rn))) >= registryNameMinScore {
			return true
		}
	}
	return false
}

// Какие записи списка сравниваются с субъектом: название компании с персоной не путаем.
var (
	organizationKinds = map[string]bool{"organization": true, "other": true}
	personKinds       = map[string]bool{"person": true, "other": true}
)

// screenSanctions ищет субъекта в санкционных списках. Флаг — один на субъект, по лучшему
// кандидату: на распространённом имени иначе была бы лавина флагов.
func (s *RuleScreener) screenSanctions(ctx context.Context, name string, kinds map[string]bool, subjectLabel string) ([]Finding, error) {
	var (
		best      repository.SearchSanctionsRow
		bestScore float64
	)
	for _, query := range NormalizeVariants(name) {
		if len([]rune(query)) < minQueryLen {
			continue
		}
		candidates, err := s.store.SearchSanctions(ctx, repository.SearchSanctionsParams{
			Query: query, MinSimilarity: candidateMinSimilarity, MaxCandidates: maxSanctionCandidates,
		})
		if err != nil {
			return nil, fmt.Errorf("search sanctions: %w", err)
		}
		for _, c := range candidates {
			if !kinds[c.EntityKind] {
				continue
			}
			if score := roundScore(nameScore(query, c.NameNormalized)); score > bestScore {
				best, bestScore = c, score
			}
		}
	}
	severity, ok := sanctionsSeverity(bestScore)
	if !ok {
		return nil, nil
	}
	return []Finding{{
		Rule:     repository.AmlRuleSanctionsMatch,
		Severity: severity,
		Details: fmt.Sprintf("Совпадение %s «%s» со списком %s (запись %s «%s»), score %.2f",
			subjectLabel, name, best.Source, best.ExternalID, best.Name, bestScore),
	}}, nil
}

func sanctionsSeverity(score float64) (repository.AmlSeverity, bool) {
	switch {
	case score >= sanctionsHighScore:
		return repository.AmlSeverityHigh, true
	case score >= sanctionsMediumScore:
		return repository.AmlSeverityMedium, true
	default:
		return "", false
	}
}

func roundScore(score float64) float64 {
	return math.Round(score*scorePrecision) / scorePrecision
}
