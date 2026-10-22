package handler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/applicationsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/authapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/offersapi"
)

// Через HTTP до paramError почти ничего не доходит: неверные параметры раньше отбивает
// validateRequests. Поэтому ошибки сгенерированного парсера проверяются напрямую —
// из разных <tag>api, ведь у каждого пакета свои копии этих типов.
func TestParamName(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"invalid format", &applicationsapi.InvalidParamFormatError{ParamName: "applicationId", Err: errors.New("bad uuid")}, "applicationId"},
		{"required", &offersapi.RequiredParamError{ParamName: "offerId"}, "offerId"},
		{"required header", &authapi.RequiredHeaderError{ParamName: "X-CSRF-Token"}, "X-CSRF-Token"},
		{"unmarshal", &applicationsapi.UnmarshalingParamError{ParamName: "page", Err: errors.New("NaN")}, "page"},
		{"too many", &offersapi.TooManyValuesForParamError{ParamName: "status", Count: 2}, "status"},
		{"cookie", &authapi.UnescapedCookieParamError{ParamName: "session", Err: errors.New("%zz")}, "session"},
		{"обёрнутая", fmt.Errorf("parse: %w", &offersapi.InvalidParamFormatError{ParamName: "offerId", Err: errors.New("x")}), "offerId"},
		{"тело запроса", errors.New("invalid character '}'"), ""},
		{"чужой тип с ParamName", fmt.Errorf("x: %w", &foreignParamError{ParamName: "nope"}), ""},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := paramName(tt.err); got != tt.want {
				t.Fatalf("paramName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParamErrorMapsToValidation(t *testing.T) {
	var e *apperr.Error

	err := paramError(&offersapi.InvalidParamFormatError{ParamName: "offerId", Err: errors.New("bad uuid")})
	if !errors.As(err, &e) || e.Code != apperr.CodeValidation || len(e.Details) != 1 || e.Details[0].Field != "offerId" {
		t.Fatalf("параметр: got %#v", err)
	}

	err = paramError(errors.New("unexpected EOF"))
	if !errors.As(err, &e) || e.Code != apperr.CodeValidation || len(e.Details) != 0 {
		t.Fatalf("тело: got %#v", err)
	}
}

// foreignParamError — тот же shape (указатель на struct с ParamName string), но не из
// сгенерированного пакета: paramName обязан его игнорировать.
type foreignParamError struct{ ParamName string }

func (e *foreignParamError) Error() string { return e.ParamName }
