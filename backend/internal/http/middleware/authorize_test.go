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

// specWithOperation — минимальная спека с одной операцией GET /x; ext — строки YAML
// уровня операции (security, x-roles, x-kyc-required).
func specWithOperation(t *testing.T, ext string) *openapi3.T {
	t.Helper()
	src := `
openapi: 3.0.3
info: { title: t, version: "1" }
security: [ { cookieAuth: [] } ]
components:
  securitySchemes:
    cookieAuth: { type: apiKey, in: cookie, name: session }
paths:
  /x:
    get:
      operationId: x
` + ext + `
      responses:
        '200': { description: OK }
`
	doc, err := openapi3.NewLoader().LoadFromData([]byte(src))
	if err != nil {
		t.Fatalf("тестовая спека не разбирается: %v", err)
	}
	return doc
}

func TestNewAccessRulesRejectsBadSpec(t *testing.T) {
	tests := []struct {
		name string
		ext  string
	}{
		{"защищённая без x-roles", ``},
		{"пустой x-roles", `      x-roles: []`},
		{"неизвестная роль", `      x-roles: [superuser]`},
		{"x-roles у публичной", "      security: []\n      x-roles: [borrower]"},
		{"x-kyc-required не bool", "      x-roles: [borrower]\n      x-kyc-required: \"yes\""},
		{"x-kyc-required у admin", "      x-roles: [admin]\n      x-kyc-required: true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewAccessRules(specWithOperation(t, tt.ext)); err == nil {
				t.Fatal("ошибки нет, а спека некорректна")
			}
		})
	}

	// Контроль: корректная спека того же вида разбирается — иначе таблица выше проверяла бы опечатку.
	if _, err := NewAccessRules(specWithOperation(t, "      x-roles: [borrower]\n      x-kyc-required: true")); err != nil {
		t.Fatalf("корректная спека: %v", err)
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
