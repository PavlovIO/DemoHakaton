package middleware

import (
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/openapi"
)

// isProtected — операция требует сессии: security не задан (наследует глобальный cookieAuth)
// или задан непустым.
func isProtected(op *openapi3.Operation) bool {
	return op.Security == nil || len(*op.Security) > 0
}

func TestAccessRulesFromEmbeddedSpec(t *testing.T) {
	doc := openapi.MustDoc()
	rules, err := NewAccessRules(doc)
	if err != nil {
		t.Fatalf("NewAccessRules: %v", err)
	}

	var public, protected int
	for _, item := range doc.Paths.Map() {
		for _, op := range item.Operations() {
			rule, ok := rules.Rule(op.OperationID)
			if !ok {
				t.Errorf("нет правила для %s", op.OperationID)
				continue
			}
			if rule.Public {
				public++
			} else {
				protected++
			}
		}
	}
	if public != 7 || protected != 44 {
		t.Errorf("публичных %d, защищённых %d; want 7 и 44", public, protected)
	}

	all := []string{"borrower", "lender", "admin"}
	tests := []struct {
		operationID string
		want        AccessRule
	}{
		{"createApplication", AccessRule{Roles: []string{"borrower"}, KYCRequired: true}},
		{"createOffer", AccessRule{Roles: []string{"lender"}, KYCRequired: true}},
		{"adminBlockUser", AccessRule{Roles: []string{"admin"}}},
		{"getMe", AccessRule{Roles: all}},
		{"healthz", AccessRule{Public: true}},
	}
	for _, tt := range tests {
		got, ok := rules.Rule(tt.operationID)
		if !ok {
			t.Errorf("%s: правила нет", tt.operationID)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %+v, want %+v", tt.operationID, got, tt.want)
		}
	}
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
