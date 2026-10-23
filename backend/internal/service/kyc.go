package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// maxOwnershipPercent — сумма долей бенефициаров; в БД не проверяется (beneficiaries.md).
var maxOwnershipPercent = decimal.NewFromInt(100)

// Beneficiary — конечный бенефициар компании.
type Beneficiary struct {
	ID                     uuid.UUID
	FullName               string
	CitizenshipCountryCode string
	OwnershipPercent       decimal.Decimal
}

// OrganizationProfile — профиль компании для GET/PATCH /organizations/me и submitKyc.
type OrganizationProfile struct {
	Org           *Org
	Beneficiaries []Beneficiary // не nil: в ответе — [], а не null
}

// BeneficiaryInput — бенефициар из PATCH /organizations/me.
type BeneficiaryInput struct {
	FullName               string
	CitizenshipCountryCode string
	OwnershipPercent       decimal.Decimal
}

// OrganizationUpdateInput — частичное обновление профиля. nil — поле не трогать.
// LEI: "" — очистить. Beneficiaries: nil — список не трогать, пустой срез — очистить,
// иначе заменить целиком.
type OrganizationUpdateInput struct {
	Name               *string
	RegistrationNumber *string
	LegalAddress       *string
	LEI                *string
	Beneficiaries      *[]BeneficiaryInput
}

// KycStore — чтения вне транзакции.
type KycStore interface {
	GetOrganizationByID(ctx context.Context, id uuid.UUID) (repository.Organization, error)
	ListBeneficiariesByOrg(ctx context.Context, organizationID uuid.UUID) ([]repository.Beneficiary, error)
}

// KycTx — транзакция, в которой выполняются изменения профиля, документов и KYC.
type KycTx interface {
	InTx(ctx context.Context, fn func(q repository.Querier) error) error
}

// KycDeps — зависимости сервиса KYC.
type KycDeps struct {
	Store KycStore
	Tx    KycTx
}

// Kyc — профиль компании, бенефициары, KYC-документы и отправка на проверку.
type Kyc struct {
	store KycStore
	tx    KycTx
}

// NewKyc создаёт сервис KYC.
func NewKyc(d KycDeps) *Kyc {
	return &Kyc{store: d.Store, tx: d.Tx}
}

// ownOrganization — организация заёмщика или кредитора. Роль проверяет и middleware
// (x-roles), здесь — защита в глубину: у admin организации нет.
func ownOrganization(p Principal) (uuid.UUID, error) {
	if p.OrgID == nil || (p.Role != string(repository.UserRoleBorrower) && p.Role != string(repository.UserRoleLender)) {
		return uuid.Nil, apperr.Forbidden("Операция доступна только заёмщику и кредитору")
	}
	return *p.OrgID, nil
}

// MyOrganization — профиль своей компании с бенефициарами.
func (k *Kyc) MyOrganization(ctx context.Context, p Principal) (OrganizationProfile, error) {
	orgID, err := ownOrganization(p)
	if err != nil {
		return OrganizationProfile{}, err
	}
	return k.profile(ctx, k.store, orgID)
}

// UpdateMyOrganization правит профиль и (если передан) заменяет список бенефициаров.
// Разрешено только в draft и rejected: строка организации блокируется, и статус
// проверяется под блокировкой — параллельный submit не проскочит между проверкой и UPDATE.
func (k *Kyc) UpdateMyOrganization(ctx context.Context, p Principal, in OrganizationUpdateInput) (OrganizationProfile, error) {
	orgID, err := ownOrganization(p)
	if err != nil {
		return OrganizationProfile{}, err
	}
	if in.Beneficiaries != nil {
		if err := checkOwnershipTotal(*in.Beneficiaries); err != nil {
			return OrganizationProfile{}, err
		}
	}

	var profile OrganizationProfile
	err = k.tx.InTx(ctx, func(q repository.Querier) error {
		org, err := q.GetOrganizationForUpdate(ctx, orgID)
		if err != nil {
			return pgerr.Map(err, "Организация не найдена")
		}
		if _, err := nextKycStatus(org.KycStatus, KycEdit); err != nil {
			return err
		}
		if _, err := q.UpdateOrganization(ctx, updateOrganizationParams(orgID, in)); err != nil {
			if errors.Is(err, pgx.ErrNoRows) { // охрана статуса в WHERE; под блокировкой не должна сработать
				return kycEditDenied()
			}
			// Уникальность registration_number в стране и lei → 409 CONFLICT.
			return pgerr.Map(err, "Организация не найдена")
		}
		if in.Beneficiaries != nil {
			if err := replaceBeneficiaries(ctx, q, orgID, *in.Beneficiaries); err != nil {
				return err
			}
		}
		profile, err = k.profile(ctx, q, orgID)
		return err
	})
	if err != nil {
		return OrganizationProfile{}, err
	}
	return profile, nil
}

