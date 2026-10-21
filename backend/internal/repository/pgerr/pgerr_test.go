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
		{name: "other pg error", err: &pgconn.PgError{Code: "40001"}, wantCode: apperr.CodeInternal, wantSame: true},
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
