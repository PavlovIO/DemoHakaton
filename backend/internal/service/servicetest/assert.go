package servicetest

import (
	"net/netip"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// Ptr — указатель на копию v: необязательные поля входов сервисов.
func Ptr[T any](v T) *T { return &v }

// MetaFromIP — ClientMeta с заданным IP и тестовым User-Agent.
func MetaFromIP(ip string) service.ClientMeta {
	addr := netip.MustParseAddr(ip)
	return service.ClientMeta{IP: &addr, UserAgent: "ua"}
}

// WantCode — err является apperr.Error с кодом code.
func WantCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	if e, ok := apperr.As(err); !ok || e.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

// WantValidationField — 400 VALIDATION_ERROR с записью field в details.
func WantValidationField(t *testing.T, err error, field string) {
	t.Helper()
	WantCode(t, err, apperr.CodeValidation)
	e, _ := apperr.As(err)
	for _, d := range e.Details {
		if d.Field == field {
			return
		}
	}
	t.Fatalf("details = %+v, want field %q", e.Details, field)
}
