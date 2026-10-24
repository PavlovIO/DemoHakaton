package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
)

// testWriteError — упрощённый handler.WriteError для тестов пакета: тот же формат тела,
// статусы только для кодов, которые отдают middleware.
func testWriteError(w http.ResponseWriter, _ *http.Request, err error) {
	code, message := apperr.CodeOf(err), "Внутренняя ошибка сервера"
	if appErr, ok := apperr.As(err); ok {
		message = appErr.Message
	}
	status := http.StatusInternalServerError
	switch code {
	case apperr.CodeUnauthorized:
		status = http.StatusUnauthorized
	case apperr.CodeCSRFInvalid:
		status = http.StatusForbidden
	default:
		code, message = apperr.CodeInternal, "Внутренняя ошибка сервера"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": string(code), "message": message}})
}
