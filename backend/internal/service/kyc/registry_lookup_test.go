package kyc_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	service "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/kyc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// recordingVerifier запоминает, с чем его вызвали, и отдаёт заданный ответ.
type recordingVerifier struct {
	result      registry.Result
	err         error
	calls       int
	countryCode string
	number      string
}

func (f *recordingVerifier) Lookup(_ context.Context, countryCode, number string) (registry.Result, error) {
	f.calls++
	f.countryCode, f.number = countryCode, number
	return f.result, f.err
}

func TestRegistryLookupReturnsVerifierResult(t *testing.T) {
	verifier := &recordingVerifier{result: registry.Result{Source: registry.SourceMock, Query: "7700000009", Found: true, Name: "ООО «Альфа»"}}
	svc := kyc.NewRegistry(verifier, service.NewMemoryRateLimiter())

	got, err := svc.Lookup(context.Background(), servicetest.MetaFromIP("203.0.113.1"), "RU", "7700000009")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if !got.Found || got.Name != "ООО «Альфа»" {
		t.Errorf("Lookup() = %+v, want найденную «Альфу»", got)
	}
	if verifier.countryCode != "RU" || verifier.number != "7700000009" {
		t.Errorf("verifier вызван с %q, %q; want RU, 7700000009", verifier.countryCode, verifier.number)
	}
}

func TestRegistryLookupValidatesNumber(t *testing.T) {
	tests := []struct {
		name        string
		countryCode string
		number      string
		wantNumber  string // что получает verifier; "" — запрос отклонён до реестра
	}{
		{name: "ИНН юрлица", countryCode: "RU", number: "7700000009", wantNumber: "7700000009"},
		{name: "ОГРН", countryCode: "RU", number: "1027700132195", wantNumber: "1027700132195"},
		{name: "8 цифр", countryCode: "RU", number: "77000000"},
		{name: "буквы в ИНН", countryCode: "RU", number: "77000000AB"},
		{name: "LEI в нижнем регистре", countryCode: "DE", number: "demo00gammagmbh00148", wantNumber: "DEMO00GAMMAGMBH00148"},
		{name: "не LEI", countryCode: "DE", number: "DE123456789"},
		{name: "LEI для RU", countryCode: "RU", number: "DEMO00GAMMAGMBH00148"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := &recordingVerifier{}
			svc := kyc.NewRegistry(verifier, service.NewMemoryRateLimiter())

			_, err := svc.Lookup(context.Background(), servicetest.MetaFromIP("203.0.113.1"), tt.countryCode, tt.number)
			if tt.wantNumber != "" {
				if err != nil {
					t.Fatalf("Lookup() error = %v", err)
				}
				if verifier.number != tt.wantNumber {
					t.Errorf("verifier получил %q, want %q", verifier.number, tt.wantNumber)
				}
				return
			}
			assertFieldError(t, err, "number")
			if verifier.calls != 0 {
				t.Errorf("verifier вызван %d раз, want 0", verifier.calls)
			}
		})
	}
}

func assertFieldError(t *testing.T, err error, field string) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("err = %v, want %s", err, apperr.CodeValidation)
	}
	if len(e.Details) != 1 || e.Details[0].Field != field {
		t.Errorf("Details = %+v, want одно поле %q", e.Details, field)
	}
}

// fakeClock — часы, которые тест двигает вручную.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func lookupAlpha(svc *kyc.Registry, ip string) error {
	_, err := svc.Lookup(context.Background(), servicetest.MetaFromIP(ip), "RU", "7700000009")
	return err
}

func TestRegistryLookupLimitsPerIP(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	svc := kyc.NewRegistry(&recordingVerifier{}, service.NewMemoryRateLimiterWithClock(clock.Now))

	for i := range 10 {
		if err := lookupAlpha(svc, "203.0.113.1"); err != nil {
			t.Fatalf("запрос %d: error = %v", i+1, err)
		}
	}
	err := lookupAlpha(svc, "203.0.113.1")
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeRateLimited || e.RetryAfter <= 0 {
		t.Fatalf("11-й запрос: err = %v, want %s с RetryAfter > 0", err, apperr.CodeRateLimited)
	}
	if err := lookupAlpha(svc, "203.0.113.2"); err != nil {
		t.Errorf("другой IP: error = %v, want ok", err)
	}

	clock.Advance(61 * time.Second)
	if err := lookupAlpha(svc, "203.0.113.1"); err != nil {
		t.Errorf("через 61 с: error = %v, want ok", err)
	}
}

func TestRegistryLookupLimitsGlobally(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	svc := kyc.NewRegistry(&recordingVerifier{}, service.NewMemoryRateLimiterWithClock(clock.Now))

	for i := range 60 {
		if err := lookupAlpha(svc, fmt.Sprintf("203.0.113.%d", i+1)); err != nil {
			t.Fatalf("запрос %d: error = %v", i+1, err)
		}
	}
	err := lookupAlpha(svc, "198.51.100.1")
	if e, ok := apperr.As(err); !ok || e.Code != apperr.CodeRateLimited || e.RetryAfter <= 0 {
		t.Fatalf("61-й запрос с нового IP: err = %v, want %s с RetryAfter > 0", err, apperr.CodeRateLimited)
	}
}

func TestRegistryLookupUpstreamUnavailable(t *testing.T) {
	verifier := &recordingVerifier{err: fmt.Errorf("mock registry: %w", registry.ErrUnavailable)}
	svc := kyc.NewRegistry(verifier, service.NewMemoryRateLimiter())

	err := lookupAlpha(svc, "203.0.113.1")
	if code := apperr.CodeOf(err); code != apperr.CodeUpstreamUnavailable {
		t.Fatalf("code = %s, want %s (err = %v)", code, apperr.CodeUpstreamUnavailable, err)
	}
}

func TestRegistryLookupNotFound(t *testing.T) {
	verifier := &recordingVerifier{result: registry.Result{Source: registry.SourceMock, Query: "7707083893"}}
	svc := kyc.NewRegistry(verifier, service.NewMemoryRateLimiter())

	got, err := svc.Lookup(context.Background(), servicetest.MetaFromIP("203.0.113.1"), "RU", "7707083893")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if got.Found {
		t.Error("Found = true, want false")
	}
}
