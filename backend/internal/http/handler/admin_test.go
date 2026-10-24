package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/kyc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/lending"
)

// fakeAdmin запоминает аргументы и отдаёт заданную ошибку.
type fakeAdmin struct {
	err         error
	details     admin.OrganizationDetails
	gotOrgs     *admin.OrganizationFilter
	gotUsers    *admin.UserFilter
	gotComment  *string
	gotReason   string
	gotID       uuid.UUID
	gotMetaUA   string
	unblockedID uuid.UUID
}

var registryResultMock = registry.Result{
	Source: registry.SourceMock, Found: true, Name: "ООО Альфа", CheckedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
}

var adminTestOrg = &service.Org{
	ID: uuid.New(), Type: "borrower", Name: "ООО Альфа", CountryCode: "RU", RegistrationNumber: "7707083893",
	LegalAddress: "Москва", KycStatus: "under_review", CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
}

func adminTestProfile() kyc.OrganizationProfile {
	return kyc.OrganizationProfile{Org: adminTestOrg, Beneficiaries: []kyc.Beneficiary{}}
}

func (f *fakeAdmin) ListOrganizations(_ context.Context, _ service.Principal, filter admin.OrganizationFilter) (admin.OrganizationPage, error) {
	f.gotOrgs = &filter
	item := adminTestProfile()
	if f.details.Profile.Org != nil { // тот же профиль, что в карточке: сверка маппингов
		item = f.details.Profile
	}
	return admin.OrganizationPage{Items: []kyc.OrganizationProfile{item}, Total: 1, Page: 1, PageSize: 20}, f.err
}

func (f *fakeAdmin) GetOrganization(_ context.Context, _ service.Principal, id uuid.UUID) (admin.OrganizationDetails, error) {
	f.gotID = id
	return f.details, f.err
}

func (f *fakeAdmin) ApproveKyc(_ context.Context, _ service.Principal, id uuid.UUID, comment *string, meta service.ClientMeta) (kyc.OrganizationProfile, error) {
	f.gotID, f.gotComment, f.gotMetaUA = id, comment, meta.UserAgent
	return adminTestProfile(), f.err
}

func (f *fakeAdmin) RejectKyc(_ context.Context, _ service.Principal, id uuid.UUID, reason string, _ service.ClientMeta) (kyc.OrganizationProfile, error) {
	f.gotID, f.gotReason = id, reason
	p := adminTestProfile()
	org := *p.Org
	org.KycStatus, org.KycRejectReason = "rejected", &reason
	p.Org = &org
	return p, f.err
}

func (f *fakeAdmin) ApproveApplication(_ context.Context, _ service.Principal, id uuid.UUID, comment *string, _ service.ClientMeta) (lending.Application, error) {
	f.gotID, f.gotComment = id, comment
	return lending.Application{ID: id, Status: lending.AppStatusPublished, Currency: "RUB"}, f.err
}

func (f *fakeAdmin) RejectApplication(_ context.Context, _ service.Principal, id uuid.UUID, reason string, _ service.ClientMeta) (lending.Application, error) {
	f.gotID, f.gotReason = id, reason
	return lending.Application{ID: id, Status: lending.AppStatusRejected, Currency: "RUB", ModerationComment: &reason}, f.err
}

func (f *fakeAdmin) ListUsers(_ context.Context, _ service.Principal, filter admin.UserFilter) (admin.UserPage, error) {
	f.gotUsers = &filter
	return admin.UserPage{Items: []admin.User{{ID: uuid.New(), Email: "admin@example.com", Role: "admin", Status: "active"}}, Total: 1, Page: 1, PageSize: 20}, f.err
}

func (f *fakeAdmin) BlockUser(_ context.Context, _ service.Principal, id uuid.UUID, reason string, _ service.ClientMeta) (admin.User, error) {
	f.gotID, f.gotReason = id, reason
	orgID := uuid.New()
	return admin.User{ID: id, Email: "b@example.com", Role: "borrower", Status: "blocked", OrganizationID: &orgID}, f.err
}

