package aml

import (
	"context"

	"github.com/shopspring/decimal"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// API пакета по шагу 1 плана BE-13; правила — RuleScreener (rules.go).

// Person — бенефициар для скрининга.
type Person struct {
	FullName               string
	CitizenshipCountryCode string
}

// RegistryOutcome — чем закончилась сверка с реестром (BE-06).
type RegistryOutcome string

// Исходы сверки с реестром.
const (
	RegistryChecked      RegistryOutcome = "checked"       // реестр ответил; Found может быть false
	RegistryUnavailable  RegistryOutcome = "unavailable"   // реестр не ответил — нужна ручная сверка
	RegistryNoIdentifier RegistryOutcome = "no_identifier" // не-RU без LEI — искать не по чему
)

// RegistryResult — сверка компании с реестром на момент submit KYC.
type RegistryResult struct {
	Outcome RegistryOutcome
	Found   bool
	Names   []string // краткое и полное название по реестру
	Status  string   // нормализованный статус (см. план BE-06)
}

// OrganizationSubject — компания на скрининге при submitKyc.
type OrganizationSubject struct {
	Name          string
	CountryCode   string
	Beneficiaries []Person
	Registry      *RegistryResult // nil — сверка не подключена, правило реестра не применяется
}

// ApplicationSubject — заявка на скрининге при submitApplication.
type ApplicationSubject struct {
	TargetCountryCode string
	Amount            decimal.Decimal
	CurrencyCode      string
}

// Finding — одно срабатывание правила; в aml_flags его пишет вызывающий сервис.
type Finding struct {
	Rule     repository.AmlRule
	Severity repository.AmlSeverity
	Details  string
	// MatchKey — ключ дедупликации: Details без того, что меняется при обновлении данных
	// (дата FATF, score, имя алиаса). Пустой — дедупликация по Details.
	MatchKey string
}

// Screener проверяет компанию и заявку по AML-правилам. Ошибка — операция откатывается
// целиком (fail-closed): без скрининга статус не меняется. store — querier транзакции
// вызывающего: скрининг идёт под блокировкой строки, и второе соединение из пула на каждый
// submit исчерпало бы пул при параллельных подачах.
type Screener interface {
	ScreenOrganization(ctx context.Context, store Store, s OrganizationSubject) ([]Finding, error)
	ScreenApplication(ctx context.Context, store Store, s ApplicationSubject) ([]Finding, error)
}

// NoopScreener — скрининг без находок: для тестов, где AML не проверяется.
type NoopScreener struct{}

var _ Screener = NoopScreener{}

// ScreenOrganization — находок нет.
func (NoopScreener) ScreenOrganization(context.Context, Store, OrganizationSubject) ([]Finding, error) {
	return nil, nil
}

// ScreenApplication — находок нет.
func (NoopScreener) ScreenApplication(context.Context, Store, ApplicationSubject) ([]Finding, error) {
	return nil, nil
}
