package registry

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"
)

//go:embed mockdata/companies.json
var mockCompaniesJSON []byte

// mockCompany — фикстура мок-реестра; компании — из таблицы демо-данных BE-16.
type mockCompany struct {
	CountryCode        string `json:"country_code"`
	Number             string `json:"number"` // ИНН для RU, LEI для остальных стран
	Name               string `json:"name"`
	FullName           string `json:"full_name"`
	RegistrationNumber string `json:"registration_number"`
	LegalAddress       string `json:"legal_address"`
	Status             string `json:"status"`
}

type mockKey struct{ countryCode, number string }

// mockUnavailableKey — номер, на котором мок «падает»: показать 502 у lookup и мягкую
// деградацию submitKyc без настоящего сбоя сети.
var mockUnavailableKey = mockKey{countryCode: "RU", number: "0000000000"}

// Mock — реестр на встроенных фикстурах (REGISTRY_PROVIDER=mock): демо работает без
// сети и API-ключей.
type Mock struct {
	companies map[mockKey]json.RawMessage
}

var _ CompanyVerifier = (*Mock)(nil)

// NewMock загружает фикстуры. Битый встроенный JSON — ошибка сборки, поэтому паника.
func NewMock() *Mock {
	var records []json.RawMessage
	if err := json.Unmarshal(mockCompaniesJSON, &records); err != nil {
		panic(fmt.Sprintf("registry: parse mock fixtures: %v", err))
	}
	companies := make(map[mockKey]json.RawMessage, len(records))
	for _, raw := range records {
		var c mockCompany
		if err := json.Unmarshal(raw, &c); err != nil {
			panic(fmt.Sprintf("registry: parse mock fixture: %v", err))
		}
		companies[mockKey{c.CountryCode, c.Number}] = raw
	}
	return &Mock{companies: companies}
}

// Lookup ищет фикстуру по паре (страна, номер).
func (m *Mock) Lookup(_ context.Context, countryCode, number string) (Result, error) {
	key := mockKey{countryCode, number}
	if key == mockUnavailableKey {
		return Result{}, fmt.Errorf("mock registry: %w", ErrUnavailable)
	}
	result := Result{Source: SourceMock, Query: number, CheckedAt: time.Now()}
	raw, ok := m.companies[key]
	if !ok {
		return result, nil
	}
	var c mockCompany
	if err := json.Unmarshal(raw, &c); err != nil {
		return Result{}, fmt.Errorf("registry: parse mock fixture: %w", err)
	}
	result.Found = true
	result.Name = c.Name
	result.FullName = c.FullName
	result.RegistrationNumber = c.RegistrationNumber
	result.LegalAddress = c.LegalAddress
	result.Status = c.Status
	result.Raw = raw
	return result, nil
}
