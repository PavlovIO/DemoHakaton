// Package aml — rule-based AML-скрининг (ADR-0001, п. 6): санкционные списки,
// FATF-юрисдикции, сверка с реестром, порог суммы.
//
// RuleScreener только находит ([]Finding): флаги в aml_flags пишет вызывающий сервис в
// транзакции перехода статуса (service.SaveAmlFindings), решение принимает админ.
// Санкции: кандидаты — pg_trgm (SearchSanctions), финальный score — Jaro-Winkler
// нормализованных имён (Normalize, NormalizeVariants). Правила и пороги — rules.go.
package aml
