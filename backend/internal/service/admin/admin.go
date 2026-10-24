// Package admin — решения администратора (BE-12): очередь и карточка компании, KYC,
// модерация заявок, блокировка пользователей.
//
// В отличие от остальных доменов, admin стоит над kyc и lending и импортирует их
// (ADR-0005, поправка BE-12): переходы KYC и заявок остаются в своих доменах (ADR-0001),
// admin их вызывает, а не дублирует. kyc и lending admin не импортируют — циклов нет.
package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/kyc"
)

// Store — запросы вне транзакции (чтение).
type Store interface {
	kyc.ProfileReader
	ListOrganizations(ctx context.Context, arg repository.ListOrganizationsParams) ([]repository.Organization, error)
	CountOrganizations(ctx context.Context, arg repository.CountOrganizationsParams) (int64, error)
	ListBeneficiariesByOrganizations(ctx context.Context, organizationIds []uuid.UUID) ([]repository.Beneficiary, error)
	ListKycDocumentsByOrg(ctx context.Context, organizationID uuid.UUID) ([]repository.KycDocument, error)
	ListAmlFlagsByOrganization(ctx context.Context, organizationID uuid.UUID) ([]repository.ListAmlFlagsByOrganizationRow, error)
	ListUsersByOrganization(ctx context.Context, organizationID pgtype.UUID) ([]repository.ListUsersByOrganizationRow, error)
	GetLatestRegistryCheck(ctx context.Context, organizationID pgtype.UUID) (repository.RegistryCheck, error)
	ListUsers(ctx context.Context, arg repository.ListUsersParams) ([]repository.ListUsersRow, error)
	CountUsers(ctx context.Context, arg repository.CountUsersParams) (int64, error)
}

// TxRunner — транзакция, в которой выполняется каждое решение.
type TxRunner interface {
	InTx(ctx context.Context, fn func(q repository.Querier) error) error
}

// Deps — зависимости сервиса админки.
type Deps struct {
	Store Store
	Tx    TxRunner
}

// Admin — операции тега admin. Каждый метод сначала проверяет роль admin: ту же проверку
// делает middleware.Authorize по x-roles, сервис повторяет её (ADR-0004).
type Admin struct {
	store    Store
	tx       TxRunner
	notifier service.Notifier
}

// New создаёт сервис админки.
func New(d Deps) *Admin {
	return &Admin{store: d.Store, tx: d.Tx}
}

func requireAdmin(p service.Principal) error {
	return p.RequireRole(repository.UserRoleAdmin)
}