func (f *fakeAdmin) UnblockUser(_ context.Context, _ service.Principal, id uuid.UUID, _ service.ClientMeta) (admin.User, error) {
	f.unblockedID = id
	return admin.User{ID: id, Email: "b@example.com", Role: "borrower", Status: "active"}, f.err
}

func newAdminServer(t *testing.T, fake *fakeAdmin) *httptest.Server {
	t.Helper()
	fixture, deps := authFixture()
	deps.Auth = &cookieAuth{fakeAuth: *fixture}
	deps.Dicts, deps.DB, deps.Admin = fakeDicts{}, okDB{}, fake
	return newTestServer(t, deps)
}

func decodeObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return raw
}

func TestAdminListOrganizations(t *testing.T) {
	fake := &fakeAdmin{}
	srv := newAdminServer(t, fake)

	resp, body := sessionRequest(t, srv, "admin", "GET",
		"/api/v1/admin/organizations?kyc_status=under_review&kyc_status=rejected&q=a_b&page=2&page_size=5", "", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
	f := fake.gotOrgs
	if f == nil || strings.Join(f.KycStatuses, ",") != "under_review,rejected" || *f.Q != "a_b" || f.Page != 2 || f.PageSize != 5 {
		t.Fatalf("filter = %+v", f)
	}
	raw := decodeObject(t, body)
	items, _ := raw["items"].([]any)
	if raw["total"] != float64(1) || len(items) != 1 {
		t.Fatalf("body = %s", body)
	}
	if b, ok := items[0].(map[string]any)["beneficiaries"].([]any); !ok || len(b) != 0 {
		t.Fatalf("beneficiaries = %v, want []", items[0])
	}
}

func TestAdminListOrganizationsBogusStatus(t *testing.T) {
	srv := newAdminServer(t, &fakeAdmin{})
	resp, body := sessionRequest(t, srv, "admin", "GET", "/api/v1/admin/organizations?kyc_status=bogus", "", nil)
	wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "kyc_status")
}

func TestAdminOperationsForbiddenForCompanies(t *testing.T) {
	srv := newAdminServer(t, &fakeAdmin{})
	for _, session := range []string{"lender-verified", "borrower-verified"} {
		resp, body := sessionRequest(t, srv, session, "GET", "/api/v1/admin/organizations", "", nil)
		wantError(t, resp, body, http.StatusForbidden, "FORBIDDEN", "")
		resp, body = sessionRequest(t, srv, session, "POST", "/api/v1/admin/organizations/"+uuid.NewString()+"/kyc/approve", "", nil)
		wantError(t, resp, body, http.StatusForbidden, "FORBIDDEN", "")
	}
}

