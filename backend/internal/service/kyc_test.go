package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeKycStore — БД одной организации в памяти. Транзакция (fakeKycTx) работает с ней же:
// откат не моделируется, тесты проверяют, что дошло до вызовов, а не до коммита.
type fakeKycStore struct {
	repository.Querier
	org           repository.Organization
	beneficiaries []repository.Beneficiary
	locked        bool // GetOrganizationForUpdate вызван
}

func newFakeKycStore(status repository.KycStatus) *fakeKycStore {
	return &fakeKycStore{org: repository.Organization{
		ID:                 uuid.New(),
		Type:               repository.OrganizationTypeBorrower,
		Name:               "ООО Альфа",
		CountryCode:        "RU",
		RegistrationNumber: "7707083893",
		LegalAddress:       "Москва",
		KycStatus:          status,
	}}
}

func (s *fakeKycStore) principal() service.Principal {
	orgID := s.org.ID
	return service.Principal{UserID: uuid.New(), OrgID: &orgID, Role: string(s.org.Type), KycStatus: string(s.org.KycStatus)}
}

func (s *fakeKycStore) GetOrganizationByID(_ context.Context, id uuid.UUID) (repository.Organization, error) {
	if id != s.org.ID {
		return repository.Organization{}, pgx.ErrNoRows
	}
	return s.org, nil
}

func (s *fakeKycStore) GetOrganizationForUpdate(ctx context.Context, id uuid.UUID) (repository.Organization, error) {
	s.locked = true
	return s.GetOrganizationByID(ctx, id)
}

func (s *fakeKycStore) UpdateOrganization(_ context.Context, arg repository.UpdateOrganizationParams) (repository.Organization, error) {
	if arg.ID != s.org.ID || (s.org.KycStatus != repository.KycStatusDraft && s.org.KycStatus != repository.KycStatusRejected) {
		return repository.Organization{}, pgx.ErrNoRows
	}
	if arg.Name.Valid {
		s.org.Name = arg.Name.String
	}
	if arg.RegistrationNumber.Valid {
		s.org.RegistrationNumber = arg.RegistrationNumber.String
	}
	if arg.LegalAddress.Valid {
		s.org.LegalAddress = arg.LegalAddress.String
	}
	if arg.Lei.Valid {
		s.org.Lei = pgtype.Text{String: arg.Lei.String, Valid: arg.Lei.String != ""}
	}
	return s.org, nil
}

