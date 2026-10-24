package service

import (
	"slices"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
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

// RequireRole — 403 FORBIDDEN, если роли пользователя нет среди allowed. Ту же проверку
// делает middleware.Authorize по x-roles; сервис повторяет её (ADR-0004, защита в
// глубину), чтобы ошибка в спеке или вызов мимо HTTP не открыли операцию.
// Пустой allowed закрывает операцию для всех.
func (p Principal) RequireRole(allowed ...repository.UserRole) error {
	if slices.Contains(allowed, p.ViewerRole()) {
		return nil
	}
	return apperr.Forbidden("Недостаточно прав для операции")
}

// RequireKYCVerified — 403 KYC_NOT_VERIFIED, если компания не в статусе verified.
// Повторяет x-kyc-required из middleware.Authorize. Проверять в транзакции создания не
// нужно: у verified нет исходящих переходов (ADR-0001), статус не испортится до INSERT.
func (p Principal) RequireKYCVerified() error {
	if p.KycStatus != string(repository.KycStatusVerified) {
		return apperr.KYCNotVerified()
	}
	return nil
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