func TestAdminGetOrganization(t *testing.T) {
	t.Run("пустая карточка — массивы [], registry_check null", func(t *testing.T) {
		fake := &fakeAdmin{details: admin.OrganizationDetails{
			Profile: adminTestProfile(), Documents: []repository.KycDocument{}, AmlFlags: []admin.AmlFlag{}, Users: []admin.User{},
		}}
		srv := newAdminServer(t, fake)

		resp, body := sessionRequest(t, srv, "admin", "GET", "/api/v1/admin/organizations/"+adminTestOrg.ID.String(), "", nil)

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d; body %s", resp.StatusCode, body)
		}
		raw := decodeObject(t, body)
		for _, key := range []string{"documents", "aml_flags", "users", "beneficiaries"} {
			if list, ok := raw[key].([]any); !ok || len(list) != 0 {
				t.Fatalf("%s = %v, want []; body %s", key, raw[key], body)
			}
		}
		if v, ok := raw["registry_check"]; !ok || v != nil {
			t.Fatalf("registry_check = %v (есть: %v), want null", v, ok)
		}
		if raw["id"] != adminTestOrg.ID.String() || raw["kyc_status"] != "under_review" {
			t.Fatalf("body = %s", body)
		}
	})

	t.Run("флаг с компанией, пользователь, сверка", func(t *testing.T) {
		orgID := adminTestOrg.ID
		fake := &fakeAdmin{details: admin.OrganizationDetails{
			Profile:   adminTestProfile(),
			Documents: []repository.KycDocument{},
			AmlFlags: []admin.AmlFlag{{
				ID: 7, Organization: admin.OrgShort{ID: orgID, Name: "ООО Альфа", CountryCode: "RU"},
				Rule: "sanctions_match", Severity: "high", Details: "совпадение", Status: "open",
			}},
			Users:         []admin.User{{ID: uuid.New(), Email: "a@example.com", Role: "borrower", Status: "active", OrganizationID: &orgID}},
			RegistryCheck: &registryResultMock,
		}}
		srv := newAdminServer(t, fake)

		resp, body := sessionRequest(t, srv, "admin", "GET", "/api/v1/admin/organizations/"+orgID.String(), "", nil)

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d; body %s", resp.StatusCode, body)
		}
		raw := decodeObject(t, body)
		flag := raw["aml_flags"].([]any)[0].(map[string]any)
		org, _ := flag["organization"].(map[string]any)
		if flag["id"] != float64(7) || org["name"] != "ООО Альфа" || flag["severity"] != "high" {
			t.Fatalf("aml_flag = %v", flag)
		}
		if user := raw["users"].([]any)[0].(map[string]any); user["organization_id"] != orgID.String() {
			t.Fatalf("user = %v", user)
		}
		if rc, _ := raw["registry_check"].(map[string]any); rc["source"] != "mock" || rc["found"] != true {
			t.Fatalf("registry_check = %v", raw["registry_check"])
		}
	})

	// organizationDetailsToAPI копирует поля Organization вручную: кодоген разворачивает allOf
	// в плоскую структуру. Новое поле Organization, забытое в копии, компилятор не поймает —
	// ловит этот тест: карточка отдаёт каждое поле Organization так же, как список.
	t.Run("поля Organization — как в списке", func(t *testing.T) {
		lei, reason := "253400JT3MQWNDKMJE44", "нет устава"
		verifiedAt := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
		org := *adminTestOrg
		org.LEI, org.KycRejectReason, org.VerifiedAt = &lei, &reason, &verifiedAt
		fake := &fakeAdmin{details: admin.OrganizationDetails{
			Profile: kyc.OrganizationProfile{Org: &org, Beneficiaries: []kyc.Beneficiary{{
				ID: uuid.New(), FullName: "Иванов И.И.", CitizenshipCountryCode: "RU", OwnershipPercent: decimal.NewFromInt(60),
			}}},
			Documents: []repository.KycDocument{}, AmlFlags: []admin.AmlFlag{}, Users: []admin.User{},
		}}
		srv := newAdminServer(t, fake)

		_, listBody := sessionRequest(t, srv, "admin", "GET", "/api/v1/admin/organizations", "", nil)
		_, cardBody := sessionRequest(t, srv, "admin", "GET", "/api/v1/admin/organizations/"+org.ID.String(), "", nil)

		item := decodeObject(t, listBody)["items"].([]any)[0].(map[string]any)
		card := decodeObject(t, cardBody)
		fields := reflect.TypeOf(api.Organization{})
		for i := range fields.NumField() {
			key, _, _ := strings.Cut(fields.Field(i).Tag.Get("json"), ",")
			if _, ok := item[key]; !ok {
				t.Fatalf("в фикстуре не заполнено поле Organization %q: дополни профиль теста", key)
			}
			if !reflect.DeepEqual(card[key], item[key]) {
				t.Errorf("%s: карточка %v, список %v — organizationDetailsToAPI не копирует поле", key, card[key], item[key])
			}
		}
	})

	t.Run("NotFound от сервиса — 404", func(t *testing.T) {
		srv := newAdminServer(t, &fakeAdmin{err: apperr.NotFound("Организация не найдена")})
		resp, body := sessionRequest(t, srv, "admin", "GET", "/api/v1/admin/organizations/"+uuid.NewString(), "", nil)
		wantError(t, resp, body, http.StatusNotFound, "NOT_FOUND", "")
	})
}

