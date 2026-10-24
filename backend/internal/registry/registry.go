package registry

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Источники egrul_dadata и gleif остаются в Source, потому что повторяют enum БД; в MVP
// используется только Mock.

// Source — источник ответа; значения совпадают с enum registry_source в БД.
type Source string

// Источники реестра.
const (
	SourceEgrulDadata Source = "egrul_dadata"
	SourceGleif       Source = "gleif"
	SourceMock        Source = "mock"
)

// Result — ответ реестра о компании.
type Result struct {
	Source             Source
	Query              string // что искали: ИНН/ОГРН или LEI
	Found              bool
	Name               string // краткое: DaData name.short_with_opf, GLEIF legalName.name
	FullName           string // DaData name.full_with_opf; у GLEIF совпадает с Name
	RegistrationNumber string // DaData data.inn, GLEIF entity.registeredAs
	LegalAddress       string
	Status             string          // нормализованный: active, liquidating, liquidated, bankrupt, reorganizing, inactive
	Raw                json.RawMessage // сырой ответ (≤ 1 МБ) для registry_checks.raw_response
	CheckedAt          time.Time
}

// CompanyVerifier ищет компанию в реестре страны по рег. номеру (RU) или LEI.
type CompanyVerifier interface {
	Lookup(ctx context.Context, countryCode, number string) (Result, error)
}

// ErrUnavailable — реестр не ответил: таймаут, 5xx, 429, 401/403.
var ErrUnavailable = errors.New("registry unavailable")
