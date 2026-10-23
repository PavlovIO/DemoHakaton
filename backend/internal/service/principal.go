package service

import (
	"github.com/google/uuid"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Principal — кто выполняет запрос: пользователь, его организация, роль и KYC-статус компании.
// Строится из строки сессии на каждый запрос и не кэшируется, поэтому одобрение KYC
// админом действует со следующего запроса.
type Principal struct {
	UserID    uuid.UUID
	OrgID     *uuid.UUID
	Role      string
	KycStatus string // kyc_status организации; "" у admin
}

// ViewerRole — параметр viewer_role для запросов *Visible.
func (p Principal) ViewerRole() repository.UserRole {
	return repository.UserRole(p.Role)
}

// ViewerOrg — параметр viewer_org для запросов *Visible. У admin организации нет, и он
// получает uuid.Nil: ветка admin в SQL его не читает, а при перепутанной роли uuid.Nil
// не совпадёт ни с одной организацией — запрос вернёт пусто, а не чужое.
func (p Principal) ViewerOrg() uuid.UUID {
	if p.OrgID == nil {
		return uuid.Nil
	}
	return *p.OrgID
}
