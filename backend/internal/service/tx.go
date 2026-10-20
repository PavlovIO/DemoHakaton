package service

import (
	"context"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/jackc/pgx/v5"
)

// TxBeginner описывает интерфейс для начала новой транзакции базы данных.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// TxManager управляет выполнением функций внутри транзакции БД с автоматическим откатом (rollback) при ошибках или паниках.
type TxManager struct {
	beginner TxBeginner
}

// NewTxManager создаёт новый экземпляр TxManager.
func NewTxManager(b TxBeginner) *TxManager {
	return &TxManager{beginner: b}
}

// InTx выполняет переданную функцию fn внутри транзакции.
// Если fn возвращает ошибку или вызывает панику, транзакция автоматически откатывается.
func (m *TxManager) InTx(ctx context.Context, fn func(q repository.Querier) error) error {
	tx, err := m.beginner.Begin(ctx)
	if err != nil {
		return err
	}

	committed := false
	defer func() {
		if panicValue := recover(); panicValue != nil {
			_ = tx.Rollback(ctx)
			panic(panicValue)
		}
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if err := fn(repository.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}
