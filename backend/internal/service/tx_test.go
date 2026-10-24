package service_test

import (
	"context"
	"errors"
	"testing"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeTx struct {
	pgx.Tx
	commitCalls    int
	rollbackCalls  int
	commitErr      error
	rollbackErr    error
	rollbackCtxErr error
}

func (tx *fakeTx) Commit(context.Context) error {
	tx.commitCalls++
	return tx.commitErr
}

func (tx *fakeTx) Rollback(ctx context.Context) error {
	tx.rollbackCalls++
	tx.rollbackCtxErr = ctx.Err()
	return tx.rollbackErr
}

type fakeTxBeginner struct {
	tx         pgx.Tx
	err        error
	beginCalls int
}

func (b *fakeTxBeginner) Begin(context.Context) (pgx.Tx, error) {
	b.beginCalls++
	return b.tx, b.err
}

func TestTxManagerInTx(t *testing.T) {
	beginErr := errors.New("begin failed")
	callbackErr := errors.New("callback failed")
	commitErr := errors.New("commit failed")

	tests := []struct {
		name          string
		beginErr      error
		callback      func(repository.Querier) error
		wantErr       error
		commitErr     error
		rollbackErr   error
		wantCommit    int
		wantRollback  int
		wantBeginCall int
		wantPanic     any
	}{
		{
			name: "success commits",
			callback: func(q repository.Querier) error {
				if q == nil {
					t.Fatal("callback received a nil querier")
				}
				return nil
			},
			wantCommit:    1,
			wantBeginCall: 1,
		},
		{
			name: "callback error rolls back",
			callback: func(repository.Querier) error {
				return callbackErr
			},
			wantErr:       callbackErr,
			rollbackErr:   errors.New("rollback failed"),
			wantRollback:  1,
			wantBeginCall: 1,
		},
		{
			name: "commit error is returned",
			callback: func(repository.Querier) error {
				return nil
			},
			commitErr:     commitErr,
			wantErr:       commitErr,
			wantCommit:    1,
			wantRollback:  1,
			wantBeginCall: 1,
		},
		{
			name: "panic rolls back and propagates",
			callback: func(repository.Querier) error {
				panic("callback panic")
			},
			wantRollback:  1,
			wantBeginCall: 1,
			wantPanic:     "callback panic",
		},
		{
			name:          "begin error is returned",
			beginErr:      beginErr,
			wantErr:       beginErr,
			wantBeginCall: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &fakeTx{commitErr: tt.commitErr, rollbackErr: tt.rollbackErr}
			beginner := &fakeTxBeginner{tx: tx, err: tt.beginErr}
			manager := service.NewTxManager(beginner)

			var panicValue any
			func() {
				defer func() {
					panicValue = recover()
				}()
				err := manager.InTx(context.Background(), tt.callback)
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("InTx() error = %v, want %v", err, tt.wantErr)
				}
			}()

			if panicValue != tt.wantPanic {
				t.Errorf("panic = %v, want %v", panicValue, tt.wantPanic)
			}
			if beginner.beginCalls != tt.wantBeginCall {
				t.Errorf("Begin() calls = %d, want %d", beginner.beginCalls, tt.wantBeginCall)
			}
			if tx.commitCalls != tt.wantCommit {
				t.Errorf("Commit() calls = %d, want %d", tx.commitCalls, tt.wantCommit)
			}
			if tx.rollbackCalls != tt.wantRollback {
				t.Errorf("Rollback() calls = %d, want %d", tx.rollbackCalls, tt.wantRollback)
			}
		})
	}
}

// fakeSnapshotBeginner — пул: умеет BeginTx и запоминает опции транзакции.
type fakeSnapshotBeginner struct {
	fakeTxBeginner
	opts *pgx.TxOptions
}

func (b *fakeSnapshotBeginner) BeginTx(_ context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	b.opts = &opts
	return b.tx, b.err
}

func TestTxManagerReadSnapshot(t *testing.T) {
	tx := &fakeTx{}
	beginner := &fakeSnapshotBeginner{fakeTxBeginner: fakeTxBeginner{tx: tx}}

	err := service.NewTxManager(beginner).ReadSnapshot(context.Background(), func(q repository.Querier) error {
		if q == nil {
			t.Fatal("callback received a nil querier")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	if beginner.opts == nil || *beginner.opts != want {
		t.Fatalf("BeginTx opts = %+v, want %+v", beginner.opts, want)
	}
	if beginner.beginCalls != 0 || tx.commitCalls != 1 {
		t.Fatalf("Begin() calls = %d, Commit() calls = %d; want 0 и 1", beginner.beginCalls, tx.commitCalls)
	}
}

// Без BeginTx (pgx.Tx в интеграционных тестах) снимок — обычный Begin: savepoint внешней транзакции.
func TestTxManagerReadSnapshotFallsBackToBegin(t *testing.T) {
	tx := &fakeTx{}
	beginner := &fakeTxBeginner{tx: tx}

	if err := service.NewTxManager(beginner).ReadSnapshot(context.Background(), func(repository.Querier) error { return nil }); err != nil {
		t.Fatal(err)
	}

	if beginner.beginCalls != 1 || tx.commitCalls != 1 {
		t.Fatalf("Begin() calls = %d, Commit() calls = %d; want 1 и 1", beginner.beginCalls, tx.commitCalls)
	}
}

func TestTxManagerRollbackWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	callbackErr := errors.New("callback failed")
	tx := &fakeTx{}
	beginner := &fakeTxBeginner{tx: tx}
	manager := service.NewTxManager(beginner)

	err := manager.InTx(ctx, func(repository.Querier) error {
		return callbackErr
	})
	if !errors.Is(err, callbackErr) {
		t.Fatalf("InTx() error = %v, want %v", err, callbackErr)
	}
	if tx.rollbackCalls != 1 {
		t.Fatalf("Rollback() calls = %d, want 1", tx.rollbackCalls)
	}
	if tx.rollbackCtxErr != nil {
		t.Errorf("Rollback() context error = %v, want nil", tx.rollbackCtxErr)
	}
}

func TestTxManagerIntegrationRollback(t *testing.T) {
	pool := servicetest.NewPool(t)
	ctx := context.Background()
	action := "tx-manager-integration-rollback-" + uuid.NewString()
	wantErr := errors.New("force rollback")

	err := service.NewTxManager(pool).InTx(ctx, func(q repository.Querier) error {
		_, err := q.CreateAuditLog(ctx, repository.CreateAuditLogParams{
			Action: action,
			Meta:   []byte(`{}`),
		})
		if err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("InTx() error = %v, want the callback error %v", err, wantErr)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE action = $1", action).Scan(&count); err != nil {
		t.Fatalf("count audit log rows: %v", err)
	}
	if count != 0 {
		t.Errorf("audit log rows after rollback = %d, want 0", count)
	}
}