func TestAdminApproveKyc(t *testing.T) {
	t.Run("без тела — 200, comment nil, User-Agent в meta", func(t *testing.T) {
		fake := &fakeAdmin{}
		srv := newAdminServer(t, fake)
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/organizations/"+adminTestOrg.ID.String()+"/kyc/approve", "", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d; body %s", resp.StatusCode, body)
		}
		if fake.gotID != adminTestOrg.ID || fake.gotComment != nil || fake.gotMetaUA == "" {
			t.Fatalf("id = %s, comment = %v, ua = %q", fake.gotID, fake.gotComment, fake.gotMetaUA)
		}
	})

	t.Run("с comment — передан в сервис", func(t *testing.T) {
		fake := &fakeAdmin{}
		srv := newAdminServer(t, fake)
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/organizations/"+adminTestOrg.ID.String()+"/kyc/approve",
			"application/json", strings.NewReader(`{"comment":"проверено"}`))
		if resp.StatusCode != http.StatusOK || fake.gotComment == nil || *fake.gotComment != "проверено" {
			t.Fatalf("status = %d, comment = %v; body %s", resp.StatusCode, fake.gotComment, body)
		}
	})

	t.Run("CONFLICT от сервиса — 409 CONFLICT", func(t *testing.T) {
		srv := newAdminServer(t, &fakeAdmin{err: apperr.Conflict("У компании есть открытые AML-флаги высокой серьёзности")})
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/organizations/"+adminTestOrg.ID.String()+"/kyc/approve", "", nil)
		wantError(t, resp, body, http.StatusConflict, "CONFLICT", "")
	})
}

func TestAdminRejectKyc(t *testing.T) {
	t.Run("короткая причина — 400 reason", func(t *testing.T) {
		srv := newAdminServer(t, &fakeAdmin{})
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/organizations/"+adminTestOrg.ID.String()+"/kyc/reject",
			"application/json", strings.NewReader(`{"reason":"no"}`))
		wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "reason")
	})

	t.Run("без тела — 400", func(t *testing.T) {
		srv := newAdminServer(t, &fakeAdmin{})
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/organizations/"+adminTestOrg.ID.String()+"/kyc/reject", "", nil)
		wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "")
	})

	t.Run("с причиной — 200, kyc_reject_reason в ответе", func(t *testing.T) {
		fake := &fakeAdmin{}
		srv := newAdminServer(t, fake)
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/organizations/"+adminTestOrg.ID.String()+"/kyc/reject",
			"application/json", strings.NewReader(`{"reason":"нет устава"}`))
		if resp.StatusCode != http.StatusOK || fake.gotReason != "нет устава" {
			t.Fatalf("status = %d, reason = %q; body %s", resp.StatusCode, fake.gotReason, body)
		}
		if raw := decodeObject(t, body); raw["kyc_status"] != "rejected" || raw["kyc_reject_reason"] != "нет устава" {
			t.Fatalf("body = %s", body)
		}
	})

	t.Run("INVALID_STATUS_TRANSITION от сервиса — 409", func(t *testing.T) {
		srv := newAdminServer(t, &fakeAdmin{err: apperr.InvalidTransition("verified", "rejected")})
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/organizations/"+adminTestOrg.ID.String()+"/kyc/reject",
			"application/json", strings.NewReader(`{"reason":"нет устава"}`))
		wantError(t, resp, body, http.StatusConflict, "INVALID_STATUS_TRANSITION", "")
	})
}

