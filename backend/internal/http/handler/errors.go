package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

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
	field := paramName(err)
	if field == "" {
		// Ошибка декодирования тела запроса (strict-сервер).
		return apperr.Wrap(apperr.CodeValidation, "Некорректное тело запроса", err)
	}
	e := apperr.Validation("Некорректный параметр запроса", apperr.FieldError{Field: field, Message: "некорректное значение"})
	e.Err = err
	return e
}

// generatedPkgPrefix — родитель пакетов internal/http/api/<tag>api, сгенерированных oapi-codegen.
// Берётся из пакета моделей, а не строкой: переименование модуля его не ломает.
var generatedPkgPrefix = reflect.TypeOf(api.Error{}).PkgPath() + "/"

// isGeneratedPkg — прямой подпакет api с именем <tag>api; прочие подпакеты (вложенные или
// с другим суффиксом) — нет.
func isGeneratedPkg(pkgPath string) bool {
	tag, ok := strings.CutPrefix(pkgPath, generatedPkgPrefix)
	return ok && !strings.Contains(tag, "/") && strings.HasSuffix(tag, "api")
}

// paramName достаёт имя параметра из ошибки сгенерированного парсера (InvalidParamFormatError,
// RequiredParamError, RequiredHeaderError, UnmarshalingParamError, TooManyValuesForParamError,
// UnescapedCookieParamError). Эти типы генерируются заново в каждый <tag>api (ADR-0003), так что
// errors.As по конкретному типу потребовал бы 6 × 10 веток. У всех них есть поле ParamName string —
// его и читаем, но только у типов из сгенерированных пакетов. "" — ошибка не про параметр.
// Дерево ошибок обходится через Unwrap() error и Unwrap() []error (errors.Join, %w: %w);
// методы As(any) bool, в отличие от errors.As, не вызываются.
func paramName(err error) string {
	if err == nil {
		return ""
	}
	v := reflect.ValueOf(err)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			// Типизированный nil: сгенерированный Unwrap читает e.Err без проверки и запаникует.
			return ""
		}
		if v.Elem().Kind() == reflect.Struct && isGeneratedPkg(v.Elem().Type().PkgPath()) {
			if f := v.Elem().FieldByName("ParamName"); f.IsValid() && f.Kind() == reflect.String {
				return f.String()
			}
		}
	}
	switch u := err.(type) { //nolint:errorlint // ручной обход цепочки ошибок
	case interface{ Unwrap() error }:
		return paramName(u.Unwrap())
	case interface{ Unwrap() []error }:
		for _, e := range u.Unwrap() {
			if name := paramName(e); name != "" {
				return name
			}
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
