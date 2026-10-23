package middleware

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/openapi"
)

// isProtected — операция требует сессии: security не задан (наследует глобальный cookieAuth)
// или задан непустым.
func isProtected(op *openapi3.Operation) bool {
	return op.Security == nil || len(*op.Security) > 0
}

// Любая защищённая операция может вернуть 403 (роль, KYC, CSRF), поэтому контракт обязан его
// объявлять. Тест не даёт новой операции появиться без него.
func TestProtectedOperationsDeclare403(t *testing.T) {
	for path, item := range openapi.MustDoc().Paths.Map() {
		for method, op := range item.Operations() {
			if isProtected(op) && op.Responses.Value("403") == nil {
				t.Errorf("%s (%s %s): защищённая операция без ответа '403'", op.OperationID, method, path)
			}
		}
	}
}
