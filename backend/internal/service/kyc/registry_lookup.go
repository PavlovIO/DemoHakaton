package kyc

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// Лимиты публичного lookup: ручка без логина, поэтому ограничиваем и один IP, и всех
// вместе — так перебор с множества адресов тоже упирается в потолок.
const (
	lookupPerIPLimit  = 10
	lookupGlobalLimit = 60
	lookupWindow      = time.Minute
)

var (
	// ruRegistryNumber — ИНН юрлица (10 цифр) или ОГРН (13 цифр).
	ruRegistryNumber = regexp.MustCompile(`^(\d{10}|\d{13})$`)
	// leiNumber — LEI (ISO 17442): 20 латинских букв и цифр.
	leiNumber = regexp.MustCompile(`^[A-Z0-9]{20}$`)
)

// Registry — публичный поиск компании в реестре для автозаполнения формы регистрации.
type Registry struct {
	verifier CompanyVerifier
	limiter  service.RateLimiter
}

// NewRegistry создаёт сервис поиска по реестру.
func NewRegistry(verifier CompanyVerifier, limiter service.RateLimiter) *Registry {
	if verifier == nil || limiter == nil {
		panic("service: nil registry verifier or rate limiter")
	}
	return &Registry{verifier: verifier, limiter: limiter}
}

// Lookup ищет компанию по ИНН/ОГРН (RU) или LEI.
//
// Сначала формат, потом лимит: битые запросы (опечатка в форме) не тратят квоту честного
// клиента; от перебора защищает глобальный лимит.
func (r *Registry) Lookup(ctx context.Context, meta service.ClientMeta, countryCode, number string) (registry.Result, error) {
	number, err := normalizeRegistryNumber(countryCode, number)
	if err != nil {
		return registry.Result{}, err
	}
	if err := r.allow(meta); err != nil {
		return registry.Result{}, err
	}
	result, err := r.verifier.Lookup(ctx, countryCode, number)
	if errors.Is(err, registry.ErrUnavailable) {
		// 502, а не 500: фронт предлагает заполнить реквизиты вручную.
		return registry.Result{}, apperr.UpstreamUnavailable("Реестр временно недоступен, заполните данные вручную", err)
	}
	if err != nil {
		return registry.Result{}, fmt.Errorf("registry lookup: %w", err)
	}
	return result, nil
}

// normalizeRegistryNumber проверяет формат номера. Контрольные суммы ИНН и LEI не
// проверяются: неверный номер реестр и так вернёт как «не найдено».
func normalizeRegistryNumber(countryCode, number string) (string, error) {
	if countryCode == countryRU {
		if !ruRegistryNumber.MatchString(number) {
			return "", numberError("Для RU укажите ИНН (10 цифр) или ОГРН (13 цифр)")
		}
		return number, nil
	}
	lei := strings.ToUpper(number)
	if !leiNumber.MatchString(lei) {
		return "", numberError("Укажите LEI: 20 латинских букв и цифр")
	}
	return lei, nil
}

func numberError(msg string) error {
	return apperr.Validation("Некорректный номер", apperr.FieldError{Field: "number", Message: msg})
}

// allow учитывает запрос в лимите IP, затем в глобальном. Если глобальный отказал, слот
// IP уже потрачен — это допустимо: запрос всё равно не прошёл.
func (r *Registry) allow(meta service.ClientMeta) error {
	if retryAfter, ok := r.limiter.Allow("registry_lookup:ip:"+meta.IPKey(), lookupPerIPLimit, lookupWindow); !ok {
		return apperr.RateLimitedAfter(retryAfter)
	}
	if retryAfter, ok := r.limiter.Allow("registry_lookup:global", lookupGlobalLimit, lookupWindow); !ok {
		return apperr.RateLimitedAfter(retryAfter)
	}
	return nil
}
