package registry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
)

func TestMockLookup(t *testing.T) {
	tests := []struct {
		name        string
		countryCode string
		number      string
		wantName    string
		wantRegNo   string
		wantStatus  string
	}{
		// Компании и номера — таблица демо-данных BE-16, шаг 1.
		{name: "Альфа", countryCode: "RU", number: "7700000009", wantName: "ООО «Альфа»", wantRegNo: "7700000009", wantStatus: "active"},
		{name: "Вектор", countryCode: "RU", number: "7700000016", wantName: "АО «Вектор»", wantRegNo: "7700000016", wantStatus: "active"},
		{name: "Омега ликвидирована", countryCode: "RU", number: "7700000023", wantName: "ООО «Омега»", wantRegNo: "7700000023", wantStatus: "liquidated"},
		{name: "Сигма", countryCode: "RU", number: "7700000030", wantName: "ООО «Сигма»", wantRegNo: "7700000030", wantStatus: "active"},
		{name: "Beta по LEI", countryCode: "CY", number: "DEMO00BETACORP000162", wantName: "Beta Corp Ltd", wantRegNo: "CY-123456", wantStatus: "active"},
		{name: "Gamma по LEI", countryCode: "DE", number: "DEMO00GAMMAGMBH00148", wantName: "Gamma GmbH", wantRegNo: "HRB-555555", wantStatus: "active"},
		{name: "Delta по LEI", countryCode: "KW", number: "DEMO00DELTATRADE0174", wantName: "Delta Trading", wantRegNo: "KW-777777", wantStatus: "active"},
		{name: "Epsilon по LEI", countryCode: "SG", number: "DEMO00EPSILONCAP0155", wantName: "Epsilon Capital Pte", wantRegNo: "201988888E", wantStatus: "active"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := registry.NewMock().Lookup(context.Background(), tt.countryCode, tt.number)
			if err != nil {
				t.Fatalf("Lookup() error = %v", err)
			}
			if !got.Found {
				t.Fatal("Found = false, want true")
			}
			if got.Source != registry.SourceMock {
				t.Errorf("Source = %q, want %q", got.Source, registry.SourceMock)
			}
			if got.Query != tt.number {
				t.Errorf("Query = %q, want %q", got.Query, tt.number)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.FullName == "" {
				t.Error("FullName пуст")
			}
			if got.RegistrationNumber != tt.wantRegNo {
				t.Errorf("RegistrationNumber = %q, want %q", got.RegistrationNumber, tt.wantRegNo)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if len(got.Raw) == 0 {
				t.Error("Raw пуст, want запись фикстуры")
			}
			if got.CheckedAt.IsZero() {
				t.Error("CheckedAt не заполнен")
			}
		})
	}
}

func TestMockLookupNotFound(t *testing.T) {
	got, err := registry.NewMock().Lookup(context.Background(), "RU", "7707083893")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if got.Found {
		t.Error("Found = true, want false")
	}
	if got.Source != registry.SourceMock || got.Query != "7707083893" {
		t.Errorf("Source, Query = %q, %q; want mock, 7707083893", got.Source, got.Query)
	}
}

// Номер ИНН из демо-фикстуры есть, но у другой страны — не найдено: ключ — пара.
func TestMockLookupKeyIncludesCountry(t *testing.T) {
	got, err := registry.NewMock().Lookup(context.Background(), "DE", "7700000009")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if got.Found {
		t.Error("Found = true, want false")
	}
}

func TestMockLookupUnavailable(t *testing.T) {
	_, err := registry.NewMock().Lookup(context.Background(), "RU", "0000000000")
	if !errors.Is(err, registry.ErrUnavailable) {
		t.Fatalf("Lookup() error = %v, want ErrUnavailable", err)
	}
}
