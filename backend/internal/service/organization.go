package service

import (
	"time"

	"github.com/google/uuid"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Org — доменная модель организации: ответ `Me` (auth) и профиль KYC (kyc).
type Org struct {
	ID                 uuid.UUID
	Type               string
	Name               string
	CountryCode        string
	RegistrationNumber string
	LEI                *string
	LegalAddress       string
	KycStatus          string
	CreatedAt          time.Time
	VerifiedAt         *time.Time
	KycRejectReason    *string
}

// OrgFromRepository переводит строку organizations в Org.
func OrgFromRepository(o repository.Organization) *Org {
	org := Org{
		ID:                 o.ID,
		Type:               string(o.Type),
		Name:               o.Name,
		CountryCode:        o.CountryCode,
		RegistrationNumber: o.RegistrationNumber,
		LegalAddress:       o.LegalAddress,
		KycStatus:          string(o.KycStatus),
		CreatedAt:          o.CreatedAt.Time,
	}
	if o.Lei.Valid {
		v := o.Lei.String
		org.LEI = &v
	}
	if o.VerifiedAt.Valid {
		v := o.VerifiedAt.Time
		org.VerifiedAt = &v
	}
	if o.KycRejectReason.Valid {
		v := o.KycRejectReason.String
		org.KycRejectReason = &v
	}
	return &org
}
