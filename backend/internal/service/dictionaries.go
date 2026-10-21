// Package service — бизнес-логика: статусные машины, правила доступа, транзакции.
// Здесь живут модульные тесты бизнес-логики (ADR-0001).
//
// Соглашения:
//   - сервис зависит от узкого интерфейса над sqlc (только нужные методы), а не от *repository.Queries —
//     так в тестах хватает маленького фейка;
//   - ошибки — apperr; ошибки БД переводятся через pgerr.Map.
package service

import (
	"context"
	"fmt"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// DictionaryRepo — подмножество repository.Querier, нужное справочникам.
type DictionaryRepo interface {
	ListCountries(ctx context.Context) ([]repository.ListCountriesRow, error)
	ListCurrencies(ctx context.Context) ([]repository.ListCurrenciesRow, error)
	ListTransferMethods(ctx context.Context) ([]repository.TransferMethod, error)
}

// Dictionaries отдаёт справочники стран, валют и способов перевода.
// Логики пока нет: признак fatf_high_risk считается в SQL (fatf_list IS NOT NULL).
// Если справочники станут горячими — сюда добавляется кэш в памяти.
type Dictionaries struct {
	repo DictionaryRepo
}

func NewDictionaries(repo DictionaryRepo) *Dictionaries {
	return &Dictionaries{repo: repo}
}

func (s *Dictionaries) Countries(ctx context.Context) ([]repository.ListCountriesRow, error) {
	rows, err := s.repo.ListCountries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list countries: %w", err)
	}
	return rows, nil
}

func (s *Dictionaries) Currencies(ctx context.Context) ([]repository.ListCurrenciesRow, error) {
	rows, err := s.repo.ListCurrencies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list currencies: %w", err)
	}
	return rows, nil
}

func (s *Dictionaries) TransferMethods(ctx context.Context) ([]repository.TransferMethod, error) {
	rows, err := s.repo.ListTransferMethods(ctx)
	if err != nil {
		return nil, fmt.Errorf("list transfer methods: %w", err)
	}
	return rows, nil
}