// checkOwnershipTotal — сумма долей бенефициаров не больше 100 %.
func checkOwnershipTotal(list []BeneficiaryInput) error {
	total := decimal.Zero
	for _, b := range list {
		total = total.Add(b.OwnershipPercent)
	}
	if total.GreaterThan(maxOwnershipPercent) {
		return apperr.Validation("Запрос не соответствует контракту", apperr.FieldError{
			Field:   "beneficiaries",
			Message: fmt.Sprintf("сумма долей %s%% больше 100%%", total.String()),
		})
	}
	return nil
}

// replaceBeneficiaries заменяет список целиком: delete + insert в транзакции вызывающего.
func replaceBeneficiaries(ctx context.Context, q repository.Querier, orgID uuid.UUID, list []BeneficiaryInput) error {
	if err := q.DeleteBeneficiariesByOrg(ctx, orgID); err != nil {
		return fmt.Errorf("delete beneficiaries: %w", err)
	}
	for _, b := range list {
		if _, err := q.CreateBeneficiary(ctx, repository.CreateBeneficiaryParams{
			OrganizationID:         orgID,
			FullName:               b.FullName,
			CitizenshipCountryCode: b.CitizenshipCountryCode,
			OwnershipPercent:       b.OwnershipPercent,
		}); err != nil {
			return pgerr.Map(err, "Страна гражданства не найдена")
		}
	}
	return nil
}

func updateOrganizationParams(orgID uuid.UUID, in OrganizationUpdateInput) repository.UpdateOrganizationParams {
	return repository.UpdateOrganizationParams{
		ID:                 orgID,
		Name:               optionalText(in.Name),
		RegistrationNumber: optionalText(in.RegistrationNumber),
		LegalAddress:       optionalText(in.LegalAddress),
		Lei:                optionalText(in.LEI), // "" доходит до SQL и очищает LEI
	}
}

func optionalText(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

// profileReader — то, что нужно для чтения профиля и вне транзакции, и внутри неё.
type profileReader interface {
	GetOrganizationByID(ctx context.Context, id uuid.UUID) (repository.Organization, error)
	ListBeneficiariesByOrg(ctx context.Context, organizationID uuid.UUID) ([]repository.Beneficiary, error)
}

func (k *Kyc) profile(ctx context.Context, r profileReader, orgID uuid.UUID) (OrganizationProfile, error) {
	org, err := r.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return OrganizationProfile{}, pgerr.Map(err, "Организация не найдена")
	}
	rows, err := r.ListBeneficiariesByOrg(ctx, orgID)
	if err != nil {
		return OrganizationProfile{}, fmt.Errorf("list beneficiaries: %w", err)
	}
	beneficiaries := make([]Beneficiary, len(rows))
	for i, b := range rows {
		beneficiaries[i] = Beneficiary{
			ID:                     b.ID,
			FullName:               b.FullName,
			CitizenshipCountryCode: b.CitizenshipCountryCode,
			OwnershipPercent:       b.OwnershipPercent,
		}
	}
	return OrganizationProfile{Org: organizationFromRepository(org), Beneficiaries: beneficiaries}, nil
}
