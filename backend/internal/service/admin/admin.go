// Package admin — решения администратора (BE-12): очередь и карточка компании, KYC,
// модерация заявок, блокировка пользователей.
//
// В отличие от остальных доменов, admin стоит над kyc и lending и импортирует их
// (ADR-0005, поправка BE-12): переходы KYC и заявок остаются в своих доменах (ADR-0001),
// admin их вызывает, а не дублирует. kyc и lending admin не импортируют — циклов нет.
package admin

import (
	"context"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// TxRunner — транзакции админки (service.TxManager): InTx — для решений, ReadSnapshot — для
// чтений из нескольких запросов. Отдельного Store нет: каждое чтение админки — это несколько
// запросов (страница и счётчик, карточка из шести частей), и вне снимка они расходятся, если
// параллельно другой админ принимает решение.
type TxRunner interface {
	InTx(ctx context.Context, fn func(q repository.Querier) error) error
	ReadSnapshot(ctx context.Context, fn func(q repository.Querier) error) error
}

// Deps — зависимости сервиса админки.
type Deps struct {
	Tx TxRunner
}

// Admin — операции тега admin. Каждый метод сначала проверяет роль admin: ту же проверку
// делает middleware.Authorize по x-roles, сервис повторяет её (ADR-0004).
type Admin struct {
	tx       TxRunner
	notifier service.Notifier
}

// New создаёт сервис админки.
func New(d Deps) *Admin {
	return &Admin{tx: d.Tx}
}

func requireAdmin(p service.Principal) error {
	return p.RequireRole(repository.UserRoleAdmin)
}
