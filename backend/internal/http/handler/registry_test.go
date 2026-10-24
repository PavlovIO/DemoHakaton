package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeRegistry запоминает запрос и отдаёт заданный ответ.
type fakeRegistry struct {
	result      registry.Result
	err         error
	meta        service.ClientMeta
	countryCode string
	number      string
}

func (f *fakeRegistry) Lookup(_ context.Context, meta service.ClientMeta, countryCode, number string) (registry.Result, error) {
	f.meta, f.countryCode, f.number = meta, countryCode, number
	return f.result, f.err
}

var registryCheckedAt = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func TestRegistryLookupHandlerFound(t *testing.T) {
	reg := &fakeRegistry{result: registry.Result{
		Source: registry.SourceMock, Query: "7700000009", Found: true,
		Name: "ООО «Альфа»", FullName: "Общество с ограниченной ответственностью «Альфа»",
		RegistrationNumber: "7700000009", LegalAddress: "125009, г. Москва, ул. Тверская, д. 1",
		Status: "active", CheckedAt: registryCheckedAt,
	}}
	srv := newTestServer(t, handler.Deps{Dicts: fakeDicts{}, Registry: reg})

	// Без cookie: маршрут публичный — им пользуются на шаге регистрации.
	resp, body := request(t, srv, http.MethodGet, "/api/v1/registry/lookup?country_code=RU&number=7700000009", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", resp.StatusCode, body)
	}
	var got api.RegistryLookupResult
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if got.Source != api.RegistryLookupResultSourceMock || !got.Found {
		t.Errorf("source, found = %q, %v; want mock, true", got.Source, got.Found)
	}
	if deref(got.Name) != "ООО «Альфа»" || deref(got.RegistrationNumber) != "7700000009" ||
		deref(got.LegalAddress) != "125009, г. Москва, ул. Тверская, д. 1" || deref(got.RegistryStatus) != "active" {
		t.Errorf("поля компании = %s", body)
	}
	if got.CheckedAt == nil || !got.CheckedAt.Equal(registryCheckedAt) {
		t.Errorf("checked_at = %v, want %v", got.CheckedAt, registryCheckedAt)
	}
	if reg.countryCode != "RU" || reg.number != "7700000009" || reg.meta.IP == nil {
		t.Errorf("сервис вызван с %q, %q, IP %v", reg.countryCode, reg.number, reg.meta.IP)
	}
}

func TestRegistryLookupHandlerNotFound(t *testing.T) {
	reg := &fakeRegistry{result: registry.Result{Source: registry.SourceMock, Query: "7707083893", CheckedAt: registryCheckedAt}}
	srv := newTestServer(t, handler.Deps{Dicts: fakeDicts{}, Registry: reg})

	resp, body := request(t, srv, http.MethodGet, "/api/v1/registry/lookup?country_code=RU&number=7707083893", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", resp.StatusCode, body)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if raw["found"] != false || raw["checked_at"] == nil {
		t.Errorf("body = %s, want found=false и checked_at", body)
	}
	for _, field := range []string{"name", "registration_number", "legal_address", "registry_status"} {
		if v := raw[field]; v != nil {
			t.Errorf("%s = %v, want null", field, v)
		}
	}
}

func TestRegistryLookupHandlerUpstreamUnavailable(t *testing.T) {
	reg := &fakeRegistry{err: apperr.UpstreamUnavailable("Реестр временно недоступен", errors.New("timeout"))}
	srv := newTestServer(t, handler.Deps{Dicts: fakeDicts{}, Registry: reg})

	resp, body := request(t, srv, http.MethodGet, "/api/v1/registry/lookup?country_code=RU&number=0000000000", "")
	var e api.Error
	_ = json.Unmarshal(body, &e)
	if resp.StatusCode != http.StatusBadGateway || e.Error.Code != string(apperr.CodeUpstreamUnavailable) {
		t.Fatalf("status = %d, code = %s; want 502 UPSTREAM_UNAVAILABLE; body %s", resp.StatusCode, e.Error.Code, body)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
