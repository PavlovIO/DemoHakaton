// Package apperr — доменные ошибки приложения.
//
// Слои service и repository возвращают *Error с машиночитаемым кодом,
// а HTTP-слой (internal/http/handler) превращает код в статус и тело Error из спеки.
// Пакет ни от чего не зависит, поэтому его можно импортировать из любого слоя без циклов.
package apperr

import (
	"errors"
	"fmt"
	"time"
)

// Code — машиночитаемый код ошибки. Список совпадает с api/openapi.yaml (info.description → «Ошибки»).
type Code string

const (
	CodeValidation        Code = "VALIDATION_ERROR"
	CodeUnauthorized      Code = "UNAUTHORIZED"
	CodeForbidden         Code = "FORBIDDEN"
	CodeCSRFInvalid       Code = "CSRF_INVALID"
	CodeKYCNotVerified    Code = "KYC_NOT_VERIFIED"
	CodeNotFound          Code = "NOT_FOUND"
	CodeConflict          Code = "CONFLICT"
	CodeInvalidTransition Code = "INVALID_STATUS_TRANSITION"
	CodeRateLimited       Code = "RATE_LIMITED"
	CodeInternal          Code = "INTERNAL"
	CodeNotImplemented    Code = "NOT_IMPLEMENTED"
	CodeUnavailable       Code = "UNAVAILABLE"
)

// FieldError — ошибка конкретного поля запроса (попадает в error.details).
type FieldError struct {
	Field   string
	Message string
}

// Error — доменная ошибка. Message уходит клиенту как есть, Err — только в лог.
type Error struct {
	Code       Code
	Message    string
	Details    []FieldError
	RetryAfter time.Duration
	Err        error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// Is позволяет сравнивать по коду: errors.Is(err, &apperr.Error{Code: apperr.CodeNotFound}).
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return t.Code == e.Code
}

// New создаёт ошибку с произвольным кодом.
func New(code Code, msg string) *Error { return &Error{Code: code, Message: msg} }

// Wrap создаёт ошибку с кодом и причиной (причина видна только в логах).
func Wrap(code Code, msg string, err error) *Error {
	return &Error{Code: code, Message: msg, Err: err}
}

// As достаёт *Error из цепочки. ok=false — ошибка не доменная (значит, INTERNAL).
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// CodeOf возвращает код ошибки; для недоменных — CodeInternal.
func CodeOf(err error) Code {
	if e, ok := As(err); ok {
		return e.Code
	}
	return CodeInternal
}

func Validation(msg string, details ...FieldError) *Error {
	return &Error{Code: CodeValidation, Message: msg, Details: details}
}

func Unauthorized(msg string) *Error { return New(CodeUnauthorized, msg) }
func Forbidden(msg string) *Error    { return New(CodeForbidden, msg) }
func CSRFInvalid() *Error            { return New(CodeCSRFInvalid, "Некорректный CSRF-токен") }
func KYCNotVerified() *Error {
	return New(CodeKYCNotVerified, "Компания не прошла KYC")
}
func NotFound(msg string) *Error { return New(CodeNotFound, msg) }
func Conflict(msg string) *Error { return New(CodeConflict, msg) }
func RateLimited() *Error        { return New(CodeRateLimited, "Слишком много запросов") }
func RateLimitedAfter(retryAfter time.Duration) *Error {
	return &Error{Code: CodeRateLimited, Message: "Слишком много запросов", RetryAfter: retryAfter}
}
func NotImplemented(op string) *Error {
	return New(CodeNotImplemented, "Операция ещё не реализована: "+op)
}
func Unavailable(msg string, err error) *Error { return Wrap(CodeUnavailable, msg, err) }

// InvalidTransition — запрещённый переход статусной машины (например, заявка published → draft).
func InvalidTransition(from, to string) *Error {
	return New(CodeInvalidTransition, fmt.Sprintf("Недопустимый переход статуса: %s → %s", from, to))
}