func TestAdminModerateApplication(t *testing.T) {
	appID := uuid.New()

	t.Run("approve — 200 published", func(t *testing.T) {
		fake := &fakeAdmin{}
		srv := newAdminServer(t, fake)
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/applications/"+appID.String()+"/approve", "", nil)
		if resp.StatusCode != http.StatusOK || fake.gotID != appID {
			t.Fatalf("status = %d, id = %s; body %s", resp.StatusCode, fake.gotID, body)
		}
		if raw := decodeObject(t, body); raw["status"] != "published" || raw["id"] != appID.String() {
			t.Fatalf("body = %s", body)
		}
	})

	t.Run("reject — 200, moderation_comment", func(t *testing.T) {
		fake := &fakeAdmin{}
		srv := newAdminServer(t, fake)
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/applications/"+appID.String()+"/reject",
			"application/json", strings.NewReader(`{"reason":"уточните цель"}`))
		if resp.StatusCode != http.StatusOK || fake.gotReason != "уточните цель" {
			t.Fatalf("status = %d, reason = %q; body %s", resp.StatusCode, fake.gotReason, body)
		}
		if raw := decodeObject(t, body); raw["status"] != "rejected" || raw["moderation_comment"] != "уточните цель" {
			t.Fatalf("body = %s", body)
		}
	})

	t.Run("reject без причины — 400", func(t *testing.T) {
		srv := newAdminServer(t, &fakeAdmin{})
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/applications/"+appID.String()+"/reject",
			"application/json", strings.NewReader(`{}`))
		wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "reason")
	})
}

func TestAdminUsers(t *testing.T) {
	userID := uuid.New()

	t.Run("список — фильтры переданы", func(t *testing.T) {
		fake := &fakeAdmin{}
		srv := newAdminServer(t, fake)
		resp, body := sessionRequest(t, srv, "admin", "GET", "/api/v1/admin/users?role=borrower&status=blocked&q=a_b", "", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d; body %s", resp.StatusCode, body)
		}
		if f := fake.gotUsers; f == nil || *f.Role != "borrower" || *f.Status != "blocked" || *f.Q != "a_b" {
			t.Fatalf("filter = %+v", f)
		}
		item := decodeObject(t, body)["items"].([]any)[0].(map[string]any)
		if _, ok := item["password_hash"]; ok || item["organization_id"] != nil {
			t.Fatalf("item = %v", item)
		}
	})

	t.Run("block — 200 blocked", func(t *testing.T) {
		fake := &fakeAdmin{}
		srv := newAdminServer(t, fake)
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/users/"+userID.String()+"/block",
			"application/json", strings.NewReader(`{"reason":"мошенничество"}`))
		if resp.StatusCode != http.StatusOK || fake.gotID != userID || fake.gotReason != "мошенничество" {
			t.Fatalf("status = %d, id = %s, reason = %q; body %s", resp.StatusCode, fake.gotID, fake.gotReason, body)
		}
		if raw := decodeObject(t, body); raw["status"] != "blocked" {
			t.Fatalf("body = %s", body)
		}
	})

	t.Run("block без причины — 400", func(t *testing.T) {
		srv := newAdminServer(t, &fakeAdmin{})
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/users/"+userID.String()+"/block", "", nil)
		wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "")
	})

	t.Run("block администратора — 403 от сервиса", func(t *testing.T) {
		srv := newAdminServer(t, &fakeAdmin{err: apperr.Forbidden("Нельзя менять статус администратора")})
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/users/"+userID.String()+"/block",
			"application/json", strings.NewReader(`{"reason":"по ошибке"}`))
		wantError(t, resp, body, http.StatusForbidden, "FORBIDDEN", "")
	})

	t.Run("unblock — 200 active", func(t *testing.T) {
		fake := &fakeAdmin{}
		srv := newAdminServer(t, fake)
		resp, body := sessionRequest(t, srv, "admin", "POST", "/api/v1/admin/users/"+userID.String()+"/unblock", "", nil)
		if resp.StatusCode != http.StatusOK || fake.unblockedID != userID {
			t.Fatalf("status = %d; body %s", resp.StatusCode, body)
		}
		if raw := decodeObject(t, body); raw["status"] != "active" {
			t.Fatalf("body = %s", body)
		}
	})
}