func (s *fakeKycStore) ListBeneficiariesByOrg(_ context.Context, orgID uuid.UUID) ([]repository.Beneficiary, error) {
	var out []repository.Beneficiary
	for _, b := range s.beneficiaries {
		if b.OrganizationID == orgID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (s *fakeKycStore) DeleteBeneficiariesByOrg(_ context.Context, orgID uuid.UUID) error {
	kept := s.beneficiaries[:0]
	for _, b := range s.beneficiaries {
		if b.OrganizationID != orgID {
			kept = append(kept, b)
		}
	}
	s.beneficiaries = kept
	return nil
}

func (s *fakeKycStore) CreateBeneficiary(_ context.Context, arg repository.CreateBeneficiaryParams) (repository.Beneficiary, error) {
	b := repository.Beneficiary{
		ID: uuid.New(), OrganizationID: arg.OrganizationID, FullName: arg.FullName,
		CitizenshipCountryCode: arg.CitizenshipCountryCode, OwnershipPercent: arg.OwnershipPercent,
	}
	s.beneficiaries = append(s.beneficiaries, b)
	return b, nil
}

type fakeKycTx struct{ store *fakeKycStore }

func (t fakeKycTx) InTx(_ context.Context, fn func(q repository.Querier) error) error {
	return fn(t.store)
}

func newKyc(store *fakeKycStore) *service.Kyc {
	return service.NewKyc(service.KycDeps{Store: store, Tx: fakeKycTx{store: store}})
}

func ptr[T any](v T) *T { return &v }

func beneficiary(name, percent string) service.BeneficiaryInput {
	return service.BeneficiaryInput{FullName: name, CitizenshipCountryCode: "RU", OwnershipPercent: decimal.RequireFromString(percent)}
}

func wantCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	if e, ok := apperr.As(err); !ok || e.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

// wantValidationField — 400 VALIDATION_ERROR с записью field в details.
func wantValidationField(t *testing.T, err error, field string) {
	t.Helper()
	wantCode(t, err, apperr.CodeValidation)
	e, _ := apperr.As(err)
	for _, d := range e.Details {
		if d.Field == field {
			return
		}
	}
	t.Fatalf("details = %+v, want field %q", e.Details, field)
}

func TestKycUpdateMyOrganization(t *testing.T) {
	ctx := context.Background()

	t.Run("draft — новые поля и бенефициары", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusDraft)
		got, err := newKyc(store).UpdateMyOrganization(ctx, store.principal(), service.OrganizationUpdateInput{
			Name:          ptr("ООО Альфа Плюс"),
			Beneficiaries: &[]service.BeneficiaryInput{beneficiary("Иванов И.И.", "60"), beneficiary("Петров П.П.", "40")},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Org.Name != "ООО Альфа Плюс" || len(got.Beneficiaries) != 2 || got.Beneficiaries[0].FullName != "Иванов И.И." {
			t.Fatalf("profile = %+v", got)
		}
		if !store.locked {
			t.Error("строка организации не заблокирована (GetOrganizationForUpdate)")
		}
	})

	t.Run("under_review — 409", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusUnderReview)
		_, err := newKyc(store).UpdateMyOrganization(ctx, store.principal(), service.OrganizationUpdateInput{Name: ptr("Другое имя")})
		wantCode(t, err, apperr.CodeInvalidTransition)
		if store.org.Name != "ООО Альфа" {
			t.Errorf("имя изменилось: %q", store.org.Name)
		}
	})

	t.Run("сумма долей 60 + 50 — 400", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusDraft)
		_, err := newKyc(store).UpdateMyOrganization(ctx, store.principal(), service.OrganizationUpdateInput{
			Beneficiaries: &[]service.BeneficiaryInput{beneficiary("А", "60"), beneficiary("Б", "50")},
		})
		wantValidationField(t, err, "beneficiaries")
	})

	t.Run("beneficiaries не передан — список не тронут", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusRejected)
		store.beneficiaries = []repository.Beneficiary{{ID: uuid.New(), OrganizationID: store.org.ID, FullName: "Иванов И.И."}}
		got, err := newKyc(store).UpdateMyOrganization(ctx, store.principal(), service.OrganizationUpdateInput{LegalAddress: ptr("Санкт-Петербург")})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got.Beneficiaries) != 1 || len(store.beneficiaries) != 1 {
			t.Fatalf("бенефициары = %+v, want один, не тронутый", got.Beneficiaries)
		}
	})

	t.Run("beneficiaries: [] — список очищен", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusDraft)
		store.beneficiaries = []repository.Beneficiary{{ID: uuid.New(), OrganizationID: store.org.ID, FullName: "Иванов И.И."}}
		got, err := newKyc(store).UpdateMyOrganization(ctx, store.principal(), service.OrganizationUpdateInput{Beneficiaries: &[]service.BeneficiaryInput{}})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got.Beneficiaries) != 0 || len(store.beneficiaries) != 0 {
			t.Fatalf("бенефициары = %+v, want пусто", got.Beneficiaries)
		}
	})
}

func TestKycMyOrganization(t *testing.T) {
	ctx := context.Background()

	t.Run("без бенефициаров — пустой срез", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusDraft)
		got, err := newKyc(store).MyOrganization(ctx, store.principal())
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Org.ID != store.org.ID || got.Beneficiaries == nil {
			t.Fatalf("profile = %+v, want своя организация и не-nil бенефициары", got)
		}
	})

	t.Run("admin без организации — 403", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusDraft)
		_, err := newKyc(store).MyOrganization(ctx, service.Principal{UserID: uuid.New(), Role: "admin"})
		wantCode(t, err, apperr.CodeForbidden)
	})
}
