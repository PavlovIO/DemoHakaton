package middleware

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Расширения операции в спеке, из которых строится AccessRule.
const (
	extRoles       = "x-roles"
	extKYCRequired = "x-kyc-required"
)

// AccessRule — кто может вызвать операцию: из security, x-roles и x-kyc-required спеки.
type AccessRule struct {
	Public      bool     // security: []
	Roles       []string // x-roles
	KYCRequired bool     // x-kyc-required
}

// AccessRules — правила доступа по operationId, прочитанные из спеки, и роутер kin-openapi,
// который находит операцию по запросу (тот же, что внутри валидатора).
type AccessRules struct {
	router      routers.Router
	byOperation map[string]AccessRule
}

// NewAccessRules разбирает спеку один раз при старте. Ошибка = баг в спеке, сервер не стартует.
func NewAccessRules(doc *openapi3.T) (*AccessRules, error) {
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		return nil, fmt.Errorf("router: %w", err)
	}
	rules := &AccessRules{router: router, byOperation: make(map[string]AccessRule)}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			rule, err := accessRuleOf(op)
			if err != nil {
				return nil, fmt.Errorf("%s %s (%s): %w", method, path, op.OperationID, err)
			}
			rules.byOperation[op.OperationID] = rule
		}
	}
	return rules, nil
}

// Rule — правило операции (для тестов и отладки).
func (a *AccessRules) Rule(operationID string) (AccessRule, bool) {
	rule, ok := a.byOperation[operationID]
	return rule, ok
}

// ruleFor находит операцию запроса в спеке. prefix (BaseURL) снимается с пути: в спеке
// пути без него. HEAD ищется как GET — gorillamux HEAD не сопоставляет, а ServeMux да.
func (a *AccessRules) ruleFor(r *http.Request, prefix string) (AccessRule, bool) {
	path, ok := strings.CutPrefix(r.URL.Path, prefix)
	if !ok {
		return AccessRule{}, false
	}
	lookup := r.Clone(r.Context())
	lookup.URL.Path = path
	lookup.URL.RawPath = ""
	if lookup.Method == http.MethodHead {
		lookup.Method = http.MethodGet
	}
	route, _, err := a.router.FindRoute(lookup)
	if err != nil {
		return AccessRule{}, false
	}
	rule, ok := a.byOperation[route.Operation.OperationID]
	return rule, ok
}

type accessRuleContextKey struct{}

// accessRuleFromContext — правило операции, которое положил Authorize (для CSRF).
// ok == false — путь не из спеки.
func accessRuleFromContext(ctx context.Context) (AccessRule, bool) {
	rule, ok := ctx.Value(accessRuleContextKey{}).(AccessRule)
	return rule, ok
}

// Authorize проверяет доступ к операции по правилам из спеки. Порядок проверок:
//
//	| # | Ситуация                                  | Ответ                 |
//	|---|-------------------------------------------|-----------------------|
//	| 1 | путь или метод не из спеки                | next (404 — валидатор)|
//	| 2 | security: []                              | next                  |
//	| 3 | нет principal (сессии нет или невалидна)  | 401 UNAUTHORIZED      |
//	| 4 | роли нет в x-roles                        | 403 FORBIDDEN         |
//	| 5 | x-kyc-required, а KYC не verified         | 403 KYC_NOT_VERIFIED  |
//
// Стоит до валидатора: тот пропускает multipart-загрузку, и без этого роль загрузки
// не проверил бы никто. Сервис повторяет проверку роли — защита в глубину.
func Authorize(rules *AccessRules, prefix string, writeErr func(http.ResponseWriter, *http.Request, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rule, ok := rules.ruleFor(r, prefix)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), accessRuleContextKey{}, rule))
			if rule.Public {
				next.ServeHTTP(w, r)
				return
			}
			p, ok := PrincipalFromContext(r.Context())
			if !ok {
				writeErr(w, r, apperr.Unauthorized("Требуется вход"))
				return
			}
			if !slices.Contains(rule.Roles, p.Role) {
				writeErr(w, r, apperr.Forbidden("Недостаточно прав для операции"))
				return
			}
			if rule.KYCRequired && p.KycStatus != string(repository.KycStatusVerified) {
				writeErr(w, r, apperr.KYCNotVerified())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// knownRoles — значения enum user_role в БД; другой роли в x-roles быть не может.
var knownRoles = map[string]bool{
	string(repository.UserRoleBorrower): true,
	string(repository.UserRoleLender):   true,
	string(repository.UserRoleAdmin):    true,
}

// accessRuleOf строит правило операции и отвергает спеку, в которой доступ описан
// неоднозначно: защищённая операция без ролей открылась бы всем, а не никому.
func accessRuleOf(op *openapi3.Operation) (AccessRule, error) {
	_, hasRoles := op.Extensions[extRoles]
	// Security == nil — операция наследует глобальный security: [cookieAuth];
	// не-nil пустой список — security: [], операция публичная.
	if op.Security != nil && len(*op.Security) == 0 {
		if hasRoles {
			return AccessRule{}, fmt.Errorf("%s у публичной операции (security: [])", extRoles)
		}
		return AccessRule{Public: true}, nil
	}
	roles, err := rolesOf(op)
	if err != nil {
		return AccessRule{}, err
	}
	kyc, err := kycRequiredOf(op)
	if err != nil {
		return AccessRule{}, err
	}
	// У admin нет организации, а значит и kyc_status: с x-kyc-required он не прошёл бы никогда.
	if kyc && slices.Contains(roles, string(repository.UserRoleAdmin)) {
		return AccessRule{}, fmt.Errorf("%s у операции, доступной admin", extKYCRequired)
	}
	return AccessRule{Roles: roles, KYCRequired: kyc}, nil
}

// rolesOf читает x-roles: из YAML список приходит как []any.
func rolesOf(op *openapi3.Operation) ([]string, error) {
	raw, ok := op.Extensions[extRoles].([]any)
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("у защищённой операции %s должен быть непустым списком ролей", extRoles)
	}
	roles := make([]string, 0, len(raw))
	for _, r := range raw {
		role, ok := r.(string)
		if !ok || !knownRoles[role] {
			return nil, fmt.Errorf("%s: неизвестная роль %v", extRoles, r)
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func kycRequiredOf(op *openapi3.Operation) (bool, error) {
	raw, ok := op.Extensions[extKYCRequired]
	if !ok {
		return false, nil
	}
	kyc, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("%s должен быть bool, а не %T", extKYCRequired, raw)
	}
	return kyc, nil
}
