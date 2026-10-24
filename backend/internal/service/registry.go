package service

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Маппинги ответа реестра для submitKyc (план BE-06, шаг 6): пакет registry не знает
// про БД и AML, переводит сервис.

// registryCheckParams — снапшот ответа реестра для registry_checks.
func registryCheckParams(orgID uuid.UUID, r registry.Result) repository.CreateRegistryCheckParams {
	raw := []byte(r.Raw)
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	return repository.CreateRegistryCheckParams{
		OrganizationID:     pgtype.UUID{Bytes: orgID, Valid: true},
		Source:             repository.RegistrySource(r.Source),
		Query:              r.Query,
		Found:              r.Found,
		Name:               optionalText(nonEmpty(r.Name)),
		RegistrationNumber: optionalText(nonEmpty(r.RegistrationNumber)),
		LegalAddress:       optionalText(nonEmpty(r.LegalAddress)),
		RegistryStatus:     optionalText(nonEmpty(r.Status)),
		RawResponse:        raw,
	}
}

// amlRegistryResult — сверка для AML-правила registry_mismatch: ответ реестра или
// «недоступен», если сверка не удалась.
func amlRegistryResult(r registry.Result, err error) *aml.RegistryResult {
	if err != nil {
		return &aml.RegistryResult{Outcome: aml.RegistryUnavailable}
	}
	return &aml.RegistryResult{
		Outcome: aml.RegistryChecked,
		Found:   r.Found,
		Names:   []string{r.Name, r.FullName},
		Status:  r.Status,
	}
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
