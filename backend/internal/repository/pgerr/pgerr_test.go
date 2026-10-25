package pgerr

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
)

func TestMap(t *testing.T) {
	other := errors.New("connection reset")

	tests := []struct {
		name     string
		err      error
		wantCode apperr.Code
		wantSame bool // ошибка должна вернуться без изменений
	}{
		{name: "no rows", err: pgx.ErrNoRows, wantCode: apperr.CodeNotFound},
		{name: "wrapped no rows", err: fmt.Errorf("get: %w", pgx.ErrNoRows), wantCode: apperr.CodeNotFound},
		{name: "unique", err: &pgconn.PgError{Code: "23505"}, wantCode: apperr.CodeConflict},
		{name: "foreign key", err: &pgconn.PgError{Code: "23503"}, wantCode: apperr.CodeValidation},
		{name: "check", err: &pgconn.PgError{Code: "23514"}, wantCode: apperr.CodeValidation},
		{name: "statement timeout", err: &pgconn.PgError{Code: "57014"}, wantCode: apperr.CodeUnavailable},
		{name: "lock timeout", err: fmt.Errorf("lock: %w", &pgconn.PgError{Code: "55P03"}), wantCode: apperr.CodeUnavailable},
		{name: "idle in transaction timeout", err: &pgconn.PgError{Code: "25P03"}, wantCode: apperr.CodeUnavailable},
		{name: "too many connections", err: &pgconn.PgError{Code: "53300"}, wantCode: apperr.CodeUnavailable},
		{name: "serialization failure", err: &pgconn.PgError{Code: "40001"}, wantCode: apperr.CodeUnavailable},
		{name: "deadlock", err: &pgconn.PgError{Code: "40P01"}, wantCode: apperr.CodeUnavailable},
		{name: "other pg error", err: &pgconn.PgError{Code: "42P01"}, wantCode: apperr.CodeInternal, wantSame: true},
		{name: "other error", err: other, wantCode: apperr.CodeInternal, wantSame: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Map(tt.err, "не найдено")
			if code := apperr.CodeOf(got); code != tt.wantCode {
				t.Fatalf("code = %s, want %s", code, tt.wantCode)
			}
			if tt.wantSame && got != tt.err { //nolint:errorlint // нужна именно та же ошибка, без обёртки
				t.Fatalf("err = %v, want unchanged %v", got, tt.err)
			}
			if !errors.Is(got, tt.err) {
				t.Fatalf("причина потеряна: %v не оборачивает %v", got, tt.err)
			}
		})
	}

	if Map(nil, "x") != nil {
		t.Fatal("Map(nil) должен вернуть nil")
	}
}

func TestTransient(t *testing.T) {
	timeout := fmt.Errorf("lock application: %w", &pgconn.PgError{Code: "55P03"})
	if got := Transient(timeout); apperr.CodeOf(got) != apperr.CodeUnavailable || !errors.Is(got, timeout) {
		t.Fatalf("Transient(lock timeout) = %v, want UNAVAILABLE с причиной", got)
	}
	// Доменная ошибка с таймаутом внутри уже решила, что отдать клиенту.
	domain := apperr.Wrap(apperr.CodeConflict, "занято", &pgconn.PgError{Code: "55P03"})
	if got := Transient(domain); got != domain { //nolint:errorlint // нужна именно та же ошибка
		t.Fatalf("Transient(domain) = %v, want unchanged", got)
	}
	other := &pgconn.PgError{Code: "42P01"}
	if got := Transient(other); got != other { //nolint:errorlint // нужна именно та же ошибка
		t.Fatalf("Transient(other) = %v, want unchanged", got)
	}
	if Transient(nil) != nil {
		t.Fatal("Transient(nil) должен вернуть nil")
	}
}
