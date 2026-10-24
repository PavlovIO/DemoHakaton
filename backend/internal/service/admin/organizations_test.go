package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

func TestGetOrganization(t *testing.T) {
	t.Run("без документов, флагов и сверки — пустые срезы и nil", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusUnderReview)

		got, err := newAdmin(db).GetOrganization(context.Background(), adminPrincipal(), org.ID)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Profile.Org.ID != org.ID || got.Profile.Beneficiaries == nil || got.Documents == nil ||
			got.AmlFlags == nil || got.Users == nil {
			t.Fatalf("details = %+v", got)
		}
		if got.RegistryCheck != nil {
			t.Fatalf("registry_check = %+v, want nil без сверки", got.RegistryCheck)
		}
	})

	t.Run("с данными — флаг с компанией, пользователи, последняя сверка", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusUnderReview)
		db.beneficiaries = []repository.Beneficiary{{ID: uuid.New(), OrganizationID: org.ID, FullName: "Иванов И.И."}}
		db.documents = []repository.KycDocument{{ID: uuid.New(), OrganizationID: org.ID, DocType: repository.KycDocumentTypeCharter}}
		db.flags = []repository.ListAmlFlagsByOrganizationRow{{
			AmlFlag: repository.AmlFlag{
				ID: 7, OrganizationID: org.ID, Rule: repository.AmlRuleSanctionsMatch, Severity: repository.AmlSeverityHigh,
				Details: "совпадение", Status: repository.AmlFlagStatusOpen,
			},
			OrganizationName: "ООО Альфа", OrganizationCountryCode: "RU",
		}}
		userID := uuid.New()
		db.orgUsers = []repository.ListUsersByOrganizationRow{{
			ID: userID, Email: "a@example.com", Role: repository.UserRoleBorrower, Status: repository.UserStatusActive,
			OrganizationID: pgtype.UUID{Bytes: org.ID, Valid: true},
		}}
		checkedAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		db.registryCheck = &repository.RegistryCheck{
			Source: repository.RegistrySourceMock, Found: true, Name: pgtype.Text{String: "ООО Альфа", Valid: true},
			CheckedAt: pgtype.Timestamptz{Time: checkedAt, Valid: true},
		}

		got, err := newAdmin(db).GetOrganization(context.Background(), adminPrincipal(), org.ID)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got.Profile.Beneficiaries) != 1 || len(got.Documents) != 1 {
			t.Fatalf("beneficiaries = %v, documents = %v", got.Profile.Beneficiaries, got.Documents)
		}
		if len(got.AmlFlags) != 1 || got.AmlFlags[0].Organization.Name != "ООО Альфа" || got.AmlFlags[0].ID != 7 ||
			got.AmlFlags[0].Severity != "high" {
			t.Fatalf("aml_flags = %+v", got.AmlFlags)
		}
		if len(got.Users) != 1 || got.Users[0].ID != userID || *got.Users[0].OrganizationID != org.ID {
			t.Fatalf("users = %+v", got.Users)
		}
		rc := got.RegistryCheck
		if rc == nil || rc.Source != "mock" || !rc.Found || rc.Name != "ООО Альфа" || !rc.CheckedAt.Equal(checkedAt) {
			t.Fatalf("registry_check = %+v", rc)
		}
	})

	t.Run("нет компании — 404", func(t *testing.T) {
		_, err := newAdmin(newFakeDB()).GetOrganization(context.Background(), adminPrincipal(), uuid.New())
		servicetest.WantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("не админ — 403", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusUnderReview)
		_, err := newAdmin(db).GetOrganization(context.Background(), borrowerPrincipal(), org.ID)
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})
}

func TestListOrganizations(t *testing.T) {
	t.Run("фильтры, экранирование q, страница и бенефициары", func(t *testing.T) {
		db := newFakeDB()
		a, b := uuid.New(), uuid.New()
		db.listOrgs = []repository.Organization{{ID: a, Name: "A"}, {ID: b, Name: "B"}}
		db.beneficiaries = []repository.Beneficiary{{ID: uuid.New(), OrganizationID: b, FullName: "Петров"}}
		db.total = 12

		got, err := newAdmin(db).ListOrganizations(context.Background(), adminPrincipal(), admin.OrganizationFilter{
			KycStatuses: []string{"under_review", "rejected"},
			Type:        servicetest.Ptr("lender"),
			CountryCode: servicetest.Ptr("CY"),
			Q:           servicetest.Ptr("a_b%"),
			Page:        2, PageSize: 5,
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		arg := db.orgListArg
		if arg.Limit != 5 || arg.Offset != 5 || len(arg.KycStatuses) != 2 || arg.Q.String != `a\_b\%` ||
			arg.Type.OrganizationType != repository.OrganizationTypeLender || arg.CountryCode.String != "CY" {
			t.Fatalf("ListOrganizations arg = %+v", arg)
		}
		if db.orgCountArg.Q.String != `a\_b\%` || len(db.orgCountArg.KycStatuses) != 2 {
			t.Fatalf("CountOrganizations arg = %+v", db.orgCountArg)
		}
		if got.Total != 12 || got.Page != 2 || got.PageSize != 5 || len(got.Items) != 2 {
			t.Fatalf("page = %+v", got)
		}
		if got.Items[0].Beneficiaries == nil || len(got.Items[0].Beneficiaries) != 0 || len(got.Items[1].Beneficiaries) != 1 {
			t.Fatalf("beneficiaries = %v / %v", got.Items[0].Beneficiaries, got.Items[1].Beneficiaries)
		}
	})

	t.Run("без фильтров — NULL в запросе", func(t *testing.T) {
		db := newFakeDB()
		if _, err := newAdmin(db).ListOrganizations(context.Background(), adminPrincipal(), admin.OrganizationFilter{}); err != nil {
			t.Fatalf("err = %v", err)
		}
		arg := db.orgListArg
		if arg.KycStatuses != nil || arg.Q.Valid || arg.Type.Valid || arg.CountryCode.Valid || arg.Limit != 20 || arg.Offset != 0 {
			t.Fatalf("arg = %+v", arg)
		}
	})

	t.Run("не админ — 403", func(t *testing.T) {
		_, err := newAdmin(newFakeDB()).ListOrganizations(context.Background(), borrowerPrincipal(), admin.OrganizationFilter{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})
}

func TestListUsers(t *testing.T) {
	t.Run("фильтры, экранирование q, страница", func(t *testing.T) {
		db := newFakeDB()
		db.listUsers = []repository.ListUsersRow{{ID: uuid.New(), Email: "a_b@example.com", Role: repository.UserRoleAdmin}}
		db.total = 1

		got, err := newAdmin(db).ListUsers(context.Background(), adminPrincipal(), admin.UserFilter{
			Role: servicetest.Ptr("admin"), Status: servicetest.Ptr("active"), Q: servicetest.Ptr("a_b"),
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		arg := db.userListArg
		if arg.Q.String != `a\_b` || arg.Role.UserRole != repository.UserRoleAdmin || arg.Status.UserStatus != repository.UserStatusActive {
			t.Fatalf("ListUsers arg = %+v", arg)
		}
		if db.userCountArg.Q.String != `a\_b` {
			t.Fatalf("CountUsers arg = %+v", db.userCountArg)
		}
		if got.Total != 1 || len(got.Items) != 1 || got.Items[0].OrganizationID != nil || got.Page != 1 {
			t.Fatalf("page = %+v", got)
		}
	})

	t.Run("не админ — 403", func(t *testing.T) {
		_, err := newAdmin(newFakeDB()).ListUsers(context.Background(), borrowerPrincipal(), admin.UserFilter{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})
}
