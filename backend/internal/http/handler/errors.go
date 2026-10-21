package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
)

// statusByCode — единственное место, где доменный код превращается в HTTP-статус.
var statusByCode = map[apperr.Code]int{
	apperr.CodeValidation:        http.StatusBadRequest,
	apperr.CodeUnauthorized:      http.StatusUnauthorized,
	apperr.CodeForbidden:         http.StatusForbidden,
	apperr.CodeCSRFInvalid:       http.StatusForbidden,
	apperr.CodeKYCNotVerified:    http.StatusForbidden,
	apperr.CodeNotFound:          http.StatusNotFound,
	apperr.CodeConflict:          http.StatusConflict,
	apperr.CodeInvalidTransition: http.StatusConflict,
	apperr.CodeRateLimited:       http.StatusTooManyRequests,
	apperr.CodeInternal:          http.StatusInternalServerError,
	apperr.CodeNotImplemented:    http.StatusNotImplemented,
	apperr.CodeUnavailable:       http.StatusServiceUnavailable,
}

const internalMessage = "Внутренняя ошибка сервера"

// StatusOf возвращает HTTP-статус для доменного кода (неизвестный код → 500).
func StatusOf(code apperr.Code) int {
	if s, ok := statusByCode[code]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// ErrorBody переводит ошибку в тело ответа из спеки.
// Недоменные ошибки превращаются в INTERNAL без деталей — текст причины клиенту не уходит.
func ErrorBody(err error) (int, api.Error) {
	e, ok := apperr.As(err)
	if !ok || e.Code == apperr.CodeInternal {
		return http.StatusInternalServerError, api.Error{Error: api.ErrorBody{
			Code:    string(apperr.CodeInternal),
			Message: internalMessage,
		}}
	}
	body := api.ErrorBody{Code: string(e.Code), Message: e.Message}
	if len(e.Details) > 0 {
		details := make([]api.ErrorDetail, len(e.Details))
		for i, d := range e.Details {
			details[i] = api.ErrorDetail{Field: d.Field, Message: d.Message}
		}
		body.Details = details
	}
	return StatusOf(e.Code), api.Error{Error: body}
}

// WriteError пишет ошибку в едином формате Error и логирует её.
// 5xx — уровень Error с полной причиной, 4xx — Debug (это нормальная работа API).
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status, body := ErrorBody(err)

	attrs := []any{"method", r.Method, "path", r.URL.Path, "status", status, "code", body.Error.Code, "err", err}
	if status >= http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "request failed", attrs...)
	} else {
		slog.DebugContext(r.Context(), "request rejected", attrs...)
	}

	writeJSON(w, status, body)
}

// requestError — ошибки разбора запроса до вызова хендлера: битый JSON, неверный path/query-параметр.
func requestError(w http.ResponseWriter, r *http.Request, err error) {
	WriteError(w, r, paramError(err))
}

// paramError переводит ошибки сгенерированного парсера параметров в VALIDATION_ERROR с полем.
func paramError(err error) error {
	field := ""
	var (
		invalidFormat *api.InvalidParamFormatError
		required      *api.RequiredParamError
		requiredHdr   *api.RequiredHeaderError
		unmarshal     *api.UnmarshalingParamError
		tooMany       *api.TooManyValuesForParamError
		cookie        *api.UnescapedCookieParamError
	)
	switch {
	case errors.As(err, &invalidFormat):
		field = invalidFormat.ParamName
	case errors.As(err, &required):
		field = required.ParamName
	case errors.As(err, &requiredHdr):
		field = requiredHdr.ParamName
	case errors.As(err, &unmarshal):
		field = unmarshal.ParamName
	case errors.As(err, &tooMany):
		field = tooMany.ParamName
	case errors.As(err, &cookie):
		field = cookie.ParamName
	}
	if field == "" {
		// Ошибка декодирования тела запроса (strict-сервер).
		return apperr.Wrap(apperr.CodeValidation, "Некорректное тело запроса", err)
	}
	e := apperr.Validation("Некорректный параметр запроса", apperr.FieldError{Field: field, Message: "некорректное значение"})
	e.Err = err
	return e
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
