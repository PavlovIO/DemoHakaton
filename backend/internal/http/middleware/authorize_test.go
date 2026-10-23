package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/openapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
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

// TestAuthorize — порядок проверок доступа: публичная операция → нет сессии (401) →
// роль не из x-roles (403) → KYC не verified при x-kyc-required (403 KYC_NOT_VERIFIED).
func TestAuthorize(t *testing.T) {
	rules, err := NewAccessRules(openapi.MustDoc())
	if err != nil {
		t.Fatalf("NewAccessRules: %v", err)
	}
	principal := func(role, kyc string) *service.Principal {
		org := uuid.New()
		p := &service.Principal{UserID: uuid.New(), OrgID: &org, Role: role, KycStatus: kyc}
		if role == "admin" {
			p.OrgID, p.KycStatus = nil, ""
		}
		return p
	}
	id := uuid.NewString()
	tests := []struct {
		name      string
		method    string
		path      string
		principal *service.Principal
		wantCode  apperr.Code // "" — запрос ушёл в next
	}{
		{"healthz без сессии", http.MethodGet, "/api/v1/healthz", nil, ""},
		{"HEAD healthz без сессии", http.MethodHead, "/api/v1/healthz", nil, ""},
		{"login без сессии", http.MethodPost, "/api/v1/auth/login", nil, ""},
		{"registry lookup без сессии", http.MethodGet, "/api/v1/registry/lookup", nil, ""},
		{"путь не из спеки", http.MethodGet, "/api/v1/nope", nil, ""},
		{"список заявок без сессии", http.MethodGet, "/api/v1/applications", nil, apperr.CodeUnauthorized},
		{"заявка от lender", http.MethodPost, "/api/v1/applications", principal("lender", "verified"), apperr.CodeForbidden},
		{"заявка от admin", http.MethodPost, "/api/v1/applications", principal("admin", ""), apperr.CodeForbidden},
		{"заявка от borrower draft", http.MethodPost, "/api/v1/applications", principal("borrower", "draft"), apperr.CodeKYCNotVerified},
		{"заявка от borrower verified", http.MethodPost, "/api/v1/applications", principal("borrower", "verified"), ""},
		{"оффер от lender under_review", http.MethodPost, "/api/v1/applications/" + id + "/offers", principal("lender", "under_review"), apperr.CodeKYCNotVerified},
		{"accept от lender", http.MethodPost, "/api/v1/offers/" + id + "/accept", principal("lender", "verified"), apperr.CodeForbidden},
		{"загрузка документа admin", http.MethodPost, "/api/v1/organizations/me/documents", principal("admin", ""), apperr.CodeForbidden},
		{"админка от borrower", http.MethodGet, "/api/v1/admin/users", principal("borrower", "verified"), apperr.CodeForbidden},
		{"админка от admin", http.MethodGet, "/api/v1/admin/users", principal("admin", ""), ""},
		{"сделка lender draft", http.MethodGet, "/api/v1/deals/" + id, principal("lender", "draft"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotErr error
			writeErr := func(w http.ResponseWriter, _ *http.Request, err error) {
				gotErr = err
				w.WriteHeader(http.StatusTeapot)
			}
			nextCalled := false
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true })

			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.principal != nil {
				req = req.WithContext(context.WithValue(req.Context(), principalContextKey, *tt.principal))
			}
			Authorize(rules, "/api/v1", writeErr)(next).ServeHTTP(httptest.NewRecorder(), req)

			if tt.wantCode == "" {
				if !nextCalled || gotErr != nil {
					t.Fatalf("want next, got err %v", gotErr)
				}
				return
			}
			if nextCalled {
				t.Fatalf("next вызван, want %s", tt.wantCode)
			}
			if e, ok := apperr.As(gotErr); !ok || e.Code != tt.wantCode {
				t.Fatalf("err = %v, want code %s", gotErr, tt.wantCode)
			}
		})
	}
}
