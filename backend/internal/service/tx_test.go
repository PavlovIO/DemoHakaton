package service_test

import (
	"context"
	"errors"
	"testing"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeTx struct {
	pgx.Tx
	commitCalls   int
	rollbackCalls int
	commitErr     error
	rollbackErr   error
}

func (tx *fakeTx) Commit(context.Context) error {
	tx.commitCalls++
	return tx.commitErr
}

func (tx *fakeTx) Rollback(context.Context) error {
	tx.rollbackCalls++
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

	tests := []struct {
		name          string
		beginErr      error
		callback      func(repository.Querier) error
		wantErr       error
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
			tx := &fakeTx{rollbackErr: tt.rollbackErr}
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

func TestTxManagerIntegrationRollback(t *testing.T) {
	pool := newPool(t)
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
