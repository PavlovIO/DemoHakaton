package service

import (
	"context"
	"time"

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

// snapshotBeginner — пул: открывает транзакцию с уровнем изоляции. pgx.Tx (интеграционные
// тесты) так не умеет: его Begin — savepoint с уровнем изоляции внешней транзакции.
type snapshotBeginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

// InTx выполняет переданную функцию fn внутри транзакции.
// Если fn возвращает ошибку или вызывает панику, транзакция автоматически откатывается.
func (m *TxManager) InTx(ctx context.Context, fn func(q repository.Querier) error) error {
	return m.run(ctx, m.beginner.Begin, fn)
}

// ReadSnapshot выполняет чтения fn в одном снимке БД (REPEATABLE READ, READ ONLY): несколько
// запросов одной страницы (элементы и счётчики) видят одно и то же состояние таблиц.
func (m *TxManager) ReadSnapshot(ctx context.Context, fn func(q repository.Querier) error) error {
	begin := m.beginner.Begin
	if b, ok := m.beginner.(snapshotBeginner); ok {
		begin = func(ctx context.Context) (pgx.Tx, error) {
			return b.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		}
	}
	return m.run(ctx, begin, fn)
}

func (m *TxManager) run(ctx context.Context, begin func(context.Context) (pgx.Tx, error), fn func(q repository.Querier) error) error {
	tx, err := begin(ctx)
	if err != nil {
		return err
	}

	rollback := func() {
		rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rbCtx)
	}

	committed := false
	defer func() {
		if panicValue := recover(); panicValue != nil {
			rollback()
			panic(panicValue)
		}
		if !committed {
			rollback()
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
