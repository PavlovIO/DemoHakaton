package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeKyc запоминает, с чем его вызвали, и отдаёт заданный профиль или ошибку.
type fakeKyc struct {
	profile   service.OrganizationProfile
	err       error
	gotUpdate *service.OrganizationUpdateInput
}

func (f *fakeKyc) MyOrganization(context.Context, service.Principal) (service.OrganizationProfile, error) {
	return f.profile, f.err
}

func (f *fakeKyc) UpdateMyOrganization(_ context.Context, _ service.Principal, in service.OrganizationUpdateInput) (service.OrganizationProfile, error) {
	f.gotUpdate = &in
	return f.profile, f.err
}

func testProfile() service.OrganizationProfile {
	return service.OrganizationProfile{
		Org: &service.Org{
			ID: uuid.New(), Type: "borrower", Name: "ООО Альфа", CountryCode: "RU",
			RegistrationNumber: "7707083893", LegalAddress: "Москва", KycStatus: "draft",
			CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		},
		Beneficiaries: []service.Beneficiary{},
	}
}

func newKycServer(t *testing.T, kyc *fakeKyc) *httptest.Server {
	t.Helper()
	fixture, deps := authFixture()
	deps.Auth = &cookieAuth{fakeAuth: *fixture}
	deps.Dicts, deps.DB, deps.Kyc = fakeDicts{}, okDB{}, kyc
	return newTestServer(t, deps)
}

// borrowerRequest — запрос заёмщика в draft с сессией и CSRF-токеном.
func borrowerRequest(t *testing.T, srv *httptest.Server, method, path, body string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: "borrower-draft"})
	req.AddCookie(&http.Cookie{Name: middleware.CSRFCookieName, Value: "csrf"})
	req.Header.Set(middleware.CSRFHeaderName, "csrf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

func TestGetMyOrganizationHandler(t *testing.T) {
	srv := newKycServer(t, &fakeKyc{profile: testProfile()})
	resp, body := borrowerRequest(t, srv, http.MethodGet, "/api/v1/organizations/me", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["beneficiaries"]) != "[]" {
		t.Fatalf("beneficiaries = %s, want []", got["beneficiaries"])
	}
	if string(got["kyc_status"]) != `"draft"` {
		t.Fatalf("kyc_status = %s", got["kyc_status"])
	}
}

func TestUpdateMyOrganizationHandler(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		wantBeneficiaries *int // nil — список не передан в сервис
		wantLEI           *string
	}{
		{name: "beneficiaries не передан", body: `{"name":"ООО Альфа Плюс"}`},
		{name: "beneficiaries: []", body: `{"beneficiaries":[]}`, wantBeneficiaries: ptr(0)},
		{name: "два бенефициара", body: `{"beneficiaries":[{"full_name":"Иванов И.И.","citizenship_country_code":"RU","ownership_percent":"60.00"},{"full_name":"Петров П.П.","citizenship_country_code":"RU","ownership_percent":"40"}]}`, wantBeneficiaries: ptr(2)},
		{name: `lei: "" — очистить`, body: `{"lei":""}`, wantLEI: ptr("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kyc := &fakeKyc{profile: testProfile()}
			srv := newKycServer(t, kyc)
			resp, body := borrowerRequest(t, srv, http.MethodPatch, "/api/v1/organizations/me", tt.body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d; body %s", resp.StatusCode, body)
			}
			in := kyc.gotUpdate
			if in == nil {
				t.Fatal("сервис не вызван")
			}
			switch {
			case tt.wantBeneficiaries == nil && in.Beneficiaries != nil:
				t.Fatalf("Beneficiaries = %v, want nil", *in.Beneficiaries)
			case tt.wantBeneficiaries != nil && (in.Beneficiaries == nil || len(*in.Beneficiaries) != *tt.wantBeneficiaries):
				t.Fatalf("Beneficiaries = %v, want %d", in.Beneficiaries, *tt.wantBeneficiaries)
			}
			if tt.wantLEI != nil && (in.LEI == nil || *in.LEI != *tt.wantLEI) {
				t.Fatalf("LEI = %v, want %q", in.LEI, *tt.wantLEI)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
